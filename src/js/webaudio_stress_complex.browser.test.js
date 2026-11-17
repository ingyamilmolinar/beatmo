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
const goDir = path.resolve(jsDir, "../go");
const repoRoot = path.resolve(jsDir, "..", "..");
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
    "./cmd",
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
await assertSimpleDrawMode(page, false, "stress complex");
await clearSchedulerMismatches(page);

const projectJson = fs.readFileSync(path.join(repoRoot, "src", "go", "internal", "assets", "beatmo_project_fixture.json"), "utf8");

// Every row of the fixture is a builtin synth. The test never reads WAVs: it
// used to load the local, untracked assets/wav sample library when present and
// assign a sample to every row, so the same test measured synths on one machine
// and samples on another. Audio files are local tuning material only — never
// committed, never a test input (TestNoAudioFilesInRepository).
await page.evaluate(async ({ projectJson }) => {
  // Unlock AudioContext — page.evaluate() doesn't trigger user gesture events.
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();

  resetAudioScheduleMetrics?.();
  forceDraw?.();

  if (typeof importJSON === "function") { importJSON(projectJson);
  }

  forceDraw?.();

  // ensure every row uses its instrument id (and refresh once more)
  const insts = typeof instOptions === "function" ? instOptions() : [];
  const rowCount = typeof totalRows === "function" ? totalRows() : 0;
  if (typeof setRowInstrument === "function") { for (let r = 0; r < rowCount; r++) { const id = instOptions?.()[r % insts.length] ?? insts[0];
      if (id) { setRowInstrument(r, id);
      }
    }
  }

  forceDraw?.();
}, { projectJson });
// Play once the readiness gate opens (a bare startPlay() during voice warm-up
// is ignored), then probe what is actually heard across the sample window.
await startPlaybackWhenReady(page, { bpm: 210 });
await page.evaluate(() => resetPerfStats?.());
await startPlaybackProbe(page);
const SAMPLE_INTERVAL_MS = Number(process.env.STRESS_COMPLEX_INTERVAL_MS ?? 5000);
const SAMPLE_COUNT = Number(process.env.STRESS_COMPLEX_SAMPLES ?? 6);
const samples = [];
for (let i = 0; i < SAMPLE_COUNT; i++) { await page.waitForTimeout(SAMPLE_INTERVAL_MS);
  samples.push(await page.evaluate(() => ({ perf: perfStats?.(),
    audio: getAudioScheduleMetrics?.(),
  })));
}

await assertNoSchedulerMismatches(page, "stress complex: scheduler mismatches");
const { report: probe } = await stopPlaybackProbe(page, "stress_complex");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "stress_complex");
await browser.close();
server.close();

console.log("stress_complex samples:", samples);

const firstSample = samples[0] || {};
const lastSample = samples[samples.length - 1] || {};
const stats = lastSample.perf;
const audioMetrics = lastSample.audio;

console.log("stress_complex final perf:", stats);
console.log("stress_complex final audioMetrics:", audioMetrics);
console.log("stress_complex history:", audioMetrics?.history);
console.log("stress_complex drift:", { fpsStart: firstSample.perf?.fpsAvg ?? null,
  fpsEnd: stats?.fpsAvg ?? null,
  leadP99Start: firstSample.audio?.leadP99 ?? null,
  leadP99End: audioMetrics?.leadP99 ?? null,
  lagP99Start: firstSample.audio?.lagP99 ?? null,
  lagP99End: audioMetrics?.lagP99 ?? null,
});

if (!stats) { throw new Error("perfStats unavailable");
}
if (!audioMetrics) { throw new Error("audio schedule metrics unavailable");
}

