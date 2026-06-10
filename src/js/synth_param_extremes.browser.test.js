/**
 * Synth-slider extremes regression — proves the deployed WASM binary
 * carries the C-side decay/attack clamp.
 *
 * Bug class: dragging a Synth-tab slider to its Min (esp. `decay = 0` or
 * `attack = 0`) on the default startup circuit silences WASM audio
 * permanently and emits "BiquadFilterNode: state is bad" warnings. The
 * envelope formula `exp(-t * rate * (1/decayMul - 1))` evaluates to NaN
 * at decayMul=0, t=0, and the NaN propagates through AudioBuffer →
 * BufferSourceNode → BiquadFilterNode, corrupting the EQ chain.
 *
 * The C source (drums.c:1709 apply_post_params + six inline _p clamps)
 * already pins decayMul ≥ 0.01. This test mechanically distinguishes
 * "C source has the clamp" (Go test passes) from "the WASM binary the
 * user is running has the clamp" (THIS test passes) — the user's
 * symptom persists when main.wasm + drums.single.js are stale.
 *
 * Mirrors the structure of instrument_params_e2e_audio.browser.test.js
 * but drives every default-circuit instrument to its decay/attack
 * Min instead of sweeping `pitch` on snare alone.
 */

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
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Instruments shipped in src/go/internal/assets/startup_demo.json. Each is
// driven through the bridge with `decay = 0` then `attack = 0` then a
// compound all-min snapshot to lock down the failure class end-to-end.
const STARTUP_INSTRUMENTS = ["kick-1", "snare", "hihat", "clap", "cowbell", "fm-epiano-1"];

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
    typeof recipeForInstrument === "function"
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
  await page.evaluate(() => window.audioReady);

  // Confirm the bridge sees every startup-demo instrument bound to a
  // builtin recipe — guards against a renamed default circuit silently
  // skipping the assertions.
  const bindings = await page.evaluate((ids) => {
    return ids.map((id) => ({ id, recipe: recipeForInstrument(id) }));
  }, STARTUP_INSTRUMENTS);
  for (const b of bindings) {
    if (!b.recipe) {
      throw new Error(`instrument '${b.id}' is not bound to any recipe — startup_demo.json may have drifted`);
    }
  }
  console.log("[TEST] startup-demo bindings:", JSON.stringify(bindings));

  // Condition-based capture: under a parallel `make test-browser` the page is
  // starved for seconds at a time (4 SwiftShader Chromium instances share the
  // host), so a single fixed 800ms window can land entirely between sequencer
  // hits (observed: baseline peak=0.0003 on a 4-job run vs 0.0789 solo). Poll
  // capture windows until one is audible; a genuinely silent chain still fails
  // after the deadline. NaN/Inf anywhere fails immediately.
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

  // ─── Phase A: warm up the sequencer ───
  await page.evaluate(() => {
    startPlay();
  });
  await page.waitForTimeout(800);

  // ─── Phase B: baseline capture ───
  const baseline = await captureAudibleWindow("baseline");
  const basePeak = peakOf(baseline);
  const baseEnergy = energyOf(baseline);
  console.log(`[TEST] baseline: ${baseline.length} samples, peak=${basePeak.toFixed(4)}, energy=${baseEnergy.toFixed(6)}`);
  if (basePeak < 0.001) {
    throw new Error(`baseline is silent (peak=${basePeak}). Sequencer is not actually playing — fix the test harness before trusting the rest.`);
  }

  // The cases each instrument is driven through. `decay=0` is the
  // canonical NaN trigger; the rest cover sibling envelope/saturation
  // extremes so a regression in any one knob fails loudly.
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

      // Drive the bridge at ~60 Hz (one OnDrag tick) — single setInstrumentParam
      // per knob, then let the sequencer fire a few more notes so the
      // re-rendered _p variant is what the AudioBuffer carries.
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
      // Per-case audibility check is intentionally omitted: the global mix
      // captured here aggregates *all* instruments, and the sequencer fires
      // some instruments sparsely (e.g. hihat) — a 700ms window can legitimately
      // land between hits without indicating a broken channel. The end-of-sweep
      // tail capture below catches "chain permanently silenced" by checking
      // the same mix after every case completes and every instrument is reset.
      void peak; void energy;

      // Reset this instrument before driving the next case so changes
      // don't accumulate across iterations.
      await page.evaluate((instId) => resetInstrumentParams(instId), id);
      await page.waitForTimeout(150);
    }
  }

  // Final tail capture: after every instrument has been driven through every
  // extreme and reset, the channel chain must still be producing audio.
  // (NaN/Inf in any window throws inside captureAudibleWindow.)
  const tail = await captureAudibleWindow("post-sweep tail");
  const tailPeak = peakOf(tail);
  const tailEnergy = energyOf(tail);
  console.log(`[TEST] post-sweep tail: ${tail.length} samples, peak=${tailPeak.toFixed(4)}, energy=${tailEnergy.toFixed(6)}`);
  if (tailPeak < 0.001) {
    throw new Error(`post-sweep tail is silent (peak=${tailPeak}, baseline=${basePeak}) — chain did not recover after the sweep`);
  }
  // Per-instrument render-level recovery check. The previous assertion here
  // compared wall-clock capture ENERGY to the baseline (tail/baseline >= 20%),
  // which is load-sensitive: under a parallel `make test-browser` the capture
  // windows stretch and land between sequencer hits, collapsing the measured
  // energy without any real audio defect (observed: 16.9% on a 4-job run that
  // passes solo). The render layer is deterministic: after every reset, each
  // instrument's one-shot must re-render audibly.
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
