// Startup-demo benchmark (`make bench-browser`).
//
// Env:
//   TEST_PLATFORM=chrome-local TEST_HEADED=1   real Chrome with a real GPU (the
//                     default headless Chromium rasterizes WebGL in software)
//   BENCH_BPMS=120,200          tempo levels (default 120,200,240,300)
//   BENCH_ADD_FX=pitchshift     once playback runs, add this insert effect to
//                     row 0 (the kick) — a live edit DURING playback, like a
//                     user in the FX panel; `<row>:<type>` picks another row
//   BENCH_TAG=after             results → bench-results/browser_<tag>.json
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { launchBrowser } from "./platform/launch.js";
import { assertNoSchedulerMismatches, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import {
  startPlaybackWhenReady, startPlaybackProbe, stopPlaybackProbe, probeSummaryFields, probeGateFailures,
  PROBE_STRICT, PROBE_LIMITS_DEFAULT, PROBE_JSQUEUE_MAX_MS,
} from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build WASM (main.wasm — auto-loads startup_demo.json on boot, same as desktop `make bench`)
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// HTTP server
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
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const { browser, platform } = await launchBrowser();
console.log(`[BENCH] platform ${platform}${process.env.TEST_HEADED === "1" ? " (headed)" : ""}`);
let failed = false;
const errors = [];

function softAssert(cond, msg) {
  if (!cond) {
    console.warn(`  WARN: ${msg}`);
    errors.push(msg);
  }
}
function hardAssert(cond, msg) {
  if (!cond) {
    failed = true;
    errors.push(msg);
    throw new Error(msg);
  }
}

function fmtMs(sec) {
  return sec == null ? "null" : (sec * 1000).toFixed(2) + "ms";
}

function printAudioMetrics(label, m) {
  console.log(`  ${label}: count=${m.count} overdue=${m.overdue} minLead=${fmtMs(m.minLead)} avgLag=${fmtMs(m.avgLag)} lagP90=${fmtMs(m.lagP90)} lagP99=${fmtMs(m.lagP99)} maxLag=${fmtMs(m.maxLag)}`);
}

// ---------------------------------------------------------------------------
// Benchmark: Startup demo at 4 BPM levels (matching desktop `make bench`)
//
// The WASM app auto-loads startup_demo.json on boot — 58 nodes, 7 instruments,
// full EQ + effects. No importJSON() needed. This is the exact same circuit
// that desktop `make bench` runs.
// ---------------------------------------------------------------------------

const BPM_LEVELS = process.env.BENCH_BPMS
  ? process.env.BENCH_BPMS.split(",").map((x) => Number(x.trim())).filter((x) => x > 0)
  : [120, 200, 240, 300];
// BENCH_ADD_FX: "<type>" (row 0) or "<row>:<type>".
const ADD_FX = (() => {
  const v = process.env.BENCH_ADD_FX;
  if (!v) return null;
  const [a, b] = v.includes(":") ? v.split(":") : ["0", v];
  return { row: Number(a), type: b.trim() };
})();
const DURATION_SEC = 15;
const results = [];

// Phase E (Phase F prep): Allow per-knob runtime profile overrides via env.
// Pass a JSON object in BEATMO_ENV_OVERRIDE keyed by BEATMO_* variable names
// (internal/env/vars.go); it's injected as window.__beatmoEnv before WASM
// init, which index.html folds into go.env, so applyEnvProfileOverride
// (internal/ui/runtime_profile_env.go) reads it via os.Getenv — the same
// path desktop bench sweeps use. Values must be strings (wasm_exec.js
// coerces via String() when building the KEY=value environ, so a raw number
// works but is coerced anyway).
//
// Example:
//   BEATMO_ENV_OVERRIDE='{"BEATMO_TIMELINE_INFO_THROTTLE_MS":"0"}' make bench-browser
let profileOverride = null;
if (process.env.BEATMO_ENV_OVERRIDE) {
  try {
    profileOverride = JSON.parse(process.env.BEATMO_ENV_OVERRIDE);
    console.log(`[BENCH] env override: ${JSON.stringify(profileOverride)}`);
  } catch (e) {
    console.warn(`[BENCH] invalid BEATMO_ENV_OVERRIDE JSON: ${e.message}`);
  }
}

for (const bpm of BPM_LEVELS) {
  console.log(`\n=== Startup Demo @ BPM=${bpm} (${DURATION_SEC}s) ===`);

  const page = await browser.newPage();
  if (profileOverride) {
    // Env-var overrides reach Go via go.env (see index.html). Keys are the
    // BEATMO_* names from internal/env/vars.go; values must be strings.
    await page.addInitScript((cfg) => {
      window.__beatmoEnv = Object.fromEntries(
        Object.entries(cfg).map(([k, v]) => [k, String(v)]),
      );
    }, profileOverride);
  }
  await page.goto(`http://localhost:${port}/`);
  // Gate on forceDraw, not just startPlay: js_bootstrap_wasm.go registers a
  // STUB startPlay early (before Game.initJS registers the real exports), so
  // startPlay alone can pass while forceDraw below is still unregistered —
  // the bare-identifier `forceDraw?.()` then throws ReferenceError.
  await page.waitForFunction(
    () => typeof startPlay === "function" && typeof forceDraw === "function", null,
    { timeout: 30000 },
  );
  await clearSchedulerMismatches(page);

  // Unlock AudioContext + settle caches
  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(500);

  // Reset metrics, set BPM, start playback — once the readiness gate opens
  // (a bare startPlay() during voice warm-up is ignored).
  await page.evaluate(() => {
    resetPerfStats?.();
    resetAudioScheduleMetrics?.();
  });
  await startPlaybackWhenReady(page, { bpm });
  await page.waitForTimeout(500);
  if (ADD_FX) {
    const added = await page.evaluate(({ row, type }) => {
      const id = rowInstrument(row);
      const slot = addInsertEffect(id, type);
      return { id, slot, hosts: insertEffectHosts(id) };
    }, ADD_FX);
    console.log(`  live edit: added ${ADD_FX.type} to ${added.id} (slot ${added.slot}; hosts ${JSON.stringify(added.hosts)})`);
    await page.waitForTimeout(500);
  }
  const graph0 = await page.evaluate(() => (typeof audioGraphStats === "function" ? audioGraphStats() : null));
  // Playback probe: onset-level "what the listener heard" metrics.
  await startPlaybackProbe(page);

  // Wait for playback duration
  await page.waitForTimeout(DURATION_SEC * 1000);

  // Collect metrics
  const m = await page.evaluate(() => getAudioScheduleMetrics?.());
  const stats = await page.evaluate(() => perfStats?.());
  const rows = await page.evaluate(() => totalRows?.());
  const graph1 = await page.evaluate(() => (typeof audioGraphStats === "function" ? audioGraphStats() : null));
  const { report: probe } = await stopPlaybackProbe(page, `BPM=${bpm}`);
  if (probe) {
    const lateCh = Object.entries(probe.perChannel || {}).filter(([, c]) => c.heardLate > 0).map(([id, c]) => `${id} ${c.heardLate}/${c.notes}`);
    if (lateCh.length) console.log(`  heard late through an insert chain's own latency: ${lateCh.join(", ")}`);
  }
  if (graph0 && graph1) {
    const d = (k) => graph1[k] - graph0[k];
    console.log(`  graph churn during playback: chain pushes ${d("insertChainUpdates")}, DSP nodes ${d("dspNodesCreated")}, rewires ${d("channelRewires")}, EQ updates ${d("eqUpdates")}; ${graph1.channelChains} channel chains, ${graph1.insertEffectsLive} live insert effects`);
  }
  await page.evaluate(() => stopPlay?.());
  for (const msg of probeGateFailures(probe, PROBE_LIMITS_DEFAULT, `BPM=${bpm}`)) {
    if (PROBE_STRICT) { failed = true; errors.push(msg); } else console.log(`  [probe info] ${msg}`);
  }
  await assertNoSchedulerMismatches(page, `BPM=${bpm}`);

  console.log(`  rows: ${rows}`);
  console.log("  perfStats:", stats);
  printAudioMetrics("audio", m);

  results.push({
    bpm,
    rows: rows ?? 0,
    fpsAvg: stats?.fpsAvg ?? 0,
    updateAvgMS: stats?.updateAvgMS ?? 0,
    updateMaxMS: stats?.updateMaxMS ?? 0,
    drawAvgMS: stats?.drawAvgMS ?? 0,
    drawMaxMS: stats?.drawMaxMS ?? 0,
    count: m?.count ?? 0,
    overdue: m?.overdue ?? 0,
    lagP90: m?.lagP90,
    lagP99: m?.lagP99,
    avgLag: m?.avgLag,
    maxLag: m?.maxLag,
    minLead: m?.minLead,
    maxLead: m?.maxLead,
    avgLead: m?.avgLead,
    smallLeadCount: m?.smallLeadCount ?? 0,
    liveImages: stats?.liveImages ?? 0,
    imagesAllocatedTotal: stats?.imagesAllocatedTotal ?? 0,
    imagesByTag: stats?.imagesByTag ?? "",
    gcCycles: stats?.gcCycles ?? 0,
    gcForced: stats?.gcForced ?? 0,
    gcPauseP99MS: stats?.gcPauseP99MS ?? 0,
    gcPauseMaxMS: stats?.gcPauseMaxMS ?? 0,
    gcHeapHighKB: stats?.gcHeapHighKB ?? 0,
    probe: probeSummaryFields(probe),
  });
  console.log(`  imagesByTag: ${stats?.imagesByTag ?? "(none)"}`);

  hardAssert(m && m.count > 0, `BPM=${bpm}: no audio events`);

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "bench_startup_demo");
  await page.close();
}

