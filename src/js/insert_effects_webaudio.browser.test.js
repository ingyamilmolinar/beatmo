// insert_effects_webaudio.browser.test.js
//
// JS Test Charter §1 (WebAudio behavior): in the browser, insert effects are
// rendered by the WebAudio node graph built in createInsertEffectSubgraph()
// (src/js/audio.js) — NOT the Go/C DSP (the AudioWorklet path is intentionally
// dormant to avoid a per-channel WASM cost; see the buildSchroederReverbCore
// comment). So EVERY insert effect must have a real WebAudio implementation
// here; a passthrough placeholder means the effect silently does nothing.
//
// This suite renders EVERY registered insert effect through the production
// createInsertEffectSubgraph() in a deterministic OfflineAudioContext and
// asserts each one measurably transforms the signal. Because rendering is
// offline (no playback timing jitter) and dry/wet share an identical source
// buffer, the comparison is exact and reproducible — unlike the played-kick
// correlation heuristic in worklet_insert_effects.browser.test.js, which can
// false-pass a passthrough.
//
// Per-effect assertion modes:
//   'diff'  — wet must differ from dry by > DIFF_MIN (relative RMS). Catches
//             passthroughs (diff == 0).
//   'gate'  — a below-threshold input must be attenuated: RMS(wet) must fall to
//             < GATE_MAX * RMS(dry). A passthrough leaves it unchanged.
//   'pitch' — a 440 Hz sine shifted +12 st must have more energy at 880 Hz than
//             at 440 Hz (true transposition, not just "different").
//
// Usage: node src/js/insert_effects_webaudio.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Minimal page importing the real audio.js module (no WASM / AudioContext at
// import — only static .gen.js data — so it loads standalone).
const TEST_HTML = `<!doctype html><html><head><meta charset="utf-8"></head><body>
<script type="module">
  import { createInsertEffectSubgraph } from './audio.js';
  window.createInsertEffectSubgraph = createInsertEffectSubgraph;
  window.__ready = true;
</script>
</body></html>`;

