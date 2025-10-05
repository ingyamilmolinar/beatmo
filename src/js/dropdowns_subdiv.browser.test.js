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

const port = 8370 + Math.floor(Math.random() * 1000);
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
await page.waitForFunction(() => typeof subdivBtnRect === 'function');

const r = await page.evaluate(() => subdivBtnRect());
if (!r) throw new Error('subdiv button rect missing');
const cx = Math.floor(r.x + r.w/2);
const cy = Math.floor(r.y + r.h/2);
await page.mouse.click(cx, cy);
await page.waitForFunction(() => Array.isArray(subdivMenuItemRects()) && subdivMenuItemRects().length > 0);

// Pick value 8 if present
const items = await page.evaluate(() => subdivMenuItemRects());
let target = items.find(it => it.val === 8) || items[0];
const tx = Math.floor(target.x + target.w/2);
const ty = Math.floor(target.y + target.h/2);
await page.mouse.click(tx, ty);
await page.waitForTimeout(80);

await page.waitForFunction(() => typeof gridSubdiv === 'function' && typeof timelineUnitsPerBeat === 'function');
const vals = await page.evaluate(() => ({ grid: gridSubdiv(), units: timelineUnitsPerBeat() }));
if (target.val === 8) {
  if (vals.grid !== 8 || vals.units !== 8) throw new Error(`subdiv not applied: ${JSON.stringify(vals)}`);
} else {
  if (vals.units !== target.val) throw new Error(`subdiv units mismatch: got ${vals.units} want ${target.val}`);
}

await browser.close();
server.close();
console.log('subdiv dropdown selection applied');

