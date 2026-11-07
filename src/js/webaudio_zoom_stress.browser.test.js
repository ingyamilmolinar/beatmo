// Zoom during playback must not add per-frame work: tiles keyed by the continuous scale re-rasterize every step and
// starve the single thread. Headless audio latency is unmeasurable, so the cause is measured instead: image allocations
// per frame, zooming vs a static camera at the same draw cadence (self-normalizing under contention).

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
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

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
await new Promise((resolve) => server.listen(0, resolve));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await page.waitForFunction(() => typeof zoomAt === "function" && typeof perfStats === "function" && typeof camScale === "function");
await assertSimpleDrawMode(page, false, "zoom stress"); // full sprite/tile-cache pipeline
await clearSchedulerMismatches(page);

// --- Tunables (env-overridable) ------------------------------------------------
const frames = Number(process.env.ZOOM_STRESS_FRAMES ?? "30");      // draws per phase per round
const rounds = Number(process.env.ZOOM_STRESS_ROUNDS ?? "3");       // A/B rounds, averaged
const extraRows = Number(process.env.ZOOM_STRESS_EXTRA_ROWS ?? "6");// circuit geometry to exercise the grid
// A scale-quantized tile cache re-rasterizes ~never, so ~0.
const allocDeltaMax = Number(process.env.ZOOM_ALLOC_DELTA_MAX ?? "1.0");

// Real geometry and dense playback: a "during playback" scenario.
await page.evaluate((rows) => {
  if (rows > 0) buildPerfRect?.(rows, 2);
  setBPM?.(200);
  resetAudioScheduleMetrics?.();
  forceDraw?.(); forceDraw?.();
  startPlay?.();
}, extraRows);

// Sanity: a single zoomAt actually moves cam.Scale (test is exercising the path).
{
  const s0 = await page.evaluate(() => camScale());
  await page.evaluate(() => zoomAt?.(window.innerWidth / 2, window.innerHeight * 0.45, 20));
  const s1 = await page.evaluate(() => camScale());
  if (!(s1 > s0)) throw new Error(`zoomAt did not increase scale: ${s0} -> ${s1}`);
  await page.evaluate(({ s0 }) => setCamScale?.(s0), { s0 });
  console.log(`zoomAt verified: scale ${s0.toFixed(3)} -> ${s1.toFixed(3)}`);
}

// In the page, so draw cadence and zoom share the game's thread.
async function abRound({ frames }) {
  return await page.evaluate(async ({ frames }) => {
    const cx = window.innerWidth / 2, cy = window.innerHeight * 0.45;
    let dir = 1;
    const settle = () => { for (let k = 0; k < 5; k++) forceDraw(); };

    // Phase A — static camera. Settle (plateau caches), then measure alloc/frame.
    settle();
    resetPerfStats();
    for (let k = 0; k < frames; k++) forceDraw();
    const aAlloc = perfStats().imagesAllocatedTotal / frames;
    const aGridMS = perfStats().drawGridMS;

    // Phase B — oscillating zoom, identical draw count. Measure alloc/frame.
    settle();
    let scaleMin = Infinity, scaleMax = -Infinity;
    resetPerfStats();
    for (let k = 0; k < frames; k++) {
      const s = camScale();
      if (s > 3.0) dir = -1; else if (s < 0.4) dir = 1;
      zoomAt(cx, cy, dir * 5);
      forceDraw();
      const sc = camScale();
      if (sc < scaleMin) scaleMin = sc;
      if (sc > scaleMax) scaleMax = sc;
    }
    const bAlloc = perfStats().imagesAllocatedTotal / frames;
    const bGridMS = perfStats().drawGridMS;
    return { aAlloc, bAlloc, aGridMS, bGridMS, scaleMin, scaleMax };
  }, { frames });
}

// Warm everything before the first measured round.
await page.evaluate(() => { for (let k = 0; k < 30; k++) forceDraw(); });
await page.waitForTimeout(400);

let staticSum = 0, zoomSum = 0, aGridSum = 0, bGridSum = 0, spanMin = Infinity, spanMax = -Infinity;
for (let r = 0; r < rounds; r++) {
  const m = await abRound({ frames });
  staticSum += m.aAlloc; zoomSum += m.bAlloc; aGridSum += m.aGridMS; bGridSum += m.bGridMS;
  spanMin = Math.min(spanMin, m.scaleMin); spanMax = Math.max(spanMax, m.scaleMax);
  console.log(`zoom_stress round ${r}:`, {
    staticAllocPerFrame: +m.aAlloc.toFixed(3), zoomAllocPerFrame: +m.bAlloc.toFixed(3),
    delta: +(m.bAlloc - m.aAlloc).toFixed(3),
    drawGridMS: [+m.aGridMS.toFixed(2), +m.bGridMS.toFixed(2)], scaleSpan: [+m.scaleMin.toFixed(2), +m.scaleMax.toFixed(2)],
  });
}

