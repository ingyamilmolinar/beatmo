/** Machine-independent gate: switching a row to an expensive voice during playback must never render on the main thread
 * (it froze UI and audio for seconds); slow renders stay on the workers and the note uses the neighbour fallback.
 * Anchored on mainThreadRenders === 0, a count. A burst of distinct cold pitches overruns the worker timeout. WASM_PREBUILT=1. */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, waitForAudioReady } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Enough cold pitches that the worker queue overruns its timeout.
const PITCHES = [-24, -19, -17, -12, -9, -7, -5, -2, 0, 3, 5, 7, 9, 12, 15, 19, 24];
const DURATION_MS = Number(process.env.SWITCH_REG_DURATION_MS || 9000);

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, "").split("?")[0]);
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (err) => { pageErrors.push(String(err)); console.log("[PAGE-ERROR]", String(err)); });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.playSoundParams === "function");
await page.waitForFunction(() => typeof window.getRenderLatencyMetrics === "function");
await page.waitForFunction(() => typeof window.__evictRenderCacheForTest === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await waitForAudioReady(page);

await page.mouse.click(20, 20);
await page.waitForTimeout(200);
await page.evaluate(async () => {
  if (window.__audioCtx && window.__audioCtx.state === "suspended") await window.__audioCtx.resume();
});
const ctxState = await page.evaluate(() => window.__audioCtx && window.__audioCtx.state);
if (ctxState !== "running") throw new Error(`AudioContext not running (state=${ctxState})`);

// Pitches warmed off-thread first must not defer.
const warm = await page.evaluate(async () => {
  const ctx = window.__audioCtx;
  const id = "organ-church";
  const pitches = [-5, 0, 7];
  window.__evictRenderCacheForTest(id);
  window.resetRenderLatencyMetrics();
  window.warmInstrument(id, JSON.stringify(pitches));
  // Wait for the warm renders to land (organ ~0.4-0.5s each on one worker).
  await new Promise((r) => setTimeout(r, 3000));
  const afterWarm = window.getRenderLatencyMetrics();
  // Now play exactly those pitches; a warm cache means zero fresh defers.
  window.resetRenderLatencyMetrics();
  for (const p of pitches) window.playSoundParams(id, 0.7, p, 1.0, ctx.currentTime + 0.06);
  await new Promise((r) => setTimeout(r, 600));
  const onPlay = window.getRenderLatencyMetrics();
  return { warmRenders: afterWarm.renders, playDeferred: onPlay.deferredPlays, playRenders: onPlay.renders };
}, {});

// Every distinct cold pitch at once: NO render may reach the main thread.
const metrics = await page.evaluate(async ({ PITCHES, DURATION_MS }) => {
  const ctx = window.__audioCtx;
  const id = "organ-church";
  window.__evictRenderCacheForTest(id);
  window.resetRenderLatencyMetrics();
  for (const p of PITCHES) {
    window.playSoundParams(id, 0.7, p, 1.0, ctx.currentTime + 0.06);
  }
  // So isPlaybackActive() sees live audio while the queue drains.
  const keepAlive = setInterval(() => {
    window.playSoundParams("snare", 0.3, 0, 1.0, ctx.currentTime + 0.06);
  }, 120);
  await new Promise((r) => setTimeout(r, DURATION_MS));
  clearInterval(keepAlive);
  await new Promise((r) => setTimeout(r, 800));
  return window.getRenderLatencyMetrics();
}, { PITCHES, DURATION_MS });

await browser.close();
server.close();

console.log(`[TEST] warm: warmRenders=${warm.warmRenders} playDeferred=${warm.playDeferred} playRenders=${warm.playRenders}`);
console.log(`[TEST] freeze: mainThreadRenders=${metrics.mainThreadRenders} workerRenders=${metrics.workerRenders} ` +
  `deferredPlays=${metrics.deferredPlays} neighborPlays=${metrics.neighborPlays} renders=${metrics.renders}`);

const failures = [];
if (warm.warmRenders < 3) {
  failures.push(`pre-warm rendered ${warm.warmRenders}/3 pitches — warmInstrument did not pre-render the pitch set`);
}
if (warm.playDeferred > 0) {
  failures.push(`${warm.playDeferred} deferred play(s) after pre-warm — warmed pitches should hit a warm cache (B1)`);
}
if (metrics.mainThreadRenders > 0) {
  failures.push(`${metrics.mainThreadRenders} render(s) ran on the MAIN THREAD during playback — sync fallback froze audio (B4 regression)`);
}
if (pageErrors.length > 0) {
  failures.push(`page emitted ${pageErrors.length} error(s): ${pageErrors.join(" | ")}`);
}

if (failures.length > 0) {
  console.error("\ninstrument_switch_regression FAILED:\n  - " + failures.join("\n  - "));
  process.exit(1);
}
console.log("\ninstrument_switch_regression browser test PASSED");
