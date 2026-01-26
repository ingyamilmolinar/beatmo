import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build main WASM
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const port = 8360 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
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
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "rows content");

// Setup: build multi-row content and warm caches.
await page.evaluate(() => {
  // Ensure Go exports initialized fully.
  forceDraw?.();
  forceDraw?.();
  buildPerfRect(6, 1);
  forceDraw?.();
  forceDraw?.();
});

// First assertion: after warmup, all visible rows should report content.
let vis = await page.evaluate(() => visibleRows?.());
let rows = await page.evaluate(() => drumRowCount?.());
let have = await page.evaluate(() => rowsContentVisibleCount?.());
let expect = Math.min(vis ?? 0, rows ?? 0);
if ((have ?? 0) < expect) {
  throw new Error(`rows content after warmup: have=${have} expect>=${expect}`);
}

// Trigger shift path: pan a bit and assert again.
await page.evaluate(() => {
  for (let i = 0; i < 20; i++) {
    panBy?.(-2, 0);
  }
  forceDraw?.();
});
vis = await page.evaluate(() => visibleRows?.());
rows = await page.evaluate(() => drumRowCount?.());
have = await page.evaluate(() => rowsContentVisibleCount?.());
expect = Math.min(vis ?? 0, rows ?? 0);
if ((have ?? 0) < expect) {
  throw new Error(`rows content after pan: have=${have} expect>=${expect}`);
}

await browser.close();
server.close();
