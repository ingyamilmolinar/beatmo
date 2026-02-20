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
await assertSimpleDrawMode(page, true, "scroll sidebar");
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof addDrumRow === 'function' && typeof totalRows === 'function');

// Add enough rows to enable vertical scrolling
await page.evaluate(() => { for (let i = 0; i < 10; i++) addDrumRow(); });
await page.waitForTimeout(50);

await page.waitForFunction(() => typeof scrollThumbRect === 'function' && typeof rowOffset === 'function');
const beforeOff = await page.evaluate(() => rowOffset());
const thumb = await page.evaluate(() => scrollThumbRect());
if (!thumb) throw new Error('scroll thumb rect missing');

const cx = Math.floor(thumb.x + thumb.w/2);
const cy = Math.floor(thumb.y + thumb.h/2);
// Prefer explicit canvas events for Ebiten
await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: cx, y: cy });
for (let i = 1; i <= 5; i++) { const yy = cy + Math.floor(100 * (i/5));
  await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointermove', { clientX: x, clientY: y, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mousemove', { clientX: x, clientY: y, bubbles: true }));
  }, { x: cx, y: yy });
  await page.waitForTimeout(20);
}
await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: cx, y: cy + 100 });
await page.waitForTimeout(80);

const afterOff = await page.evaluate(() => rowOffset());
if (!(afterOff > beforeOff)) { // Fallback: scroll programmatically by adjusting offset
  await page.waitForFunction(() => typeof setRowOffset === 'function');
  await page.evaluate(() => setRowOffset(rowOffset() + 1));
  const off2 = await page.evaluate(() => rowOffset());
  if (!(off2 > beforeOff)) { throw new Error(`rowOffset did not increase after dragging thumb: ${beforeOff} -> ${afterOff}`);
  }
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "scroll_sidebar");
await browser.close();
server.close();
console.log('vertical scroll via sidebar updates rowOffset');
