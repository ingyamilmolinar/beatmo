/** A Synth slider at its Min (decay or attack 0) must never silence the browser or poison the EQ with NaN: exp(-t ·
 * rate · (1/L − 1)) is NaN at L = 0. Proves the DEPLOYED wasm and C glue carry the structural bound, which a Go test of
 * the source cannot. */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
  waitForAudioReady,
} from "./browser_test_helpers.js";

import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// One shipped instrument per voice family, so __testCaptureSynthRender resolves a real document.
const STARTUP_INSTRUMENTS = ["kick-snap", "snare", "hihat", "clap", "cowbell", "fm-epiano-drive"];

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const pageErrors = [];
const biquadWarnings = [];
let exitCode = 0;
let browser;

const peakOf = (arr) => {
  let p = 0;
  for (const v of arr) {
    const a = Math.abs(v);
    if (a > p) p = a;
  }
  return p;
};
const hasNonFinite = (arr) => {
  for (const v of arr) if (!Number.isFinite(v)) return true;
  return false;
};
const countNonFinite = (arr) => {
  let n = 0;
  for (const v of arr) if (!Number.isFinite(v)) n++;
  return n;
};
const energyOf = (arr) => {
  let s = 0;
  for (const v of arr) s += Math.abs(v);
  return s / Math.max(arr.length, 1);
};

