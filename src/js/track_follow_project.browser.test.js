import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { spawnSync } from "child_process";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const port = 8380 + Math.floor(Math.random() * 1000);
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
await page.waitForFunction(() => typeof importJSON === "function" && typeof startPlay === "function" && typeof drumOffset === "function" && typeof nextBeatIdxs === "function");
await assertSimpleDrawMode(page, false, "track follow project");

const project = fs.readFileSync(path.resolve(__dirname, "../..", "tunkul.json"), "utf8");
await page.evaluate((txt) => {
  setFollow?.(true);
  importJSON(txt);
  forceDraw?.();
}, project);
await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay?.());

let offsetIncreased = false;
let centeredOK = false;
const length = await page.evaluate(() => drumLength?.() ?? 0);
const half = Math.floor(length / 2);
const tolerance = Math.max(8, Math.floor(length / 6));

for (let i = 0; i < 60; i++) {
  const sample = await page.evaluate(() => ({
    off: drumOffset(),
    next: (nextBeatIdxs?.() ?? [0])[0] || 0
  }));
  if (sample.off > 0) offsetIncreased = true;
  const idx = sample.next - 1 - sample.off;
  if (sample.off > 0 && Math.abs(idx - half) <= tolerance) {
    centeredOK = true;
  }
  await page.waitForTimeout(250);
}

await assertNoSchedulerMismatches(page, "track follow project: scheduler mismatches");
await browser.close();
server.close();

if (!offsetIncreased) {
  throw new Error("drumOffset never advanced after loading tunkul.json");
}
if (!centeredOK) {
  throw new Error("highlight did not remain near the center once tracking engaged");
}
