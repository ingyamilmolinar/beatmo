import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import {
  startPlaybackWhenReady, startPlaybackProbe, stopPlaybackProbe, probeGateFailures,
  PROBE_STRICT, PROBE_LIMITS_DEFAULT, PROBE_JSQUEUE_MAX_MS,
} from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// BENCH_CIRCUIT=startup (the demo instead of buildPerfRect), BENCH_BPM (default 200), BENCH_DURATION ms (default 3200;
// 15000 for the demo).
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
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
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
// BEATMO_ENV_OVERRIDE (JSON keyed by BEATMO_* names) reaches os.Getenv through window.__beatmoEnv, e.g.
// '{"BEATMO_AUDIO_LOOKAHEAD_SEC":"0.06"}'.
if (process.env.BEATMO_ENV_OVERRIDE) {
  try {
    const profileOverride = JSON.parse(process.env.BEATMO_ENV_OVERRIDE);
    console.log(`[perf.e2e] env override: ${JSON.stringify(profileOverride)}`);
    // Non-strings are silently coerced.
    await page.addInitScript((cfg) => {
      window.__beatmoEnv = Object.fromEntries(
        Object.entries(cfg).map(([k, v]) => [k, String(v)]),
      );
    }, profileOverride);
  } catch (e) {
    console.warn(`[perf.e2e] invalid BEATMO_ENV_OVERRIDE JSON: ${e.message}`);
  }
}
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
  // page.evaluate fires none of the gesture events audio.js listens for.
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();

  if (!useStartup) {
    buildPerfRect(4, 1);
  }
  if (typeof forceDraw === 'function') {
    forceDraw();
    forceDraw();
  }
}, { useStartup: useStartupDemo, bpm: benchBPM });
// A bare startPlay() during warm-up is ignored.
await startPlaybackWhenReady(page, { bpm: benchBPM });
await startPlaybackProbe(page);
// Let it run to accumulate perf stats including Draw times.
await page.waitForTimeout(benchDuration);
const stats = await page.evaluate(() => perfStats());
await assertNoSchedulerMismatches(page, "perf.e2e: scheduler mismatches");
const { report: probe } = await stopPlaybackProbe(page, "perf.e2e");
await page.evaluate(() => {
  if (typeof stopPlay === "function") stopPlay();
});
if (probe) {
  if (probe.notes.judged === 0) throw new Error('perf.e2e: playback probe judged no notes');
  if (probe.notes.missed > 0) throw new Error(`perf.e2e: ${probe.notes.missed} notes silent in the output`);
  if (probe.notes.dropped > 0) throw new Error(`perf.e2e: ${probe.notes.dropped} notes requested but never started`);
  if (probe.jsQueueMs.p90 != null && probe.jsQueueMs.p90 > PROBE_JSQUEUE_MAX_MS) throw new Error(`perf.e2e: jsQueue p90 ${probe.jsQueueMs.p90}ms > ${PROBE_JSQUEUE_MAX_MS}ms`);
  for (const m of probeGateFailures(probe, PROBE_LIMITS_DEFAULT, "perf.e2e")) {
    if (PROBE_STRICT) throw new Error(m);
    console.log(`  [probe info] ${m}`);
  }
}
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

// Printed BEFORE the thresholds, so a config that trips one still yields its numbers.
console.log('[GC_SWEEP_JSON] ' + JSON.stringify({
  gcCycles: stats.gcCycles, gcForced: stats.gcForced,
  gcPauseP50MS: stats.gcPauseP50MS, gcPauseP99MS: stats.gcPauseP99MS, gcPauseMaxMS: stats.gcPauseMaxMS,
  gcHeapHighKB: stats.gcHeapHighKB, fpsAvg: stats.fpsAvg, drawMaxMS: stats.drawMaxMS,
  audioCount: audioMetrics.count, overdue: audioMetrics.overdue,
  smallLeadCount: audioMetrics.smallLeadCount, minLead: audioMetrics.minLead, lagP99: audioMetrics.lagP99,
}));

// On the REQUESTED start, before audio.js's floor. Headless software GL dominates lateness, so these are enforced only
// with AUDIO_PROBE_STRICT=1; the probe gates stay hard.
const latenessFail = (msg) => {
  if (PROBE_STRICT) throw new Error(msg);
  console.log(`  [lateness info] ${msg}`);
};
// Startup demo uses a heavier circuit — relax audio timing thresholds.
const overdueLimit = useStartupDemo
  ? Number(process.env.PERF_E2E_OVERDUE_MAX ?? "5")
  : 0;