// A fast zoom-OUT once grew one rebuild's blits ~area/stepPx², quadratically; a multi-cell tile keeps them flat. The
// absolute count depends on the environment, so the GROWTH RATIO to full zoom-out is gated; gridTileBlits is deterministic.
const blitGrowthMax = Number(process.env.ZOOM_BLIT_GROWTH_MAX ?? "12");
// Measure the blits a single fresh grid rebuild performs at a given scale.
const rebuildBlitsAt = (s) => page.evaluate((s) => {
  setCamScale(s);
  forceDraw(); forceDraw();      // settle: cache now built at scale s
  resetPerfStats();
  setCamScale(s * 0.999);        // nudge so the next draw rebuilds exactly once
  forceDraw();
  return { scale: camScale(), blits: perfStats().gridTileBlits, drawGridMS: perfStats().drawGridMS };
}, s);
const refBlits = await rebuildBlitsAt(1.0);
// Slam to minimum scale via real zoom, then measure there.
await page.evaluate(() => { const cx = innerWidth / 2, cy = innerHeight * 0.45; for (let k = 0; k < 80; k++) zoomAt(cx, cy, -30); });
const outBlits = await rebuildBlitsAt(await page.evaluate(() => camScale()));
console.log("zoom_stress zoom-out:", {
  refScale: +refBlits.scale.toFixed(2), refBlits: refBlits.blits,
  outScale: +outBlits.scale.toFixed(3), outBlits: outBlits.blits, outDrawGridMS: +outBlits.drawGridMS.toFixed(2),
  growth: +(outBlits.blits / Math.max(1, refBlits.blits)).toFixed(2),
});

await assertNoSchedulerMismatches(page, "zoom stress: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "zoom_stress");
await browser.close();
server.close();

// Guard: the zoom-out phase must have actually reached a small scale.
if (!(outBlits.scale < 0.25)) {
  throw new Error(`zoom-out phase did not reach a small scale (got ${outBlits.scale.toFixed(3)}); cannot exercise the blit explosion`);
}
const blitGrowth = outBlits.blits / Math.max(1, refBlits.blits);
if (blitGrowth > blitGrowthMax) {
  throw new Error(
    `grid rebuild blits grew ${blitGrowth.toFixed(1)}× from scale ${refBlits.scale.toFixed(2)} ` +
    `(${refBlits.blits} blits) to scale ${outBlits.scale.toFixed(3)} (${outBlits.blits} blits, ` +
    `drawGridMS=${outBlits.drawGridMS.toFixed(1)}ms), max ${blitGrowthMax}×. The grid tiles a stepPx tile, so ` +
    `blits grow ~area/stepPx² as you zoom out — blocking Draw on the single WASM thread and starving audio. ` +
    `Tile a multi-cell block (see buildGridTile / grid_pane_draw.go).`
  );
}

const staticAlloc = staticSum / rounds;
const zoomAlloc = zoomSum / rounds;
const allocDelta = zoomAlloc - staticAlloc;
console.log("zoom_stress summary:", {
  staticAllocPerFrame: +staticAlloc.toFixed(3), zoomAllocPerFrame: +zoomAlloc.toFixed(3),
  allocDelta: +allocDelta.toFixed(3), allocDeltaMax,
  drawGridMS_avg: [+(aGridSum / rounds).toFixed(2), +(bGridSum / rounds).toFixed(2)],
});

// Guard: the zoom phase must have actually swept a meaningful scale range.
if (!(spanMax - spanMin > 0.5)) {
  throw new Error(`zoom phase did not exercise scale (span ${spanMin.toFixed(2)}..${spanMax.toFixed(2)})`);
}

// Every-frame tile re-rasterization is what stalls the thread and starves audio.
if (allocDelta > allocDeltaMax) {
  throw new Error(
    `zoom inflates per-frame texture allocation by ${allocDelta.toFixed(2)}/frame ` +
    `(static=${staticAlloc.toFixed(2)}, zoom=${zoomAlloc.toFixed(2)}, budget +${allocDeltaMax}). ` +
    `The grid-tile cache is re-rasterizing every frame because it keys on continuous cam.Scale; ` +
    `those per-frame GPU texture uploads stall the single WASM thread and starve audio scheduling.`
  );
}
console.log("zoom_stress: PASS — zoom did not inflate per-frame texture allocation");
