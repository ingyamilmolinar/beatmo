// pitchshift_webaudio_fallback.browser.test.js
//
// JS Test Charter §1 (WebAudio behavior): the browser insert-effect chain is
// applied by the WebAudio node graph built in createInsertEffectSubgraph()
// (src/js/audio.js). This is the authoritative signal path in the browser —
// the Go/C insert-effect DSP is NOT what you hear for insert FX in WASM
// (updateInsertEffects marshals the slots to JS and the WebAudio graph renders
// them; the AudioWorklet path `chain.workletInsertNode` is never wired up).
//
// BUG UNDER TEST: the 'pitchshift' case in createInsertEffectSubgraph is a
// hard-coded unity-gain passthrough (audio.js ~2357-2362), so the "Pitch
// Shifter" insert effect does nothing in the browser. The Go DSP
// (fx_pitchshift.go) and C DSP (insert_fx.c) both correctly transpose — proven
// by fx_pitchshift_test.go::TestPitchShiftOctaveUp — but that DSP is never
// reached from the WebAudio graph.
//
// This test drives the REAL production function (createInsertEffectSubgraph)
// through a deterministic OfflineAudioContext render — no WASM build, no
// playback timing jitter. It feeds a pure 440 Hz sine through a Pitch Shifter
// set to +12 semitones and asserts the rendered output has more energy at the
// shifted fundamental (880 Hz) than at the original (440 Hz).
//
// Existing coverage does NOT catch this: worklet_insert_effects.browser.test.js
// discriminates effects by waveform correlation on a played kick sample, which
// is defeated by capture-onset timing jitter (see the note in
// import_insert_effects.browser.test.js:231) and yields a false pass for a
// passthrough. A frequency-domain assertion is jitter-invariant.
//
// Usage: node src/js/pitchshift_webaudio_fallback.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Minimal page that imports the real audio.js module and exposes the
// production subgraph builder on window. audio.js has no heavy top-level side
// effects (no AudioContext / WASM at import — only static .gen.js data), so it
// loads standalone.
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

  const result = await page.evaluate(async () => {
    // Goertzel single-bin magnitude — timing-jitter-invariant frequency probe.
    function goertzel(samples, freq, sr) {
      const w = (2 * Math.PI * freq) / sr;
      const cw = Math.cos(w);
      const coeff = 2 * cw;
      let s0 = 0, s1 = 0, s2 = 0;
      for (let i = 0; i < samples.length; i++) {
        s0 = samples[i] + coeff * s1 - s2;
        s2 = s1;
        s1 = s0;
      }
      return Math.sqrt(s1 * s1 + s2 * s2 - coeff * s1 * s2);
    }

    const sr = 44100;
    const dur = 1.0;
    const n = Math.round(sr * dur);
    const off = new OfflineAudioContext(1, n, sr);

    const sg = window.createInsertEffectSubgraph(off, {
      type: "pitchshift",
      enabled: true,
      params: { pitch: 12, mix: 1, window: 50 }, // +1 octave, fully wet
    });
    if (!sg || !sg.input || !sg.output) {
      return { error: "pitchshift subgraph missing input/output" };
    }

    const osc = off.createOscillator();
    osc.type = "sine";
    osc.frequency.value = 440;
    osc.connect(sg.input);
    sg.output.connect(off.destination);
    // Start any oscillators the subgraph created (e.g. LFOs); pitchshift has none.
    if (Array.isArray(sg.oscillators)) sg.oscillators.forEach((o) => o.start(0));
    osc.start(0);

    const buf = await off.startRendering();
    const data = buf.getChannelData(0);

    // Analyze the settled tail (skip the first half for the shifter to warm up).
    const stable = data.subarray(Math.floor(n / 2));

    let energy = 0;
    for (let i = 0; i < stable.length; i++) energy += stable[i] * stable[i];
    const rms = Math.sqrt(energy / stable.length);

    return {
      mag880: goertzel(stable, 880, sr),
      mag440: goertzel(stable, 440, sr),
      rms,
    };
  });

  if (result.error) throw new Error(result.error);

  console.log(`Pitch Shifter +12st on 440Hz sine (OfflineAudioContext render):`);
  console.log(`  output RMS:      ${result.rms.toFixed(6)}`);
  console.log(`  energy @ 880 Hz: ${result.mag880.toFixed(4)}  (shifted fundamental)`);
  console.log(`  energy @ 440 Hz: ${result.mag440.toFixed(4)}  (original fundamental)`);

  if (result.rms < 1e-4) {
    throw new Error(`output is silent (RMS ${result.rms}) — subgraph produced no signal`);
  }
  if (result.mag880 <= result.mag440) {
    throw new Error(
      `Pitch Shifter did NOT transpose up an octave: ` +
      `880Hz energy (${result.mag880.toFixed(4)}) <= 440Hz energy (${result.mag440.toFixed(4)}). ` +
      `The WebAudio 'pitchshift' fallback in createInsertEffectSubgraph is a passthrough.`
    );
  }

  console.log("\nPASS: Pitch Shifter transposes 440Hz -> 880Hz through the WebAudio graph.");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
