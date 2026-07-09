/**
 * Instrument-switch regression gate (WASM).
 *
 * COVERED-BY / COMPANION: webaudio_instrument_switch_bench.browser.test.js
 * records the full latency table; THIS file is the machine-independent GATE.
 *
 * B4 — the "click"/freeze: switching a row to an expensive synth voice
 * (organ-church, ~500ms/render) during playback used to overflow the single
 * render worker's 4s timeout and fall back to a SYNCHRONOUS main-thread render,
 * freezing UI + audio for 2.6–3.5s (measured maxGap). The gate: during active
 * playback, NO render may run on the main thread — slow renders stay on the
 * worker and the note uses the on-grid neighbor fallback (or drops one note)
 * rather than blocking. Anchor is `mainThreadRenders === 0` (count, not ms), so
 * it is deterministic across machines.
 *
 * Reproduction: a freshly-switched melodic row fires several distinct pitches
 * in quick succession. Each is a cold per-pitch render; they serialize on the
 * SINGLE worker, so the queue round-trip stacks (organ ~500ms × N). Once a
 * render's round-trip exceeds the 4s worker timeout, the pre-fix code rejected
 * to a synchronous main-thread render → the freeze. We reproduce that by
 * bursting many distinct cold pitches (NOT by CPU throttle — CDP throttling
 * only slows the main thread, not the render worker). Needs no wasm rebuild
 * (render/cache/worker logic is in audio.js). Use WASM_PREBUILT=1.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Many distinct pitches so the single worker's serialized queue round-trip
// (organ ~500ms each) overruns the 4s timeout and triggers the sync fallback.
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
await page.evaluate(async () => { await window.audioReady; });

await page.mouse.click(20, 20);
await page.waitForTimeout(200);
await page.evaluate(async () => {
  if (window.__audioCtx && window.__audioCtx.state === "suspended") await window.__audioCtx.resume();
});
const ctxState = await page.evaluate(() => window.__audioCtx && window.__audioCtx.state);
if (ctxState !== "running") throw new Error(`AudioContext not running (state=${ctxState})`);

// ── Phase 1 (B1 pre-warm): warm a few organ pitches off-thread, then play
// them — they must NOT defer (the cache was pre-rendered before the notes). ──
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

// ── Phase 2 (B4 freeze gate): burst every distinct cold pitch so the single
// worker's queue overruns the 4s timeout; NO render may hit the main thread. ──
const metrics = await page.evaluate(async ({ PITCHES, DURATION_MS }) => {
  const ctx = window.__audioCtx;
  const id = "organ-church";
  window.__evictRenderCacheForTest(id);
  window.resetRenderLatencyMetrics();
  for (const p of PITCHES) {
    window.playSoundParams(id, 0.7, p, 1.0, ctx.currentTime + 0.06);
  }
  // Keep playback "active" (a note scheduled recently) while the queue drains
  // past the worker timeout, so the guard's isPlaybackActive() sees live audio.
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
