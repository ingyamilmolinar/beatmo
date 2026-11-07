// Desktop and WASM must render the same audio for the same parameters: the same C, the same post-processing, rendered
// into desktop's frame counts. The desktop reference is rendered fresh by cmd/export_audio on every run.

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import { loadDesktopAudioReference } from "./browser_test_helpers.js";

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

// Amplitude map: read from the export_audio reference to stay in sync with Go InstrumentConfigs.
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
await page.waitForFunction(() => window.__ready === true, null, { timeout: 10000 });

// ONE module instance: the reference renders its whole sequence in one process, and a fresh instance per render would
// decorrelate any advancing noise state.
await page.evaluate(async () => {
  window.__drumsModule = await window.__drumsFactory();
});

const RENDER_FUNCS = {
  // Every family renders through render_modular_p and is compared in the paramCases below; the raw case is the bare
  // voice, every param at its schema identity.
  modular: 'render_modular_p',
};

const SAMPLE_RATE = 48000; // Match desktop and production WASM (AudioContext.sampleRate)

// Exactly as the reference rendered it.
const BARE_BLOCK = (desktopData.instruments.find((i) => i.name === 'modular') || {}).block;
if (!Array.isArray(BARE_BLOCK) || BARE_BLOCK.length === 0) {
  throw new Error('desktop reference carries no block for the modular voice — regenerate it (cmd/export_audio)');
}

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
  // The C envelopes are proportional to buffer length, so the frame count must match.
  const frames = desktopInst.frames;
  const desktopSamples = desktopInst.samples;
  const desktopDuration = desktopInst.duration;

  console.log(`Rendering ${instName}: ${frames} frames (${desktopDuration.toFixed(3)}s)`);

  // Render raw C output in browser via WASM with the SAME frame count as desktop
  const rawWasmData = await page.evaluate(async ({ renderFunc, sr, frames, block }) => {
    const m = window.__drumsModule;
    if (!m) throw new Error('no shared drums module');
    const pp = m._malloc(block.length * 8);
    m.HEAPF64.set(block, pp >> 3);
    const ptr = m._malloc(frames * 4);
    m.ccall(renderFunc, null, ['number', 'number', 'number', 'number'], [ptr, sr, frames, pp]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);
    m._free(pp);
    return Array.from(data);
  }, { renderFunc, sr: SAMPLE_RATE, frames, block: desktopInst.block || BARE_BLOCK });

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

  // Same C code: very high, with float-precision tolerance.
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

  // Only float precision and Go/JS processing can differ.
  const maxSampleDiffThreshold = 0.001; // 0.1% of full scale
  if (maxDiff > maxSampleDiffThreshold) {
    const msg = `${instName}: Max sample diff too large - ${maxDiff.toFixed(6)} at index ${maxIdx}`;
    console.log(`WARN: ${msg}`);
    warnings.push(msg);
  }
}

// Each case's complete flattened block from cmd/export_audio, rebuilt by name in the browser and rendered on the SAME
// module: a divergence means the platforms' C builds or ABI tables differ for the same edit.

console.log('\n--- PARAMETERIZED RENDER COMPARISON ---');

const ABI = await import('./synth_param_abi.gen.js');
const PARAM_ABI = {
  count: ABI.MODULAR_PARAM_COUNT,
  index: ABI.MODULAR_PARAM_INDEX,
  tables: ABI.MODULAR_PARAM_TABLES,
};

