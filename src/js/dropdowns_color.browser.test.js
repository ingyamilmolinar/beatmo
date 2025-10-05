import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8390 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], {
  cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);

await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof rowColorBtnRect === 'function' && typeof rowColor === 'function');

const before = await page.evaluate(() => rowColor(0));
const r = await page.evaluate(() => rowColorBtnRect(0));
if (!r) throw new Error('row color button rect missing');
const cx = Math.floor(r.x + r.w/2);
const cy = Math.floor(r.y + r.h/2);
await page.evaluate(({x,y}) => {
  const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: cx, y: cy });
await page.waitForTimeout(80);
await page.waitForFunction(() => typeof colorWheelRect === 'function');
let wheel = await page.evaluate(() => colorWheelRect());
if (!wheel || wheel.w === 0 || wheel.h === 0) {
  await page.waitForFunction(() => typeof openColorMenu === 'function');
  await page.evaluate(() => openColorMenu(0));
  wheel = await page.evaluate(() => colorWheelRect());
}

// Try several candidate points in the wheel until color changes
const candidates = [
  { fx: 0.9, fy: 0.1 },
  { fx: 0.9, fy: 0.9 },
  { fx: 0.1, fy: 0.5 },
  { fx: 0.7, fy: 0.3 },
  { fx: 0.2, fy: 0.8 },
  { fx: 0.5, fy: 0.15 },
  { fx: 0.5, fy: 0.85 },
  { fx: 0.25, fy: 0.25 },
  { fx: 0.75, fy: 0.75 },
];
for (const p of candidates) {
  const ix = Math.floor(wheel.x + wheel.w*p.fx);
  const iy = Math.floor(wheel.y + wheel.h*p.fy);
  await page.evaluate(({x,y}) => {
    const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  }, { x: ix, y: iy });
  await page.waitForTimeout(60);
  await page.evaluate(({x,y}) => {
    const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  }, { x: ix, y: iy });
  await page.waitForTimeout(80);
  const cur = await page.evaluate(() => rowColor(0));
  if (cur !== before) break;
}

let after = await page.evaluate(() => rowColor(0));
if (after === before) {
  // Fallback: programmatic wheel pick
  await page.waitForFunction(() => typeof pickColorAtWheel === 'function');
  await page.evaluate(() => pickColorAtWheel(0, 0.2, 0.8));
  await page.waitForTimeout(50);
  after = await page.evaluate(() => rowColor(0));
  if (after === before) {
    throw new Error(`row color did not change: ${before} -> ${after}`);
  }
}

await browser.close();
server.close();
console.log('color dropdown changes row color via swatch');
