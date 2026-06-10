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

// Instantiate the WASM module ONCE and share it across every render below.
// The bespoke drum renderers carry advancing ma_noise state; the desktop
// reference (export_audio) renders its whole sequence in one process, so the
// browser must replay the same sequence on one module instance to keep the
// noise components aligned (fresh-instance-per-render decorrelates the tails).
await page.evaluate(async () => {
  window.__drumsModule = await window.__drumsFactory();
});

// ============================================================================
// Render WASM audio and compare with desktop
// ============================================================================

const RENDER_FUNCS = {
  // snare / clap migrated to the modular engine (Phase-5): render_snare /
  // render_clap deleted, and snare/clap are no longer in the export_audio
  // instruments list (their parity is covered by the snare-* paramCases via
  // render_modular_p).
  // kick migrated to the modular engine (Phase-3): render_kick deleted, and kick
  // is no longer in the export_audio instruments list (its parity is covered by
  // the kick-* paramCases via render_modular_p).
  // tom migrated to the modular engine (Phase-4): render_tom deleted (tom-*
  // paramCases via render_modular_p).
  // cymbal family migrated to the modular engine (Phase-6): render_hihat /
  // render_cowbell etc. deleted (parity via the hihat-* / cowbell-* paramCases).
  // FM family migrated to the modular engine (Phase-7, the LAST): render_fm_bass /
  // render_fm_bell etc. deleted, and fm-* are no longer in the export_audio
  // raw-render list (their parity is covered by the fm-* paramCases via
  // render_modular_p). EVERY legacy family has now migrated — the ONLY remaining
  // raw-render instrument is the unified modular voice.
  // Unified modular voice (base preset): native render_modular vs WASM
  // render_modular, both at C built-in defaults, must agree within epsilon.
  modular: 'render_modular',
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
    const m = window.__drumsModule;
    if (!m) throw new Error('no shared drums module');
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
// Parameterized render comparison (knobs, stage enables, re-voicing)
// ============================================================================
// Desktop reference: renderParamCases() in cmd/export_audio — the production
// recipe path (MergeRecipeDefaults → SynthRecipe.Render, variant 0, RAW).
// Browser side: the production audio.js semantics — the case's params written
// into the schema-keyed param block with ABI-identity fallback (PARAM_INDEX /
// PARAM_IDENTITY from synth_param_abi.gen.js), then render_X_p /
// render_modular_p on the SAME shared module.
// A divergence here means the two platforms construct the param block (or
// dispatch the renderer) differently for the same user edit — exactly the
// "knob/stage/save changes don't take effect on WASM" bug class.

console.log('\n--- PARAMETERIZED RENDER COMPARISON ---');

const ABI = await import('./synth_param_abi.gen.js');
const PARAM_ABI = {
  synthCount: ABI.SYNTH_PARAM_COUNT,
  synthIndex: ABI.SYNTH_PARAM_INDEX,
  synthIdentity: ABI.SYNTH_PARAM_IDENTITY,
  modularCount: ABI.MODULAR_PARAM_COUNT,
  modularIndex: ABI.MODULAR_PARAM_INDEX,
  modularIdentity: ABI.MODULAR_PARAM_IDENTITY,
  // Family blocks: NONE remain after Phase-7 — EVERY legacy family migrated to the
  // modular block. fm family migrated (Phase-7, the LAST): no FM_PARAM_* ABI.
  // kick family migrated to the modular block (Phase-3): no KICK_PARAM_* ABI.
  // tom family migrated to the modular block (Phase-4): no TOM_PARAM_* ABI.
  // snare family migrated to the modular block (Phase-5): no SNARE_PARAM_* ABI.
  // cymbal family migrated to the modular block (Phase-6): no CYMBAL_PARAM_* ABI.
  // bass family migrated to the modular block (Phase-2): no BASS_PARAM_* ABI.
};

// Recipe → family param block (mirrors audio.js FAMILY_PARAM_BLOCKS by
// RENDER_INFO paramBlock; here keyed by recipe prefix).
function familyForRecipe(recipe) {
  if (recipe.startsWith('fm-')) return 'fm';
  // drum-kick* (Phase-3), drum-tom* (Phase-4), drum-snare* / drum-clap (Phase-5),
  // drum-hihat* / drum-cowbell / drum-shaker / drum-ride / drum-crash (Phase-6),
  // and bass (Phase-2) all migrated to the modular engine: export_audio sets
  // paramBlock:'modular' for those cases, so familyForRecipe is never consulted
  // for them. Only the FM family is still bespoke.
  return null;
}

// Bespoke recipe → base C render function (the `_p` suffix is appended).
// EMPTY after Phase-7: EVERY family migrated to render_modular_p — drum-snare* /
// drum-clap (Phase-5), drum-tom* (Phase-4), drum-kick* (Phase-3), drum-bass*
// (Phase-2), the cymbals (Phase-6), and fm-* (Phase-7, the LAST) — so every
// paramCase carries paramBlock:'modular' and is dispatched via render_modular_p,
// never consulting this map. Kept (empty) so the familyForRecipe fallback below
// stays valid; no recipe resolves to a bespoke render_X_p any more.
const RECIPE_RENDER_BASE = {};

// IMPORTANT (desktop parity): the Go dispatch elides params that exactly
// equal the recipe's ParamDef defaults (elideRecipeDefaults) so the C engine
// keeps its exact double literals. The desktop reference was rendered through
// that path; the browser only writes the case's explicit params (everything
// else NaN identity), which is the same elision by construction — paramCases
// carry only off-default values.

for (const pc of desktopData.paramCases || []) {
  // paramBlock:'modular' (set by export_audio for migrated families like bass)
  // forces the modular path: the desktop reference rendered through the modular
  // binding, and pc.params is already the modular-named block. Otherwise infer
  // from the recipe prefix as before.
  const isModular = pc.paramBlock === 'modular' || pc.recipe.startsWith('synth-modular');
  const family = pc.paramBlock ? null : familyForRecipe(pc.recipe);
  const useModular = isModular;
  const useFamily = family && !useModular ? family : null;
  const renderFnP = useModular
    ? 'render_modular_p'
    : RECIPE_RENDER_BASE[pc.recipe] + '_p';

  const rawWasmData = await page.evaluate(async ({ pc, abi, useModular, useFamily, renderFnP }) => {
    const m = window.__drumsModule;
    if (!m) throw new Error('no shared drums module');
    // EVERY legacy family (kick/tom/snare/cymbal/bass/FM) migrated to the modular
    // block — no family FAM branch remains (the FM block was the last, removed in
    // Phase-7). useFamily is always null now; FAM stays null.
    const FAM = null;
    void useFamily;
    const COUNT = useModular ? abi.modularCount : (FAM ? FAM.count : abi.synthCount);
    const INDEX = useModular ? abi.modularIndex : (FAM ? FAM.index : abi.synthIndex);
    const IDENTITY = useModular ? abi.modularIdentity : (FAM ? FAM.identity : abi.synthIdentity);
    // Mirror audio.js ensureRenderedSample: zeroed block with a safety floor,
    // schema-keyed writes, identity fallback for unset knobs.
    const allocCount = useModular ? Math.max(COUNT, 64) : COUNT;
    const paramsPtr = m._malloc(allocCount * 4);
    const pheap = m.HEAPF32.subarray(paramsPtr >> 2, (paramsPtr >> 2) + allocCount);
    pheap.fill(0);
    for (const [name, idx] of Object.entries(INDEX)) {
      const v = pc.params[name];
      pheap[idx] = (typeof v === 'number' && Number.isFinite(v)) ? v : IDENTITY[name];
    }
    const ptr = m._malloc(pc.frames * 4);
    m.ccall(renderFnP, null, ['number', 'number', 'number', 'number'], [ptr, pc.sampleRate, pc.frames, paramsPtr]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, pc.frames).slice();
    m._free(ptr);
    m._free(paramsPtr);
    return Array.from(data);
  }, { pc: { ...pc, samples: undefined }, abi: PARAM_ABI, useModular, useFamily, renderFnP });

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
  // Hard per-sample bound (promoted from a warning by the 2026-06-06 gap
  // audit). Calibration: the inherent native↔WASM seam is ~1 float32 ULP at
  // the push ABI (NaN-sentinel double literal vs spelled float32 literal —
  // see family_push_binding_parity_test.go); observed ceiling across the full
  // param-case table on this tree was 2.09e-7, and the worst KNOWN
  // ULP-amplification (growing post-decay tail, tom-low decay=max) reaches
  // ~1.5e-5 absolute. 1e-4 sits ~6× above that amplified floor and ≥10×
  // below any real literal/dispatch drift (~1e-3+), so it cannot flake on
  // float noise but catches genuine divergence.
  if (maxDiff > 1e-4) {
    const msg = `param ${pc.name}: maxDiff ${maxDiff.toExponential(2)} at ${maxIdx} exceeds 1e-4 — ` +
      `beyond the float32-ULP seam; the platforms genuinely diverge on this edit`;
    console.log(`FAIL: ${msg}`);
    failures.push(msg);
    anyFailed = true;
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
  const m = window.__drumsModule;
  if (!m) throw new Error('no shared drums module');

  // Render individual instruments. EVERY legacy family migrated to the modular
  // engine — render_kick / render_snare / render_hihat / render_fm_bass etc. are
  // ALL deleted (FM in Phase-7, the LAST). render_modular is the only remaining
  // bespoke C renderer, so it stands in for all three voices of this self-contained
  // polyphonic headroom/clipping check (summing three render_modular voices still
  // exercises the per-voice normalize → amplitude-scale → sum headroom path).
  const instruments = ['modular-a', 'modular-b', 'modular-c'];
  const renderFuncs = {
    'modular-a': 'render_modular',
    'modular-b': 'render_modular',
    'modular-c': 'render_modular',
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
  // Skip polyphonic_mix and the parameterized cases - they have a different
  // structure and were already printed in their own sections above.
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
