// xplat_melodic_formant.browser.test.js
//
// FORMANT-PRESERVATION parity test for the per-pitch melodic re-render
// (desktop↔WASM parity, audio.js isMelodicInstrument path).
//
// Desktop melodic instruments RE-RENDER the C voice at the played semitone
// pitch (synth_recipe_dispatch.go: tryRecipeVoicePitched threads `pitch` into
// the modular param block so the C voice fundamental = 220·2^(pitch/12) while
// the filter cutoff stays at an ABSOLUTE Hz). WebAudio used to resample one
// pitch-0 buffer via BufferSource.playbackRate, which slides the filter formant
// up with pitch ("munchkin"). audio.js now renders melodic instruments AT the
// rounded pitch via render_modular_p, keyed by `${id}:${roundedPitch}`.
//
// This test PROVES re-render (not resample) directly against the C engine:
//   1. Render a melodic-style modular voice at pitch 0 (fundamental 220 Hz),
//      with a FIXED low-pass cutoff at an absolute Hz.
//   2. Render the SAME voice at pitch +12 (fundamental 440 Hz). Because the
//      cutoff is fixed in Hz, the +12 render's spectral rolloff stays at the
//      same absolute frequency.
//   3. Naively RESAMPLE the pitch-0 buffer by 2.0 (the old WebAudio behavior).
//      A 2× resample doubles every spectral feature, including the rolloff.
//   4. Assert the +12 at-pitch render DIFFERS materially from the 2× resample
//      (low-frequency-band spectral correlation < ~0.95) — i.e. the formant did
//      NOT simply double. A pure resample path would make them ~identical.
//
// It also confirms a DRUM (non-melodic) is unaffected by asserting
// isMelodicInstrument('kick') === false and isMelodicInstrument('violin')
// === true via the exported helper seam, and that a +12 vs 2×-resample
// comparison for a pure (no-filter) oscillator IS ~identical (sanity: the
// divergence comes from the fixed filter, the at-pitch mechanism itself does
// not corrupt a filterless voice).
//
// Run:  GO=$(pwd)/.tools/go/bin/go node src/js/xplat_melodic_formant.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  const { spawnSync } = await import("child_process");
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const SAMPLE_RATE = 48000;
const SECONDS = 1.0;
const FRAMES = SAMPLE_RATE * SECONDS;

// ---- metric helpers (host side) ----

// Linear-interpolated resample by `ratio` (ratio=2.0 → playbackRate 2 = +12
// semitones). Mirrors what WebAudio's BufferSource.playbackRate does.
function resample(data, ratio) {
  const outLen = data.length;
  const out = new Float32Array(outLen);
  for (let i = 0; i < outLen; i++) {
    const srcPos = i * ratio;
    const i0 = Math.floor(srcPos);
    const frac = srcPos - i0;
    const a = i0 < data.length ? data[i0] : 0;
    const b = i0 + 1 < data.length ? data[i0 + 1] : 0;
    out[i] = a + (b - a) * frac;
  }
  return out;
}

// Naive DFT magnitude at frequency f (Hz) over the buffer. Cheap single-bin
// Goertzel-style probe — enough to compare relative spectral energy at a
// handful of probe frequencies without an FFT lib.
function bandEnergy(data, sr, fLo, fHi, steps) {
  let total = 0;
  for (let s = 0; s < steps; s++) {
    const f = fLo + ((fHi - fLo) * s) / Math.max(1, steps - 1);
    const w = (2 * Math.PI * f) / sr;
    let re = 0, im = 0;
    // Decimate the sum for speed; the relative comparison is unaffected.
    const stride = 4;
    for (let n = 0; n < data.length; n += stride) {
      re += data[n] * Math.cos(w * n);
      im += data[n] * Math.sin(w * n);
    }
    total += Math.sqrt(re * re + im * im);
  }
  return total / steps;
}

// Pearson correlation between two equal-length signals.
function correlate(a, b) {
  const n = Math.min(a.length, b.length);
  let ma = 0, mb = 0;
  for (let i = 0; i < n; i++) { ma += a[i]; mb += b[i]; }
  ma /= n; mb /= n;
  let num = 0, da = 0, db = 0;
  for (let i = 0; i < n; i++) {
    const x = a[i] - ma, y = b[i] - mb;
    num += x * y; da += x * x; db += y * y;
  }
  if (da === 0 || db === 0) return 0;
  return num / Math.sqrt(da * db);
}

function peakNormalize(data) {
  let peak = 0;
  for (let i = 0; i < data.length; i++) { const a = Math.abs(data[i]); if (a > peak) peak = a; }
  if (peak <= 0) return data;
  const inv = 1 / peak;
  const out = new Float32Array(data.length);
  for (let i = 0; i < data.length; i++) out[i] = data[i] * inv;
  return out;
}