// Thresholds calibrated 2026-05-06 from 8 fresh runs of this scenario
// (5 back-to-back hot + 3 with cooldown) on the development machine:
//   fpsAvg        observed 14.86–22.04 → gate 12   (~20% headroom over worst)
//   drawAvgMS     observed 46.07–69.50 → gate 80   (~15% headroom over worst)
//   drawMaxMS     observed 102.9–196.2 → gate 250  (~28% headroom; spikes are noisy)
//   audioCallAvg  observed 0.043–0.190 → gate 0.25 (~32% headroom)
//   audioCallMax  observed 0.10–22.85  → gate 50   (covers the outlier with margin)
// Each gate is tight enough to detect a meaningful regression beyond today's
// envelope while loose enough to absorb variance from system load when this
// test runs late in the sequential phase of `make test-real`. To make a gate
// stricter for a one-off experiment, override via the matching env var.
const minFps = Number(process.env.STRESS_COMPLEX_FPS_MIN ?? "12");
const maxDrawAvg = Number(process.env.STRESS_COMPLEX_DRAW_MAX_MS ?? "80");
const maxDrawMax = Number(process.env.STRESS_COMPLEX_DRAW_MAX_SPIKE_MS ?? "250");
// audioCallAvg re-calibrated 2026-09-30 (0.25 → 0.6): the Go→JS PlayBatch
// call now STARTS the batch's voices synchronously (audio.js
// enqueueAudioEvents(…, true)) instead of only queueing them for a microtask
// that ran after the whole frame. Same work, now inside the timed span.
// Interleaved A/B on this bench (load 18-22): sync 0.20/0.23/0.27 ms vs legacy
// 0.07/0.11/0.09 ms, updateAvg unchanged (4.1-4.9 vs 3.8-4.4 ms); worst sync
// reading 0.58 ms at load 18.
const maxAudioCallAvg = Number(process.env.STRESS_COMPLEX_AUDIO_CALL_AVG_MS ?? "0.6");
const maxAudioCallMax = Number(process.env.STRESS_COMPLEX_AUDIO_CALL_MAX_MS ?? "50");
if (stats.fpsAvg < minFps) {
  throw new Error(`fpsAvg ${stats.fpsAvg.toFixed(2)} below ${minFps} in stress scenario`);
}
if (stats.drawAvgMS > maxDrawAvg) {
  throw new Error(`drawAvgMS ${stats.drawAvgMS.toFixed(2)}ms exceeded ${maxDrawAvg}ms target`);
}
if (stats.drawMaxMS > maxDrawMax) {
  throw new Error(`drawMaxMS ${stats.drawMaxMS.toFixed(2)}ms exceeded ${maxDrawMax}ms target (worst-frame spike)`);
}
// Lateness is observed on the REQUESTED start (before audio.js's 4 ms floor
// clamp — after it these could never fail). In headless it is dominated by
// software-GL frames starving the WASM thread → enforced only with
// AUDIO_PROBE_STRICT=1. The probe's structural gates stay hard.
const latenessFail = (msg) => {
  if (PROBE_STRICT) throw new Error(msg);
  console.log(`  [lateness info] ${msg}`);
};
if (probe) {
  if (probe.notes.judged === 0) throw new Error("stress_complex: playback probe judged no notes");
  // The project fixture has mute nodes, which legitimately stop a voice
  // before it sounds — so a silent note is reported here, not fatal.
  if (probe.notes.missed > 0) console.log(`  [probe info] stress_complex: ${probe.notes.missed} notes silent (fixture has mute nodes)`);
  if (probe.notes.dropped > 0) throw new Error(`stress_complex: ${probe.notes.dropped} notes requested but never started`);
  if (probe.jsQueueMs.p90 != null && probe.jsQueueMs.p90 > PROBE_JSQUEUE_MAX_MS) throw new Error(`stress_complex: jsQueue p90 ${probe.jsQueueMs.p90}ms > ${PROBE_JSQUEUE_MAX_MS}ms`);
  for (const m of probeGateFailures(probe, PROBE_LIMITS_DEFAULT, "stress_complex")) latenessFail(m);
}
if (audioMetrics.lagP90 != null && audioMetrics.lagP90 > 0.01) { latenessFail(`lagP90 ${(audioMetrics.lagP90 * 1000).toFixed(2)}ms exceeded 10ms target`);
}
if (audioMetrics.lagP99 != null && audioMetrics.lagP99 > 0.02) { latenessFail(`lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeded 20ms target`);
}
if (audioMetrics.avgLag != null && audioMetrics.avgLag > 0.005) { latenessFail(`avgLag ${(audioMetrics.avgLag * 1000).toFixed(2)}ms exceeded 5ms target`);
}
if (audioMetrics.maxLag != null && audioMetrics.maxLag > 0.05) { latenessFail(`maxLag ${(audioMetrics.maxLag * 1000).toFixed(2)}ms exceeded 50ms target`);
}
if (stats.audioCallAvg > maxAudioCallAvg) { throw new Error(`audioCallAvg ${stats.audioCallAvg.toFixed(3)}ms exceeded ${maxAudioCallAvg}ms target`);
}
if (stats.audioCallMax > maxAudioCallMax) { throw new Error(`audioCallMax ${stats.audioCallMax.toFixed(2)}ms exceeded ${maxAudioCallMax}ms target`);
}
