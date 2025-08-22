import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright and browser dependencies are installed.
spawnSync("npx", ["playwright", "install", "chromium"], {
  cwd: jsDir,
  stdio: "inherit",
});

const port = 8130 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file);
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await page.evaluate(() => startPlay());
await page.waitForTimeout(500);
await page.evaluate(async () => {
  for (let i = 0; i < 50; i++) {
    incrementBPM();
    await new Promise((r) => setTimeout(r, 5));
  }
});
const after = await page.evaluate(() => currentBeat());
await browser.close();
server.close();
if (after < 0.1) {
  throw new Error(`beat did not advance, got ${after}`);
}
