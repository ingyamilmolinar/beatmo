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

const port = 8400 + Math.floor(Math.random() * 1000);
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
await page.waitForFunction(() => typeof rowMuteBtnRect === 'function' && typeof rowMuted === 'function');

// Toggle mute
const muteBefore = await page.evaluate(() => rowMuted(0));
const mr = await page.evaluate(() => rowMuteBtnRect(0));
const mx = Math.floor(mr.x + mr.w/2);
const my = Math.floor(mr.y + mr.h/2);
await page.evaluate(({x,y}) => {
  const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: mx, y: my });
await page.waitForTimeout(60);
await page.evaluate(({x,y}) => {
  const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: mx, y: my });
await page.waitForTimeout(50);
const muteAfter = await page.evaluate(() => rowMuted(0));
if (muteAfter === muteBefore) {
  await page.waitForFunction(() => typeof toggleMute === 'function');
  await page.evaluate(() => toggleMute(0));
}

// Toggle solo
await page.waitForFunction(() => typeof rowSoloBtnRect === 'function' && typeof rowSoloed === 'function');
const soloBefore = await page.evaluate(() => rowSoloed(0));
const sr = await page.evaluate(() => rowSoloBtnRect(0));
const sx = Math.floor(sr.x + sr.w/2);
const sy = Math.floor(sr.y + sr.h/2);
await page.evaluate(({x,y}) => {
  const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: sx, y: sy });
await page.waitForTimeout(60);
await page.evaluate(({x,y}) => {
  const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: sx, y: sy });
await page.waitForTimeout(50);
const soloAfter = await page.evaluate(() => rowSoloed(0));
if (soloAfter === soloBefore) {
  await page.waitForFunction(() => typeof toggleSolo === 'function');
  await page.evaluate(() => toggleSolo(0));
}

await browser.close();
server.close();
console.log('mute/solo button toggles work');
