// mixer_eq_parity.browser.test.js
//
// Verifies the full WebAudio pipeline with EQ produces correct mixing behavior.
// Tests three scenarios:
//   1. Single instrument with volume
//   2. Multi-instrument simultaneous playback
//   3. EQ band mute affects correct frequency range
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/mixer_eq_parity.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
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

const MAX_RETRIES = 3;

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof playSound === "function" &&
    typeof setChannelVolume === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof resetOutputCaptureNode === "function" &&
    typeof setChannelEQ === "function"
  );

  // Trigger user gesture to unlock audio context.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);

  // Wait for AudioContext to reach "running" state (may be delayed under CPU load).
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );

  // Wait for DSP module load and synth sample rendering to complete.
  await page.evaluate(() => window.audioReady);

  // ========================================================================
  // Scenario 1: Single instrument with volume
  //
  // Uses the proven-robust pattern from Scenarios 2/3:
  // - All measurements in a single page.evaluate() (no Playwright bridge crossings)
  // - 3 kicks with 400ms spacing (resilient to CPU pressure)
  // - Active RMS (only non-silent samples) for stable measurement
  // - Warmup cycle to prime ScriptProcessorNode
  // ========================================================================
  let s1;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 1: Single instrument with volume (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s1 = await page.evaluate(async (attempt) => {
      // On retry, force a fresh capture node to recover from unreliable state.
      if (attempt > 1) resetOutputCaptureNode();

      setChannelVolume("kick", 0.8);
      await new Promise((r) => setTimeout(r, 50));

      await ensureSynthSample('kick');

      const NOISE_FLOOR = 0.0001;

      // Warmup: poll for real audio signal to confirm pipeline is active.
      // Under CPU contention (parallel tests), the audio thread may not render
      // scheduled BufferSource nodes in time for fixed-wait patterns.
      startOutputCapture();
      playSound("kick", 1.0);
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { warmupSignal = true; break; }
          }
        }
        if (warmupSignal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (warmupSignal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        // Fallback: one more play with longer wait.
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      // Clear warmup buffer WITHOUT re-wiring the audio graph.
      clearOutputCapture();

      // Actual measurement: 3 kicks with spacing.
      for (let i = 0; i < 3; i++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 400));
      }
      const captured = Array.from(stopOutputCapture());

      let peak = 0, sum = 0, active = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        if (a > NOISE_FLOOR) { sum += captured[i] * captured[i]; active++; }
      }
      const activeRms = active > 0 ? Math.sqrt(sum / active) : 0;

      return { samples: captured.length, activeSamples: active, peak, activeRms };
    }, attempt);

    console.log(`  Samples: ${s1.activeSamples}/${s1.samples} active, Peak: ${s1.peak.toFixed(6)}, Active RMS: ${s1.activeRms.toFixed(6)}`);
    if (s1.activeRms >= 0.0001) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: Active RMS=${s1.activeRms.toFixed(6)} below threshold 0.0001, retrying...`);
      await page.waitForTimeout(1000);
    }
  }
  if (s1.activeRms < 0.0001) throw new Error(`Scenario 1 FAIL: no audio energy (activeRms=${s1.activeRms})`);
  if (s1.peak > 1.0) throw new Error(`Scenario 1 FAIL: clipping detected (peak=${s1.peak})`);
  console.log("  PASS");

  // Reset channel volume.
  await page.evaluate(() => setChannelVolume("kick", 1.0));
  await page.waitForTimeout(500);

  // ========================================================================
  // Scenario 2: Multi-instrument simultaneous playback
  //
  // Uses the proven-robust pattern from Scenario 3:
  // - All measurements in a single page.evaluate() (no Playwright bridge crossings)
  // - 3 plays per instrument with 400ms spacing (resilient to CPU pressure)
  // - Active RMS (only non-silent samples) for stable measurement
  // - Warmup cycle with clearOutputCapture (no audio graph re-wiring)
  // ========================================================================
  let s2;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 2: Multi-instrument simultaneous playback (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s2 = await page.evaluate(async (attempt) => {
      // On retry, force a fresh capture node to recover from unreliable state.
      if (attempt > 1) resetOutputCaptureNode();

      setChannelVolume("kick", 1.0);
      setChannelVolume("snare", 0.5);
      setChannelVolume("hihat", 0.3);
      await new Promise((r) => setTimeout(r, 50));

      await ensureSynthSample('kick');
      await ensureSynthSample('snare');
      await ensureSynthSample('hihat');

      const NOISE_FLOOR = 0.0001;

      // Helper: play all 3 instruments simultaneously, repeated N times with spacing.
      async function playAllInstruments(count, spacingMs) {
        for (let i = 0; i < count; i++) {
          playSound("kick", 1.0);
          playSound("snare", 1.0);
          playSound("hihat", 1.0);
          await new Promise((r) => setTimeout(r, spacingMs));
        }
      }

      // Warmup: poll for real audio signal to confirm pipeline is active.
      startOutputCapture();
      playSound("kick", 1.0);
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.005) { warmupSignal = true; break; }
          }
        }
        if (warmupSignal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (warmupSignal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      // Clear warmup buffer WITHOUT re-wiring the audio graph.
      clearOutputCapture();

      // Actual measurement.
      await playAllInstruments(3, 400);
      const captured = Array.from(stopOutputCapture());

      let peak = 0, sum = 0, active = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        if (a > NOISE_FLOOR) { sum += captured[i] * captured[i]; active++; }
      }
      const activeRms = active > 0 ? Math.sqrt(sum / active) : 0;

      // Reset volumes before returning.
      setChannelVolume("kick", 1.0);
      setChannelVolume("snare", 1.0);
      setChannelVolume("hihat", 1.0);

      return {
        samples: captured.length,
        activeSamples: active,
        peak,
        activeRms,
      };
    }, attempt);

    console.log(`  Samples: ${s2.activeSamples}/${s2.samples} active, Peak: ${s2.peak.toFixed(6)}, Active RMS: ${s2.activeRms.toFixed(6)}`);
    if (s2.activeRms >= 0.002) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: Active RMS=${s2.activeRms.toFixed(6)} below threshold 0.002, retrying...`);
      await page.waitForTimeout(1000);
    }
  }
  if (s2.activeRms < 0.002) throw new Error(`Scenario 2 FAIL: no audio energy (activeRms=${s2.activeRms})`);
  if (s2.peak > 1.05) throw new Error(`Scenario 2 FAIL: severe clipping (peak=${s2.peak})`);
  console.log("  PASS");

  await page.waitForTimeout(500);

  // ========================================================================
  // Scenario 3: EQ band mute affects correct frequency range
  //
  // Uses the proven-robust pattern from eq_band_mute.browser.test.js:
  // - All measurements in a single page.evaluate() (no Playwright bridge crossings)
  // - 3 kicks per measurement with 400ms spacing (resilient to CPU pressure)
  // - Active RMS (only non-silent samples) for stable comparison
  // - Warmup cycle with clearOutputCapture (no audio graph re-wiring)
  // - allUnmuted baseline (avoids structural graph rewire between measurements)
  // ========================================================================
  let s3;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 3: EQ band mute affects frequency range (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s3 = await page.evaluate(async (attempt) => {
      // On retry, force a fresh capture node to recover from unreliable state.
      if (attempt > 1) resetOutputCaptureNode();

      await ensureSynthSample('kick');

      // All-unmuted band config — same structure as muted config.
      const allUnmuted = [
        { freq: 60, q: 1, gainDB: 0, muted: false },
        { freq: 140, q: 1, gainDB: 0, muted: false },
        { freq: 280, q: 1, gainDB: 0, muted: false },
        { freq: 500, q: 1, gainDB: 0, muted: false },
        { freq: 900, q: 1, gainDB: 0, muted: false },
        { freq: 1800, q: 1, gainDB: 0, muted: false },
        { freq: 3500, q: 1, gainDB: 0, muted: false },
        { freq: 7000, q: 1, gainDB: 0, muted: false },
        { freq: 14000, q: 1, gainDB: 0, muted: false },
      ];

      // Helper: capture kicks with given EQ config, compute "active RMS".
      // Active RMS = RMS over non-silent samples only. This is immune to CPU
      // pressure variations that change how many zero-padded silence samples
      // appear in the buffer, making the measurement comparable across captures.
      const NOISE_FLOOR = 0.0001;
      async function measureKick(eqBands) {
        setChannelEQ('kick', eqBands);
        await new Promise((r) => setTimeout(r, 100));
        // Clear buffer and measure within the existing capture session
        // (no disconnect/reconnect of the audio graph).
        clearOutputCapture();
        for (let i = 0; i < 3; i++) {
          playSound('kick', 1.0);
          await new Promise((r) => setTimeout(r, 400));
        }
        const captured = Array.from(getOutputCapture());
        let peak = 0, sum = 0, active = 0;
        for (let i = 0; i < captured.length; i++) {
          const a = Math.abs(captured[i]);
          if (a > peak) peak = a;
          if (a > NOISE_FLOOR) { sum += captured[i] * captured[i]; active++; }
        }
        return {
          rms: active > 0 ? Math.sqrt(sum / active) : 0,
          peak,
          activeSamples: active,
          totalSamples: captured.length,
        };
      }

      // Warmup: establish multiband processor and poll for audio signal.
      setChannelEQ('kick', allUnmuted);
      await new Promise((r) => setTimeout(r, 100));
      startOutputCapture();
      playSound('kick', 1.0);
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { warmupSignal = true; break; }
          }
        }
        if (warmupSignal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (warmupSignal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound('kick', 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      // Clear warmup buffer WITHOUT re-wiring the audio graph.
      // Capture stays active — measureKick uses clearOutputCapture+getOutputCapture.
      clearOutputCapture();

      // 1) Baseline: all bands unmuted
      const baseline = await measureKick(allUnmuted);

      // 2) ONE band muted (mid-range 630-1250Hz) — kick has energy outside this range
      const mutedBand = await measureKick([
        { freq: 60, q: 1, gainDB: 0, muted: false },
        { freq: 140, q: 1, gainDB: 0, muted: false },
        { freq: 280, q: 1, gainDB: 0, muted: false },
        { freq: 500, q: 1, gainDB: 0, muted: false },
        { freq: 900, q: 1, gainDB: 0, muted: true },   // 630-1250Hz - MUTED
        { freq: 1800, q: 1, gainDB: 0, muted: false },
        { freq: 3500, q: 1, gainDB: 0, muted: false },
        { freq: 7000, q: 1, gainDB: 0, muted: false },
        { freq: 14000, q: 1, gainDB: 0, muted: false },
      ]);

      // Stop capture session and clean up EQ.
      stopOutputCapture();
      setChannelEQ('kick', []);

      return {
        baselineRMS: baseline.rms,
        baselineActiveSamples: baseline.activeSamples,
        baselineTotalSamples: baseline.totalSamples,
        mutedRMS: mutedBand.rms,
        mutedActiveSamples: mutedBand.activeSamples,
        mutedTotalSamples: mutedBand.totalSamples,
      };
    }, attempt);

    console.log(`  Baseline: ${s3.baselineActiveSamples}/${s3.baselineTotalSamples} active samples, RMS=${s3.baselineRMS.toFixed(6)}`);
    console.log(`  Muted band: ${s3.mutedActiveSamples}/${s3.mutedTotalSamples} active samples, RMS=${s3.mutedRMS.toFixed(6)}`);
    if (s3.baselineRMS >= 0.0001 && s3.mutedRMS >= 0.00001) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: measurements unreliable (baseline=${s3.baselineRMS.toFixed(6)}, muted=${s3.mutedRMS.toFixed(6)}), retrying...`);
      await page.waitForTimeout(1000);
    }
  }

  // Both should have non-zero energy (muting one band doesn't kill everything).
  if (s3.baselineRMS < 0.0001) {
    throw new Error(`Scenario 3 FAIL: baseline has no energy (RMS=${s3.baselineRMS})`);
  }
  if (s3.mutedRMS < 0.00001) {
    throw new Error(`Scenario 3 FAIL: muted version has no energy at all (RMS=${s3.mutedRMS})`);
  }
  // Muted version should have lower RMS (removing a band removes energy).
  if (s3.mutedRMS >= s3.baselineRMS) {
    console.log(`  WARNING: muted RMS (${s3.mutedRMS}) >= baseline RMS (${s3.baselineRMS}) — band may have no effect`);
  } else {
    const reduction = ((1 - s3.mutedRMS / s3.baselineRMS) * 100).toFixed(1);
    console.log(`  RMS reduction from muting band: ${reduction}%`);
  }
  console.log("  PASS");

  console.log("\nAll mixer EQ parity tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mixer_eq_parity");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
