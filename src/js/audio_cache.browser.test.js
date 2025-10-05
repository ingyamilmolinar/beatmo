import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const port = 8370 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/cache.html" : req.url;
  if (req.url === "/" || req.url === "/cache.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module">
  import './audio.js';
</script>
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
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.playSound === 'function' && typeof window.playSoundParams === 'function');

await page.evaluate(async () => {
  window.__audioMetrics = { renders: {}, cacheHits: {} };
  window.__captureSamples = false;
  await window.playSound('snare', 0.8);
  await new Promise((r) => setTimeout(r, 50));
  await window.playSound('snare', 0.5);
  await new Promise((r) => setTimeout(r, 50));
  await window.playSoundParams('snare', 0.6, 2, 1.0);
  await new Promise((r) => setTimeout(r, 50));
});

const metrics = await page.evaluate(() => window.__audioMetrics);
await browser.close();
server.close();

if (!metrics || typeof metrics !== 'object') {
  throw new Error('missing audio metrics');
}
const renders = metrics.renders && metrics.renders.snare;
const hits = metrics.cacheHits && metrics.cacheHits.snare;
if (renders !== 1) {
  throw new Error(`expected single render, got ${renders}`);
}
if (!hits || hits < 2) {
  throw new Error(`expected cache hits >= 2, got ${hits}`);
}

console.log('audio cache verified', metrics);
