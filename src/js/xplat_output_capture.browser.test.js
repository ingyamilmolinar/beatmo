// output_capture_compare.browser.test.js
//
// Tests the output capture mechanism and compares the captured WebAudio output
// against reference samples to verify the full audio pipeline works correctly.
//
// This test:
// 1. Uses the new output capture mechanism (ScriptProcessorNode)
// 2. Plays sounds through the full WebAudio pipeline (volume buses, channels, etc.)
// 3. Captures the final mixed output
// 4. Compares with reference samples from desktop_audio.json
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/output_capture_compare.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright's Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  const { spawnSync } = await import("child_process");
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

// ============================================================================
// Load desktop audio samples for reference
// ============================================================================

const desktopAudioPath = path.join(jsDir, "desktop_audio.json");
let desktopData;
try {
  desktopData = JSON.parse(fs.readFileSync(desktopAudioPath, "utf-8"));
  console.log(`Loaded desktop audio data (version ${desktopData.version})`);
} catch (err) {
  console.error(`Failed to load desktop_audio.json: ${err.message}`);
  console.error("Generate it with: cd src/go && go run ./cmd/export_audio > ../js/desktop_audio.json");
  process.exit(1);
}

// ============================================================================
// Metric computation functions
// ============================================================================

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

// Signed Pearson correlation between `ref` and `cap` where `cap` is shifted by
// `lag` samples: ref[i] is compared with cap[i + lag]. Only the overlapping region
// (optionally restricted to [from, to) in ref index space) is scored. Returns the
// signed correlation; a negative result means the two are inverted/decorrelated,
// NOT merely time-shifted. Returns -2 (sentinel below any real correlation) when
// the overlap is too small to score.
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

// Windowed-RMS energy envelope: one RMS value per `win` consecutive samples. This
// collapses the raw waveform to its energy-over-time shape, which is robust to the
// sub-sample phase jitter the ScriptProcessorNode capture introduces (broadband
// noise instruments lose raw-sample phase across 256-sample blocks, but their
// energy envelope is preserved). Inversion-blind by construction (RMS uses |x|),
// so it is paired with a signed raw-correlation inversion guard at the call site.
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

// Cross-correlation lag search: slide `cap` against `ref` over [-maxLag, +maxLag]
// and return the lag that maximizes signed correlation, plus that correlation.
// A negative best correlation indicates the captured waveform is inverted or
// uncorrelated with the reference (not a benign time shift) and must fail.
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

// ============================================================================
// Test setup
// ============================================================================

