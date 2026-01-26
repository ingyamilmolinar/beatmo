/**
 * Node Popup Test (Playtest Harness)
 *
 * NOTE: This test uses the PLAYTEST harness (play_ui.wasm) which does NOT run
 * ebiten.RunGame(). Canvas events do NOT reach Go input handlers in this mode.
 * This test falls back to JS bridges (nodeActionAt, openNodeMenu) when canvas
 * events fail, which masks real input bugs.
 *
 * For authoritative real input testing, see:
 * - node_click_real.browser.test.js (verifies click->popup works)
 * - node_popup_actions_real.browser.test.js (verifies popup buttons work)
 *
 * This test verifies that JS exports (nodeParams, nodeMenuRect, etc.) work
 * correctly and that the playtest harness renders properly.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8150 + Math.floor(Math.random() * 1000);
// Build lightweight UI WASM that exposes JS helpers.
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => { try { console.log('[SRV]', req.url); } catch(_) {}
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
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

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => !!window.ensureDefaultPath);
await assertSimpleDrawMode(page, true, "popup node");
await page.evaluate(() => window.ensureDefaultPath());

// Open node menu for (0,0)
await page.waitForFunction(() => !!window.openNodeMenu);
await page.evaluate(() => window.openNodeMenu(0, 0));

// Read initial params
const before = await page.evaluate(() => window.nodeParams(0,0));
if (!before) throw new Error('nodeParams returned null');

// Click vol+ a few times to ensure we observe change even with coarse frames
const volp = await page.evaluate(() => window.nodeMenuRect("vol+"));
const c1 = { x: volp.x + Math.floor(volp.w/2), y: volp.y + Math.floor(volp.h/2) };
// Ensure the target lies within the canvas bounds
const cbox = await page.evaluate(() => { const c = document.querySelector('canvas');
  const r = c.getBoundingClientRect();
  return { left: Math.floor(r.left), top: Math.floor(r.top), right: Math.floor(r.right), bottom: Math.floor(r.bottom) };
});
if (c1.x < cbox.left || c1.x > cbox.right || c1.y < cbox.top || c1.y > cbox.bottom) { throw new Error(`vol+ center outside canvas: ${JSON.stringify({c1, cbox})}`);
}
for (let i = 0; i < 3; i++) { await page.evaluate(({x, y}) => { const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  }, c1);
  await page.waitForTimeout(60);
  await page.evaluate(({x, y}) => { const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  }, c1);
  await page.waitForTimeout(60);
}

// Verify volume increased
let after = await page.evaluate(() => window.nodeParams(0,0));
if (!after || !(after.volume > (before.volume ?? 1.0))) {
  // Note: The playtest harness doesn't run Ebiten's full game loop, so mouse events
  // don't reach the Go code. Use the JS bridge as fallback.
  await page.evaluate(() => window.nodeActionAt && window.nodeActionAt(0,0,'vol+'));
  await page.waitForTimeout(20);
  after = await page.evaluate(() => window.nodeParams(0,0));
  if (!after || !(after.volume > (before.volume ?? 1.0))) {
    throw new Error(`volume not increased: before=${before && before.volume} after=${after && after.volume}`);
  }
}

// Toggle AUDIBLE/SILENT and verify type flips
const aud = await page.evaluate(() => window.nodeMenuRect("aud"));
const c2 = { x: aud.x + Math.floor(aud.w/2), y: aud.y + Math.floor(aud.h/2) };
await page.evaluate(({x, y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, c2);
await page.waitForTimeout(50);
await page.evaluate(({x, y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, c2);
await page.waitForTimeout(50);
let afterType = await page.evaluate(() => window.nodeParams(0,0).type);
if (!(afterType === 0 || afterType === 2)) {
  // Note: The playtest harness doesn't run Ebiten's full game loop, so mouse events
  // don't reach the Go code. Use the JS bridge as fallback.
  await page.evaluate(() => window.nodeActionAt && window.nodeActionAt(0,0,'aud'));
  await page.waitForTimeout(20);
  afterType = await page.evaluate(() => window.nodeParams(0,0).type);
  if (!(afterType === 0 || afterType === 2)) {
    throw new Error(`unexpected node type after toggle: ${afterType}`);
  }
}

await browser.close();
server.close();
