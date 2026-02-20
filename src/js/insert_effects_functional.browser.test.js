// insert_effects_functional.browser.test.js
//
// Functional tests verifying ALL insert effects actually modify audio in WASM.
// For each of the 18 registered effects:
//   1. Capture a dry baseline (no effects)
//   2. Add the effect with aggressive parameters
//   3. Capture audio with the effect enabled
//   4. Assert the output meaningfully differs from baseline
//
// This catches the class of bug where an effect's WebAudio fallback is a
// passthrough (e.g., pitchshift, gate, transient, phaser, autowah) — the
// captured audio would be identical to baseline, failing the diff check.
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/insert_effects_functional.browser.test.js

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

// Effect configurations: aggressive params to maximize audible difference.
const EFFECT_CONFIGS = [
  { type: "distortion",  params: { drive: 15, tone: 4000, mix: 1 } },
  { type: "delay",       params: { time: 200, feedback: 0.6, mix: 0.8 } },
  { type: "reverb",      params: { room: 0.8, damping: 0.5, mix: 0.8 } },
  { type: "chorus",      params: { rate: 3, depth: 10, mix: 0.8 } },
  { type: "bitcrusher",  params: { bits: 4, rate: 0.2, mix: 1 } },
  { type: "filter",      params: { mode: 0, cutoff: 200, q: 2, mix: 1 } },
  { type: "phaser",      params: { stages: 8, rate: 2, depth: 1, feedback: 0.8, mix: 1 } },
  { type: "flanger",     params: { rate: 2, depth: 5, feedback: 0.8, mix: 1 } },
  { type: "tremolo",     params: { rate: 8, depth: 1, shape: 0, mix: 1 } },
  { type: "gate",        params: { threshold: -10, attack: 1, release: 10, range: -90 } },
  { type: "limiter",     params: { threshold: -20, release: 50, ceiling: -6 } },
  { type: "ringmod",     params: { frequency: 300, shape: 0, mix: 1 } },
  { type: "waveshaper",  params: { curve: 2, drive: 10, mix: 1 } },
  { type: "autowah",     params: { sensitivity: 1, rate: 5, depth: 1, mix: 1 } },
  { type: "compressor",  params: { threshold: -30, ratio: 20, attack: 0.1, release: 50, makeup: 12, mix: 1 } },
  { type: "transient",   params: { attack: 200, sustain: 30, speed: 5 } },
  { type: "tape",        params: { drive: 8, warmth: 0.8, wow: 0.5, flutter: 0.5, mix: 1 } },
  { type: "pitchshift",  params: { pitch: 12, mix: 1, window: 50 } },
];