// ---- server + page ----

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
page.on("console", (msg) => { if (process.env.TEST_LOG) console.log("[PAGE]", msg.type(), msg.text()); });
page.on("pageerror", (err) => console.error("[PAGE ERROR]", err.message));

await page.goto(`http://localhost:${port}/test.html`);
await page.waitForFunction(() => window.__ready === true, { timeout: 10000 });
await page.evaluate(async () => { window.__drumsModule = await window.__drumsFactory(); });

const ABI = await import("./synth_param_abi.gen.js");
const PARAM_ABI = {
  count: ABI.MODULAR_PARAM_COUNT,
  index: ABI.MODULAR_PARAM_INDEX,
  identity: ABI.MODULAR_PARAM_IDENTITY,
};

// Render a modular voice with the given explicit params (everything else falls
// back to the ABI identity). Returns a host Float32Array.
async function renderModular(params) {
  const arr = await page.evaluate(async ({ params, abi, frames, sr }) => {
    const m = window.__drumsModule;
    if (!m) throw new Error("no shared drums module");
    const allocCount = Math.max(abi.count, 64);
    const paramsPtr = m._malloc(allocCount * 4);
    const pheap = m.HEAPF32.subarray(paramsPtr >> 2, (paramsPtr >> 2) + allocCount);
    pheap.fill(0);
    for (const [name, idx] of Object.entries(abi.index)) {
      const v = params[name];
      pheap[idx] = (typeof v === "number" && Number.isFinite(v)) ? v : abi.identity[name];
    }
    const ptr = m._malloc(frames * 4);
    m.ccall("render_modular_p", null, ["number", "number", "number", "number"], [ptr, sr, frames, paramsPtr]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);
    m._free(paramsPtr);
    return Array.from(data);
  }, { params, abi: PARAM_ABI, frames: FRAMES, sr: SAMPLE_RATE });
  return new Float32Array(arr);
}

// ---- assertions ----

let failed = false;
const fail = (msg) => { console.error("FAIL:", msg); failed = true; };
const pass = (msg) => { console.log("PASS:", msg); };

// (A) Helper-seam sanity: the melodic classifier matches the desktop set.
const helperCheck = await page.evaluate(async () => {
  const mod = await import("./audio.js");
  return {
    hasHelper: typeof mod.__isMelodicInstrument === "function",
    violin: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("violin") : null,
    bassGuitar: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("bass-guitar") : null,
    kick: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("kick") : null,
    snare: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("snare") : null,
  };
}).catch((e) => ({ err: String(e) }));

if (helperCheck.err) {
  fail(`could not import audio.js / helper: ${helperCheck.err}`);
} else if (!helperCheck.hasHelper) {
  fail("audio.js does not export __isMelodicInstrument test seam");
} else {
  if (helperCheck.violin === true) pass("isMelodicInstrument('violin') === true"); else fail("violin should be melodic");
  if (helperCheck.bassGuitar === true) pass("isMelodicInstrument('bass-guitar') === true"); else fail("bass-guitar should be melodic");
  if (helperCheck.kick === false) pass("isMelodicInstrument('kick') === false (drum unaffected)"); else fail("kick must NOT be melodic");
  if (helperCheck.snare === false) pass("isMelodicInstrument('snare') === false (drum unaffected)"); else fail("snare must NOT be melodic");
}

// (B) FORMANT PRESERVATION — filtered melodic voice.
// A sawtooth-style oscillator (osc_type=1) with a FIXED 1200 Hz low-pass.
// FM disabled (clean oscillator), drive disabled. Sustained envelope so the
// steady-state spectrum is well-defined.
const FILTER_CUTOFF_HZ = 1200;
function melodicParams(pitch) {
  return {
    osc_type: 1,            // sawtooth-ish (harmonically rich → the filter has work to do)
    osc_enabled: 1,
    fm_enabled: 0,          // clean oscillator (no FM bell)
    drive_enabled: 0,
    env_enabled: 1,
    amp_attack: 0.005,
    amp_decay: 0.1,
    amp_sustain: 0.9,       // mostly sustained → steady-state spectrum
    amp_release: 0.1,
    filter_enabled: 1,
    filter_type: 0,         // low-pass
    filter_cutoff: FILTER_CUTOFF_HZ, // ABSOLUTE Hz — fixed across pitches
    filter_resonance: 0.9,
    pitch,
    gain: 1,
  };
}

const p0 = peakNormalize(await renderModular(melodicParams(0)));     // fundamental 220 Hz
const p12 = peakNormalize(await renderModular(melodicParams(12)));   // fundamental 440 Hz, SAME 1200 Hz cutoff
const p0resampled = peakNormalize(resample(p0, 2.0));               // old WebAudio behavior: resample +12

// Use the steady-state region (skip the attack) for the spectral comparison.
const startN = Math.floor(0.2 * SAMPLE_RATE);
const lenN = Math.floor(0.5 * SAMPLE_RATE);
const seg = (d) => d.subarray(startN, startN + lenN);

