// A recording tap must be acoustically TRANSPARENT: it is a parallel branch whose node reaches ctx.destination, and a
// worklet that copied input to output summed the master twice (+6 dB, distorting). Rendered on an OfflineAudioContext
// with and without the tap wired as audio.js wires it; the peaks must match. Usage: node src/js/recording_tap_transparency.browser.test.js

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

    // The second half of the buffer, past the startup transient; wireTap(ctx, limiter, captureNode) wires the tap or not.
    async function renderPeak(wireTap) {
      const ctx = new OfflineAudioContext(1, FRAMES, SR);
      await ctx.audioWorklet.addModule(`${origin}/recording_capture_worklet.js`);

      // Steady DC source standing in for the live signal.
      const src = new ConstantSourceNode(ctx, { offset: DC });
      // The last audible node before the speakers, where the master tap sits. Gain 1: a pass-through.
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

    // Exactly as startMultiChannelCapture wires it.
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

  // Starting a recording must not change the audible output.
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
