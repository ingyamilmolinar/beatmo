// Sounds through the full WebAudio pipeline, captured at the output, must match reference renders.
// Usage: GO=… node src/js/xplat_output_capture.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import { loadDesktopAudioReference } from "./browser_test_helpers.js";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright's Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  const { spawnSync } = await import("child_process");
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

// From this tree, never a committed snapshot.
let desktopData;
try {
  desktopData = loadDesktopAudioReference();
  console.log(`Rendered desktop audio reference (version ${desktopData.version})`);
} catch (err) {
  console.error(`Failed to render the desktop audio reference: ${err.message}`);
  process.exit(1);
}

function computePeak(data) {
  let peak = 0;
  for (let i = 0; i < data.length; i++) {
    const a = Math.abs(data[i]);
    if (a > peak) peak = a;
  }
  return peak;
}

function computeRMS(data) {
  if (!data || data.length === 0) return 0;
  let sum = 0;
  for (let i = 0; i < data.length; i++) sum += data[i] * data[i];
  return Math.sqrt(sum / data.length);
}

// ref[i] against cap[i + lag] over the overlap; negative means inverted or decorrelated, not shifted. -2 when the
// overlap is too small.
function correlationAtLag(ref, cap, lag, from = 0, to = Infinity) {
  const start = Math.max(from, -lag);
  const end = Math.min(to, ref.length, cap.length - lag);
  const n = end - start;
  if (n < 100) return -2;
  let sx = 0, sy = 0, sxx = 0, syy = 0, sxy = 0;
  for (let i = start; i < end; i++) {
    const a = ref[i];
    const b = cap[i + lag];
    sx += a; sy += b;
    sxx += a * a; syy += b * b;
    sxy += a * b;
  }
  const num = n * sxy - sx * sy;
  const den = Math.sqrt((n * sxx - sx * sx) * (n * syy - sy * sy)) || 1;
  return num / den;
}

// Energy over time: robust to the capture's sub-sample phase jitter, inversion-blind (so paired with a signed guard).
function rmsEnvelope(arr, win) {
  const nb = Math.floor(arr.length / win);
  const out = new Float32Array(nb);
  for (let b = 0; b < nb; b++) {
    let s = 0;
    const base = b * win;
    for (let i = 0; i < win; i++) {
      const v = arr[base + i];
      s += v * v;
    }
    out[b] = Math.sqrt(s / win);
  }
  return out;
}

// The lag maximizing signed correlation; a negative best is inversion, not a benign shift.
function bestLagCorrelation(ref, cap, maxLag, from = 0, to = Infinity) {
  let bestLag = 0;
  let bestCorr = -Infinity;
  for (let lag = -maxLag; lag <= maxLag; lag++) {
    const c = correlationAtLag(ref, cap, lag, from, to);
    if (c > bestCorr) {
      bestCorr = c;
      bestLag = lag;
    }
  }
  return { bestLag, bestCorr };
}

// Trim leading silence from a buffer
function trimLeadingSilence(arr, threshold = 1e-4) {
  let idx = 0;
  while (idx < arr.length && Math.abs(arr[idx]) <= threshold) idx++;
  return arr.slice(idx);
}

// Trim trailing silence from a buffer
function trimTrailingSilence(arr, threshold = 1e-4) {
  let idx = arr.length - 1;
  while (idx >= 0 && Math.abs(arr[idx]) <= threshold) idx--;
  return arr.slice(0, idx + 1);
}

// Trim both leading and trailing silence
function trimSilence(arr, threshold = 1e-4) {
  return trimTrailingSilence(trimLeadingSilence(arr, threshold), threshold);
}

