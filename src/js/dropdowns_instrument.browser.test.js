import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");
}

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
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

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

const before = await page.evaluate(() => (typeof rowInstrument === 'function' ? rowInstrument(0) : null));

// Open the instrument menu via API. The playtest harness does not call
// ebiten.RunGame(), so Ebiten's input system is uninitialised and mouse
// events are invisible to the game. All interaction must go through JS
// exports.
await page.waitForFunction(() => typeof openInstMenu === 'function');
await page.evaluate(() => openInstMenu(0));
await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
await page.waitForTimeout(80);

let items = await page.evaluate(() => (typeof instMenuItemRects === 'function' ? instMenuItemRects() : []));
if (!Array.isArray(items) || items.length === 0) {
  throw new Error('instMenuItemRects returned no items after openInstMenu');
}

// Find the first item different from the current instrument
let targetIdx = 0;
if (before) {
  const idx = items.findIndex((it) => it.id !== before);
  if (idx >= 0) targetIdx = idx;
}
const targetId = items[targetIdx].id;

// Select the instrument via programmatic API (triggers OnClick on the
// instrument button, which calls SetInstrument and closes the menu).
const hasSelectItem = await page.evaluate(() => typeof instMenuSelectItem === 'function');
if (hasSelectItem) {
  await page.evaluate((idx) => instMenuSelectItem(idx), targetIdx);
} else {
  // Fallback for older WASM builds without instMenuSelectItem
  await page.evaluate((id) => { if (typeof setRowInstrument === 'function') setRowInstrument(0, id); }, targetId);
}
await page.waitForTimeout(80);

const after = await page.evaluate(() => (typeof rowInstrument === 'function' ? rowInstrument(0) : null));
if (after !== targetId) {
  throw new Error(`instrument not changed: before=${before} after=${after} expected=${targetId}`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "dropdowns_instrument");
await browser.close();
server.close();
console.log('instrument dropdown changes row instrument');