if (audioMetrics.overdue > overdueLimit) {
  latenessFail(`observed ${audioMetrics.overdue} overdue audio events (limit ${overdueLimit})`);
}
// Observed at start(), after a 32-event batch has eaten ~8 ms of Go's 12 ms clamp, so the JS-visible floor is ~4 ms.
const minLeadThreshold = useStartupDemo ? 3.0 : 3.0;
const minLeadMs = audioMetrics.minLead == null ? null : (audioMetrics.minLead * 1000);
if (minLeadMs == null || minLeadMs < minLeadThreshold) {
  latenessFail(`audio min lead ${minLeadMs == null ? 'null' : minLeadMs.toFixed(2)}ms below ${minLeadThreshold}ms threshold`);
}
// Tightened: zero events should ship with <3ms lead. Was 2 (synth)/10 (startup).
const maxSmallLead = Number(process.env.PERF_E2E_SMALL_LEAD_MAX ?? (useStartupDemo ? "0" : "0"));
if (audioMetrics.smallLeadCount > maxSmallLead) {
  latenessFail(`observed ${audioMetrics.smallLeadCount} audio events scheduled with <3ms lead (max ${maxSmallLead})`);
}
// Tightened lag percentiles. Was 10ms/20ms (synth)/20ms/40ms (startup).
const lagP90Limit = useStartupDemo ? 0.008 : 0.004;
if (audioMetrics.lagP90 != null && audioMetrics.lagP90 > lagP90Limit) {
  latenessFail(`audio lagP90 ${(audioMetrics.lagP90 * 1000).toFixed(2)}ms exceeded ${(lagP90Limit * 1000).toFixed(0)}ms`);
}
const lagP99Limit = useStartupDemo ? 0.015 : 0.008;
if (audioMetrics.lagP99 != null && audioMetrics.lagP99 > lagP99Limit) {
  latenessFail(`audio lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeded ${(lagP99Limit * 1000).toFixed(0)}ms`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "perf_e2e");
await browser.close();
server.close();

if (stats.frames <= 0) { throw new Error('no frames recorded in e2e WASM');
}

// Concurrent browsers inflate every per-frame duration.
const jobs = Number(process.env.BROWSER_JOBS ?? "1");
// Startup demo has 7 rows + EQ + effects — loosen thresholds.
const baseMinFps = Number(process.env.PERF_E2E_FPS_MIN ?? (useStartupDemo ? "2" : "4"));
const baseMaxDrawAvg = Number(process.env.PERF_E2E_DRAW_MAX_MS ?? (useStartupDemo ? "80" : "40"));
const minFps = baseMinFps / Math.max(1, jobs);
const maxDrawAvg = baseMaxDrawAvg * Math.max(1, jobs);
const maxUpdateAvg = Number(process.env.PERF_E2E_UPDATE_MAX_MS ?? (useStartupDemo ? "8" : "4.5")) * Math.max(1, jobs);
// The batch's voices now start inside the timed call, which raised the average by ~0.15 ms/event with Update unchanged;
// don't read the higher call time as a bridge regression.
const maxAudioCallAvg = Number(process.env.PERF_E2E_AUDIO_CALL_MAX_MS ?? (useStartupDemo ? "0.6" : "0.45")) * Math.max(1, jobs);
const maxAudioQLatMax = Number(process.env.PERF_E2E_AUDIO_QLAT_MAX_MS ?? (useStartupDemo ? "8" : "3")) * Math.max(1, jobs);

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

// Batches beyond ~5× the mean are GC pauses or starvation the user hears.
if (stats.audioCallAvg > 0 && stats.audioCallMax > 0) {
  const ratio = stats.audioCallMax / stats.audioCallAvg;
  // With a tiny average one outlier dominates; the absolute max is gated above, this catches sustained jitter.
  const ratioLimit = Number(process.env.PERF_E2E_AUDIO_CALL_RATIO_MAX ?? (useStartupDemo ? "20" : "20"));
  if (ratio > ratioLimit) {
    throw new Error(`audioCall jitter ratio ${ratio.toFixed(2)}× exceeded ${ratioLimit}× (avg=${stats.audioCallAvg.toFixed(3)}ms max=${stats.audioCallMax.toFixed(3)}ms)`);
  }
}

console.log(`perf_e2e (${mode}): PASSED`);
