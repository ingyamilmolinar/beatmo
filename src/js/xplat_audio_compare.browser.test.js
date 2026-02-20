// xplat_audio_compare.browser.test.js
//
// Cross-platform audio buffer comparison test to verify that desktop Go and WASM
// produce identical audio output when given the same parameters.
//
// IMPORTANT: This test uses desktop's actual buffer durations (from desktop_audio.json)
// to render WASM samples, ensuring both platforms render into the same size buffer.
// Both platforms use the same C code (drums.c) for synthesis with identical
// post-processing (peak normalize + amplitude scale).
//
// To regenerate desktop samples: cd src/go && go run ./cmd/export_audio > ../js/desktop_audio.json

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
// Load desktop audio samples
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
// Metric computation functions (also used in-browser)
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

function computeDCOffset(data) {
  if (!data || data.length === 0) return 0;
  let sum = 0;
  for (let i = 0; i < data.length; i++) sum += data[i];
  return sum / data.length;
}

function correlation(x, y) {
  const n = Math.min(x.length, y.length);
  if (n === 0) return 0;
  let sx = 0, sy = 0, sxx = 0, syy = 0, sxy = 0;
  for (let i = 0; i < n; i++) {
    sx += x[i]; sy += y[i];
    sxx += x[i] * x[i]; syy += y[i] * y[i];
    sxy += x[i] * y[i];
  }
  const num = n * sxy - sx * sy;
  const den = Math.sqrt((n * sxx - sx * sx) * (n * syy - sy * sy)) || 1;
  return num / den;
}

function maxAbsDiff(x, y) {
  const n = Math.min(x.length, y.length);
  let maxDiff = 0;
  let maxIdx = -1;
  for (let i = 0; i < n; i++) {
    const diff = Math.abs(x[i] - y[i]);
    if (diff > maxDiff) {
      maxDiff = diff;
      maxIdx = i;
    }
  }
  return { maxDiff, maxIdx };
}

// ============================================================================
// Audio processing (for WASM rendering to match desktop pipeline)
// ============================================================================

// Amplitude map: read from desktop_audio.json to stay in sync with Go InstrumentConfigs.
const AMPLITUDES = {};
for (const inst of desktopData.instruments) {
  AMPLITUDES[inst.name] = inst.amplitude;
}

// Apply audio processing (peak normalize, scale by amp) - matches desktop normalizeAndScale
function applyAudioProcessing(data, amp) {
  const result = new Float32Array(data.length);
  // 1. Copy data
  for (let i = 0; i < data.length; i++) result[i] = data[i];
  // 2. Peak normalize
  const peak = computePeak(result);
  if (peak > 0) {
    const inv = 1 / peak;
    for (let i = 0; i < result.length; i++) result[i] *= inv;
  }
  // 3. Scale by amplitude
  for (let i = 0; i < result.length; i++) result[i] *= amp;
  return result;
}

// ============================================================================
// Test setup
// ============================================================================