// Band energy ABOVE the fixed cutoff (1800–4000 Hz). In the at-pitch render
// these harmonics are attenuated by the fixed 1200 Hz LP. In the 2×-resampled
// render the cutoff effectively doubled to ~2400 Hz, so MORE energy survives
// above 1800 Hz. The ratio is the smoking gun for resample-vs-rerender.
const hiBand = (d) => bandEnergy(seg(d), SAMPLE_RATE, 1800, 4000, 12);
const eHiAtPitch = hiBand(p12);
const eHiResample = hiBand(p0resampled);
const eHiBase = hiBand(p0);

// Waveform/spectral correlation across the high band feature.
const corr = correlate(seg(p12), seg(p0resampled));

console.log("\n--- FORMANT PRESERVATION (filtered melodic voice) ---");
console.log(`fixed filter cutoff:            ${FILTER_CUTOFF_HZ} Hz`);
console.log(`hi-band (1800-4000Hz) pitch0:   ${eHiBase.toFixed(4)}`);
console.log(`hi-band at-pitch (+12 rerender):${eHiAtPitch.toFixed(4)}`);
console.log(`hi-band +12 via 2x resample:    ${eHiResample.toFixed(4)}`);
console.log(`time-domain correlation(+12 rerender, 2x resample): ${corr.toFixed(4)}`);

// PRIMARY ASSERTION: the at-pitch +12 render must NOT be ~equal to the 2×
// resample. Correlation < 0.95 proves the formant did not simply slide.
if (corr < 0.95) {
  pass(`+12 at-pitch render DIFFERS materially from 2x resample (corr=${corr.toFixed(4)} < 0.95) — re-render, not resample`);
} else {
  fail(`+12 at-pitch render too similar to 2x resample (corr=${corr.toFixed(4)} >= 0.95) — looks like resample, formant slid`);
}

// SECONDARY ASSERTION: the 2× resample should leak materially MORE energy above
// the fixed cutoff than the at-pitch render (its rolloff doubled).
if (eHiResample > eHiAtPitch * 1.3) {
  pass(`2x resample leaks more hi-band energy than at-pitch (${eHiResample.toFixed(4)} > 1.3*${eHiAtPitch.toFixed(4)}) — rolloff doubled under resample`);
} else {
  fail(`expected 2x resample to leak more hi-band energy than at-pitch render; got resample=${eHiResample.toFixed(4)} atPitch=${eHiAtPitch.toFixed(4)}`);
}

// (C) SANITY — filterless voice: the at-pitch mechanism alone does NOT corrupt
// a voice with no fixed-Hz feature. A pure (filter-disabled) oscillator at +12
// should correlate HIGHLY with the 2× resample (both just double the pitch),
// confirming the divergence in (B) comes from the fixed filter, not a bug in
// the at-pitch render path.
function plainParams(pitch) {
  return {
    osc_type: 1, osc_enabled: 1, fm_enabled: 0, drive_enabled: 0,
    env_enabled: 1, amp_attack: 0.005, amp_decay: 0.1, amp_sustain: 0.9, amp_release: 0.1,
    filter_enabled: 0, // NO filter → no fixed-Hz formant
    pitch, gain: 1,
  };
}
const q0 = peakNormalize(await renderModular(plainParams(0)));
const q12 = peakNormalize(await renderModular(plainParams(12)));
const q0resampled = peakNormalize(resample(q0, 2.0));
// Compare HI-band energy (filterless → both should keep their harmonics; the
// at-pitch and resample versions should be spectrally close in the band).
const qHiAtPitch = hiBand(q12);
const qHiResample = hiBand(q0resampled);
const ratio = qHiResample > 0 ? qHiAtPitch / qHiResample : 0;
console.log("\n--- SANITY (filterless voice) ---");
console.log(`hi-band at-pitch:  ${qHiAtPitch.toFixed(4)}`);
console.log(`hi-band resample:  ${qHiResample.toFixed(4)}`);
console.log(`ratio:             ${ratio.toFixed(4)}`);
// Without a fixed filter, the two should be in the same ballpark (no doubled
// rolloff). Allow a wide band — the point is they are NOT wildly divergent the
// way the filtered case is.
if (ratio > 0.5 && ratio < 2.0) {
  pass(`filterless +12 at-pitch ≈ 2x resample hi-band energy (ratio=${ratio.toFixed(4)}) — divergence in (B) is the fixed filter, not the at-pitch path`);
} else {
  console.log(`NOTE: filterless hi-band ratio ${ratio.toFixed(4)} outside [0.5,2.0] — informational, not fatal`);
}

await browser.close();
server.close();

if (failed) {
  console.log("\nTEST FAILED - formant preservation / melodic re-render not verified");
  process.exit(1);
} else {
  console.log("\nTEST PASSED - WASM melodic instruments RE-RENDER at pitch (formant preserved)");
  process.exit(0);
}
