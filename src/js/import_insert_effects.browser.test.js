// import_insert_effects.browser.test.js
//
// Render-level guards for per-instrument insert effect chains across
// importJSON — the Go↔WebAudio boundary the Go tests cannot reach:
//
//   1. Importing a project whose instrument carries an effect chain must
//      reconfigure the live JS chain (worklet/fallback graph): captured
//      playback audibly differs from the dry baseline, and getInsertEffects
//      mirrors the imported slots (type/order/enabled/params).
//   2. Importing a project WITHOUT effects after a live chain exists must
//      clear the JS chain (audio.ClearAllInsertEffectsAndNotify →
//      updateInsertEffects(id, null)): playback returns to ~baseline.
//
// COVERED-BY-GO: chain state round-trip + stale-chain clearing + processing
// (internal/ui/insert_effects_roundtrip_test.go,
// internal/audio/effect_chain_import_process_test.go,
// internal/audio/effect_chain_sanitize_test.go). JS-owned here: the
// updateInsertEffects bridge actually rewires the audible WebAudio chain on
// import.
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/import_insert_effects.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

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
  const page = await browser.newPage();
  if (process.env.TEST_LOG) page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
  await page.goto(`http://localhost:${port}/`);

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
  await page.evaluate(() => window.audioReady);
  await page.waitForTimeout(500);
  await page.evaluate(() => ensureSynthSample("kick"));

  // capture: warm up until real signal flows (CPU contention right after the
  // WASM build can starve the first playbacks — same pattern as
  // worklet_insert_effects.browser.test.js), then play 3 kicks and summarize.
  // Retries the whole capture when it comes back silent.
  async function capture() {
    for (let attempt = 1; attempt <= 3; attempt++) {
      const out = await page.evaluate(async () => {
        startOutputCapture();
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
        await new Promise((r) => setTimeout(r, 300));
        clearOutputCapture();
        for (let i = 0; i < 3; i++) {
          playSound("kick", 1.0);
          await new Promise((r) => setTimeout(r, 400));
        }
        const captured = stopOutputCapture();
        let sumSq = 0, active = 0, peak = 0;
        for (let i = 0; i < captured.length; i++) {
          const a = Math.abs(captured[i]);
          if (a > peak) peak = a;
          if (a > 0.0001) { sumSq += captured[i] * captured[i]; active++; }
        }
        return { rms: active ? Math.sqrt(sumSq / active) : 0, peak, active };
      });
      if (out.rms > 0.001) return out;
      console.log(`[test] capture attempt ${attempt} weak (rms=${out.rms}); retrying`);
    }
    throw new Error("capture stayed silent across 3 attempts");
  }

  // --- Scenario 1: dry baseline ---
  const baseline = await capture();
  console.log(`[test] dry baseline: rms=${baseline.rms.toFixed(5)} peak=${baseline.peak.toFixed(5)} active=${baseline.active}`);
  if (!(baseline.rms > 0.001)) throw new Error(`baseline capture silent (rms=${baseline.rms})`);

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

  let wet = await capture();
  console.log(`[test] post-import wet: rms=${wet.rms.toFixed(5)} peak=${wet.peak.toFixed(5)}`);
  let rmsChange = Math.abs(wet.rms - baseline.rms) / baseline.rms;
  if (rmsChange < 0.05) {
    wet = await capture(); // one re-measure before failing — captures are timing-noisy
    rmsChange = Math.abs(wet.rms - baseline.rms) / baseline.rms;
  }
  if (rmsChange < 0.05) {
    failures.push(
      `imported chain not audible: wet rms=${wet.rms.toFixed(5)} vs dry ${baseline.rms.toFixed(5)} (${(rmsChange * 100).toFixed(1)}% change, want >5%) — updateInsertEffects did not rewire the JS chain`
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
  let dryAgain = await capture();
  console.log(`[test] post-clear: rms=${dryAgain.rms.toFixed(5)} (dry=${baseline.rms.toFixed(5)} wet=${wet.rms.toFixed(5)})`);
  let dDry = Math.abs(dryAgain.rms - baseline.rms);
  let dWet = Math.abs(dryAgain.rms - wet.rms);
  if (dDry >= dWet) {
    dryAgain = await capture(); // one re-measure before failing
    console.log(`[test] post-clear re-measure: rms=${dryAgain.rms.toFixed(5)}`);
    dDry = Math.abs(dryAgain.rms - baseline.rms);
    dWet = Math.abs(dryAgain.rms - wet.rms);
  }
  if (dDry >= dWet) {
    failures.push(
      `stale JS chain still audible after effects-free import: rms=${dryAgain.rms.toFixed(5)} is closer to wet (${wet.rms.toFixed(5)}) than dry (${baseline.rms.toFixed(5)})`
    );
  }

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