for (const pc of desktopData.paramCases || []) {
  const renderFnP = 'render_modular_p';

  const rawWasmData = await page.evaluate(async ({ pc, abi, renderFnP }) => {
    const m = window.__drumsModule;
    if (!m) throw new Error('no shared drums module');
    const COUNT = abi.count;
    const INDEX = abi.index;
    // A missing field is a reference/ABI mismatch, never a default.
    const paramsPtr = m._malloc(COUNT * 8);
    const pheap = m.HEAPF64.subarray(paramsPtr >> 3, (paramsPtr >> 3) + COUNT);
    pheap.fill(0); // _malloc does not zero: table cells outside INDEX were garbage
    for (const [name, idx] of Object.entries(INDEX)) {
      const v = pc.params[name];
      if (typeof v !== 'number' || !Number.isFinite(v)) throw new Error(`param case ${pc.name}: no value for ${name}`);
      pheap[idx] = v;
    }
    // Table columns lie outside MODULAR_PARAM_INDEX: every cell comes from the reference too.
    for (const [col, [first, rows]] of Object.entries(abi.tables)) {
      for (let i = 0; i < rows; i++) {
        const v = pc.params[`${col}[${i}]`];
        if (typeof v !== 'number' || !Number.isFinite(v)) throw new Error(`param case ${pc.name}: no value for ${col}[${i}]`);
        pheap[first + i] = v;
      }
    }
    const ptr = m._malloc(pc.frames * 4);
    m.ccall(renderFnP, null, ['number', 'number', 'number', 'number'], [ptr, pc.sampleRate, pc.frames, paramsPtr]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, pc.frames).slice();
    m._free(ptr);
    m._free(paramsPtr);
    return Array.from(data);
  }, { pc: { ...pc, samples: undefined }, abi: PARAM_ABI, renderFnP });

  const wasmPeak = computePeak(rawWasmData);
  const wasmRMS = computeRMS(rawWasmData);
  const corr = correlation(rawWasmData, pc.samples);
  const { maxDiff, maxIdx } = maxAbsDiff(rawWasmData, pc.samples);
  console.log(
    `  ${pc.name} [${renderFnP}]: corr=${corr.toFixed(6)} ` +
    `peak wasm=${wasmPeak.toFixed(4)} desk=${pc.peak.toFixed(4)} ` +
    `rms wasm=${wasmRMS.toFixed(5)} desk=${pc.rms.toFixed(5)} maxDiff=${maxDiff.toExponential(2)}@${maxIdx}`,
  );

  results['param:' + pc.name] = {
    renderFnP, frames: pc.frames,
    wasm: { peak: wasmPeak, rms: wasmRMS },
    desktop: { peak: pc.peak, rms: pc.rms },
    comparison: { correlation: corr, maxSampleDiff: maxDiff },
  };

  if (pc.peak === 0) {
    // Silence case (e.g. osc disabled) must be silent on BOTH platforms.
    if (wasmPeak !== 0) {
      const msg = `param ${pc.name}: desktop is silent but WASM rendered peak=${wasmPeak}`;
      console.log(`FAIL: ${msg}`);
      failures.push(msg);
      anyFailed = true;
    }
    continue;
  }
  if (wasmPeak === 0) {
    const msg = `param ${pc.name}: WASM rendered SILENCE (desktop peak=${pc.peak.toFixed(4)}) — ` +
      `the parameterized render path lost the voice (stale ABI / wrong dispatch / dead generator).`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
    continue;
  }
  if (corr < 0.999) {
    const msg = `param ${pc.name}: correlation ${corr.toFixed(6)} < 0.999 — platforms render the same edit differently`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
  }
  if (Math.abs(wasmPeak - pc.peak) > 0.01 || Math.abs(wasmRMS - pc.rms) > 0.01) {
    const msg = `param ${pc.name}: level mismatch (peak Δ${Math.abs(wasmPeak - pc.peak).toFixed(4)}, rms Δ${Math.abs(wasmRMS - pc.rms).toFixed(4)})`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
  }
  // The inherent seam is ~1 float32 ULP and the worst known amplification ~1.5e-5; 1e-4 sits well above both and ≥ 10×
  // below real literal or dispatch drift.
  if (maxDiff > 1e-4) {
    const msg = `param ${pc.name}: maxDiff ${maxDiff.toExponential(2)} at ${maxIdx} exceeds 1e-4 — ` +
      `beyond the float32-ULP seam; the platforms genuinely diverge on this edit`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
  }
}

// Several simultaneous voices must sum through the per-voice headroom without hard clipping.

console.log('\n--- POLYPHONIC MIXING TEST ---');

const polyResult = await page.evaluate(async ({ AMPLITUDES, SAMPLE_RATE, BARE_BLOCK }) => {
  const m = window.__drumsModule;
  if (!m) throw new Error('no shared drums module');

  // render_modular is the only C renderer, so three of its voices exercise the normalize → amplitude → sum path.
  const instruments = ['modular-a', 'modular-b', 'modular-c'];
  const renderFuncs = {
    'modular-a': 'render_modular_p',
    'modular-b': 'render_modular_p',
    'modular-c': 'render_modular_p',
  };

  // Use a common frame count (shortest typical duration)
  const frames = Math.floor(SAMPLE_RATE * 0.3); // 300ms

  const samples = {};
  for (const inst of instruments) {
    const pp = m._malloc(BARE_BLOCK.length * 8);
    m.HEAPF64.set(BARE_BLOCK, pp >> 3);
    const ptr = m._malloc(frames * 4);
    m.ccall(renderFuncs[inst], null, ['number', 'number', 'number', 'number'], [ptr, SAMPLE_RATE, frames, pp]);
    m._free(pp);
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
}, { AMPLITUDES, SAMPLE_RATE, BARE_BLOCK });

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

console.log('\n============================================================');
console.log('CROSS-PLATFORM AUDIO COMPARISON RESULTS');
console.log('Desktop samples from: cmd/export_audio (rendered fresh)');
console.log('============================================================\n');

console.log('Test Configuration:');
console.log(`  Sample Rate: ${SAMPLE_RATE} Hz`);
console.log(`  Instruments Tested: ${Object.keys(results).join(', ')}\n`);

for (const [inst, r] of Object.entries(results)) {
  // Printed in their own sections above.
  if (inst === 'polyphonic_mix' || inst.startsWith('param:')) continue;

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

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "xplat_audio_compare");
await browser.close();
server.close();

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