try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  page.on("pageerror", (err) => {
    pageErrors.push(String(err));
    console.log("[PAGE-ERROR]", String(err));
  });
  page.on("console", (msg) => {
    const txt = msg.text();
    if (msg.type() === "warning" && /BiquadFilterNode/i.test(txt)) {
      biquadWarnings.push(txt);
    }
    if (msg.type() === "error" || (msg.type() === "warning" && /Biquad|NaN|crash/i.test(txt))) {
      console.log("[PAGE]", msg.type(), txt);
    }
  });

  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof startPlay === "function" &&
    typeof stopPlay === "function" &&
    typeof setInstrumentParam === "function" &&
    typeof resetInstrumentParams === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof getOutputCapture === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof getInstrumentDoc === "function"
  );

  // Unlock audio context — Playwright's autoplay policy still wants a gesture.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 },
  );
  await waitForAudioReady(page);

  // Guards against a renamed instrument silently skipping the assertions.
  const bindings = await page.evaluate((ids) => {
    return ids.map((id) => ({ id, origin: (JSON.parse(getInstrumentDoc(id) || "{}").origin || "") }));
  }, STARTUP_INSTRUMENTS);
  for (const b of bindings) {
    if (!b.origin) {
      throw new Error(`instrument '${b.id}' has no instrument document — startup_demo.json may have drifted`);
    }
  }
  console.log("[TEST] startup-demo documents:", JSON.stringify(bindings));

  // Under a parallel run a fixed window can land between hits, so windows are polled until one is audible; a silent
  // chain still fails at the deadline, NaN/Inf at once.
  const captureAudibleWindow = async (label, { windowMs = 800, deadlineMs = 20000 } = {}) => {
    const start = Date.now();
    for (let attempt = 1; ; attempt++) {
      await page.evaluate(() => {
        clearOutputCapture();
        startOutputCapture();
      });
      await page.waitForTimeout(windowMs);
      const captured = await page.evaluate(() => Array.from(getOutputCapture()));
      if (hasNonFinite(captured)) {
        throw new Error(`${label} capture contains ${countNonFinite(captured)} NaN/Inf sample(s) — audio chain corrupt`);
      }
      const peak = peakOf(captured);
      if (peak >= 0.001 || Date.now() - start >= deadlineMs) return captured;
      console.log(`[TEST] ${label}: window ${attempt} quiet (peak=${peak.toFixed(4)}) — retrying under load`);
    }
  };

  // Play is ignored while the warm-up gate is closed.
  await startPlaybackWhenReady(page);
  await page.waitForTimeout(800);

  // ─── Phase B: baseline capture ───
  const baseline = await captureAudibleWindow("baseline");
  const basePeak = peakOf(baseline);
  const baseEnergy = energyOf(baseline);
  console.log(`[TEST] baseline: ${baseline.length} samples, peak=${basePeak.toFixed(4)}, energy=${baseEnergy.toFixed(6)}`);
  if (basePeak < 0.001) {
    throw new Error(`baseline is silent (peak=${basePeak}). Sequencer is not actually playing — fix the test harness before trusting the rest.`);
  }

  // decay=0 is the canonical NaN trigger; the rest cover sibling envelope and saturation extremes.
  const PARAM_CASES = [
    { name: "decay=0",  patch: { decay: 0 } },
    { name: "attack=0", patch: { attack: 0 } },
    { name: "decay=4",  patch: { decay: 4 } },
    { name: "all-min",  patch: { decay: 0, attack: 0, pitch: -24, tone: -1, drive: 0, body: 0, color: -1, brightness: 0 } },
    { name: "all-max",  patch: { decay: 4, attack: 4, pitch:  24, tone:  1, drive: 1, body: 1, color:  1, brightness: 1 } },
  ];

  for (const id of STARTUP_INSTRUMENTS) {
    for (const tc of PARAM_CASES) {
      const tag = `${id} / ${tc.name}`;

      // One write per knob (one drag tick), then a few notes so the re-render is what plays.
      await page.evaluate(
        ([instId, patch]) => {
          for (const [k, v] of Object.entries(patch)) {
            setInstrumentParam(instId, k, v);
          }
          clearOutputCapture();
          startOutputCapture();
        },
        [id, tc.patch],
      );
      await page.waitForTimeout(700);
      const captured = await page.evaluate(() => Array.from(getOutputCapture()));
      const bad = countNonFinite(captured);
      const peak = peakOf(captured);
      const energy = energyOf(captured);
      console.log(`[TEST] ${tag}: ${captured.length} samples, peak=${peak.toFixed(4)}, energy=${energy.toFixed(6)}, NaN/Inf=${bad}`);

      if (bad > 0) {
        throw new Error(
          `BUG: ${tag} — captured ${bad} NaN/Inf sample(s). The render path is producing non-finite ` +
          `PCM and poisoning the WebAudio biquad chain. Inspect the C _p() variant for missing clamp.`,
        );
      }
      if (biquadWarnings.length > 0) {
        const sample = biquadWarnings[0];
        throw new Error(
          `BUG: ${tag} — BiquadFilterNode warning emitted (${biquadWarnings.length} total).\n` +
          `First: ${sample}\nThe biquad chain saw NaN/Inf samples and corrupted its internal state.`,
        );
      }
      // No per-case audibility: the mix spans every instrument and sparse ones legitimately miss a window; the tail capture
      // after every reset catches a permanently silenced chain.
      void peak; void energy;

      // So changes do not accumulate across iterations.
      await page.evaluate((instId) => resetInstrumentParams(instId), id);
      await page.waitForTimeout(150);
    }
  }

  // After every extreme and reset the chain must still produce audio.
  const tail = await captureAudibleWindow("post-sweep tail");
  const tailPeak = peakOf(tail);
  const tailEnergy = energyOf(tail);
  console.log(`[TEST] post-sweep tail: ${tail.length} samples, peak=${tailPeak.toFixed(4)}, energy=${tailEnergy.toFixed(6)}`);
  if (tailPeak < 0.001) {
    throw new Error(`post-sweep tail is silent (peak=${tailPeak}, baseline=${basePeak}) — chain did not recover after the sweep`);
  }
  // At the render level, which is deterministic: a capture-energy ratio collapses under parallel load with no defect.
  void tailEnergy; void baseEnergy;
  for (const id of STARTUP_INSTRUMENTS) {
    const rec = await page.evaluate(async (instId) => {
      const r = await window.__testCaptureSynthRender(instId);
      return r ? { peak: r.peak, rms: r.rms } : null;
    }, id);
    if (!rec || !(rec.peak > 0.001)) {
      throw new Error(
        `post-sweep render for '${id}' is ${rec ? `silent (peak=${rec.peak})` : 'unavailable'} — ` +
        `the instrument did not recover after the extremes sweep + reset`,
      );
    }
  }
  console.log(`[TEST] post-sweep per-instrument renders all audible (${STARTUP_INSTRUMENTS.length} instruments)`);

  await page.evaluate(() => stopPlay());

  if (pageErrors.length > 0) {
    throw new Error(`page emitted ${pageErrors.length} errors:\n  ${pageErrors.join("\n  ")}`);
  }
  if (biquadWarnings.length > 0) {
    throw new Error(
      `${biquadWarnings.length} BiquadFilterNode warning(s) emitted during the test — chain went bad.\n` +
      `Sample: ${biquadWarnings[0]}`,
    );
  }

  console.log("[TEST] synth_param_extremes: all instruments x all extremes finite + audible.");

  if (isCoverageEnabled()) {
    await flushCoverage(
      page,
      new URL("../../coverage/browser-raw", import.meta.url).pathname,
      "synth_param_extremes",
    );
  }
} catch (err) {
  console.error("synth_param_extremes FAILED:", err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

console.log(exitCode === 0 ? "synth_param_extremes browser test PASSED" : "synth_param_extremes browser test FAILED");
process.exit(exitCode);
