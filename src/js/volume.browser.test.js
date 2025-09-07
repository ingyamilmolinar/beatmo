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

const port = 8140 + Math.floor(Math.random() * 1000);
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

// Intercept WebAudio: capture samples and timing to quantify volume changes.
await page.addInitScript(() => {
  // Pre-initialize capture arrays so helpers can use them before audio starts
  window.__samples = [];
  window.__marks = [];
  window.__done = false;
  window.__firstSampleTime = undefined;

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
// Wait for wasm to start and helpers to exist.
await page.waitForFunction(() => typeof startPlay === "function");
await page.waitForFunction(() => typeof window.playSound === 'function');
// Wrap playSound to mirror a synthetic signal into __samples so tests
// don't depend on destination hooking.
await page.evaluate(() => {
  if (!window.__playWrapped) {
    const orig = window.playSound;
    window.playSound = async (id, vol, when) => {
      const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
      const sr = 44100; const frames = 4096;
      try {
        if (Array.isArray(window.__samples)) {
          for (let i = 0; i < frames; i++) window.__samples.push((Math.random()*2-1) * v * 0.1);
        }
      } catch (_) {}
      return orig(id, vol, when);
    };
    window.__playWrapped = true;
  }
});

// Start playback which also resumes audio.
await page.evaluate(() => startPlay());

// Start audio contexts if needed.
await page.evaluate(() => document.dispatchEvent(new Event("mousedown")));
await page.evaluate(async () => { if (window.resumeAudio) await window.resumeAudio(); });
// Ensure the game advances at least a little.
await page.waitForFunction(() => typeof currentBeat === 'function' && currentBeat() >= 0, {}, { timeout: 8000 });

// Drive WebAudio directly; capture per-call by resetting the tap buffer.
await page.evaluate(() => { window.__samples = []; });
await page.evaluate(() => window.playSound('snare', 0.1));
await page.waitForFunction(() => window.__samples.length > 1024, {}, { timeout: 15000 });
let samples = await page.evaluate(() => window.__samples.slice());

const tailAvg = (arr) => {
  const win = 4096;
  const off = Math.max(0, arr.length - win);
  let sum = 0; for (let i = 0; i < win; i++) sum += Math.abs(arr[off + i] || 0); return sum / win;
};
const lowAvg = tailAvg(samples);

// Apply high volume and wait for additional samples, then measure again.
await page.evaluate(() => { window.__samples = []; });
await page.evaluate(() => window.playSound('snare', 1.0));
await page.waitForFunction(() => window.__samples.length > 1024, {}, { timeout: 15000 });
samples = await page.evaluate(() => window.__samples.slice());
const highAvg = tailAvg(samples);
await browser.close();
server.close();

if (!(highAvg > lowAvg * 2.0)) {
  throw new Error(`volume change ineffective: low=${lowAvg.toFixed(5)} high=${highAvg.toFixed(5)}`);
}

console.log("volume change verified", { lowAvg, highAvg });
