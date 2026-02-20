import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  [
    "build",
    "-ldflags",
    "-X main.defaultLog=INFO",
    "-o",
    path.join(jsDir, "main.wasm"),
    "./cmd/...",
  ],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

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
await new Promise((resolve) => server.listen(0, resolve));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await assertSimpleDrawMode(page, false, "pan stress");
await clearSchedulerMismatches(page);

// Verify single panBy produces correct pixel shift (from pan_camera suite)
await page.waitForFunction(() => typeof camOffset === "function");
{
  const c0 = await page.evaluate(() => camOffset());
  const dx = 42, dy = 18;
  await page.evaluate(({dx,dy}) => panBy?.(dx, dy), { dx, dy });
  await page.waitForTimeout(50);
  const c1 = await page.evaluate(() => camOffset());
  const tol = 3;
  if (Math.abs((c1.x - c0.x) - dx) > tol || Math.abs((c1.y - c0.y) - dy) > tol) {
    throw new Error(`panBy pixel-shift mismatch: moved=(${c1.x-c0.x},${c1.y-c0.y}) want~=(${dx},${dy})`);
  }
  console.log("panBy pixel-shift verified");
  // Reset camera for stress test
  await page.evaluate(({dx,dy}) => panBy?.(-dx, -dy), { dx, dy });
}

await page.evaluate(() => {
  resetAudioScheduleMetrics?.();
  forceDraw?.();
  forceDraw?.();
  startPlay?.();
});

// warm-up pan
await page.evaluate(() => {
  const steps = 200;
  const dx = -2;
  for (let i = 0; i < steps; i++) {
    panBy?.(dx, 0);
  }
  forceDraw?.();
});

await page.evaluate(() => resetPerfStats?.());
const before = await page.evaluate(() => perfStats?.());

for (let iter = 0; iter < 3; iter++) { await page.evaluate(() => { const steps = 600;
    const dx = -1;
    for (let i = 0; i < steps; i++) { panBy?.(dx, 0);
      if (i % 120 === 0) { forceDraw?.();
      }
    }
    forceDraw?.();
  });
}

await page.waitForTimeout(2500);
const after = await page.evaluate(() => perfStats?.());

await assertNoSchedulerMismatches(page, "pan stress: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "pan_stress");
await browser.close();
server.close();

if (!after) { throw new Error("perfStats unavailable");
}

const fps = after.fpsAvg;
const drawAvg = after.drawAvgMS;
const jobs = Number(process.env.BROWSER_JOBS ?? "1");
const baseMinFps = Number(process.env.PAN_STRESS_FPS_MIN ?? "2.0");
const baseMaxDrawAvg = Number(process.env.PAN_STRESS_DRAW_MAX_MS ?? "90");
const minFps = baseMinFps / Math.max(1, jobs);
const maxDrawAvg = baseMaxDrawAvg * Math.max(1, jobs) ** 1.5;

console.log("pan_stress perf:", { fps, drawAvg, before, after });

if (fps < minFps) {
  throw new Error(`fpsAvg ${fps.toFixed(2)} below floor ${minFps}`);
}
if (drawAvg > maxDrawAvg) {
  throw new Error(`drawAvgMS ${drawAvg.toFixed(2)}ms exceeded ${maxDrawAvg}ms target`);
}
