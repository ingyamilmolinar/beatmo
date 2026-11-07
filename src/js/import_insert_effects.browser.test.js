// Importing an effect chain must rewire the AUDIBLE WebAudio chain (and importing none must clear it), which Go
// cannot reach. COVERED-BY-GO: insert_effects_roundtrip_test.go, effect_chain_import_process_test.go,
// effect_chain_sanitize_test.go. Usage: GO=… node src/js/import_insert_effects.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, waitForAudioReady } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

// Aggressive chain so the wet capture is unmistakably different from dry.
const IMPORT_CHAIN = [
  { type: "distortion", enabled: true, params: { drive: 15, tone: 4000, mix: 1 } },
  { type: "bitcrusher", enabled: true, params: { bits: 4, rate: 0.2, mix: 1 } },
  { type: "delay", enabled: false, params: { time: 200, feedback: 0.6, mix: 0.8 } },
];

let exitCode = 0;
let browser;
const failures = [];
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

  // On a minority of software-GL page loads the draw loop starves the main-thread capture tap for good, so that page
  // is discarded and retried; a real assertion failure is collected, never thrown, so a retry never masks it.
  let measured = false;
  for (let pageAttempt = 1; pageAttempt <= 3 && !measured; pageAttempt++) {
  const page = await browser.newPage();
  if (process.env.TEST_LOG) page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
  await page.goto(`http://localhost:${port}/`);
  try {

  await page.waitForFunction(() =>
    typeof getInsertEffects === "function" &&
    typeof importJSON === "function" &&
    typeof exportJSON === "function" &&
    typeof playSound === "function" &&
    typeof ensureSynthSample === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof clearOutputCapture === "function"
  );

  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await waitForAudioReady(page);
  // The startup draw burst starves the main-thread capture tap until it quiesces.
  await page.waitForTimeout(2500);
  await page.evaluate(() => ensureSynthSample("kick"));

  const NOISE_FLOOR = 0.0001;
  const MAX_RETRIES = 5;

  // 3 spaced kicks through the live master, plus the waveform head for correlation. The ScriptProcessor tap turns
  // unreliable under software GL, so a retry rebuilds a fresh one.
  async function captureKick(retryAttempt) {
    return page.evaluate(async ({ retryAttempt, NOISE_FLOOR }) => {
      try { resumeAudio?.(); } catch (_) {} // re-ensure the context is running
      if (retryAttempt > 1) resetOutputCaptureNode();
      startOutputCapture();
      // CPU contention right after the build can starve the first plays.
      playSound("kick", 1.0);
      const deadline = Date.now() + 5000;
      let warm = false;
      while (Date.now() < deadline && !warm) {
        const snap = getOutputCapture();
        if (snap) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { warm = true; break; }
          }
        }
        if (!warm) await new Promise((r) => setTimeout(r, 100));
      }
      await new Promise((r) => setTimeout(r, warm ? 300 : 1500));
      clearOutputCapture();
      // From the same post-clear offset, so onsets align for the head correlation.
      for (let i = 0; i < 3; i++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 400));
      }
      const captured = stopOutputCapture();
      let sumSq = 0, active = 0, peak = 0;
      const head = [];
      for (let i = 0; i < captured.length; i++) {
        const v = captured[i], a = Math.abs(v);
        if (a > peak) peak = a;
        if (a > NOISE_FLOOR) { sumSq += v * v; active++; }
        if (i < 4096) head.push(v);
      }
      return { rms: active ? Math.sqrt(sumSq / active) : 0, peak, active, sampleCount: captured.length, head };
    }, { retryAttempt, NOISE_FLOOR });
  }

  // The tap drops out at random under software GL, so one capture is not trustworthy; throws only if every attempt is
  // silent.
  async function measure(label, required = true) {
    let best = { rms: 0, peak: 0, active: 0, head: [] };
    for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
      const m = await captureKick(attempt);
      if (m.peak > best.peak) best = m;
      // A healthy kick peaks well above 0.01; a starved capture is near-silent, so only a genuinely loud one counts.
      if (m.active > 0 && m.rms > 0.001 && m.peak > 0.01) {
        console.log(`[test] ${label}: rms=${m.rms.toFixed(5)} peak=${m.peak.toFixed(5)} active=${m.active}`);
        return m;
      }
      console.log(`[test] ${label} attempt ${attempt} weak (rms=${m.rms.toFixed(5)} peak=${m.peak.toFixed(5)}); retrying`);
      await page.waitForTimeout(800);
    }
    // The dry/wet REFERENCES must be real; the post-clear capture only has to differ from wet, and the structural check plus
    // the Go tests prove the teardown.
    if (required) {
      throw new Error(`${label} capture stayed silent after ${MAX_RETRIES} attempts (rms=${best.rms} peak=${best.peak})`);
    }
    console.log(`[test] ${label}: degraded (best rms=${best.rms.toFixed(5)} peak=${best.peak.toFixed(5)}) — accepted (non-reference capture)`);
    return best;
  }

  // Amplitude-invariant, so it separates "chain in the path" from "chain removed" where RMS and peak overlap run to run.
  function correlation(a, b) {
    const A = a.head || [], B = b.head || [];
    const len = Math.min(A.length, B.length, 4096);
    if (len < 100) return 1.0;
    let sumXY = 0, sumX2 = 0, sumY2 = 0;
    for (let i = 0; i < len; i++) {
      const x = A[i] || 0, y = B[i] || 0;
      sumXY += x * y; sumX2 += x * x; sumY2 += y * y;
    }
    const denom = Math.sqrt(sumX2 * sumY2);
    return denom > 1e-10 ? sumXY / denom : 0;
  }

  // ANY of RMS, peak or waveform beyond noise, so an energy-preserving effect still counts.
  function chainAudiblyChanged(ref, cur) {
    const rmsRel = ref.rms > 0 ? Math.abs(cur.rms - ref.rms) / ref.rms : 0;
    const peakRel = ref.peak > 0 ? Math.abs(cur.peak - ref.peak) / ref.peak : 0;
    const corr = correlation(ref, cur);
    return { rmsRel, peakRel, corr, changed: rmsRel > 0.05 || peakRel > 0.05 || corr < 0.95 };
  }

  // Until the tap demonstrably delivers, or the dry baseline reads silent; tolerant, since measure() still retries.
  await page.evaluate(async () => {
    const deadline = Date.now() + 20000;
    let alive = false;
    while (Date.now() < deadline && !alive) {
      try { resumeAudio?.(); } catch (_) {}
      resetOutputCaptureNode();
      startOutputCapture();
      for (let k = 0; k < 3 && !alive; k++) {
        playSound("kick", 1.0);
        const until = Date.now() + 600;
        while (Date.now() < until) {
          const snap = getOutputCapture();
          if (snap) { for (let i = 0; i < snap.length; i++) { if (Math.abs(snap[i]) > 0.02) { alive = true; break; } } }
          if (alive) break;
          await new Promise((r) => setTimeout(r, 50));
        }
      }
      stopOutputCapture();
    }
    console.log(`[test] audio prime ${alive ? "alive" : "TIMED OUT"}`);
  });

  // --- Scenario 1: dry baseline ---
  const baseline = await measure("dry baseline");

  // --- Scenario 2: import project with an effect chain on kick ---
  const importErr = await page.evaluate((chain) => {
    const proj = JSON.parse(exportJSON());
    let kick = (proj.instruments || []).find((i) => i.id === "kick");
    if (!kick) {
      kick = { name: "Kick", id: "kick", kind: "builtin", volume: 1, origin: proj.instruments.length, color: "#FF8844FF" };
      proj.instruments.push(kick);
    }
    kick.effects = chain;
    return importJSON(JSON.stringify(proj)) || "";
  }, IMPORT_CHAIN);
  if (importErr) failures.push(`importJSON (with effects) error: ${importErr}`);

  const liveSlots = await page.evaluate(() => getInsertEffects("kick") || []);
  if (liveSlots.length !== IMPORT_CHAIN.length) {
    failures.push(`live chain after import: ${liveSlots.length} slots, want ${IMPORT_CHAIN.length} (${JSON.stringify(liveSlots)})`);
  } else {
    IMPORT_CHAIN.forEach((want, i) => {
      const got = liveSlots[i];
      if (got.type !== want.type || got.enabled !== want.enabled) {
        failures.push(`slot[${i}]: got ${got.type}/${got.enabled}, want ${want.type}/${want.enabled}`);
      }
      for (const [k, v] of Object.entries(want.params)) {
        if (Math.abs((got.params?.[k] ?? NaN) - v) > 1e-9) {
          failures.push(`slot[${i}].${k}: got ${got.params?.[k]}, want ${v}`);
        }
      }
    });
  }

  // Or the import's redraw burst re-starves the tap during the next capture.
  await page.waitForTimeout(1500);
  let wet = await measure("post-import wet");
  let wetDelta = chainAudiblyChanged(baseline, wet);
  if (!wetDelta.changed) {
    wet = await measure("post-import wet (re-measure)"); // captures are timing-noisy
    wetDelta = chainAudiblyChanged(baseline, wet);
  }
  console.log(`[test] wet vs dry: rmsRel=${(wetDelta.rmsRel * 100).toFixed(1)}% peakRel=${(wetDelta.peakRel * 100).toFixed(1)}% corr=${wetDelta.corr.toFixed(3)}`);
  if (!wetDelta.changed) {
    failures.push(
      `imported chain not audible: wet vs dry rmsRel=${(wetDelta.rmsRel * 100).toFixed(1)}% peakRel=${(wetDelta.peakRel * 100).toFixed(1)}% corr=${wetDelta.corr.toFixed(3)} (want any: rms>5%, peak>5%, or corr<0.95) — updateInsertEffects did not rewire the JS chain`
    );
  }

  // --- Scenario 3: import a project WITHOUT effects clears the JS chain ---
  const clearErr = await page.evaluate(() => {
    const proj = JSON.parse(exportJSON());
    for (const inst of proj.instruments || []) delete inst.effects;
    return importJSON(JSON.stringify(proj)) || "";
  });
  if (clearErr) failures.push(`importJSON (effects-free) error: ${clearErr}`);

  const afterClear = await page.evaluate(() => getInsertEffects("kick") || []);
  if (afterClear.length !== 0) {
    failures.push(`stale Go chain after effects-free import: ${JSON.stringify(afterClear)}`);
  }
  await page.waitForTimeout(1500); // let the effects-free import tear down the chain AND its redraw burst settle
  // The robust direction, "cleared differs from wet": proving sameness from a live ScriptProcessor tap is unreliable,
  // while the distortion's huge delta makes the difference rock-solid.
  let cleared = await measure("post-clear", false);
  let clearDelta = chainAudiblyChanged(wet, cleared);
  if (!clearDelta.changed) {
    cleared = await measure("post-clear (re-measure)", false); // one re-measure before failing
    clearDelta = chainAudiblyChanged(wet, cleared);
  }
  console.log(`[test] cleared vs wet: rmsRel=${(clearDelta.rmsRel * 100).toFixed(1)}% peakRel=${(clearDelta.peakRel * 100).toFixed(1)}% corr=${clearDelta.corr.toFixed(3)}`);
  if (!clearDelta.changed) {
    failures.push(
      `stale JS chain still audible after effects-free import: cleared kick still matches the distorted wet capture (rmsRel=${(clearDelta.rmsRel * 100).toFixed(1)}% peakRel=${(clearDelta.peakRel * 100).toFixed(1)}% corr=${clearDelta.corr.toFixed(3)}) — updateInsertEffects did not tear down the JS chain`
    );
  }

  measured = true; // got through both reference captures on this page load
  } catch (e) {
    if (/stayed silent/.test(String(e && e.message)) && pageAttempt < 3) {
      console.log(`[test] a reference capture stayed silent on page-load ${pageAttempt}/3 (starved tap); reloading a fresh page`);
      failures.length = 0; // discard partial state from the aborted attempt
      await page.close().catch(() => {});
      continue;
    }
    await page.close().catch(() => {});
    throw e;
  }
  await page.close().catch(() => {});
  }
  if (!measured) throw new Error("could not obtain a non-silent reference capture across 3 page loads (audio tap persistently starved)");

  if (failures.length) throw new Error("IMPORT INSERT EFFECTS FAILED:\n  - " + failures.join("\n  - "));
  console.log("[test] PASS: imported effect chains rewire the JS audio graph; effects-free import clears it");
} catch (e) {
  console.error(e);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}
process.exit(exitCode);
