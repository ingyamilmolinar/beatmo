import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// ---------------------------------------------------------------------------
// Configuration via env vars:
//   BENCH_CIRCUIT=startup  — Use startup demo (58 nodes, 7 instruments, EQ+FX)
//                            instead of synthetic buildPerfRect(4,1)
//   BENCH_BPM=200          — Override BPM (default 200)
//   BENCH_DURATION=15000   — Override playback duration in ms (default 3200,
//                            or 15000 recommended for startup demo)
// ---------------------------------------------------------------------------
const useStartupDemo = process.env.BENCH_CIRCUIT === "startup";
const benchBPM = Number(process.env.BENCH_BPM ?? "200");
const benchDuration = Number(process.env.BENCH_DURATION ?? (useStartupDemo ? "15000" : "3200"));

const mode = useStartupDemo ? "startup-demo" : "synthetic";
console.log(`perf_e2e: mode=${mode} bpm=${benchBPM} duration=${benchDuration}ms`);

// Build the main WASM so JS exports are present.
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Serve the whole src/js directory so index.html can load main.wasm and audio.js.
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
// Wait for wasm runtime to be ready (either exported hook or a while for game to start).
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "perf.e2e");

// For startup demo, let caches warm before we start measuring.
if (useStartupDemo) {
  await page.evaluate(() => {
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(500);
}

// Prep perf counters, then build graph.
await page.evaluate(() => {
  if (typeof resetAudioScheduleMetrics === 'function') resetAudioScheduleMetrics();
  if (typeof resetPerfStats === 'function') resetPerfStats();
});
await clearSchedulerMismatches(page);
// Build stress graph (or skip for startup demo), set BPM, start playback.
await page.evaluate(({ useStartup, bpm }) => {
  // Unlock AudioContext — page.evaluate() doesn't trigger user gesture
  // events that audio.js listens for.
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();

  if (!useStartup) {
    buildPerfRect(4, 1);
  }
  if (typeof forceDraw === 'function') {
    forceDraw();
    forceDraw();
  }
  setBPM(bpm);
  startPlay();
}, { useStartup: useStartupDemo, bpm: benchBPM });
// Let it run to accumulate perf stats including Draw times.
await page.waitForTimeout(benchDuration);
const stats = await page.evaluate(() => perfStats());
await assertNoSchedulerMismatches(page, "perf.e2e: scheduler mismatches");
await page.evaluate(() => {
  if (typeof stopPlay === "function") stopPlay();
});
console.log('perf.e2e.browser:', stats);

const audioMetrics = await page.evaluate(() => { if (typeof getAudioScheduleMetrics !== 'function') { return null;
  }
  return getAudioScheduleMetrics();
});
console.log('perf.e2e.audioMetrics:', audioMetrics);
console.log('perf.e2e.audioHistory:', audioMetrics?.history);

if (!audioMetrics) { throw new Error('audio schedule metrics unavailable');
}
if (audioMetrics.count <= 0) { throw new Error('no audio events observed during e2e playback');
}

// Startup demo uses a heavier circuit — relax audio timing thresholds.
const overdueLimit = useStartupDemo
  ? Number(process.env.PERF_E2E_OVERDUE_MAX ?? "5")
  : 0;
if (audioMetrics.overdue > overdueLimit) {
  throw new Error(`observed ${audioMetrics.overdue} overdue audio events (limit ${overdueLimit})`);
}
const minLeadThreshold = useStartupDemo ? 2.0 : 3.5;
const minLeadMs = audioMetrics.minLead == null ? null : (audioMetrics.minLead * 1000);
if (minLeadMs == null || minLeadMs < minLeadThreshold) {
  throw new Error(`audio min lead ${minLeadMs == null ? 'null' : minLeadMs.toFixed(2)}ms below ${minLeadThreshold}ms threshold`);
}
const maxSmallLead = Number(process.env.PERF_E2E_SMALL_LEAD_MAX ?? (useStartupDemo ? "10" : "2"));
if (audioMetrics.smallLeadCount > maxSmallLead) {
  throw new Error(`observed ${audioMetrics.smallLeadCount} audio events scheduled with <4ms lead (max ${maxSmallLead})`);
}
const lagP90Limit = useStartupDemo ? 0.02 : 0.01;
if (audioMetrics.lagP90 != null && audioMetrics.lagP90 > lagP90Limit) {
  throw new Error(`audio lagP90 ${(audioMetrics.lagP90 * 1000).toFixed(2)}ms exceeded ${(lagP90Limit * 1000).toFixed(0)}ms`);
}
const lagP99Limit = useStartupDemo ? 0.04 : 0.02;
if (audioMetrics.lagP99 != null && audioMetrics.lagP99 > lagP99Limit) {
  throw new Error(`audio lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeded ${(lagP99Limit * 1000).toFixed(0)}ms`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "perf_e2e");
await browser.close();
server.close();

if (stats.frames <= 0) { throw new Error('no frames recorded in e2e WASM');
}

// Scale timing thresholds by parallel job count — CPU contention from concurrent
// browser instances inflates all per-frame durations, not just render-bound ones.
const jobs = Number(process.env.BROWSER_JOBS ?? "1");
// Startup demo has 7 rows + EQ + effects — loosen thresholds.
const baseMinFps = Number(process.env.PERF_E2E_FPS_MIN ?? (useStartupDemo ? "2" : "4"));
const baseMaxDrawAvg = Number(process.env.PERF_E2E_DRAW_MAX_MS ?? (useStartupDemo ? "80" : "40"));
const minFps = baseMinFps / Math.max(1, jobs);
const maxDrawAvg = baseMaxDrawAvg * Math.max(1, jobs);
const maxUpdateAvg = Number(process.env.PERF_E2E_UPDATE_MAX_MS ?? (useStartupDemo ? "8" : "4.5")) * Math.max(1, jobs);
const maxAudioCallAvg = Number(process.env.PERF_E2E_AUDIO_CALL_MAX_MS ?? (useStartupDemo ? "1.0" : "0.45")) * Math.max(1, jobs);
const maxAudioQLatMax = Number(process.env.PERF_E2E_AUDIO_QLAT_MAX_MS ?? (useStartupDemo ? "20" : "10")) * Math.max(1, jobs);

if (stats.fpsAvg < minFps) { throw new Error(`fpsAvg ${stats.fpsAvg.toFixed(2)} below floor ${minFps}`);
}
if (stats.updateAvgMS > maxUpdateAvg) { throw new Error(`updateAvgMS ${stats.updateAvgMS.toFixed(3)}ms exceeded limit ${maxUpdateAvg}ms`);
}
if (stats.drawAvgMS > maxDrawAvg) { throw new Error(`drawAvgMS ${stats.drawAvgMS.toFixed(3)}ms exceeded limit ${maxDrawAvg}ms`);
}
if (stats.audioCallAvg > maxAudioCallAvg) { throw new Error(`audioCallAvg ${stats.audioCallAvg.toFixed(3)}ms exceeded limit ${maxAudioCallAvg}ms`);
}
if (stats.audioQLatMax > maxAudioQLatMax) { throw new Error(`audioQLatMax ${stats.audioQLatMax.toFixed(3)}ms exceeded limit ${maxAudioQLatMax}ms`);
}

console.log(`perf_e2e (${mode}): PASSED`);
