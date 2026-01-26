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

// Build main WASM so we exercise real UI layout.
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

// Serve src/js directory
const port = 8350 + Math.floor(Math.random() * 1000);
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
await page.waitForFunction(() => typeof perfStats === 'function');
await assertSimpleDrawMode(page, false, "ui presence");

// Warm caches and ensure a few frames render.
await page.evaluate(() => {
  forceDraw?.();
});
const ok1 = await page.evaluate(() => uiLayoutOk?.());
await page.evaluate(() => {
  // Simulate a brief pan to exercise cached row shifts.
  for (let i = 0; i < 10; i++) {
    panBy?.(-2, 0);
  }
  forceDraw?.();
});
const ok2 = await page.evaluate(() => uiLayoutOk?.());
await page.evaluate(() => {
  resetPerfStats?.();
  forceDraw?.();
});
const stats = await page.evaluate(() => perfStats?.());

await browser.close();
server.close();

if (!ok1 || !ok2) {
  throw new Error("uiLayoutOk returned false: drum view not drawable (web)");
}
if (!stats || stats.frames <= 0) {
  throw new Error("perfStats.frames missing; WASM UI did not render any frames");
}
