import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

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

const port = 8340 + Math.floor(Math.random() * 1000);
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
await new Promise((resolve) => server.listen(port, resolve));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await assertSimpleDrawMode(page, false, "pan stress");
await clearSchedulerMismatches(page);

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
await browser.close();
server.close();

if (!after) { throw new Error("perfStats unavailable");
}

const fps = after.fpsAvg;
const drawAvg = after.drawAvgMS;
const minFps = Number(process.env.PAN_STRESS_FPS_MIN ?? "2.0");
const maxDrawAvg = Number(process.env.PAN_STRESS_DRAW_MAX_MS ?? "90");

console.log("pan_stress perf:", { fps, drawAvg, before, after });

if (fps < minFps) {
  throw new Error(`fpsAvg ${fps.toFixed(2)} below floor ${minFps}`);
}
if (drawAvg > maxDrawAvg) {
  throw new Error(`drawAvgMS ${drawAvg.toFixed(2)}ms exceeded ${maxDrawAvg}ms target`);
}
