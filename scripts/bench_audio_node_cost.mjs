// bench_audio_node_cost.mjs
//
// Data-centric attribution of WebAudio audio-thread CPU cost per insert-effect
// type. Headless Chromium's audio thread is fast enough that it never underruns
// (the captured master WAV is clean even under heavy FX), so realtime drift
// can't reproduce the user's real-HW choppiness. But the *relative CPU cost*
// of each node is deterministic and measurable via OfflineAudioContext, which
// renders as fast as the CPU allows — so wall-time to render N seconds of audio
// IS the graph's total CPU cost, independent of headless audio-thread quirks.
//
// For each effect type we build K faithful copies of the exact node structure
// from audio.js createInsertEffectSubgraph (driven in parallel by one source),
// render RENDER_SEC of audio, and measure the median render wall-time over
// REPEATS. Ranking the per-effect cost confirms or refutes the
// reverb-convolver hypothesis. A "gain" baseline and a bare "none" graph
// bracket the cheap end.
//
// Run: node scripts/bench_audio_node_cost.mjs

import { chromium } from "../src/js/node_modules/playwright/index.mjs";

const K = Number(process.env.BENCH_K ?? 24);          // copies per effect (~6ch × 4fx)
const RENDER_SEC = Number(process.env.BENCH_RENDER_SEC ?? 4);
const REPEATS = Number(process.env.BENCH_REPEATS ?? 5);
const SR = 48000;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (m) => { if (process.env.TEST_LOG) console.log("[page]", m.text()); });