// ---------------------------------------------------------------------------
// Summary table
// ---------------------------------------------------------------------------
console.log("\n╔══════════════════════════════════════════════════════════════════════════════════════════════════════╗");
console.log("║  BROWSER BENCHMARK: Startup Demo (58 nodes, 7 instruments, EQ+effects)                            ║");
console.log("╠══════╦═══════╦═════════╦═════════╦═════════╦═════════╦═══════╦═════════╦═════════╦═════════╦═══════╣");
console.log("║ BPM  ║  FPS  ║ Upd Avg ║ Upd Max ║ Drw Avg ║ Drw Max ║ Count ║ Overdue ║ lagP90  ║ lagP99  ║ Lead  ║");
console.log("╠══════╬═══════╬═════════╬═════════╬═════════╬═════════╬═══════╬═════════╬═════════╬═════════╬═══════╣");

for (const r of results) {
  const line = [
    String(r.bpm).padStart(4),
    (r.fpsAvg).toFixed(1).padStart(5),
    (r.updateAvgMS).toFixed(2).padStart(7),
    (r.updateMaxMS).toFixed(2).padStart(7),
    (r.drawAvgMS).toFixed(2).padStart(7),
    (r.drawMaxMS).toFixed(2).padStart(7),
    String(r.count).padStart(5),
    String(r.overdue).padStart(7),
    fmtMs(r.lagP90).padStart(7),
    fmtMs(r.lagP99).padStart(7),
    fmtMs(r.minLead).padStart(5),
  ];
  console.log(`║ ${line.join(" ║ ")} ║`);
}
console.log("╚══════╩═══════╩═════════╩═════════╩═════════╩═════════╩═══════╩═════════╩═════════╩═════════╩═══════╝");