const server = http.createServer((req, res) => {
  if (req.url === "/" || req.url === "/test.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module">
  import drumsFactory from './drums.single.js';
  window.__drumsFactory = drumsFactory;
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on('console', (msg) => {
  if (process.env.TEST_LOG) console.log('[PAGE]', msg.type(), msg.text());
});

await page.goto(`http://localhost:${port}/test.html`);
await page.waitForFunction(() => window.__ready === true, { timeout: 10000 });

// ============================================================================
// Render WASM audio and compare with desktop
// ============================================================================

const RENDER_FUNCS = {
  snare: 'render_snare',
  kick: 'render_kick',
  hihat: 'render_hihat',
  tom: 'render_tom',
  clap: 'render_clap',
  cowbell: 'render_cowbell',
};

const SAMPLE_RATE = 48000; // Match desktop and production WASM (AudioContext.sampleRate)

const results = {};
let anyFailed = false;
const failures = [];
const warnings = [];

for (const desktopInst of desktopData.instruments) {
  const instName = desktopInst.name;
  const renderFunc = RENDER_FUNCS[instName];

  if (!renderFunc) {
    console.log(`Skipping ${instName} - no render function defined`);
    continue;
  }

  const amp = AMPLITUDES[instName] || 0.6;
  // Use the SAME frame count as desktop to ensure identical buffer sizes
  // This is critical because the C synth envelopes are proportional to buffer length
  const frames = desktopInst.frames;
  const desktopSamples = desktopInst.samples;
  const desktopDuration = desktopInst.duration;

  console.log(`Rendering ${instName}: ${frames} frames (${desktopDuration.toFixed(3)}s)`);

  // Render raw C output in browser via WASM with the SAME frame count as desktop
  const rawWasmData = await page.evaluate(async ({ renderFunc, sr, frames }) => {
    const factory = window.__drumsFactory;
    if (!factory) throw new Error('no drumsFactory');
    const m = await factory();
    const ptr = m._malloc(frames * 4);
    m.ccall(renderFunc, null, ['number', 'number', 'number'], [ptr, sr, frames]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);
    return Array.from(data);
  }, { renderFunc, sr: SAMPLE_RATE, frames });

  // Apply same processing as desktop (peak normalize + amplitude scale)
  const wasmBuf = applyAudioProcessing(new Float32Array(rawWasmData), amp);

  // Compute metrics for both
  const wasmPeak = computePeak(wasmBuf);
  const wasmRMS = computeRMS(wasmBuf);
  const wasmDC = computeDCOffset(wasmBuf);

  const desktopPeak = desktopInst.peak;
  const desktopRMS = desktopInst.rms;
  const desktopDC = desktopInst.dcOffset;

  // Compare sample-by-sample
  const corr = correlation(Array.from(wasmBuf), desktopSamples);
  const { maxDiff, maxIdx } = maxAbsDiff(Array.from(wasmBuf), desktopSamples);

  // Find first divergence point (where samples differ by more than tolerance)
  const TOLERANCE = 1e-6;
  let firstDivergence = -1;
  let numDivergent = 0;
  for (let i = 0; i < Math.min(wasmBuf.length, desktopSamples.length); i++) {
    if (Math.abs(wasmBuf[i] - desktopSamples[i]) > TOLERANCE) {
      if (firstDivergence < 0) firstDivergence = i;
      numDivergent++;
    }
  }

  results[instName] = {
    sampleRate: SAMPLE_RATE,
    frames,
    amplitude: amp,
    wasm: { peak: wasmPeak, rms: wasmRMS, dcOffset: wasmDC },
    desktop: { peak: desktopPeak, rms: desktopRMS, dcOffset: desktopDC },
    comparison: {
      correlation: corr,
      peakDiff: Math.abs(wasmPeak - desktopPeak),
      rmsDiff: Math.abs(wasmRMS - desktopRMS),
      maxSampleDiff: maxDiff,
      maxSampleDiffIdx: maxIdx,
      firstDivergence,
      numDivergentSamples: numDivergent,
      divergenceRate: numDivergent / frames,
    },
  };

  // ============================================================================
  // ASSERTIONS
  // ============================================================================

  // 1. Correlation should be very high (both use same C code)
  // Allow some tolerance for float precision differences
  if (corr < 0.99) {
    const msg = `${instName}: Correlation too low - ${corr.toFixed(6)} (expected >= 0.99)`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
  } else if (corr < 0.999) {
    warnings.push(`${instName}: Correlation slightly low - ${corr.toFixed(6)}`);
  }

  // 2. Peak values should be very close
  const peakDiffThreshold = 0.01; // 1% tolerance
  if (Math.abs(wasmPeak - desktopPeak) > peakDiffThreshold) {
    const msg = `${instName}: Peak differs significantly - wasm=${wasmPeak.toFixed(6)}, desktop=${desktopPeak.toFixed(6)}`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
  }

  // 3. RMS values should be very close
  const rmsDiffThreshold = 0.01; // 1% tolerance
  if (Math.abs(wasmRMS - desktopRMS) > rmsDiffThreshold) {
    const msg = `${instName}: RMS differs significantly - wasm=${wasmRMS.toFixed(6)}, desktop=${desktopRMS.toFixed(6)}`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
  }

  // 4. Max sample difference should be small (float precision)
  // Both use the same C render functions, so differences should only be from
  // float32 vs float64 precision and any Go/JS processing differences
  const maxSampleDiffThreshold = 0.001; // 0.1% of full scale
  if (maxDiff > maxSampleDiffThreshold) {
    const msg = `${instName}: Max sample diff too large - ${maxDiff.toFixed(6)} at index ${maxIdx}`;
    console.log(`WARN: ${msg}`);
    warnings.push(msg);
  }
}

// ============================================================================
// Polyphonic Mixing Test
// ============================================================================
// This test verifies that multiple simultaneous voices mix correctly without
// hard clipping artifacts. The desktop mixer applies -12dB headroom (0.25)
// to prevent distortion when summing multiple voices.

console.log('\n--- POLYPHONIC MIXING TEST ---');

const polyResult = await page.evaluate(async ({ AMPLITUDES, SAMPLE_RATE }) => {
  const factory = window.__drumsFactory;
  if (!factory) throw new Error('no drumsFactory');
  const m = await factory();

  // Render individual instruments
  const instruments = ['kick', 'snare', 'hihat'];
  const renderFuncs = {
    kick: 'render_kick',
    snare: 'render_snare',
    hihat: 'render_hihat',
  };

  // Use a common frame count (shortest typical duration)
  const frames = Math.floor(SAMPLE_RATE * 0.3); // 300ms

  const samples = {};
  for (const inst of instruments) {
    const ptr = m._malloc(frames * 4);
    m.ccall(renderFuncs[inst], null, ['number', 'number', 'number'], [ptr, SAMPLE_RATE, frames]);
    const raw = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);

    // Peak normalize then scale by amplitude (matching desktop pipeline)
    let peak = 0;
    for (let i = 0; i < raw.length; i++) {
      const a = Math.abs(raw[i]);
      if (a > peak) peak = a;
    }
    if (peak > 0) {
      const inv = 1 / peak;
      for (let i = 0; i < raw.length; i++) raw[i] *= inv;
    }
    const amp = AMPLITUDES[inst] || 0.6;
    for (let i = 0; i < raw.length; i++) raw[i] *= amp;

    samples[inst] = raw;
  }

  // Mix all voices together (simulating polyphonic playback)
  const mixed = new Float32Array(frames);
  for (const inst of instruments) {
    for (let i = 0; i < frames; i++) {
      mixed[i] += samples[inst][i];
    }
  }

  // Apply desktop-style headroom (-12dB = 0.25)
  const mixHeadroom = 0.25;
  const mixedWithHeadroom = new Float32Array(frames);
  for (let i = 0; i < frames; i++) {
    mixedWithHeadroom[i] = mixed[i] * mixHeadroom;
  }

  // Check for clipping (samples at exactly ±1.0 before headroom)
  let clippedSamplesNoHeadroom = 0;
  let clippedSamplesWithHeadroom = 0;
  let peakNoHeadroom = 0;
  let peakWithHeadroom = 0;

  for (let i = 0; i < frames; i++) {
    const absNoHR = Math.abs(mixed[i]);
    const absWithHR = Math.abs(mixedWithHeadroom[i]);

    if (absNoHR > peakNoHeadroom) peakNoHeadroom = absNoHR;
    if (absWithHR > peakWithHeadroom) peakWithHeadroom = absWithHR;

    // Would clip without headroom
    if (absNoHR >= 1.0) clippedSamplesNoHeadroom++;
    // Would clip with headroom
    if (absWithHR >= 1.0) clippedSamplesWithHeadroom++;
  }

  return {
    frames,
    instruments,
    peakNoHeadroom,
    peakWithHeadroom,
    clippedSamplesNoHeadroom,
    clippedSamplesWithHeadroom,
    wouldClipWithoutHeadroom: clippedSamplesNoHeadroom > 0,
    clipsWithHeadroom: clippedSamplesWithHeadroom > 0,
  };
}, { AMPLITUDES, SAMPLE_RATE });

console.log(`  Instruments mixed: ${polyResult.instruments.join(' + ')}`);
console.log(`  Frames: ${polyResult.frames}`);
console.log(`  Peak WITHOUT headroom: ${polyResult.peakNoHeadroom.toFixed(4)} ${polyResult.wouldClipWithoutHeadroom ? '(WOULD CLIP!)' : ''}`);
console.log(`  Peak WITH headroom: ${polyResult.peakWithHeadroom.toFixed(4)} ${polyResult.clipsWithHeadroom ? '(CLIPS!)' : '(OK)'}`);
console.log(`  Clipped samples without headroom: ${polyResult.clippedSamplesNoHeadroom}`);
console.log(`  Clipped samples with headroom: ${polyResult.clippedSamplesWithHeadroom}`);

// Verify headroom prevents clipping
if (polyResult.clipsWithHeadroom) {
  const msg = `Polyphonic mixing: Still clips with headroom! Peak=${polyResult.peakWithHeadroom.toFixed(4)}`;
  console.log(`FAIL: ${msg}`);
  failures.push(msg);
  anyFailed = true;
} else if (polyResult.wouldClipWithoutHeadroom) {
  console.log(`  OK: Headroom correctly prevents clipping (would have ${polyResult.clippedSamplesNoHeadroom} clipped samples without it)`);
} else {
  console.log(`  OK: No clipping detected (voices don't sum high enough to clip)`);
}

results['polyphonic_mix'] = {
  instruments: polyResult.instruments,
  frames: polyResult.frames,
  peakNoHeadroom: polyResult.peakNoHeadroom,
  peakWithHeadroom: polyResult.peakWithHeadroom,
  clippedSamplesNoHeadroom: polyResult.clippedSamplesNoHeadroom,
  clippedSamplesWithHeadroom: polyResult.clippedSamplesWithHeadroom,
  headroomEffective: polyResult.wouldClipWithoutHeadroom && !polyResult.clipsWithHeadroom,
};

// ============================================================================
// Output diagnostic information
// ============================================================================

console.log('\n============================================================');
console.log('CROSS-PLATFORM AUDIO COMPARISON RESULTS');
console.log('Desktop samples from: desktop_audio.json');
console.log('============================================================\n');

console.log('Test Configuration:');
console.log(`  Sample Rate: ${SAMPLE_RATE} Hz`);
console.log(`  Instruments Tested: ${Object.keys(results).join(', ')}\n`);

for (const [inst, r] of Object.entries(results)) {
  // Skip polyphonic_mix - it has a different structure and was already printed
  if (inst === 'polyphonic_mix') continue;

  console.log(`--- ${inst.toUpperCase()} ---`);
  console.log(`  Frames: ${r.frames} (${(r.frames / SAMPLE_RATE).toFixed(3)}s)`);
  console.log(`  Amplitude: ${r.amplitude}`);
  console.log(`  WASM:    peak=${r.wasm.peak.toFixed(6)}, rms=${r.wasm.rms.toFixed(6)}, dc=${r.wasm.dcOffset.toFixed(8)}`);
  console.log(`  Desktop: peak=${r.desktop.peak.toFixed(6)}, rms=${r.desktop.rms.toFixed(6)}, dc=${r.desktop.dcOffset.toFixed(8)}`);
  console.log(`  Correlation: ${r.comparison.correlation.toFixed(8)}`);
  console.log(`  Peak diff: ${r.comparison.peakDiff.toFixed(8)}`);
  console.log(`  RMS diff: ${r.comparison.rmsDiff.toFixed(8)}`);
  console.log(`  Max sample diff: ${r.comparison.maxSampleDiff.toFixed(8)} at idx ${r.comparison.maxSampleDiffIdx}`);
  console.log(`  First divergence: ${r.comparison.firstDivergence >= 0 ? `idx ${r.comparison.firstDivergence}` : 'none'}`);
  console.log(`  Divergent samples: ${r.comparison.numDivergentSamples} (${(r.comparison.divergenceRate * 100).toFixed(2)}%)`);
  console.log('');
}

if (warnings.length > 0) {
  console.log('============================================================');
  console.log('WARNINGS:');
  console.log('============================================================');
  for (const w of warnings) {
    console.log(`  - ${w}`);
  }
  console.log('');
}

// ============================================================================
// Cleanup
// ============================================================================

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "xplat_audio_compare");
await browser.close();
server.close();

// ============================================================================
// Final test result
// ============================================================================

if (anyFailed) {
  console.log('============================================================');
  console.log('TEST FAILED');
  console.log('============================================================');
  console.log(`\n${failures.length} failure(s):`);
  for (const f of failures) {
    console.log(`  - ${f}`);
  }
  console.log('\nBoth platforms should produce similar audio output.');
  console.log('If this fails, investigate differences between:');
  console.log('  - Go: src/go/internal/audio/ (variants.go, drums_c.go)');
  console.log('  - JS: src/js/audio.js ensureRenderedSample');
  console.log('  - C: src/c/drums.c render functions');
  process.exit(1);
} else {
  console.log('============================================================');
  console.log('TEST PASSED - Cross-platform audio comparison successful');
  console.log('============================================================');
  if (warnings.length > 0) {
    console.log(`(${warnings.length} warning(s) - see above)`);
  }
  process.exit(0);
}
