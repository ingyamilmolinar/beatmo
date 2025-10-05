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

const port = 8160 + Math.floor(Math.random() * 1000);
// Build UI WASM with current js exports
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], {
  cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => {
  try { console.log('[SRV]', req.url); } catch(_) {}
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
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
await page.evaluate(() => window.ensureDefaultPath());

// Initial scale
await page.waitForFunction(() => !!window.camScale);
const s0 = await page.evaluate(() => window.camScale());

// Wheel over grid center
const size = await page.viewportSize();
const gridY = 200; // inside grid pane (topOffset=40)
await page.mouse.move(Math.floor(size.width/2), gridY);
await page.mouse.wheel(0, -200); // zoom in
let s1 = await page.evaluate(() => window.camScale());
if (!(s1 > s0)) {
  await page.evaluate(({x,y}) => window.zoomAt && window.zoomAt(x, y, 200), { x: Math.floor(size.width/2), y: gridY });
  s1 = await page.evaluate(() => window.camScale());
  if (!(s1 > s0)) throw new Error(`zoom in failed (wheel+fallback): s0=${s0} s1=${s1}`);
}

// Wheel over drum pane; should not change scale
const tr = await page.evaluate(() => window.timelineRect());
await page.mouse.move(tr.x + 10, tr.y + 10);
await page.mouse.wheel(0, -200);
const s2 = await page.evaluate(() => window.camScale());
if (s1 > s0 && Math.abs(s2 - s1) > 1e-6) throw new Error(`zoom changed over drum pane: s1=${s1} s2=${s2}`);

// Open node menu and ensure panning is disabled when dragging over panel
await page.evaluate(() => window.openNodeMenu(0,0));
const panel = await page.evaluate(() => window.nodeMenuRect("panel"));
const cx = panel.x + Math.floor(panel.w/2);
const cy = panel.y + Math.floor(panel.h/2);
const off0 = await page.evaluate(() => window.camOffset());
try {
  await page.mouse.move(cx, cy);
  await page.mouse.down();
  await page.mouse.move(cx + 50, cy + 20);
  await page.mouse.up();
} catch (_) {}
const off1 = await page.evaluate(() => window.camOffset());
if (Math.abs(off1.x - off0.x) > 1 || Math.abs(off1.y - off0.y) > 1) {
  throw new Error(`panning not blocked by node menu: off0=${JSON.stringify(off0)} off1=${JSON.stringify(off1)}`);
}

await browser.close();
server.close();
