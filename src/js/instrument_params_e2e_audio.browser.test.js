/**
 * End-to-end Synth-tab audio regression — Phase 5 follow-up.
 *
 * Reproduces the user-reported "audio stops in WASM when synth sliders
 * change" bug using the FULL APP (main.wasm), not the playtest harness.
 * Drives the actual sequencer via startPlay(), captures WebAudio output
 * via startOutputCapture(), and dispatches setInstrumentParam calls at
 * 60Hz mid-playback. Asserts the captured stream stays audible + finite
 * before, during, and after the slider drag.
 *
 * This is the canonical "did Phase 5 actually fix the user's session"
 * regression — if this passes, the bug they're seeing has a different
 * root cause and we need more telemetry from their browser.
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

// Build full WASM app (the same binary the user is running).
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
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof getOutputCapture === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof recipeForInstrument === "function"
  );

  // Unlock audio context.
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

  // Sanity: there is at least one row + the snare is bound.
  const sanity = await page.evaluate(() => {
    const recipe = recipeForInstrument("snare");
    return { recipe };
  });
  if (sanity.recipe !== "drum-snare") {
    throw new Error(`snare is not bound to drum-snare; got '${sanity.recipe}'`);
  }
  console.log("[TEST] snare → drum-snare binding confirmed");

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
  const energyOf = (arr) => {
    let s = 0;
    for (const v of arr) s += Math.abs(v);
    return s / Math.max(arr.length, 1);
  };

  // ─── Phase A: start playback + warm-up the audio ───
  await page.evaluate(() => {
    if (typeof resetInstrumentParams === "function") {
      resetInstrumentParams("snare");
    }
    startPlay();
  });
  // Let the sequencer fire 5+ notes so the channel chain is fully built.
  await page.waitForTimeout(800);

  // ─── Phase B: capture baseline audio while sequencer runs ───
  await page.evaluate(() => {
    clearOutputCapture();
    startOutputCapture();
  });
  await page.waitForTimeout(800);
  const baseline = await page.evaluate(() => Array.from(getOutputCapture()));
  if (hasNonFinite(baseline)) {
    throw new Error(`baseline capture contains NaN/Inf — audio chain corrupt BEFORE slider interaction`);
  }
  const basePeak = peakOf(baseline);
  const baseEnergy = energyOf(baseline);
  console.log(`[TEST] baseline capture: ${baseline.length} samples, peak=${basePeak.toFixed(4)}, energy=${baseEnergy.toFixed(6)}`);
  if (basePeak < 0.001) {
    throw new Error(`baseline is silent (peak=${basePeak}). Sequencer is not actually playing.`);
  }

  // ─── Phase C: simulate slider drag while playback continues ───
  // 60Hz spam matches the OnDrag firing pattern in the real UI.
  await page.evaluate(async () => {
    clearOutputCapture();
    startOutputCapture();
    // Sweep pitch from 0 → 19 over ~1 second at 60Hz.
    const frames = 60;
    for (let i = 0; i < frames; i++) {
      const pitch = (19 * i) / frames;
      setInstrumentParam("snare", "pitch", pitch);
      await new Promise((r) => setTimeout(r, 16));
    }
  });
  // Capture for an additional 500ms so we see post-drag tail.
  await page.waitForTimeout(500);
  const duringDrag = await page.evaluate(() => Array.from(getOutputCapture()));
  if (hasNonFinite(duringDrag)) {
    throw new Error(`during-drag capture contains NaN/Inf — slider drag corrupted the audio chain`);
  }
  const dragPeak = peakOf(duringDrag);
  const dragEnergy = energyOf(duringDrag);
  console.log(`[TEST] during-drag capture: ${duringDrag.length} samples, peak=${dragPeak.toFixed(4)}, energy=${dragEnergy.toFixed(6)}`);

  // Even with pitch=19 (high), the snare must still produce some audio.
  // We allow energy to drop because pitch-shifting time-compresses the
  // sample, but the peak should still be non-trivial.
  if (dragPeak < 0.001) {
    throw new Error(
      `BUG: audio stopped during slider drag — peak=${dragPeak} (baseline=${basePeak}). ` +
      `This is the user-reported "audio stops in WASM" symptom.`,
    );
  }

  // ─── Phase D: capture audio AFTER the drag completes ───
  // The most damning bug pattern: drag completes but audio stays
  // silent. Verifies the channel chain recovers.
  await page.evaluate(async () => {
    // Park the slider at a neutral value, then wait for the sequencer.
    setInstrumentParam("snare", "pitch", 0);
    clearOutputCapture();
    startOutputCapture();
  });
  await page.waitForTimeout(1500);
  const postDrag = await page.evaluate(() => Array.from(getOutputCapture()));
  if (hasNonFinite(postDrag)) {
    throw new Error(`post-drag capture contains NaN/Inf — slider drag permanently corrupted the audio chain`);
  }
  const postPeak = peakOf(postDrag);
  const postEnergy = energyOf(postDrag);
  console.log(`[TEST] post-drag capture: ${postDrag.length} samples, peak=${postPeak.toFixed(4)}, energy=${postEnergy.toFixed(6)}`);
  if (postPeak < 0.001) {
    throw new Error(
      `BUG: audio remained silent AFTER the drag completed — peak=${postPeak} (baseline=${basePeak}). ` +
      `This is the user-reported "audio stops, nothing can be heard" symptom.`,
    );
  }

  // The post-drag audio should be within a factor of 5 of baseline
  // (allowing for sequencer phase differences + slight headroom drift).
  const postRatio = postEnergy / Math.max(baseEnergy, 1e-12);
  if (postRatio < 0.2) {
    throw new Error(
      `audio energy collapsed post-drag: ${(postRatio * 100).toFixed(1)}% of baseline ` +
      `(post=${postEnergy} baseline=${baseEnergy})`,
    );
  }
  console.log(`[TEST] post-drag energy ratio = ${(postRatio * 100).toFixed(1)}% of baseline`);

  // ─── Phase E: surface BiquadFilterNode warnings + page errors ───
  await page.evaluate(() => stopPlay());

  if (pageErrors.length > 0) {
    throw new Error(`page emitted ${pageErrors.length} errors:\n  ${pageErrors.join("\n  ")}`);
  }
  // BiquadFilterNode warnings are a real symptom of NaN/Inf propagating
  // through the EQ chain. Any warning during the test is a regression.
  if (biquadWarnings.length > 0) {
    throw new Error(
      `${biquadWarnings.length} BiquadFilterNode warning(s) emitted during the test — the audio chain went into a bad state.\n` +
      `Sample: ${biquadWarnings[0]}`,
    );
  }

  console.log("[TEST] End-to-end synth-slider audio test passed.");

  if (isCoverageEnabled()) {
    await flushCoverage(
      page,
      new URL("../../coverage/browser-raw", import.meta.url).pathname,
      "instrument_params_e2e_audio",
    );
  }
} catch (err) {
  console.error("instrument_params_e2e_audio FAILED:", err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

console.log(exitCode === 0 ? "instrument_params_e2e_audio browser test PASSED" : "instrument_params_e2e_audio browser test FAILED");
process.exit(exitCode);
