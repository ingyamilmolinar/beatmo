import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright's Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8160 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  try { console.log('[SRV]', req.url); } catch(_) {}
  const file = req.url === "/" ? "/consistency.html" : req.url;
  if (req.url === "/" || req.url === "/consistency.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module">
  import drumsFactory from './drums.single.js';
  window.__drumsFactory = drumsFactory;
</script>
<script type="module" src="audio.js"></script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
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
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

// Intercept WebAudio to capture the output of playSound.
await page.addInitScript(() => {
  window.__samples = [];
  const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 32768;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      sp.addEventListener("audioprocess", (e) => {
        const data = e.inputBuffer.getChannelData(0);
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
});

await page.goto(`http://localhost:${port}/`);
// Render a reference snare buffer directly via drums.js, applying the same
// normalization and base amplitude as audio.js.
const ref = await page.evaluate(async () => {
  const sr = 44100;
  const sec = 0.25; // snare length in audio.js
  const frames = Math.floor(sr * sec);
  const factory = window.__drumsFactory; if (!factory) throw new Error('no drumsFactory');
  const m = await factory();
  const ptr = m._malloc(frames * 4);
  m.ccall('render_snare', null, ['number','number','number'], [ptr, sr, frames]);
  const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
  m._free(ptr);
  let peak = 0; for (let i = 0; i < data.length; i++) { const a = Math.abs(data[i]); if (a > peak) peak = a; }
  if (peak > 0) { const inv = 1/peak; for (let i = 0; i < data.length; i++) data[i] *= inv; }
  const amp = 0.2; // snare base amp in audio.js
  for (let i = 0; i < data.length; i++) data[i] *= amp;
  return Array.from(data);
});

// Now play via the high-level API and capture its buffer.
await page.evaluate(() => { window.__samples = []; window.__captureSamples = true; });
await page.evaluate(() => window.playSound('snare', 1.0));
await page.waitForFunction(() => window.__done === true, {}, { timeout: 10000 });
const out = await page.evaluate(() => window.__samples.slice(0, 44100));
// Compare by correlation; require near-identical shape.
const corr = (x, y) => {
  const n = Math.min(x.length, y.length);
  let sx = 0, sy = 0, sxx = 0, syy = 0, sxy = 0;
  for (let i = 0; i < n; i++) { const a = x[i], b = y[i]; sx += a; sy += b; sxx += a*a; syy += b*b; sxy += a*b; }
  const num = n * sxy - sx * sy;
  const den = Math.sqrt((n*sxx - sx*sx) * (n*syy - sy*sy)) || 1;
  return Math.abs(num / den);
};

const c = corr(ref, out);
if (c < 0.98) {
  throw new Error(`miniaudio JS render mismatch (snare): corr=${c.toFixed(4)}`);
}
console.log('drums.js consistency verified (snare)', { corr: c.toFixed(4) });

// Repeat for kick to cover a tonal/decaying instrument.
await (async () => {
  const sr = 44100;
  const sec = 0.5; // kick length in audio.js
  const frames = Math.floor(sr * sec);
  const kickRef = await page.evaluate(async ({ frames, sr }) => {
    const factory = window.__drumsFactory;
    if (typeof factory !== 'function') throw new Error('no drumsFactory');
    const m = await factory();
    const ptr = m._malloc(frames * 4);
    m.ccall('render_kick', null, ['number','number','number'], [ptr, sr, frames]);
    const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
    m._free(ptr);
    // Normalize + base amp (1.0 for kick)
    let peak = 0; for (let i = 0; i < data.length; i++) { const a = Math.abs(data[i]); if (a > peak) peak = a; }
    if (peak > 0) { const inv = 1/peak; for (let i = 0; i < data.length; i++) data[i] *= inv; }
    return Array.from(data);
  }, { frames, sr });
  await page.evaluate(() => { window.__samples = []; window.__done = false; window.__captureSamples = true; });
  await page.evaluate(() => window.playSound('kick', 1.0));
  await page.waitForFunction(() => window.__done === true, {}, { timeout: 10000 });
  const kickOut = await page.evaluate(() => window.__samples.slice(0, 44100));
  const ck = corr(kickRef, kickOut);
  if (ck < 0.98) throw new Error(`miniaudio JS render mismatch (kick): corr=${ck.toFixed(4)}`);
  console.log('drums.js consistency verified (kick)', { corr: ck.toFixed(4) });
})();

await browser.close();
server.close();
