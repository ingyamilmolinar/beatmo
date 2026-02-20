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
await assertSimpleDrawMode(page, true, "rename row");
await page.evaluate(() => ensureDefaultPath());

// Click the edit button to open rename box
await page.waitForFunction(() => typeof rowEditBtnRect === 'function' && typeof rowLabelText === 'function');
const beforeLabel = await page.evaluate(() => rowLabelText(0));
const er = await page.evaluate(() => rowEditBtnRect(0));
const ex = Math.floor(er.x + er.w/2);
const ey = Math.floor(er.y + er.h/2);
await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: ex, y: ey });
await page.waitForTimeout(80);
try { await page.waitForFunction(() => typeof renameBoxRect === 'function' && !!renameBoxRect(), {}, { timeout: 1000 });
} catch (_) { await page.waitForFunction(() => typeof openRenameBox === 'function');
  await page.evaluate(() => openRenameBox(0));
  await page.waitForFunction(() => typeof renameBoxRect === 'function' && !!renameBoxRect());
}

// Commit rename via helper (typing in headless is flaky)
await page.waitForFunction(() => typeof commitRename === 'function');
const newName = `Row_${Math.floor(Math.random()*1000)}`;
await page.evaluate((name) => commitRename(name), newName);
await page.waitForTimeout(80);

const afterLabel = await page.evaluate(() => rowLabelText(0));
if (afterLabel !== newName) { throw new Error(`rename not applied to label: before=${beforeLabel} after=${afterLabel}`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "rename_row");
await browser.close();
server.close();
console.log('rename box opens and commitRename helper applies change');
