import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const port = 8300 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const p = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, p);
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith(".wasm") ? "application/wasm" : filePath.endsWith(".html") ? "text/html" : "application/javascript";
    res.writeHead(200, {"Content-Type": ct});
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
// Wait for wasm and helpers
await page.waitForFunction(() => typeof timelineRect === 'function');

// Read timeline rect and compute a click at ~75%
const rect = await page.evaluate(() => timelineRect());
const before = await page.evaluate(() => drumOffset());
const x = rect.x + Math.floor(rect.w * 0.75);
const y = rect.y + Math.floor(rect.h * 0.5);
await page.mouse.click(x, y);
// Give the game a tick to process input
await page.waitForTimeout(50);
const after = await page.evaluate(() => drumOffset());

if (after === before) {
  throw new Error(`timeline click did not change offset: ${before}`);
}

// Verify centering math: desired = center - length/2 clamped
const tb = await page.evaluate(() => timelineBeats());
const len = await page.evaluate(() => drumLength());
const center = Math.floor(0.75 * tb);
let desired = center - Math.floor(len/2);
const maxOff = Math.max(0, tb - len);
desired = Math.max(0, Math.min(maxOff, desired));
if (Math.abs(after - desired) > 1) {
  throw new Error(`offset not centered: got=${after} want~=${desired}`);
}

await browser.close();
server.close();
console.log("timeline click centering verified");