const MAX_RETRIES = 3;
const NOISE_FLOOR = 0.0001;

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  if (process.env.TEST_LOG) {
    page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
  }
  await page.goto(`http://localhost:${port}/`);

  // Wait for all required JS exports.
  await page.waitForFunction(() =>
    typeof addInsertEffectJS === "function" &&
    typeof removeInsertEffectJS === "function" &&
    typeof getInsertEffectsJS === "function" &&
    typeof insertEffectCatalogJS === "function" &&
    typeof setInsertEffectParamJS === "function" &&
    typeof playSound === "function" &&
    typeof ensureSynthSample === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof resetOutputCaptureNode === "function"
  );

  // Unlock audio context.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await page.evaluate(() => window.audioReady);

  // Settle: let game loop finish buildDemo / initial import.
  await page.waitForTimeout(500);

  // Ensure the kick sample is pre-rendered for consistent captures.
  await page.evaluate(() => ensureSynthSample("kick"));

  // Clean slate: remove any leftover effects.
  await page.evaluate(() => {
    const json = getInsertEffectsJS("kick");
    const slots = JSON.parse(json || "[]") || [];
    for (let i = slots.length - 1; i >= 0; i--) removeInsertEffectJS("kick", i);
  });

  // ========================================================================
  // Helper: capture audio with optional effect
  // Returns { rms, peak, samples, sampleCount }
  // ========================================================================
  async function captureAudio(page, effectType, effectParams, retryAttempt) {
    return await page.evaluate(async ({ effectType, effectParams, retryAttempt, NOISE_FLOOR }) => {
      if (retryAttempt > 1) resetOutputCaptureNode();

      // Clean slate
      const existing = JSON.parse(getInsertEffectsJS("kick") || "[]") || [];
      for (let i = existing.length - 1; i >= 0; i--) removeInsertEffectJS("kick", i);

      // Add effect if specified
      if (effectType) {
        const idx = addInsertEffectJS("kick", effectType);
        if (effectParams) {
          for (const [name, value] of Object.entries(effectParams)) {
            setInsertEffectParamJS("kick", idx, name, value);
          }
        }
        // Verify effect was added
        const check = JSON.parse(getInsertEffectsJS("kick"));
        if (!check || check.length === 0) {
          return { error: `Failed to add effect ${effectType}` };
        }
      }

      // Start capture
      startOutputCapture();

      // Warmup: play kick and poll for real audio signal
      playSound("kick", 1.0);
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = getOutputCapture();
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
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Actual measurement: play 3 kicks with spacing
      for (let i = 0; i < 3; i++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 400));
      }

      const captured = stopOutputCapture();
      const sampleCount = captured.length;

      // Compute RMS over active samples (above noise floor)
      let sumSq = 0;
      let activeCount = 0;
      let peak = 0;
      // Also collect raw samples for correlation analysis (first 4096)
      const rawSamples = [];
      for (let i = 0; i < captured.length; i++) {
        const v = captured[i];
        const a = Math.abs(v);
        if (a > peak) peak = a;
        if (a > NOISE_FLOOR) {
          sumSq += v * v;
          activeCount++;
        }
        if (i < 4096) rawSamples.push(v);
      }
      const rms = activeCount > 0 ? Math.sqrt(sumSq / activeCount) : 0;

      // Remove effect for clean slate
      if (effectType) {
        const slots = JSON.parse(getInsertEffectsJS("kick") || "[]") || [];
        for (let i = slots.length - 1; i >= 0; i--) removeInsertEffectJS("kick", i);
      }

      return { rms, peak, sampleCount, activeCount, rawSamples };
    }, { effectType, effectParams, retryAttempt, NOISE_FLOOR });
  }

  // ========================================================================
  // Scenario 1: Capture dry baseline
  // ========================================================================
  let baseline;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Baseline capture (attempt ${attempt}/${MAX_RETRIES}) ---`);
    baseline = await captureAudio(page, null, null, attempt);
    if (baseline.error) throw new Error(`Baseline FAIL: ${baseline.error}`);
    console.log(`  Samples: ${baseline.sampleCount}, Active: ${baseline.activeCount}, RMS: ${baseline.rms.toFixed(6)}, Peak: ${baseline.peak.toFixed(6)}`);
    if (baseline.activeCount > 0 && baseline.rms > 0.001) break;
    if (attempt < MAX_RETRIES) {
      console.log("  WARNING: weak baseline signal, retrying...");
      await page.waitForTimeout(1000);
    }
  }
  if (baseline.activeCount === 0) throw new Error("Baseline FAIL: no active samples captured");
  if (baseline.rms < 0.001) throw new Error(`Baseline FAIL: RMS too low (${baseline.rms})`);
  console.log("  PASS\n");

  // ========================================================================
  // Scenario 2: Test each effect modifies audio
  // ========================================================================
  let passed = 0;
  let failed = 0;
  const failures = [];

  for (const config of EFFECT_CONFIGS) {
    let result;
    let testPassed = false;

    for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
      console.log(`--- Effect: ${config.type} (attempt ${attempt}/${MAX_RETRIES}) ---`);
      result = await captureAudio(page, config.type, config.params, attempt);

      if (result.error) {
        console.log(`  ERROR: ${result.error}`);
        if (attempt < MAX_RETRIES) {
          await page.waitForTimeout(1000);
          continue;
        }
        break;
      }

      console.log(`  Samples: ${result.sampleCount}, Active: ${result.activeCount}, RMS: ${result.rms.toFixed(6)}, Peak: ${result.peak.toFixed(6)}`);

      if (result.activeCount === 0 || result.rms < 0.0001) {
        console.log("  WARNING: no active samples, retrying...");
        if (attempt < MAX_RETRIES) {
          await page.waitForTimeout(1000);
          continue;
        }
        break;
      }

      // Compare with baseline using multiple metrics:
      // 1. RMS difference (catches energy-changing effects)
      const rmsDiff = Math.abs(result.rms - baseline.rms);
      const rmsRelDiff = baseline.rms > 0 ? rmsDiff / baseline.rms : 0;

      // 2. Peak difference (catches clipping/limiting effects)
      const peakDiff = Math.abs(result.peak - baseline.peak);
      const peakRelDiff = baseline.peak > 0 ? peakDiff / baseline.peak : 0;

      // 3. Sample-level correlation (catches waveform-changing effects that preserve energy)
      let correlation = 1.0;
      if (result.rawSamples && baseline.rawSamples) {
        const len = Math.min(result.rawSamples.length, baseline.rawSamples.length, 4096);
        if (len > 100) {
          let sumXY = 0, sumX2 = 0, sumY2 = 0;
          for (let i = 0; i < len; i++) {
            const x = baseline.rawSamples[i] || 0;
            const y = result.rawSamples[i] || 0;
            sumXY += x * y;
            sumX2 += x * x;
            sumY2 += y * y;
          }
          const denom = Math.sqrt(sumX2 * sumY2);
          correlation = denom > 1e-10 ? sumXY / denom : 0;
        }
      }

      console.log(`  RMS diff: ${(rmsRelDiff * 100).toFixed(1)}%, Peak diff: ${(peakRelDiff * 100).toFixed(1)}%, Correlation: ${correlation.toFixed(4)}`);

      // Effect passes if ANY of these conditions hold:
      // - RMS changed by >5% relative
      // - Peak changed by >5% relative
      // - Waveform correlation dropped below 0.95
      const rmsChanged = rmsRelDiff > 0.05;
      const peakChanged = peakRelDiff > 0.05;
      const waveformChanged = correlation < 0.95;

      if (rmsChanged || peakChanged || waveformChanged) {
        testPassed = true;
        break;
      }

      console.log("  WARNING: effect did not measurably change audio");
      if (attempt < MAX_RETRIES) {
        await page.waitForTimeout(1000);
      }
    }

    if (testPassed) {
      console.log(`  PASS\n`);
      passed++;
    } else {
      const reason = result?.error || "effect did not modify audio (possible passthrough)";
      console.log(`  FAIL: ${reason}\n`);
      failures.push({ type: config.type, reason });
      failed++;
    }
  }

  // ========================================================================
  // Summary
  // ========================================================================
  console.log("========================================");
  console.log(`Results: ${passed} passed, ${failed} failed out of ${EFFECT_CONFIGS.length} effects`);
  if (failures.length > 0) {
    console.log("\nFailed effects:");
    for (const f of failures) {
      console.log(`  - ${f.type}: ${f.reason}`);
    }
    exitCode = 1;
  } else {
    console.log("\nAll insert effects functional tests passed.");
  }

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "insert_effects_functional");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
