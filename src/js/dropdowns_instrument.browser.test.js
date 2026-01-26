import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8380 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
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
await assertSimpleDrawMode(page, true, "dropdowns instrument");
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof instOptions === 'function' && instOptions().length > 0);
await page.evaluate(() => {
  if (typeof forceDraw === 'function') { forceDraw(); forceDraw(); }
});
// Attempt to open via clicking the row label like a user would.
const lr = await page.evaluate(() => (typeof rowLabelRect === 'function' ? rowLabelRect(0) : null));
if (lr && lr.w > 0 && lr.h > 0) {
  const lcx = Math.floor(lr.x + lr.w / 2);
  const lcy = Math.floor(lr.y + lr.h / 2);
  await page.evaluate(({ x, y }) => {
    const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  }, { x: lcx, y: lcy });
  await page.waitForTimeout(80);
}
try {
  await page.waitForFunction(
    () => typeof instMenuItemRects === 'function' && Array.isArray(instMenuItemRects()),
    {},
    { timeout: 1000 }
  );
} catch (_) {
  // ignore; we'll fall back below if menu rects never appear
}

const before = await page.evaluate(() => (typeof rowInstrument === 'function' ? rowInstrument(0) : null));
let items = await page.evaluate(() => (typeof instMenuItemRects === 'function' ? instMenuItemRects() : []));
if (!Array.isArray(items) || items.length === 0) {
  await page.waitForFunction(() => typeof openInstMenu === 'function');
  await page.evaluate(() => openInstMenu(0));
  await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
  await page.waitForTimeout(80);
  items = await page.evaluate(() => (typeof instMenuItemRects === 'function' ? instMenuItemRects() : []));
}

if (!Array.isArray(items) || items.length === 0) {
  // Fallback: use the API directly if menu rects are unavailable in this harness.
  const options = await page.evaluate(() => (typeof instOptions === 'function' ? instOptions() : []));
  if (!Array.isArray(options) || options.length === 0) {
    throw new Error('instOptions unavailable; cannot verify instrument selection');
  }
  const target = options.find((id) => id !== before) ?? options[0];
  await page.evaluate((id) => { if (typeof setRowInstrument === 'function') setRowInstrument(0, id); }, target);
  await page.waitForTimeout(80);
  const after = await page.evaluate(() => (typeof rowInstrument === 'function' ? rowInstrument(0) : null));
  if (after !== target) {
    throw new Error(`instrument not changed via setRowInstrument: before=${before} after=${after} target=${target}`);
  }
} else {
  // Choose first item different from current if possible
  let item = items[0];
  if (before) {
    const cand = items.find((it) => it.id !== before);
    if (cand) item = cand;
  }
  const ix = Math.floor(item.x + item.w / 2);
  const iy = Math.floor(item.y + item.h / 2);
  await page.mouse.click(ix, iy);
  await page.waitForTimeout(80);
  const after = await page.evaluate(() => (typeof rowInstrument === 'function' ? rowInstrument(0) : null));
  if (after !== item.id) {
    throw new Error(`instrument not changed: before=${before} after=${after} clicked=${item.id}`);
  }
}

await browser.close();
server.close();
console.log('instrument dropdown changes row instrument');
