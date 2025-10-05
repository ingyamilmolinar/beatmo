import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Build the main WASM so JS exports are present.
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

// Serve the whole src/js directory so index.html can load main.wasm and audio.js.
const port = 8330 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
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
await page.goto(`http://localhost:${port}/`);
// Wait for wasm runtime to be ready (either exported hook or a while for game to start).
await page.waitForFunction(() => typeof startPlay === 'function');
// Enable simplified draw mode for perf measurement, then build graph.
await page.evaluate(() => { if (typeof setSimpleDraw === 'function') setSimpleDraw(true); });
// Build stress graph, set BPM, start playback.
await page.evaluate(() => { buildPerfRect(4, 1); setBPM(200); startPlay(); });
// Let it run for a few seconds to accumulate perf stats including Draw times.
await page.waitForTimeout(3000);
const stats = await page.evaluate(() => perfStats());
console.log('perf.e2e.browser:', stats);

await browser.close();
server.close();

if (stats.frames <= 0) {
  throw new Error('no frames recorded in e2e WASM');
}