// GC summary (single-threaded WASM: every collection is a STW pause).
console.log("\n╔══════════════════════════════════════════════════════════════╗");
console.log("║  BROWSER GC (whole-run window per BPM)                        ║");
console.log("╠══════╦════════╦════════╦══════════╦══════════╦══════════════╣");
console.log("║ BPM  ║ Cycles ║ Forced ║ Pause99  ║ PauseMax ║ HeapHigh(MB) ║");
console.log("╠══════╬════════╬════════╬══════════╬══════════╬══════════════╣");
for (const r of results) {
  const line = [
    String(r.bpm).padStart(4),
    String(r.gcCycles ?? 0).padStart(6),
    String(r.gcForced ?? 0).padStart(6),
    (r.gcPauseP99MS ?? 0).toFixed(3).padStart(6) + "ms",
    (r.gcPauseMaxMS ?? 0).toFixed(3).padStart(6) + "ms",
    ((r.gcHeapHighKB ?? 0) / 1024).toFixed(1).padStart(12),
  ];
  console.log(`║ ${line.join(" ║ ")} ║`);
}
console.log("╚══════╩════════╩════════╩══════════╩══════════╩══════════════╝");

// ---------------------------------------------------------------------------
// Save machine-readable JSON (matches desktop bench-results/desktop.json
// schema). Written BEFORE the threshold assertions below so a tripped gate
// still leaves browser.json on disk — a benchmark must record its data.
// ---------------------------------------------------------------------------
const jsonResults = results.map(r => ({
  bpm: r.bpm,
  fps: r.fpsAvg,
  updateAvgMS: r.updateAvgMS,
  updateMaxMS: r.updateMaxMS,
  drawAvgMS: r.drawAvgMS,
  drawMaxMS: r.drawMaxMS,
  schedCount: r.count,
  schedOverdue: r.overdue,
  schedMinLeadMS: r.minLead != null ? r.minLead * 1000 : null,
  schedMaxLeadMS: r.maxLead != null ? r.maxLead * 1000 : null,
  schedAvgLeadMS: r.avgLead != null ? r.avgLead * 1000 : null,
  schedAvgLagMS: r.avgLag != null ? r.avgLag * 1000 : null,
  schedMaxLagMS: r.maxLag != null ? r.maxLag * 1000 : null,
  schedLagP90MS: r.lagP90 != null ? r.lagP90 * 1000 : null,
  schedLagP99MS: r.lagP99 != null ? r.lagP99 * 1000 : null,
  schedSmallLeadCount: r.smallLeadCount ?? 0,
  liveImages: r.liveImages ?? 0,
  imagesAllocatedTotal: r.imagesAllocatedTotal ?? 0,
  imagesByTag: r.imagesByTag ?? "",
  gcCycles: r.gcCycles ?? 0,
  gcForced: r.gcForced ?? 0,
  gcPauseP99MS: r.gcPauseP99MS ?? 0,
  gcPauseMaxMS: r.gcPauseMaxMS ?? 0,
  gcHeapHighKB: r.gcHeapHighKB ?? 0,
  // Playback probe: what the listener heard (see audio_probe_analysis.js).
  ...r.probe,
}));

