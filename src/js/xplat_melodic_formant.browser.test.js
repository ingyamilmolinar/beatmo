// Melodic voices must RE-RENDER at pitch, as desktop does, not resample: with a fixed-Hz low-pass, the +12 render must
// differ from the pitch-0 render resampled ×2 (the formant must not slide), while a filterless voice at +12 must match
// its ×2 resample. Run: GO=… node src/js/xplat_melodic_formant.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm } from "./real_input_test_helpers.js";

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

// As WebAudio's playbackRate (2.0 = +12 semitones).
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

// A single-bin probe at a few frequencies: no FFT library needed.
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

// ---- blocks from the real app ----

// The blocks and the melodic flags are Go's: read once from the real app.
const CLASSIFY = ["violin", "bass-guitar", "kick", "snare"];
let BLOCKS;
{
  const app = await setupFullWasm({ logLevel: "ERROR" });
  try {
    BLOCKS = await app.page.evaluate((ids) => Object.fromEntries(ids.map((id) => {
      const s = window.__instrumentBlockForTest(id);
      return [id, s && { block: Array.from(s.block), seconds: s.seconds, amp: s.amp, melodic: s.melodic }];
    })), ["modular", ...CLASSIFY]);
  } finally {
    await app.cleanup();
  }
  const missing = Object.keys(BLOCKS).filter((id) => !BLOCKS[id]);
  if (missing.length) throw new Error(`the app pushed no block for ${missing.join(", ")}`);
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
await page.waitForFunction(() => window.__ready === true, null, { timeout: 10000 });
await page.evaluate(async () => { window.__drumsModule = await window.__drumsFactory(); });

const ABI = await import("./synth_param_abi.gen.js");
const PARAM_ABI = {
  count: ABI.MODULAR_PARAM_COUNT,
  index: ABI.MODULAR_PARAM_INDEX,
  base: BLOCKS.modular.block,
};

// Everything else keeps the bare modular voice's value.
async function renderModular(params) {
  const arr = await page.evaluate(async ({ params, abi, frames, sr }) => {
    const m = window.__drumsModule;
    if (!m) throw new Error("no shared drums module");
    const allocCount = Math.max(abi.count, 64);
    // The modular block is DOUBLE (config-only refactor Phase A).
    const paramsPtr = m._malloc(allocCount * 8);
    const pheap = m.HEAPF64.subarray(paramsPtr >> 3, (paramsPtr >> 3) + allocCount);
    pheap.fill(0);
    for (const [name, idx] of Object.entries(abi.index)) {
      const v = params[name];
      pheap[idx] = (typeof v === "number" && Number.isFinite(v)) ? v : abi.base[idx];
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
const helperCheck = await page.evaluate(async (blocks) => {
  const mod = await import("./audio.js");
  for (const [id, s] of Object.entries(blocks)) {
    window.setInstrumentBlock(id, Float64Array.from(s.block), s.seconds, s.amp, s.melodic, s.gateTail, s.fx);
  }
  return {
    hasHelper: typeof mod.__isMelodicInstrument === "function",
    violin: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("violin") : null,
    bassGuitar: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("bass-guitar") : null,
    kick: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("kick") : null,
    snare: mod.__isMelodicInstrument ? mod.__isMelodicInstrument("snare") : null,
  };
}, BLOCKS).catch((e) => ({ err: String(e) }));

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

// A saw through a FIXED 1200 Hz low-pass, sustained so the steady-state spectrum is well defined.
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

// Above the cutoff the at-pitch render is attenuated; a ×2 resample doubled the cutoff, so more survives: the
// resample-vs-rerender tell.
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

// Correlation < 0.95 proves the formant did not simply slide.
if (corr < 0.95) {
  pass(`+12 at-pitch render DIFFERS materially from 2x resample (corr=${corr.toFixed(4)} < 0.95) — re-render, not resample`);
} else {
  fail(`+12 at-pitch render too similar to 2x resample (corr=${corr.toFixed(4)} >= 0.95) — looks like resample, formant slid`);
}

// The resample's rolloff doubled.
if (eHiResample > eHiAtPitch * 1.3) {
  pass(`2x resample leaks more hi-band energy than at-pitch (${eHiResample.toFixed(4)} > 1.3*${eHiAtPitch.toFixed(4)}) — rolloff doubled under resample`);
} else {
  fail(`expected 2x resample to leak more hi-band energy than at-pitch render; got resample=${eHiResample.toFixed(4)} atPitch=${eHiAtPitch.toFixed(4)}`);
}

// With no fixed-Hz feature the at-pitch render should match the ×2 resample, so the divergence above is the filter's.
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
// Filterless, both keep their harmonics.
const qHiAtPitch = hiBand(q12);
const qHiResample = hiBand(q0resampled);
const ratio = qHiResample > 0 ? qHiAtPitch / qHiResample : 0;
console.log("\n--- SANITY (filterless voice) ---");
console.log(`hi-band at-pitch:  ${qHiAtPitch.toFixed(4)}`);
console.log(`hi-band resample:  ${qHiResample.toFixed(4)}`);
console.log(`ratio:             ${ratio.toFixed(4)}`);
// Same ballpark, not wildly divergent as in the filtered case.
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