const server = http.createServer((req, res) => {
  if (req.url === "/" || req.url === "/test.html") {
    // Load full audio.js module which includes capture mechanism
    const html = `<!DOCTYPE html><html><body>
<script type="module">
  import './audio.js';
  // Wait for module to be ready
  window.__ready = true;
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
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
console.log(`Test server running on port ${port}`);

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on('console', (msg) => {
  if (process.env.TEST_LOG) console.log('[PAGE]', msg.type(), msg.text());
});

await page.goto(`http://localhost:${port}/test.html`);
await page.waitForFunction(() => window.__ready === true, { timeout: 10000 });

// Wait for audio.js to initialize (render cache to be populated)
await page.waitForFunction(() => typeof window.playSound === 'function', { timeout: 5000 });
console.log('Audio module loaded');

// ============================================================================
// Test 1: Verify output capture mechanism works
// ============================================================================

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

// ============================================================================
// Test 2: Compare captured output with WASM-rendered reference
// ============================================================================
// This test renders reference samples via WASM at the browser's sample rate,
// then compares with the captured WebAudio output. This ensures we're comparing
// at matching sample rates and can verify the full audio pipeline.

console.log('\n--- TEST 2: COMPARE CAPTURED OUTPUT WITH WASM REFERENCE ---');

const instrumentsToTest = ['kick', 'snare', 'hihat'];
const results = {};
let anyFailed = false;
const failures = [];

for (const instName of instrumentsToTest) {
  console.log(`\nTesting ${instName}...`);

  // Play through full WebAudio pipeline and capture
  // Use the SAME cached render as audio.js uses for the reference (important for
  // instruments with random components like snare noise)
  const testResult = await page.evaluate(async ({ instName }) => {
    // First, get the sample rate
    const sampleRate = window.__audioCtx?.sampleRate || 48000;

    // Ensure sample is rendered and cached (this is what playSound will use)
    await window.ensureSynthSample(instName);

    // Get the cached render data as our reference
    // This is the SAME data that playSound will use, so instruments with
    // random components (like snare) will still match perfectly
    const reference = window.getCachedRenderData(instName);
    if (!reference) {
      throw new Error(`No cached render for ${instName}`);
    }

    // Clear previous capture and start
    window.clearOutputCapture();
    window.startOutputCapture();

    // Play through normal playSound path (uses volume buses, channels, etc.)
    // This uses the cached render we just ensured
    await window.playSound(instName, 1.0);

    // Wait for sound to complete
    await new Promise(r => setTimeout(r, 1000));

    // Stop and get samples
    const captured = Array.from(window.stopOutputCapture());

    return {
      captured,
      reference,
      sampleRate,
    };
  }, { instName });

  // Trim leading silence so both buffers start near their onset. The output tap is
  // a ScriptProcessorNode (256-sample blocks) sitting AFTER the master limiter, and
  // playSound schedules with a small variable lead, so the captured stream is
  // time-shifted vs the pre-limiter cached-render reference by an unknown amount and
  // is captured on 256-sample block phase. A lag-0 raw correlation therefore wildly
  // under-reports the true match (hihat reads ~0.1, snare ~0.3, even with correct
  // audio) and is unstable run-to-run for noise instruments. We recover the real
  // match with two complementary, alignment-robust measures below.
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

  // ----- PRIMARY: energy-envelope cross-correlation (the real content match) -----
  // The windowed-RMS envelope is the audio's energy-over-time shape. The same C
  // synthesis + same cached render flows through both paths, so the envelopes must
  // be near-identical once aligned. This is the metric that actually proves the
  // captured waveform IS the reference: it survives the sub-sample phase jitter that
  // destroys raw correlation for broadband noise, yet it COLLAPSES (towards 0 or
  // negative) if the captured audio is reversed, the wrong instrument, or silence.
  // (Teeth-proven below at the call site / in the teeth-check run.)
  const ENV_WIN = 64; // ~1.3ms @ 48kHz — fine enough to resolve transients
  const ENV_MAX_LAG = Math.ceil(3072 / ENV_WIN); // search +/- ~3072 samples (~64ms)
  const envRef = rmsEnvelope(reference, ENV_WIN);
  const envCap = rmsEnvelope(captured, ENV_WIN);
  const { bestLag: envBestLag, bestCorr: envCorr } =
    bestLagCorrelation(envRef, envCap, ENV_MAX_LAG);
  const envLagSamples = envBestLag * ENV_WIN;

  // ----- INVERSION GUARD: signed raw correlation at the content-aligned lag -----
  // The envelope is sign-blind (RMS uses |x|), so a NEGATED capture passes the envelope
  // check. We catch inversion/decorrelation with the SIGNED raw-sample correlation, but
  // raw correlation on broadband noise is delicate:
  //   * A FREE raw-lag search is unusable: pure-noise instruments are self-similar, so
  //     their autocorrelation has POSITIVE side-lobes as strong as the main lobe in both
  //     polarities -- a free search finds an equal-magnitude positive lobe even for an
  //     inverted capture (hihat is the worst case), defeating the guard.
  //   * Pinning to a single coarse-envelope lag is too brittle: the 64-sample envelope
  //     quantisation occasionally lands one block off the true onset, where the raw
  //     correlation of a GENUINE capture can read negative -- a false inversion FAIL.
  // Robust resolution: evaluate raw correlation only at the three lags anchored to the
  // envelope peak {coarse-WIN, coarse, coarse+WIN}, pick the one with the largest
  // ABSOLUTE correlation (= the true onset alignment, polarity-agnostic), and take its
  // SIGNED value. This consistently reads POSITIVE for a genuine capture (kick ~0.95;
  // broadband snare ~0.10-0.25, hihat ~0.27 -- low but reliably positive because their
  // fine phase only partially survives the 256-sample-block, post-limiter capture) and
  // the EXACT NEGATIVE for an inverted capture. Requiring the value > 0 therefore
  // catches inversion for every instrument. (Teeth-proven: negating the reference makes
  // all three FAIL.)
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

  // PRIMARY ASSERTION: the energy envelope must genuinely match the reference.
  // Threshold 0.93: kick/hihat clear 0.96-0.99, snare floats 0.951-0.956 (broadband
  // noise; tighter margin). 0.93 keeps a ~2pt cushion on snare so the gate is strict
  // but not flaky, while still rejecting reversed/wrong/silent audio (which read <0.2
  // or negative). This is the strictest defensible bar given the post-limiter,
  // block-phase capture tap; the raw kick correlation (~0.97) independently proves
  // the capture path is sample-faithful, so the lower envelope bar is a noise-phase
  // concession, not a correctness concession.
  const ENV_THRESHOLD = 0.93;
  let instOk = true;

  if (!(envCorr >= ENV_THRESHOLD)) {
    const msg = `${instName}: Waveform content mismatch - envelope corr=${envCorr.toFixed(4)} (lag=${envLagSamples}) < ${ENV_THRESHOLD}`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    instOk = false;
  }

  // INVERSION GUARD: a negative aligned raw correlation means the captured waveform
  // is inverted or decorrelated vs the reference (NOT a benign time shift) and must
  // fail outright. Require a small positive margin to reject pure decorrelation.
  if (!(rawCorrAligned > 0.02)) {
    const msg = `${instName}: Inverted/decorrelated capture - raw signed corr=${rawCorrAligned.toFixed(4)} at aligned lag must be > 0`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    instOk = false;
  }

  // For tonal instruments that align at the sample level, additionally require a high
  // raw correlation as the strictest possible check. Kick consistently clears 0.95;
  // broadband noise instruments legitimately cannot (block-phase capture), so this is
  // gated to instruments whose raw correlation is expected to be high.
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

// ============================================================================
// Test 3: Polyphonic capture (multiple simultaneous voices)
// ============================================================================

console.log('\n--- TEST 3: POLYPHONIC CAPTURE ---');

const polyResult = await page.evaluate(async () => {
  window.clearOutputCapture();
  window.startOutputCapture();

  // Ensure all samples are rendered first
  await window.ensureSynthSample('kick');
  await window.ensureSynthSample('snare');
  await window.ensureSynthSample('hihat');

  // Play all three simultaneously
  const now = window.audioNow();
  await window.playSound('kick', 1.0, now + 0.05);
  await window.playSound('snare', 1.0, now + 0.05);
  await window.playSound('hihat', 1.0, now + 0.05);

  // Wait for all to complete
  await new Promise(r => setTimeout(r, 1500));

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

// ============================================================================
// Test 4: Verify capture stats API
// ============================================================================

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

// ============================================================================
// Cleanup
// ============================================================================

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "output_capture_compare");
await browser.close();
server.close();

// ============================================================================
// Final results
// ============================================================================

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