const server = http.createServer((req, res) => {
  if (req.url === "/" || req.url === "/index.html") {
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(TEST_HTML);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, "").split("?")[0]);
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    else if (filePath.endsWith(".html")) ct = "text/html";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

// Every registered effect (mirrors EffectTypeOrder / the registry) with
// aggressive params (from ifxGoldenHotParams) chosen to exercise the wet path,
// plus its assertion mode and input signal. If a new effect is registered in
// Go, add it here — a missing entry is intentional test friction.
const EFFECTS = [
  { type: "distortion", params: { drive: 15, tone: 4000, mix: 1 },                              signal: "broadband", mode: "diff" },
  { type: "delay",      params: { time: 80, feedback: 0.5, mix: 0.8 },                          signal: "broadband", mode: "diff" },
  { type: "reverb",     params: { room: 0.8, damping: 0.5, mix: 0.8 },                          signal: "broadband", mode: "diff" },
  { type: "chorus",     params: { rate: 3, depth: 10, mix: 0.9 },                               signal: "broadband", mode: "diff" },
  { type: "bitcrusher", params: { bits: 4, rate: 0.2, mix: 1 },                                 signal: "broadband", mode: "diff" },
  { type: "filter",     params: { mode: 0, cutoff: 300, q: 2, mix: 1 },                         signal: "broadband", mode: "diff" },
  { type: "phaser",     params: { stages: 8, rate: 2, depth: 1, feedback: 0.8, mix: 1 },        signal: "broadband", mode: "diff" },
  { type: "flanger",    params: { rate: 2, depth: 5, feedback: 0.8, mix: 1 },                   signal: "broadband", mode: "diff" },
  { type: "tremolo",    params: { rate: 8, depth: 1, shape: 0, mix: 1 },                        signal: "broadband", mode: "diff" },
  { type: "gate",       params: { threshold: -20, attack: 1, release: 10, range: -90 },         signal: "quiet",     mode: "gate" },
  { type: "limiter",    params: { threshold: -20, release: 50, ceiling: -6 },                   signal: "broadband", mode: "diff" },
  { type: "ringmod",    params: { frequency: 300, shape: 0, mix: 1 },                           signal: "broadband", mode: "diff" },
  { type: "waveshaper", params: { curve: 2, drive: 10, mix: 1 },                                signal: "broadband", mode: "diff" },
  { type: "autowah",    params: { sensitivity: 1, rate: 5, depth: 1, mix: 1 },                  signal: "broadband", mode: "diff" },
  { type: "compressor", params: { threshold: -30, ratio: 20, attack: 0.1, release: 50, makeup: 12, mix: 1 }, signal: "broadband", mode: "diff" },
  { type: "transient",  params: { attack: 200, sustain: 30, speed: 5 },                         signal: "broadband", mode: "diff" },
  { type: "tape",       params: { drive: 8, warmth: 0.8, wow: 0.5, flutter: 0.5, mix: 1 },      signal: "broadband", mode: "diff" },
  { type: "pitchshift", params: { pitch: 12, mix: 1, window: 50 },                              signal: "sine",      mode: "pitch" },
];

const DIFF_MIN = 0.03; // wet must differ from dry by >3% relative RMS
const GATE_MAX = 0.3;  // gated wet must drop below 30% of dry RMS

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch();
  const page = await browser.newPage();
  if (process.env.TEST_LOG) {
    page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
    page.on("pageerror", (err) => console.log(`  [pageerror] ${err.message}`));
  }
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready === true, {}, { timeout: 10000 });

  const results = await page.evaluate(async ({ EFFECTS, DIFF_MIN, GATE_MAX }) => {
    const SR = 44100;
    const DUR = 0.5;
    const N = Math.round(SR * DUR);

    // Deterministic broadband noise (integer LCG) at the given amplitude.
    function noise(n, amp) {
      const out = new Float32Array(n);
      let s = 0x2545f491 >>> 0;
      for (let i = 0; i < n; i++) {
        s = (Math.imul(1664525, s) + 1013904223) >>> 0;
        out[i] = ((s >>> 8) / (1 << 24) * 2 - 1) * amp;
      }
      return out;
    }
    function sine(n, freq, amp) {
      const out = new Float32Array(n);
      for (let i = 0; i < n; i++) out[i] = amp * Math.sin((2 * Math.PI * freq * i) / SR);
      return out;
    }
    function makeSignal(kind) {
      if (kind === "sine") return sine(N, 440, 0.8);
      if (kind === "quiet") return noise(N, 0.03); // ~-30 dB: below a -20 dB gate
      return noise(N, 0.8);                         // broadband, near full-scale
    }

    function goertzel(x, freq) {
      const w = (2 * Math.PI * freq) / SR;
      const coeff = 2 * Math.cos(w);
      let s1 = 0, s2 = 0;
      for (let i = 0; i < x.length; i++) { const s0 = x[i] + coeff * s1 - s2; s2 = s1; s1 = s0; }
      return Math.sqrt(s1 * s1 + s2 * s2 - coeff * s1 * s2);
    }
    function rms(x) { let e = 0; for (let i = 0; i < x.length; i++) e += x[i] * x[i]; return Math.sqrt(e / x.length); }

    // Render `data` through an optional effect subgraph in a fresh offline ctx.
    async function render(data, effect) {
      const off = new OfflineAudioContext(1, N, SR);
      const buf = off.createBuffer(1, N, SR);
      buf.getChannelData(0).set(data);
      const src = off.createBufferSource();
      src.buffer = buf;

      let tail = src;
      if (effect) {
        const sg = window.createInsertEffectSubgraph(off, {
          type: effect.type, enabled: true, params: effect.params,
        });
        if (!sg || !sg.input || !sg.output) throw new Error(`${effect.type}: subgraph missing input/output`);
        src.connect(sg.input);
        tail = sg.output;
        // NB: createInsertEffectSubgraph starts its own oscillators internally;
        // sg.oscillators is for teardown (stop), not for the caller to start.
      }
      tail.connect(off.destination);
      src.start(0);
      const rendered = await off.startRendering();
      return rendered.getChannelData(0);
    }

    const out = [];
    for (const eff of EFFECTS) {
      try {
        const input = makeSignal(eff.signal);
        const dry = await render(input, null);
        const wet = await render(input, eff);

        // Analyze the settled tail (skip first 10% for warmup / latency).
        const skip = Math.floor(N / 10);
        const d = dry.subarray(skip), w = wet.subarray(skip);
        const dryRMS = rms(d), wetRMS = rms(w);

        // Relative RMS of the (wet - dry) difference signal.
        let de = 0;
        for (let i = 0; i < d.length; i++) { const diff = w[i] - d[i]; de += diff * diff; }
        const diffRMS = Math.sqrt(de / d.length);
        const relDiff = dryRMS > 1e-9 ? diffRMS / dryRMS : 0;

        const row = { type: eff.type, mode: eff.mode, dryRMS, wetRMS, relDiff };

        if (eff.mode === "gate") {
          row.pass = wetRMS < GATE_MAX * dryRMS;
          row.detail = `wetRMS ${wetRMS.toExponential(2)} vs GATE_MAX*dryRMS ${(GATE_MAX * dryRMS).toExponential(2)}`;
        } else if (eff.mode === "pitch") {
          const mag880 = goertzel(w, 880), mag440 = goertzel(w, 440);
          row.mag880 = mag880; row.mag440 = mag440;
          row.pass = wetRMS > 1e-4 && mag880 > mag440;
          row.detail = `880Hz ${mag880.toFixed(2)} vs 440Hz ${mag440.toFixed(2)}`;
        } else {
          row.pass = wetRMS > 1e-6 && relDiff > DIFF_MIN;
          row.detail = `relDiff ${(relDiff * 100).toFixed(1)}%`;
        }
        out.push(row);
      } catch (err) {
        out.push({ type: eff.type, mode: eff.mode, pass: false, detail: `error: ${err.message}` });
      }
    }
    return out;
  }, { EFFECTS, DIFF_MIN, GATE_MAX });

  let passed = 0;
  const failures = [];
  for (const r of results) {
    const tag = r.pass ? "PASS" : "FAIL";
    console.log(`  [${tag}] ${r.type.padEnd(11)} (${r.mode}) — ${r.detail}`);
    if (r.pass) passed++;
    else failures.push(r.type);
  }

  console.log("\n========================================");
  console.log(`Results: ${passed}/${results.length} insert effects transform audio through the WebAudio graph`);
  if (failures.length > 0) {
    console.log(`FAIL: ${failures.length} effect(s) are passthrough / non-functional in the browser: ${failures.join(", ")}`);
    exitCode = 1;
  } else {
    console.log("PASS: every insert effect is correctly wired.");
  }
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
