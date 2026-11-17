// recording_tap_transparency.browser.test.js
//
// Regression test for the "audio gets louder/distorted while recording" bug.
//
// The WASM recording pipeline taps the audible graph by inserting one
// `recording-capture-processor` AudioWorkletNode per channel (and one for the
// master) as a PARALLEL branch — see startMultiChannelCapture in audio.js:
//
//     setup.source.connect(node);        // limiter/channel-gain → capture node
//     node.connect(c.destination);       // capture node → speakers
//
// A capture tap MUST be acoustically transparent: starting a recording must
// not change one sample of what the user hears. The bug is that the worklet
// used to copy its input to its output verbatim (`output[0].set(input[0])`)
// AND was connected straight to ctx.destination. Because the tapped source
// (the master limiter) is ALSO already wired to ctx.destination, the limiter
// signal reaches the speakers TWICE and is summed → +6 dB, i.e. doubled
// amplitude → audibly louder and clipping/distorting against the soft-clip
// knee. The same parallel re-injection happens for every per-instrument tap.
//
// This test drives the REAL worklet on an OfflineAudioContext (whose
// `destination` renders to a readable buffer — unlike a live AudioContext
// where the final mix is unobservable). It compares the rendered output peak
// WITH and WITHOUT the recording tap wired exactly as audio.js wires it. The
// peaks must match: a transparent tap adds nothing. No WASM build needed.
//
// Usage: node src/js/recording_tap_transparency.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    const ct = filePath.endsWith(".js") ? "application/javascript" : "text/html";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

let failures = 0;
function check(name, cond, detail = "") {
  if (cond) {
    console.log(`✓ ${name}`);
  } else {
    console.error(`✗ ${name} ${detail}`);
    failures++;
  }
}

let browser;
try {
  browser = await chromium.launch({
    args: ["--autoplay-policy=no-user-gesture-required"],
  });
  const page = await browser.newPage();
  if (process.env.TEST_LOG) {
    page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
  }
  await page.goto(`http://localhost:${port}/recording_capture_worklet.js`, {
    waitUntil: "commit",
  });
  await page.setContent("<html><body></body></html>");

  const result = await page.evaluate(async (origin) => {
    const SR = 48000;
    const FRAMES = SR / 10; // 0.1 s — plenty of steady-state quanta
    const DC = 0.25; // steady DC level fed through the "limiter"

    // Render a graph through an OfflineAudioContext and return the peak abs
    // sample over the second half of the buffer (skip the worklet/source
    // startup transient). `wireTap` receives (ctx, limiter, captureNode) and
    // is responsible for wiring the recording tap — or doing nothing.
    async function renderPeak(wireTap) {
      const ctx = new OfflineAudioContext(1, FRAMES, SR);
      await ctx.audioWorklet.addModule(`${origin}/recording_capture_worklet.js`);

      // Steady DC source standing in for the live signal.
      const src = new ConstantSourceNode(ctx, { offset: DC });
      // "limiter" == the last audible node before the speakers, mirroring
      // _buildMasterTap()'s tap point. Gain 1 so it is a pass-through.
      const limiter = new GainNode(ctx, { gain: 1 });
      src.connect(limiter);
      limiter.connect(ctx.destination); // the normal audible path

      if (wireTap) {
        const captureNode = new AudioWorkletNode(ctx, "recording-capture-processor", {
          numberOfInputs: 1,
          numberOfOutputs: 1,
          outputChannelCount: [1],
        });
        wireTap(ctx, limiter, captureNode);
      }

      src.start();
      const buf = await ctx.startRendering();
      const data = buf.getChannelData(0);
      let peak = 0;
      for (let i = (data.length >> 1); i < data.length; i++) {
        const a = Math.abs(data[i]);
        if (a > peak) peak = a;
      }
      return peak;
    }

    // Baseline: no recording tap. Peak should be ~DC.
    const baseline = await renderPeak(null);

    // With the recording tap wired EXACTLY as startMultiChannelCapture does:
    //   setup.source.connect(node);  node.connect(c.destination);
    const withTap = await renderPeak((ctx, limiter, captureNode) => {
      limiter.connect(captureNode);
      captureNode.connect(ctx.destination);
    });

    return { baseline, withTap, DC };
  }, `http://localhost:${port}`);

  console.log(
    `  baseline peak=${result.baseline.toFixed(4)} ` +
      `withTap peak=${result.withTap.toFixed(4)} (DC=${result.DC})`,
  );

  // Sanity: the baseline graph actually produced the DC signal.
  check(
    "baseline graph renders the source signal",
    Math.abs(result.baseline - result.DC) < 0.02,
    `expected ~${result.DC}, got ${result.baseline.toFixed(4)}`,
  );

  // THE BUG: starting a recording tap must not change the audible output.
  // Pre-fix this fails with withTap ≈ 2×baseline (the doubled, distorting mix).
  check(
    "recording tap is acoustically transparent (output unchanged)",
    Math.abs(result.withTap - result.baseline) < 0.02,
    `tap changed output: baseline=${result.baseline.toFixed(4)} ` +
      `withTap=${result.withTap.toFixed(4)} ` +
      `(ratio ${(result.withTap / result.baseline).toFixed(3)}× — a transparent tap is 1.000×)`,
  );
} catch (err) {
  console.error(`✗ FATAL: ${err.message}`);
  console.error(err.stack);
  failures++;
} finally {
  await browser?.close();
  server.close();
  if (failures > 0) {
    console.error(`\n${failures} check(s) failed`);
    process.exitCode = 1;
  } else {
    console.log(`\nAll checks passed`);
  }
}
