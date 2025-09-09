import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Build the UI playtest wasm that keeps Go alive and exposes export/import.
import { spawnSync } from "child_process";
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

// Serve the wasm app
const port = 8200 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const p = req.url === "/" ? "/play_ui.html" : req.url;
  const filePath = path.join(jsDir, p);
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith(".wasm") ? "application/wasm" : filePath.endsWith(".html") ? "text/html" : "application/javascript";
    res.writeHead(200, {"Content-Type": ct});
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch();
const page = await browser.newPage();

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await page.evaluate(() => startPlay());
await page.waitForFunction(() => typeof exportJSON === 'function' && typeof importJSON === 'function');

// Intercept download and import pickers by stubbing helpers.
await page.evaluate(() => {
  window.__exportText = "";
  window.downloadJSON = (name, text) => { window.__exportText = text; };
});

// Export current state to JSON.
const initialJSON = await page.evaluate(() => exportJSON());
if (!initialJSON || initialJSON.indexOf('"version"') < 0) {
  throw new Error("exportJSON returned empty or invalid JSON");
}

// Import the same JSON and export again to verify the path.
await page.evaluate((txt) => importJSON(txt), initialJSON);
const roundtrip = await page.evaluate(() => exportJSON());
if (!roundtrip || roundtrip.indexOf('"version"') < 0) {
  throw new Error("roundtrip exportJSON failed");
}

await browser.close();
server.close();
console.log("import/export browser flow verified");
