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

const port = 8370 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");
const loopJSON = fs.readFileSync(path.join(goDir, "internal/ui/testdata/future_cache_loop.json"), "utf8");

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

await page.waitForFunction(() => typeof importJSON === 'function');
await assertSimpleDrawMode(page, true, "dropdowns subdiv");
await page.evaluate((json) => importJSON(json), loopJSON);
await page.waitForFunction(() => typeof forceDraw === 'function');
await page.evaluate(() => forceDraw());
await page.waitForFunction(() => typeof subdivBtnRect === 'function');
await page.waitForFunction(() => typeof stopPlay === 'function');
await page.evaluate(() => stopPlay?.());

const r = await page.evaluate(() => subdivBtnRect());
if (!r) throw new Error('subdiv button rect missing');
const cx = Math.floor(r.x + r.w/2);
const cy = Math.floor(r.y + r.h/2);
await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: cx, y: cy });
await page.waitForTimeout(80);
let open = await page.evaluate(() => Array.isArray(subdivMenuItemRects?.()) && subdivMenuItemRects().length > 0);
if (!open) { await page.waitForFunction(() => typeof openSubdivMenu === 'function');
  await page.evaluate(() => openSubdivMenu());
  await page.waitForTimeout(40);
}
await page.waitForFunction(() => Array.isArray(subdivMenuItemRects()) && subdivMenuItemRects().length > 0);

// Pick value 8 if present
const items = await page.evaluate(() => subdivMenuItemRects());
await page.waitForFunction(() => typeof gridSubdiv === 'function' && typeof timelineUnitsPerBeat === 'function');
const initial = await page.evaluate(() => ({ grid: gridSubdiv(), units: timelineUnitsPerBeat() }));
if (initial.grid <= 0 || initial.units <= 0) { throw new Error(`unexpected subdiv state: ${JSON.stringify(initial)}`);
}
const preferred = [16, 32, 8];
let applied = null;
for (const candidate of preferred) { const choice = items.find((it) => it.val === candidate);
  if (!choice || candidate === initial.grid) { continue;
  }
  const tx = Math.floor(choice.x + choice.w / 2);
  const ty = Math.floor(choice.y + choice.h / 2);
  await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  }, { x: tx, y: ty });
  await page.waitForTimeout(50);
  await page.waitForFunction(() => typeof applySubdivValue === 'function');
  await page.evaluate((val) => applySubdivValue(val), candidate);
  await page.waitForTimeout(80);
  const vals = await page.evaluate(() => ({ grid: gridSubdiv(), units: timelineUnitsPerBeat() }));
  if (vals.grid === candidate && vals.units === candidate) { applied = { candidate, vals };
    break;
  }
}

if (!applied) { throw new Error(`subdiv change failed: initial=${JSON.stringify(initial)}`);
}

await browser.close();
server.close();
console.log('subdiv dropdown selection applied');