// The REAL app: a page with audio.js alone has no instruments, and once rendered every family as the sustained bare
// voice, whose tail spilled into the next window and "found" inverted kicks.
if (!buildMainWasm({ logLevel: "ERROR" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;
console.log(`Test server running on port ${port}`);

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on('console', (msg) => {
  if (process.env.TEST_LOG) console.log('[PAGE]', msg.type(), msg.text());
});

await page.goto(`http://localhost:${port}/`);
// Exports exist AND the readiness gate opened.
await page.waitForFunction(() => typeof window.playSound === 'function' &&
  typeof window.warmupProgress === 'function' && window.warmupProgress().ready === true,
  null, { timeout: 90000, polling: 200 });
console.log('App loaded and warm');

console.log('\n--- TEST 1: OUTPUT CAPTURE MECHANISM ---');

const captureTest = await page.evaluate(async () => {
  // Enable capture before any audio
  window.enableOutputCapture();

  const stats1 = window.getOutputCaptureStats();
  if (!stats1.hasNode) {
    return { error: 'Capture node not created after enableOutputCapture()' };
  }

  // Start capturing
  window.startOutputCapture();

  // Play a kick drum
  await window.ensureSynthSample('kick');
  await window.playSound('kick', 1.0);

  // Wait for sound to complete (kick is about 0.5s)
  await new Promise(r => setTimeout(r, 800));

  // Get captured samples
  await window.flushOutputCapture?.(); // every block captured so far has arrived
  const captured = window.stopOutputCapture();
  const stats2 = window.getOutputCaptureStats();

  // Analyze captured audio
  let peak = 0;
  let nonZeroCount = 0;
  for (let i = 0; i < captured.length; i++) {
    const a = Math.abs(captured[i]);
    if (a > peak) peak = a;
    if (a > 1e-6) nonZeroCount++;
  }

  return {
    capturedSamples: captured.length,
    sampleRate: stats2.sampleRate,
    durationSec: captured.length / stats2.sampleRate,
    peak,
    nonZeroCount,
    hasAudio: nonZeroCount > 100 && peak > 0.01,
  };
});

if (captureTest.error) {
  console.log(`FAIL: ${captureTest.error}`);
  process.exit(1);
}

console.log(`  Captured samples: ${captureTest.capturedSamples}`);
console.log(`  Sample rate: ${captureTest.sampleRate} Hz`);
console.log(`  Duration: ${captureTest.durationSec.toFixed(3)}s`);
console.log(`  Peak: ${captureTest.peak.toFixed(6)}`);
console.log(`  Non-zero samples: ${captureTest.nonZeroCount}`);

if (!captureTest.hasAudio) {
  console.log('FAIL: Capture did not record any audio');
  process.exit(1);
}
console.log('  OK: Capture mechanism working');

// Against references rendered in the page at its own rate, so both sides match in rate.

console.log('\n--- TEST 2: COMPARE CAPTURED OUTPUT WITH WASM REFERENCE ---');

// None is a row of the startup circuit, so each plays through a clean channel; the demo's snare row carries EQ and FX.
const instrumentsToTest = ['kick', 'rimshot', 'hihat'];
const results = {};
let anyFailed = false;
const failures = [];

for (const instName of instrumentsToTest) {
  console.log(`\nTesting ${instName}...`);

  // The SAME cached render as the reference, so random components (noise) still match.
  const testResult = await page.evaluate(async ({ instName }) => {
    // First, get the sample rate
    const sampleRate = window.__audioCtx?.sampleRate || 48000;

    // Ensure sample is rendered and cached (this is what playSound will use)
    await window.ensureSynthSample(instName);

    // Exactly the data playSound will use.
    const reference = window.getCachedRenderData(instName);
    if (!reference) {
      throw new Error(`No cached render for ${instName}`);
    }

    // Clear previous capture and start
    window.clearOutputCapture();
    window.startOutputCapture();

    // The normal playSound path through the channels.
    await window.playSound(instName, 1.0);

    // Wait for sound to complete
    await new Promise(r => setTimeout(r, 1000));

    // Stop and get samples
    await window.flushOutputCapture?.(); // every block captured so far has arrived
    const captured = Array.from(window.stopOutputCapture());

    return {
      captured,
      reference,
      sampleRate,
    };
  }, { instName });

  // The capture is shifted by the variable lead and taken on block phase, so a lag-0 raw correlation under-reports a true
  // match; two alignment-robust measures follow.
  const captured = trimLeadingSilence(testResult.captured, 1e-3);
  const reference = trimLeadingSilence(testResult.reference, 1e-3);

  // Compute metrics on the (longer) trailing-silence-trimmed buffers for reporting.
  const capturedPeak = computePeak(captured);
  const capturedRMS = computeRMS(captured);
  const refPeak = computePeak(reference);
  const refRMS = computeRMS(reference);

  if (reference.length < 1000 || captured.length < 1000) {
    const msg = `${instName}: Buffer too short for comparison (captured=${captured.length}, ref=${reference.length})`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    continue;
  }

  // The aligned energy envelopes must be near-identical: this survives noise phase jitter yet collapses for reversed, wrong
  // or silent audio.
  const ENV_WIN = 64; // ~1.3ms @ 48kHz — fine enough to resolve transients
  const ENV_MAX_LAG = Math.ceil(3072 / ENV_WIN); // search +/- ~3072 samples (~64ms)
  const envRef = rmsEnvelope(reference, ENV_WIN);
  const envCap = rmsEnvelope(captured, ENV_WIN);
  const { bestLag: envBestLag, bestCorr: envCorr } =
    bestLagCorrelation(envRef, envCap, ENV_MAX_LAG);
  const envLagSamples = envBestLag * ENV_WIN;

  // The envelope is sign-blind, so inversion is caught by SIGNED raw correlation at the three lags around the envelope
  // peak, taking the largest |r|: a free search finds positive side-lobes in noise, a single coarse lag can land a block
  // off. It reads positive for a genuine capture and negative for an inverted one.
  const rawWinEnd = Math.min(4000, reference.length);
  let rawAbsBest = -1;
  let rawCorrAligned = -2;
  let rawLagAligned = envLagSamples;
  for (const lag of [envLagSamples - ENV_WIN, envLagSamples, envLagSamples + ENV_WIN]) {
    const c = correlationAtLag(reference, captured, lag, 0, rawWinEnd);
    if (Math.abs(c) > rawAbsBest) {
      rawAbsBest = Math.abs(c);
      rawCorrAligned = c;
      rawLagAligned = lag;
    }
  }

  // Amplitude corroborators (secondary, never a substitute for the gates above).
  const peakDiff = Math.abs(capturedPeak - refPeak);
  const rmsRatio = Math.min(capturedRMS, refRMS) / Math.max(capturedRMS, refRMS);
  const peakRatio = Math.min(capturedPeak, refPeak) / Math.max(capturedPeak, refPeak);

  results[instName] = {
    capturedSamples: captured.length,
    refSamples: reference.length,
    capturedPeak,
    refPeak,
    peakDiff,
    capturedRMS,
    refRMS,
    envCorr,
    envLagSamples,
    rawCorrAligned,
    rawLagAligned,
    rmsRatio,
    peakRatio,
    sampleRate: testResult.sampleRate,
  };

  console.log(`  Sample rate: ${testResult.sampleRate} Hz`);
  console.log(`  Captured: ${captured.length} samples, peak=${capturedPeak.toFixed(4)}, rms=${capturedRMS.toFixed(4)}`);
  console.log(`  Reference: ${reference.length} samples, peak=${refPeak.toFixed(4)}, rms=${refRMS.toFixed(4)}`);
  console.log(`  Peak diff: ${peakDiff.toFixed(6)}`);
  console.log(`  Envelope best-lag correlation: ${envCorr.toFixed(6)} (lag=${envLagSamples} samples, ${(envLagSamples / testResult.sampleRate * 1000).toFixed(2)} ms)`);
  console.log(`  Raw signed correlation @aligned lag: ${rawCorrAligned.toFixed(6)} (lag=${rawLagAligned})`);

  // Strict but not flaky: noise-like instruments sit just above it, while reversed, wrong or silent audio reads < 0.2.
  const ENV_THRESHOLD = 0.93;
  let instOk = true;

  if (!(envCorr >= ENV_THRESHOLD)) {
    const msg = `${instName}: Waveform content mismatch - envelope corr=${envCorr.toFixed(4)} (lag=${envLagSamples}) < ${ENV_THRESHOLD}`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    instOk = false;
  }

  // A small positive margin rejects pure decorrelation too.
  if (!(rawCorrAligned > 0.02)) {
    const msg = `${instName}: Inverted/decorrelated capture - raw signed corr=${rawCorrAligned.toFixed(4)} at aligned lag must be > 0`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    instOk = false;
  }

  // Only tonal instruments align at sample level; noise cannot under block-phase capture.
  const RAW_TONAL_THRESHOLD = 0.90;
  const tonalInstruments = new Set(['kick']);
  if (tonalInstruments.has(instName) && !(rawCorrAligned >= RAW_TONAL_THRESHOLD)) {
    const msg = `${instName}: Tonal raw waveform mismatch - raw corr=${rawCorrAligned.toFixed(4)} < ${RAW_TONAL_THRESHOLD}`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    instOk = false;
  }

  if (instOk) {
    console.log(`  OK: Waveform matches reference (envCorr=${envCorr.toFixed(4)} >= ${ENV_THRESHOLD}, raw signed=${rawCorrAligned.toFixed(4)} > 0)`);
    if (rmsRatio < 0.85 || peakRatio < 0.85) {
      console.log(`  WARN: amplitude ratios looser than expected (rmsRatio=${rmsRatio.toFixed(3)}, peakRatio=${peakRatio.toFixed(3)})`);
    }
  }

  // Peak should be reasonably close (within 15%) -- reported as a warning only.
  const peakTolerance = refPeak * 0.15;
  if (peakDiff > peakTolerance) {
    console.log(`  WARN: Peak differs by ${(peakDiff / refPeak * 100).toFixed(1)}%`);
  }
}

console.log('\n--- TEST 3: POLYPHONIC CAPTURE ---');

const polyResult = await page.evaluate(async () => {
  window.clearOutputCapture();
  window.startOutputCapture();

  // Ensure all samples are rendered first
  await window.ensureSynthSample('kick');
  await window.ensureSynthSample('rimshot');
  await window.ensureSynthSample('hihat');

  // Play all three simultaneously
  const now = window.audioNow();
  await window.playSound('kick', 1.0, now + 0.05);
  await window.playSound('rimshot', 1.0, now + 0.05);
  await window.playSound('hihat', 1.0, now + 0.05);

  // Wait for all to complete
  await new Promise(r => setTimeout(r, 1500));

  await window.flushOutputCapture?.(); // every block captured so far has arrived
  const captured = Array.from(window.stopOutputCapture());

  // Analyze
  let peak = 0;
  let clippedSamples = 0;
  for (let i = 0; i < captured.length; i++) {
    const a = Math.abs(captured[i]);
    if (a > peak) peak = a;
    if (a >= 1.0) clippedSamples++;
  }

  return {
    samples: captured.length,
    peak,
    clippedSamples,
    clipsDetected: clippedSamples > 0,
  };
});

console.log(`  Captured samples: ${polyResult.samples}`);
console.log(`  Peak: ${polyResult.peak.toFixed(4)}`);
console.log(`  Clipped samples: ${polyResult.clippedSamples}`);

// Verify no clipping (WebAudio main gain is 1.0, but individual voices are attenuated)
if (polyResult.clipsDetected) {
  console.log(`WARN: Clipping detected in polyphonic mix (${polyResult.clippedSamples} samples)`);
  // This is a warning, not failure - WebAudio may have different headroom handling
} else {
  console.log('  OK: No clipping in polyphonic mix');
}

// Verify we captured actual audio
if (polyResult.peak < 0.01) {
  const msg = 'Polyphonic capture: No audio detected';
  console.log(`FAIL: ${msg}`);
  failures.push(msg);
  anyFailed = true;
} else {
  console.log('  OK: Polyphonic audio captured successfully');
}

console.log('\n--- TEST 4: CAPTURE STATS API ---');

const statsTest = await page.evaluate(async () => {
  window.clearOutputCapture();

  const statsBefore = window.getOutputCaptureStats();

  window.startOutputCapture();
  await window.playSound('kick', 1.0);
  await new Promise(r => setTimeout(r, 300));

  const statsDuring = window.getOutputCaptureStats();

  window.stopOutputCapture();
  const statsAfter = window.getOutputCaptureStats();

  return {
    before: statsBefore,
    during: statsDuring,
    after: statsAfter,
  };
});

console.log(`  Before: enabled=${statsTest.before.enabled}, samples=${statsTest.before.samples}`);
console.log(`  During: enabled=${statsTest.during.enabled}, samples=${statsTest.during.samples}`);
console.log(`  After: enabled=${statsTest.after.enabled}, samples=${statsTest.after.samples}`);

if (!statsTest.during.enabled) {
  failures.push('Stats API: enabled not true during capture');
  anyFailed = true;
}
if (statsTest.during.samples === 0) {
  failures.push('Stats API: no samples during capture');
  anyFailed = true;
}
if (statsTest.after.enabled) {
  failures.push('Stats API: still enabled after stop');
  anyFailed = true;
}
if (!anyFailed) {
  console.log('  OK: Stats API working correctly');
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "output_capture_compare");
await browser.close();
server.close();

console.log('\n============================================================');
if (anyFailed) {
  console.log('TEST FAILED');
  console.log('============================================================');
  console.log(`\n${failures.length} failure(s):`);
  for (const f of failures) {
    console.log(`  - ${f}`);
  }
  process.exit(1);
} else {
  console.log('TEST PASSED - Output capture and comparison successful');
  console.log('============================================================');
  console.log('\nResults summary:');
  for (const [inst, r] of Object.entries(results)) {
    console.log(`  ${inst}: envCorr=${r.envCorr.toFixed(4)} (lag=${r.envLagSamples}), raw signed=${r.rawCorrAligned.toFixed(4)} (lag=${r.rawLagAligned}), peak=${r.capturedPeak.toFixed(4)}`);
  }
  process.exit(0);
}
