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

const port = 8210 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
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
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await page.evaluate(() => ensureDefaultPath());

await page.waitForFunction(() => typeof nodeRect === 'function');
const r0 = await page.evaluate(() => nodeRect(0,0));
const cx = Math.floor(r0.x + r0.w/2);
const cy = Math.floor(r0.y + r0.h/2);

// Zoom in at node center; anchored zoom should keep node under cursor
await page.mouse.move(cx, cy);
await page.mouse.wheel(0, -200);
await page.waitForTimeout(50);

const r1 = await page.evaluate(() => nodeRect(0,0));
const nx = Math.floor(r1.x + r1.w/2);
const ny = Math.floor(r1.y + r1.h/2);
const tol = 2;
if (Math.abs(nx - cx) > tol || Math.abs(ny - cy) > tol) {
  throw new Error(`anchored zoom failed: node moved to (${nx},${ny}) want~=(${cx},${cy})`);
}

await browser.close();
server.close();
console.log('anchored zoom verified');

