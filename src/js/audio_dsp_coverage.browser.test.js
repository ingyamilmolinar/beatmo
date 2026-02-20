// audio_dsp_coverage.browser.test.js
//
// Exercises Go audio DSP paths through WASM:
//   1. Compressor signal path — multi-instrument max volume, peak limiting
//   2. EQ filter path — lowpass reduces high-frequency energy
//   3. Multi-voice accumulation — no NaN/Inf in output
//   4. Headroom verification — per-voice headroom prevents clipping
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_dsp_coverage.browser.test.js

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
    typeof setChannelEQ === "function" &&
    typeof ensureSynthSample === "function"
  );

  // Trigger user gesture to unlock audio context.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);

  // Wait for AudioContext to reach "running" state.
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );

  // Wait for DSP module load and synth sample rendering to complete.
  await page.evaluate(() => window.audioReady);

  // ========================================================================
  // Scenario 1: Compressor signal path
  //
  // Play 4+ instruments simultaneously at max volume. The compressor/limiter
  // should prevent the output peak from exceeding 1.0. We compare the multi-
  // instrument peak against the sum of individual peaks to verify compression.
  // ========================================================================
  let s1;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 1: Compressor signal path (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s1 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      const instruments = ["kick", "snare", "hihat", "tom", "clap", "cowbell"];
      for (const inst of instruments) {
        setChannelVolume(inst, 1.0);
        await ensureSynthSample(inst);
      }
      await new Promise((r) => setTimeout(r, 50));

      const NOISE_FLOOR = 0.0001;

      // Helper: warmup audio pipeline by polling for real signal.
      async function warmup() {
        startOutputCapture();
        playSound("kick", 1.0);
        const deadline = Date.now() + 5000;
        let signal = false;
        while (Date.now() < deadline) {
          const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
          if (snap && snap.length > 0) {
            for (let i = 0; i < snap.length; i++) {
              if (Math.abs(snap[i]) > 0.01) { signal = true; break; }
            }
          }
          if (signal) break;
          await new Promise((r) => setTimeout(r, 100));
        }
        if (signal) {
          await new Promise((r) => setTimeout(r, 300));
        } else {
          playSound("kick", 1.0);
          await new Promise((r) => setTimeout(r, 1500));
        }
        clearOutputCapture();
      }

      await warmup();

      // Measure individual instrument peaks first.
      const individualPeaks = {};
      for (const inst of instruments) {
        clearOutputCapture();
        playSound(inst, 1.0);
        await new Promise((r) => setTimeout(r, 500));
        const captured = Array.from(getOutputCapture());
        let peak = 0;
        for (let i = 0; i < captured.length; i++) {
          const a = Math.abs(captured[i]);
          if (a > peak) peak = a;
        }
        individualPeaks[inst] = peak;
      }

      // Now play all 6 simultaneously (3 rounds for reliability).
      clearOutputCapture();
      for (let round = 0; round < 3; round++) {
        for (const inst of instruments) {
          playSound(inst, 1.0);
        }
        await new Promise((r) => setTimeout(r, 500));
      }
      const combined = Array.from(stopOutputCapture());

      let combinedPeak = 0;
      let activeSamples = 0;
      for (let i = 0; i < combined.length; i++) {
        const a = Math.abs(combined[i]);
        if (a > combinedPeak) combinedPeak = a;
        if (a > NOISE_FLOOR) activeSamples++;
      }

      const sumOfPeaks = Object.values(individualPeaks).reduce((a, b) => a + b, 0);

      return {
        individualPeaks,
        sumOfPeaks,
        combinedPeak,
        activeSamples,
        totalSamples: combined.length,
      };
    }, attempt);

    console.log(`  Individual peaks: ${JSON.stringify(s1.individualPeaks)}`);
    console.log(`  Sum of individual peaks: ${s1.sumOfPeaks.toFixed(4)}`);
    console.log(`  Combined peak (all 6 simultaneous): ${s1.combinedPeak.toFixed(4)}`);
    console.log(`  Active samples: ${s1.activeSamples}/${s1.totalSamples}`);
    if (s1.activeSamples > 0 && s1.combinedPeak > 0) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: no audio signal detected, retrying...`);
      await page.waitForTimeout(1000);
    }
  }

  if (s1.activeSamples === 0) throw new Error("Scenario 1 FAIL: no audio energy captured");
  // The limiter/compressor should keep output at or below 1.0.
  if (s1.combinedPeak > 1.01) {
    throw new Error(`Scenario 1 FAIL: combined peak ${s1.combinedPeak.toFixed(4)} exceeds limiter threshold 1.0`);
  }
  // With 6 instruments at full volume, sum of peaks should be well above 1.0, but
  // the combined output should be limited. Verify compression is happening.
  if (s1.sumOfPeaks > 1.5 && s1.combinedPeak >= s1.sumOfPeaks) {
    throw new Error(`Scenario 1 FAIL: no compression detected (combined=${s1.combinedPeak.toFixed(4)} >= sum=${s1.sumOfPeaks.toFixed(4)})`);
  }
  console.log("  PASS");

  await page.waitForTimeout(500);

  // ========================================================================
  // Scenario 2: EQ filter path
  //
  // Apply a strong lowpass EQ filter on hihat (a bright instrument). Compare
  // the output spectrum with and without the filter. The filtered version
  // should have reduced high-frequency energy.
  // ========================================================================
  let s2;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 2: EQ filter path (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s2 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      await ensureSynthSample("hihat");
      setChannelVolume("hihat", 1.0);
      await new Promise((r) => setTimeout(r, 50));

      const NOISE_FLOOR = 0.0001;

      // Helper: measure hihat with given EQ config, compute active RMS and
      // high-frequency energy estimate via simple zero-crossing rate.
      async function measureHihat(eqBands) {
        setChannelEQ("hihat", eqBands);
        await new Promise((r) => setTimeout(r, 100));
        clearOutputCapture();
        for (let i = 0; i < 3; i++) {
          playSound("hihat", 1.0);
          await new Promise((r) => setTimeout(r, 400));
        }
        const captured = Array.from(getOutputCapture());
        let peak = 0, sum = 0, active = 0;
        let zeroCrossings = 0;
        for (let i = 0; i < captured.length; i++) {
          const a = Math.abs(captured[i]);
          if (a > peak) peak = a;
          if (a > NOISE_FLOOR) { sum += captured[i] * captured[i]; active++; }
          if (i > 0 && captured[i] * captured[i - 1] < 0) zeroCrossings++;
        }
        const rms = active > 0 ? Math.sqrt(sum / active) : 0;
        // Zero-crossing rate is a rough proxy for high-frequency content.
        const zeroCrossingRate = captured.length > 1 ? zeroCrossings / (captured.length - 1) : 0;
        return { rms, peak, active, total: captured.length, zeroCrossingRate };
      }

      // Warmup.
      startOutputCapture();
      playSound("hihat", 1.0);
      const deadline = Date.now() + 5000;
      let signal = false;
      while (Date.now() < deadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.005) { signal = true; break; }
          }
        }
        if (signal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (signal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound("hihat", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Baseline: no EQ.
      const baseline = await measureHihat([]);

      // Lowpass at 500 Hz — should drastically cut high frequencies from hihat.
      const lowpassed = await measureHihat([
        { type: "lowpass", freq: 500, q: 1, gainDB: 0 },
      ]);

      stopOutputCapture();
      setChannelEQ("hihat", []);

      return {
        baselineRMS: baseline.rms,
        baselineZCR: baseline.zeroCrossingRate,
        baselineActive: baseline.active,
        baselineTotal: baseline.total,
        lowpassRMS: lowpassed.rms,
        lowpassZCR: lowpassed.zeroCrossingRate,
        lowpassActive: lowpassed.active,
        lowpassTotal: lowpassed.total,
      };
    }, attempt);

    console.log(`  Baseline: RMS=${s2.baselineRMS.toFixed(6)}, ZCR=${s2.baselineZCR.toFixed(4)}, active=${s2.baselineActive}/${s2.baselineTotal}`);
    console.log(`  Lowpass 500Hz: RMS=${s2.lowpassRMS.toFixed(6)}, ZCR=${s2.lowpassZCR.toFixed(4)}, active=${s2.lowpassActive}/${s2.lowpassTotal}`);
    if (s2.baselineRMS >= 0.0001 && s2.baselineActive > 0) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: measurements unreliable, retrying...`);
      await page.waitForTimeout(1000);
    }
  }

  if (s2.baselineRMS < 0.0001) {
    throw new Error(`Scenario 2 FAIL: baseline has no energy (RMS=${s2.baselineRMS})`);
  }
  // The lowpass filter should reduce high-frequency energy. We check two indicators:
  // 1. Zero-crossing rate should decrease (fewer high-freq oscillations).
  // 2. RMS may also decrease since hihat energy is predominantly high-frequency.
  // The lowpass filter should reduce either high-frequency energy (ZCR) or overall RMS.
  // Use a generous 5% threshold to account for WebAudio biquad implementation variance.
  const zcrReduced = s2.baselineZCR > 0 && s2.lowpassZCR < s2.baselineZCR * 0.95;
  const rmsReduced = s2.baselineRMS > 0 && s2.lowpassRMS < s2.baselineRMS * 0.95;
  if (s2.baselineZCR > 0) {
    const zcrReduction = ((1 - s2.lowpassZCR / s2.baselineZCR) * 100).toFixed(1);
    console.log(`  Zero-crossing rate reduction: ${zcrReduction}%`);
  }
  if (s2.baselineRMS > 0) {
    const rmsReduction = ((1 - s2.lowpassRMS / s2.baselineRMS) * 100).toFixed(1);
    console.log(`  RMS reduction: ${rmsReduction}%`);
  }
  if (!zcrReduced && !rmsReduced) {
    throw new Error(`Scenario 2 FAIL: lowpass filter had no measurable effect (ZCR baseline=${s2.baselineZCR.toFixed(4)}, lowpass=${s2.lowpassZCR.toFixed(4)}; RMS baseline=${s2.baselineRMS.toFixed(6)}, lowpass=${s2.lowpassRMS.toFixed(6)})`);
  }
  console.log("  PASS");

  await page.waitForTimeout(500);

  // ========================================================================
  // Scenario 3: Multi-voice accumulation — no NaN/Inf
  //
  // Rapidly trigger many instruments simultaneously to stress the mixer's
  // accumulation path. Verify the output contains no NaN or Infinity values.
  // ========================================================================
  let s3;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 3: Multi-voice accumulation (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s3 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      const instruments = ["kick", "snare", "hihat", "tom", "clap", "cowbell"];
      for (const inst of instruments) {
        setChannelVolume(inst, 1.0);
        await ensureSynthSample(inst);
      }
      await new Promise((r) => setTimeout(r, 50));

      // Warmup.
      startOutputCapture();
      playSound("kick", 1.0);
      const deadline = Date.now() + 5000;
      let signal = false;
      while (Date.now() < deadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { signal = true; break; }
          }
        }
        if (signal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (signal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Rapid-fire: play all instruments 5 times in quick succession (~10ms apart).
      // This creates 30 simultaneous voices stressing the accumulation path.
      for (let burst = 0; burst < 5; burst++) {
        for (const inst of instruments) {
          playSound(inst, 1.0);
        }
        await new Promise((r) => setTimeout(r, 10));
      }
      // Wait for all voices to decay.
      await new Promise((r) => setTimeout(r, 2000));

      const captured = Array.from(stopOutputCapture());

      let nanCount = 0, infCount = 0, peak = 0, activeSamples = 0;
      for (let i = 0; i < captured.length; i++) {
        const v = captured[i];
        if (Number.isNaN(v)) { nanCount++; continue; }
        if (!Number.isFinite(v)) { infCount++; continue; }
        const a = Math.abs(v);
        if (a > peak) peak = a;
        if (a > 0.0001) activeSamples++;
      }

      return {
        totalSamples: captured.length,
        activeSamples,
        peak,
        nanCount,
        infCount,
      };
    }, attempt);

    console.log(`  Samples: ${s3.activeSamples}/${s3.totalSamples} active, Peak: ${s3.peak.toFixed(4)}`);
    console.log(`  NaN count: ${s3.nanCount}, Inf count: ${s3.infCount}`);
    if (s3.activeSamples > 0) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: no audio signal detected, retrying...`);
      await page.waitForTimeout(1000);
    }
  }

  if (s3.nanCount > 0) {
    throw new Error(`Scenario 3 FAIL: ${s3.nanCount} NaN samples detected in output`);
  }
  if (s3.infCount > 0) {
    throw new Error(`Scenario 3 FAIL: ${s3.infCount} Infinity samples detected in output`);
  }
  if (s3.activeSamples === 0) {
    throw new Error("Scenario 3 FAIL: no audio energy captured during rapid-fire playback");
  }
  console.log("  PASS");

  await page.waitForTimeout(500);

  // ========================================================================
  // Scenario 4: Headroom verification
  //
  // Play a small number of voices (1-2 instruments) and verify that per-voice
  // headroom (0.25x attenuation on desktop; WebAudio gain bus on browser)
  // keeps all output samples within [-1, 1].
  // ========================================================================
  let s4;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 4: Headroom verification (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s4 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      // Use only 2 instruments to stay well within headroom.
      const instruments = ["kick", "snare"];
      for (const inst of instruments) {
        setChannelVolume(inst, 1.0);
        await ensureSynthSample(inst);
      }
      await new Promise((r) => setTimeout(r, 50));

      // Warmup.
      startOutputCapture();
      playSound("kick", 1.0);
      const deadline = Date.now() + 5000;
      let signal = false;
      while (Date.now() < deadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { signal = true; break; }
          }
        }
        if (signal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (signal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Play 2 instruments, one at a time, 3 rounds each.
      for (let round = 0; round < 3; round++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 300));
        playSound("snare", 1.0);
        await new Promise((r) => setTimeout(r, 300));
      }

      const captured = Array.from(stopOutputCapture());

      let peak = 0, activeSamples = 0, clippedCount = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        if (a > 0.0001) activeSamples++;
        if (a > 1.0) clippedCount++;
      }

      return {
        totalSamples: captured.length,
        activeSamples,
        peak,
        clippedCount,
      };
    }, attempt);

    console.log(`  Samples: ${s4.activeSamples}/${s4.totalSamples} active, Peak: ${s4.peak.toFixed(6)}`);
    console.log(`  Clipped samples: ${s4.clippedCount}`);
    if (s4.activeSamples > 0) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: no audio signal detected, retrying...`);
      await page.waitForTimeout(1000);
    }
  }

  if (s4.activeSamples === 0) {
    throw new Error("Scenario 4 FAIL: no audio energy captured");
  }
  // With only 2 instruments at normal volume, headroom should keep everything in [-1, 1].
  if (s4.peak > 1.0) {
    throw new Error(`Scenario 4 FAIL: peak ${s4.peak.toFixed(6)} exceeds 1.0 — headroom insufficient for 2 voices`);
  }
  if (s4.clippedCount > 0) {
    throw new Error(`Scenario 4 FAIL: ${s4.clippedCount} samples exceeded [-1, 1] range`);
  }
  console.log("  PASS");

  console.log("\nAll audio DSP coverage tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_dsp_coverage");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
