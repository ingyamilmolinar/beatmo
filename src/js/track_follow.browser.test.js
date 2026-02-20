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

// Build main WASM to exercise real Update/Draw loop with follow logic.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
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
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "track follow");

await page.evaluate(() => {
  window.__followTestGraphBuilt = false;
});

async function verifyFollowDefault() {
  await page.evaluate(() => {
    stopPlay?.();
    if (typeof setFollow === 'function') setFollow(true);
    if (typeof setBPM === 'function') setBPM(200);
    if (!window.__followTestGraphBuilt) {
      buildPerfRect?.(2, 1);
      window.__followTestGraphBuilt = true;
    }
    clickTimelineAt?.(0);
    forceDraw?.();
    forceDraw?.();
  });

  // Allow any pending seek freeze countdown to settle.
  await page.waitForTimeout(100);

  const before = await page.evaluate(() => ({ off: drumOffset?.(),
    state: rowsLayerState?.(),
    cache: rowCacheOffset?.(0),
    nexts: nextBeatIdxs?.() || []
  }));

  await clearSchedulerMismatches(page);
  await page.evaluate(() => startPlay?.());
  // Wait ~2.5s at 200 BPM (~8.3 beats) so desired offset should increase.
  await page.waitForTimeout(2500);
  const after = await page.evaluate(() => ({ off: drumOffset?.(),
    state: rowsLayerState?.(),
    cache: rowCacheOffset?.(0),
    nexts: nextBeatIdxs?.() || []
  }));
  await assertNoSchedulerMismatches(page, "track follow: scheduler mismatches");
  await page.evaluate(() => stopPlay?.());

  if (!before || typeof before.off !== 'number' || !after || typeof after.off !== 'number') {
    throw new Error(`invalid offsets: before=${JSON.stringify(before)} after=${JSON.stringify(after)}`);
  }
  if (after.off <= before.off) {
    const beforeNext = Array.isArray(before.nexts) && before.nexts.length > 0 ? before.nexts[0] : before.off;
    const afterNext = Array.isArray(after.nexts) && after.nexts.length > 0 ? after.nexts[0] : after.off;
    if (afterNext <= beforeNext) {
      throw new Error(`follow tracking did not advance offset: ${before.off} -> ${after.off}`);
    }
  }
  if (typeof before.cache !== 'number' || typeof after.cache !== 'number') {
    throw new Error(`missing cache offsets: before=${JSON.stringify(before)} after=${JSON.stringify(after)}`);
  }
  // Additional cache/layer assertions skipped in headless environments where striping is disabled.
}

await verifyFollowDefault();

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "track_follow");
await browser.close();
server.close();