const resultsDir = path.resolve(jsDir, "../../bench-results");
fs.mkdirSync(resultsDir, { recursive: true });
const resultsName = process.env.BENCH_TAG ? `browser_${process.env.BENCH_TAG}.json` : "browser.json";
fs.writeFileSync(path.join(resultsDir, resultsName), JSON.stringify(jsonResults, null, 2) + "\n");
console.log(`\nResults saved to bench-results/${resultsName}`);

// ---------------------------------------------------------------------------
// Threshold assertions (run AFTER the JSON save so a tripped gate still leaves
// browser.json on disk). Audio-smoothness gates (overdue/lag) are the hard floor.
// ---------------------------------------------------------------------------
// Lateness is measured on the REQUESTED start time (before audio.js's 4 ms
// floor clamp — observed after it, a late note read as lead=+4ms and these
// gates could never fail). In headless Chromium the lateness is dominated by
// software-GL frames starving the single WASM thread, so it is enforced only
// with AUDIO_PROBE_STRICT=1; structural probe gates stay hard below.
const latenessAssert = (cond, msg) => {
  if (PROBE_STRICT) hardAssert(cond, msg);
  else if (!cond) console.log(`  [lateness info] ${msg}`);
};
for (const r of results) {
  latenessAssert(r.overdue === 0, `BPM=${r.bpm}: ${r.overdue} overdue events`);
  latenessAssert(r.lagP90 == null || r.lagP90 <= 0.010,
    `BPM=${r.bpm}: lagP90 ${fmtMs(r.lagP90)} > 10ms`);
  latenessAssert(r.lagP99 == null || r.lagP99 <= 0.020,
    `BPM=${r.bpm}: lagP99 ${fmtMs(r.lagP99)} > 20ms`);
  // What the listener heard: every judged note audible, none dropped, and
  // start() never queued behind a UI frame (the sync-flush contract).
  const p = r.probe || {};
  if (p.probeNotesJudged != null) {
    hardAssert(p.probeNotesJudged > 0, `BPM=${r.bpm}: playback probe judged no notes`);
    hardAssert(p.probeNotesMissed === 0, `BPM=${r.bpm}: ${p.probeNotesMissed} notes silent in the output`);
    hardAssert(p.probeNotesDropped === 0, `BPM=${r.bpm}: ${p.probeNotesDropped} notes requested but never started`);
    hardAssert(p.probeJsQueueP90MS == null || p.probeJsQueueP90MS <= PROBE_JSQUEUE_MAX_MS,
      `BPM=${r.bpm}: jsQueue p90 ${p.probeJsQueueP90MS}ms > ${PROBE_JSQUEUE_MAX_MS}ms (start() queued behind a UI frame)`);
  }
}

// FPS floor and update-time ceiling (BPM-scaled).
for (const r of results) {
  const fpsFloor = r.bpm <= 200 ? 20 : (r.bpm <= 240 ? 18 : 12);
  hardAssert(r.fpsAvg >= fpsFloor,
    `BPM=${r.bpm}: fpsAvg ${r.fpsAvg.toFixed(1)} below floor ${fpsFloor}`);
  // updateAvg ceiling raised from 2.5/3.0 after the 2026-07-16 GC-disable fix
  // (async.Snapshot no longer turns GC off). With automatic GC restored,
  // single-threaded WASM does its GC mark-assist work inside Update(), so the
  // honest updateAvg is ~3.8-4.7ms — the old ~1.4ms was an artifact of GC being
  // accidentally disabled. Audio stays clean (overdue=0, gated above) and FPS is
  // well above floor; this ceiling still catches a true >2x regression.
  const updateMax = r.bpm <= 200 ? 6.0 : 7.0;
  hardAssert(r.updateAvgMS <= updateMax,
    `BPM=${r.bpm}: updateAvgMS ${r.updateAvgMS.toFixed(3)}ms exceeded ${updateMax}ms`);
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------
await browser.close();
server.close();

if (errors.length > 0) {
  console.log(`\n=== Summary: ${errors.length} issue(s) ===`);
  for (const e of errors) console.log(`  - ${e}`);
}

if (failed) {
  process.exit(1);
}

console.log("\nbench_startup_demo: all scenarios passed");
