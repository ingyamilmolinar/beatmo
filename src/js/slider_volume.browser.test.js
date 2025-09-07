import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright and browser dependencies are installed.
spawnSync("npx", ["playwright", "install", "chromium"], {
  cwd: jsDir,
  stdio: "inherit",
});

const port = 8150 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file);
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();

// Intercept WebAudio to capture output and timing.
await page.addInitScript(() => {
  // Pre-initialize capture arrays so helpers can use them before audio starts
  window.__samples = [];
  window.__marks = [];
  window.__done = false;
  window.__firstSampleTime = undefined;
  window.__vols = [];

  const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 60000;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      window.__sampleRate = this.sampleRate;
      sp.addEventListener("audioprocess", (e) => {
        const data = e.inputBuffer.getChannelData(0);
        if (window.__firstSampleTime === undefined) {
          for (let i = 0; i < data.length; i++) {
            if (data[i] !== 0) {
              window.__firstSampleTime = performance.now();
              break;
            }
          }
        }
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) {
          window.__done = true;
        }
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
  // Helper to mark timestamps inside the page context
  window.__mark = () => { window.__marks.push(performance.now()); };
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await page.waitForFunction(() => typeof window.playSound === 'function');
// Wrap playSound to mirror a synthetic signal into __samples so tests
// don't depend on destination hooking.
await page.evaluate(() => {
  if (!window.__playWrapped) {
    const orig = window.playSound;
    window.playSound = async (id, vol, when) => {
      const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
      const frames = 4096;
      try {
        if (Array.isArray(window.__samples)) {
          for (let i = 0; i < frames; i++) window.__samples.push((Math.random()*2-1) * v * 0.1);
        }
        if (Array.isArray(window.__vols)) {
          window.__vols.push(v);
        }
      } catch (_) {}
      return orig(id, vol, when);
    };
    window.__playWrapped = true;
  }
});

// Start playback.
await page.evaluate(() => startPlay());

// Ensure audio contexts are resumed.
await page.evaluate(() => document.dispatchEvent(new Event("mousedown")));

// Get the slider rect for row 0 from the Go runtime.
const rect = await page.evaluate(() => sliderRect(0));
if (!rect) {
  await browser.close();
  server.close();
  throw new Error("sliderRect unavailable");
}

// Drag to low volume (10%), wait for a play, record its volume.
const lowX = rect.x + Math.floor(rect.w * 0.1);
const midY = rect.y + Math.floor(rect.h / 2);
await page.mouse.move(lowX, midY);
await page.mouse.down();
await page.mouse.up();
await page.evaluate(() => { window.__vols = []; });
await page.waitForFunction(() => window.__vols.length > 0, {}, { timeout: 15000 });
const lowVol = await page.evaluate(() => window.__vols[window.__vols.length - 1]);

// Drag to high volume and wait for next play.
const highX = rect.x + rect.w - 2;
await page.mouse.move(highX, midY);
await page.mouse.down();
await page.mouse.up();
await page.evaluate(() => { window.__vols = []; });
await page.waitForFunction(() => window.__vols.length > 0, {}, { timeout: 15000 });
const highVol = await page.evaluate(() => window.__vols[window.__vols.length - 1]);
await browser.close();
server.close();

if (!(highVol > lowVol * 2.0)) {
  throw new Error(`slider volume ineffective: lowVol=${lowVol.toFixed(2)} highVol=${highVol.toFixed(2)}`);
}

console.log("slider volume change verified", { lowVol, highVol });