const result = await page.evaluate(async ({ K, RENDER_SEC, REPEATS, SR }) => {
  // Faithful copies of audio.js createInsertEffectSubgraph node structures.
  // Each builder returns { input, output } already connected; caller wires
  // src -> input and output -> destination.
  const builders = {
    none(c) { const g = c.createGain(); return { input: g, output: g }; },
    gain(c) {
      // 4 gains, matching the dry/wet/input/output overhead common to effects.
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      i.connect(d); d.connect(o); i.connect(w); w.connect(o);
      return { input: i, output: o };
    },
    filter(c) {
      const bq = c.createBiquadFilter(); bq.type = "lowpass"; bq.frequency.value = 1000; bq.Q.value = 0.707;
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      d.gain.value = 0; w.gain.value = 1;
      i.connect(d); d.connect(o); i.connect(bq); bq.connect(w); w.connect(o);
      return { input: i, output: o };
    },
    delay(c) {
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      d.gain.value = 0.2; w.gain.value = 0.8;
      const delay = c.createDelay(2); delay.delayTime.value = 0.05;
      const fb = c.createGain(); fb.gain.value = 0.6;
      i.connect(d); d.connect(o); i.connect(delay); delay.connect(w); w.connect(o);
      delay.connect(fb); fb.connect(delay);
      return { input: i, output: o };
    },
    chorus(c) {
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      const delay = c.createDelay(0.1); delay.delayTime.value = 0.01;
      const lfo = c.createOscillator(); lfo.type = "sine"; lfo.frequency.value = 3;
      const lg = c.createGain(); lg.gain.value = 0.01; lfo.connect(lg); lg.connect(delay.delayTime); lfo.start();
      i.connect(d); d.connect(o); i.connect(delay); delay.connect(w); w.connect(o);
      return { input: i, output: o };
    },
    distortion(c) {
      const ws = c.createWaveShaper();
      const n = 8192, curve = new Float32Array(n);
      for (let k = 0; k < n; k++) { const x = (k * 2) / n - 1; curve[k] = Math.tanh(x * 15); }
      ws.curve = curve; ws.oversample = "2x";
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      d.gain.value = 0; w.gain.value = 1;
      i.connect(d); d.connect(o); i.connect(ws); ws.connect(w); w.connect(o);
      return { input: i, output: o };
    },
    compressor(c) {
      const comp = c.createDynamicsCompressor();
      comp.threshold.value = -30; comp.ratio.value = 20; comp.attack.value = 0.001; comp.release.value = 0.05;
      const i = c.createGain(), o = c.createGain();
      i.connect(comp); comp.connect(o);
      return { input: i, output: o };
    },
    // synth/sampler are SOURCE voices, not processors — they self-drive an
    // internal BufferSource through the anti-pop fade gain (audio.js
    // processAudioEvent), so the measured cost is K played voices. `input` is a
    // harmless dead-end the framework's shared source feeds. Confirms synth /
    // sampler playback is cheap (a BufferSource + gain), NOT the choppiness
    // cost — node-logic rebuild (internal/audio node-logic cost table) is.
    synth(c) {
      const i = c.createGain(); // dead input (shared src feeds it; goes nowhere)
      const src = c.createBufferSource();
      const buf = c.createBuffer(1, c.sampleRate, c.sampleRate);
      const d = buf.getChannelData(0);
      for (let k = 0; k < d.length; k++) d[k] = Math.sin(2 * Math.PI * 220 * k / c.sampleRate) * Math.exp(-3 * k / c.sampleRate);
      src.buffer = buf; src.loop = true; src.start();
      const fade = c.createGain(); fade.gain.value = 0.18; // CHAIN_SPEC.voiceHeadroom
      const o = c.createGain();
      src.connect(fade); fade.connect(o);
      return { input: i, output: o };
    },
    sampler(c) {
      const i = c.createGain();
      const src = c.createBufferSource();
      const buf = c.createBuffer(1, c.sampleRate * 2, c.sampleRate); // longer sample
      const d = buf.getChannelData(0);
      for (let k = 0; k < d.length; k++) d[k] = Math.sin(2 * Math.PI * 330 * k / c.sampleRate);
      src.buffer = buf; src.loop = true;
      src.playbackRate.value = 1.3; // resampling cost (pitch shift)
      src.start();
      const fade = c.createGain(); fade.gain.value = 0.18;
      const o = c.createGain();
      src.connect(fade); fade.connect(o);
      return { input: i, output: o };
    },
    reverb_old(c) { // OLD: per-channel ConvolverNode (the CPU hog this fix removes)
      const room = 0.8, damping = 0.5, mix = 0.8;      // the heavy circuit's hot params
      const irLen = Math.floor(c.sampleRate * (0.5 + room * 2.5)); // up to 3s
      const irBuf = c.createBuffer(1, irLen, c.sampleRate);
      const irData = irBuf.getChannelData(0);
      const decay = 2 + damping * 6;
      for (let k = 0; k < irLen; k++) { const t = k / c.sampleRate; irData[k] = (Math.random() * 2 - 1) * Math.exp(-decay * t); }
      const conv = c.createConvolver(); conv.buffer = irBuf;
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      d.gain.value = 1 - mix; w.gain.value = mix;
      i.connect(d); d.connect(o); i.connect(conv); conv.connect(w); w.connect(o);
      return { input: i, output: o };
    },
    reverb(c) { // NEW: algorithmic Schroeder (matches audio.js buildSchroederReverbCore)
      const room = 0.8, damping = 0.5, mix = 0.8;
      const sr = c.sampleRate;
      const COMB = [1116, 1188, 1277, 1356], AP = [556, 441];
      const fb = 0.7 + 0.28 * room;
      let fc = (-Math.log(Math.max(1e-4, Math.min(0.9999, damping))) * sr) / (2 * Math.PI);
      fc = Math.max(200, Math.min(sr * 0.45, fc));
      const i = c.createGain(), d = c.createGain(), w = c.createGain(), o = c.createGain();
      d.gain.value = 1 - mix; w.gain.value = mix;
      i.connect(d); d.connect(o);
      const combSum = c.createGain(); combSum.gain.value = 1 / COMB.length;
      for (const dl of COMB) {
        const delay = c.createDelay(0.1); delay.delayTime.value = dl / 44100;
        const fbg = c.createGain(); fbg.gain.value = fb;
        i.connect(delay); delay.connect(combSum); delay.connect(fbg); fbg.connect(delay);
      }
      const damp = c.createBiquadFilter(); damp.type = "lowpass"; damp.frequency.value = fc;
      combSum.connect(damp);
      let node = damp;
      for (const dl of AP) {
        const delay = c.createDelay(0.05); delay.delayTime.value = dl / 44100;
        const ff = c.createGain(); ff.gain.value = -0.5; const fbg = c.createGain(); fbg.gain.value = 0.5;
        const out = c.createGain();
        node.connect(delay); node.connect(ff); delay.connect(out); ff.connect(out); delay.connect(fbg); fbg.connect(delay);
        node = out;
      }
      node.connect(w); w.connect(o);
      return { input: i, output: o };
    },
  };

  async function renderCost(effect) {
    const oc = new OfflineAudioContext(1, Math.floor(SR * RENDER_SEC), SR);
    // One source feeding K parallel effect copies summed to destination.
    const src = oc.createBufferSource();
    const noise = oc.createBuffer(1, SR, SR);
    const nd = noise.getChannelData(0);
    for (let k = 0; k < nd.length; k++) nd[k] = Math.random() * 2 - 1;
    src.buffer = noise; src.loop = true; src.start();
    const sum = oc.createGain(); sum.gain.value = 1 / K; sum.connect(oc.destination);
    for (let k = 0; k < K; k++) {
      const { input, output } = builders[effect](oc);
      src.connect(input); output.connect(sum);
    }
    const t0 = performance.now();
    await oc.startRendering();
    return performance.now() - t0;
  }

  const effects = ["none", "gain", "synth", "sampler", "filter", "compressor", "delay", "chorus", "distortion", "reverb_old", "reverb"];
  const out = {};
  for (const e of effects) {
    const times = [];
    for (let r = 0; r < REPEATS; r++) times.push(await renderCost(e));
    times.sort((a, b) => a - b);
    out[e] = +times[Math.floor(times.length / 2)].toFixed(1); // median wall-ms
  }
  return out;
}, { K, RENDER_SEC, REPEATS, SR });

await browser.close();

const base = result.gain || 1;
console.log(`\nAudio-node CPU cost — ${K} copies, ${RENDER_SEC}s audio, median of ${REPEATS} (OfflineAudioContext wall-ms)`);
console.log("effect        wall-ms   ×gain-baseline   per-node-ms");
const rows = Object.entries(result).sort((a, b) => a[1] - b[1]);
for (const [e, ms] of rows) {
  console.log(`${e.padEnd(12)}  ${String(ms).padStart(7)}   ${(ms / base).toFixed(1).padStart(8)}×       ${(ms / K).toFixed(2)}`);
}
const reverb = result.reverb, next = rows.filter(([e]) => e !== "reverb" && e !== "none" && e !== "gain").reduce((m, [, v]) => Math.max(m, v), 0);
console.log(`\nreverb is ${(reverb / Math.max(1, next)).toFixed(1)}× the next-most-expensive effect.`);
