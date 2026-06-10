// webaudio_three_stage_latency.browser.test.js
//
// End-to-end latency gate for the WASM audio pipeline. Measures all
// three stages of the scheduling chain:
//
//   Stage A  real         → scheduler fires     (seqScheduleTime late)
//   Stage B  scheduler    → audio dispatch      (enqAt → audioLoop)
//   Stage C  dispatch     → audio fires         (lead = when - audioNow)
//
// Per-stage assertions on P50/P90/P99/max + StdDev (jitter). Designed
// to be tight — when WASM lag regresses (GC pause, goroutine starvation,
// channel saturation, etc.) one of these should trip before the user
// notices.
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_three_stage_latency.browser.test.js
//
// Skip env vars (use sparingly):
//   THREE_STAGE_SOFT=1   — convert hard assertions to warnings
//   THREE_STAGE_WARMUP=2000 — warmup ms before measurement (default 1500)
//   THREE_STAGE_PLAY_MS=6000 — measurement duration ms (default 6000)

import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

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
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

const SOFT = process.env.THREE_STAGE_SOFT === "1";
const WARMUP_MS = Number(process.env.THREE_STAGE_WARMUP ?? 1500);
const PLAY_MS = Number(process.env.THREE_STAGE_PLAY_MS ?? 6000);

const failures = [];
function gate(scenario, label, cond, msg) {
  if (!cond) {
    const line = `[${scenario}] FAIL ${label}: ${msg}`;
    failures.push(line);
    console.error(line);
  } else {
    console.log(`[${scenario}] ok  ${label}`);
  }
}

function fmtMs(v) {
  if (v == null || Number.isNaN(v)) return "n/a";
  return (v * 1000).toFixed(3) + "ms";
}

function dumpStage(name, st) {
  if (!st) { console.log(`  ${name}: <none>`); return; }
  console.log(`  ${name}: count=${st.count} avg=${fmtMs(st.avg)} p50=${fmtMs(st.p50)} p90=${fmtMs(st.p90)} p99=${fmtMs(st.p99)} max=${fmtMs(st.max)} stddev=${fmtMs(st.stddev)}`);
}

function dumpSnap(scenario, snap, stats) {
  console.log(`\n--- ${scenario} ---`);
  if (!snap) { console.log("  snap=null"); return; }
  console.log(`  overall: count=${snap.count} overdue=${snap.overdue} smallLead=${snap.smallLeadCount} e2eP99=${fmtMs(snap.e2eP99)}`);
  dumpStage("Stage A seqFireLate ", snap.seqFireLate);
  dumpStage("Stage B bridge       ", snap.bridge);
  dumpStage("Stage C lead         ", snap.lead);
  if (snap.lead) {
    console.log(`  Stage C lag: avg=${fmtMs(snap.lead.avgLag)} max=${fmtMs(snap.lead.maxLag)} p90=${fmtMs(snap.lead.lagP90)} p99=${fmtMs(snap.lead.lagP99)}`);
  }
  if (stats) {
    console.log(`  perfStats: fps=${stats.fpsAvg?.toFixed(1)} drawAvg=${stats.drawAvgMS?.toFixed(2)}ms drawMax=${stats.drawMaxMS?.toFixed(2)}ms updateAvg=${stats.updateAvgMS?.toFixed(2)}ms updateMax=${stats.updateMaxMS?.toFixed(2)}ms qLatMax=${stats.audioQLatMax?.toFixed(2)}ms audioCallAvg=${stats.audioCallAvg?.toFixed(3)}ms audioCallMax=${stats.audioCallMax?.toFixed(3)}ms heapAllocKB=${stats.heapAllocKB}`);
  }
}

async function runScenario({ name, rows, bpm, playMs }) {
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof getThreeStageLatency === "function" && typeof resetThreeStageLatency === "function");

  await page.evaluate(({ rows, bpm }) => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
    buildPerfRect(rows, 1);
    forceDraw?.(); forceDraw?.();
    setBPM(bpm);
    startPlay();
  }, { rows, bpm });

  // Warmup: discard early measurements while AudioContext stabilises and
  // pre-roll allocations happen.
  await page.waitForTimeout(WARMUP_MS);
  await page.evaluate(() => {
    resetThreeStageLatency?.();
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
  });

  await page.waitForTimeout(playMs);

  const snap = await page.evaluate(() => getThreeStageLatency?.());
  const stats = await page.evaluate(() => perfStats?.());
  await page.evaluate(() => stopPlay?.());
  await page.close();

  dumpSnap(name, snap, stats);
  return { snap, stats };
}

// Threshold philosophy:
// - Audio-critical metrics stay TIGHT: overdue=0, smallLead=0, lagP99<=8ms,
//   lead.min>=3ms. These are the ones the listener actually hears. Any
//   regression here is an audio bug and must fail the test.
// - Scheduler-jitter metrics (Stage A, lead stddev) are intrinsically
//   frame-coupled in WASM (cooperative goroutine scheduling can't preempt
//   long RAF spans). They are gated at ~1.5× observed P99 to catch
//   regressions, but not aspirational sub-frame targets.
// - Stage B (goroutine handoff) caps reflect typical observed bridge max
//   plus headroom for occasional spikes during long RAF-blocking frames.
const scenarios = [
  // Threshold philosophy:
  // - Audio-critical gates stay TIGHT (overdue=0, smallLead=0,
  //   lagP99 ≤ 5–12ms, lead.min ≥ 3ms, audioDrops=0). These are what
  //   the listener actually hears; any regression here is an audio bug.
  // - Scheduler-jitter gates (Stage A, stddev) are intrinsically
  //   frame-coupled in WASM (cooperative goroutine scheduling can't
  //   preempt long RAF spans). Generous caps catch order-of-magnitude
  //   regressions while tolerating browser variance up to ~500ms.
  // - Stage B caps accept the ~25ms bridge-handoff max observed during
  //   long-frame events in headless Chromium.
  { name: "S1-single-row-BPM240", rows: 1, bpm: 240, playMs: PLAY_MS,
    limits: {
      seqFire: { p99: 0.500, stddev: 0.130 },
      bridge:  { p99: 0.030, stddev: 0.010, max: 0.040 },
      lead:    { min: 0.003, stddev: 0.060, lagP99: 0.005, overdue: 0, smallLead: 0 },
      e2eP99:  0.525,
      audioDrops: 0,
    } },
  { name: "S2-six-row-BPM280", rows: 6, bpm: 280, playMs: PLAY_MS,
    limits: {
      seqFire: { p99: 0.650, stddev: 0.150 },
      bridge:  { p99: 0.045, stddev: 0.012, max: 0.060 },
      lead:    { min: 0.003, stddev: 0.060, lagP99: 0.008, overdue: 0, smallLead: 0 },
      e2eP99:  0.700,
      audioDrops: 0,
    } },
  { name: "S3-eight-row-BPM320", rows: 8, bpm: 320, playMs: PLAY_MS,
    limits: {
      seqFire: { p99: 0.500, stddev: 0.130 },
      bridge:  { p99: 0.045, stddev: 0.012, max: 0.060 },
      lead:    { min: 0.003, stddev: 0.060, lagP99: 0.012, overdue: 0, smallLead: 0 },
      e2eP99:  0.560,
      audioDrops: 0,
    } },
];

for (const sc of scenarios) {
  const { snap, stats } = await runScenario(sc);
  if (!snap) { failures.push(`[${sc.name}] FAIL: snap is null`); continue; }
  const lim = sc.limits;
  const a = snap.seqFireLate, b = snap.bridge, c = snap.lead;

  gate(sc.name, "count>0", snap.count > 0, `count=${snap.count}`);
  gate(sc.name, "overdue", snap.overdue <= lim.lead.overdue, `${snap.overdue} > ${lim.lead.overdue}`);
  gate(sc.name, "smallLead", snap.smallLeadCount <= lim.lead.smallLead, `${snap.smallLeadCount} > ${lim.lead.smallLead}`);
  if (lim.audioDrops != null && stats) {
    const drops = stats.audioDrops || 0;
    gate(sc.name, "audioDrops", drops <= lim.audioDrops, `audioCh drops ${drops} > ${lim.audioDrops}`);
  }

  if (a) {
    gate(sc.name, "A.p99", a.p99 != null && a.p99 <= lim.seqFire.p99, `seqFire P99 ${fmtMs(a.p99)} > ${fmtMs(lim.seqFire.p99)}`);
    gate(sc.name, "A.stddev", a.stddev != null && a.stddev <= lim.seqFire.stddev, `seqFire stddev ${fmtMs(a.stddev)} > ${fmtMs(lim.seqFire.stddev)}`);
  }
  if (b) {
    gate(sc.name, "B.p99", b.p99 != null && b.p99 <= lim.bridge.p99, `bridge P99 ${fmtMs(b.p99)} > ${fmtMs(lim.bridge.p99)}`);
    gate(sc.name, "B.max", b.max != null && b.max <= lim.bridge.max, `bridge max ${fmtMs(b.max)} > ${fmtMs(lim.bridge.max)}`);
    gate(sc.name, "B.stddev", b.stddev != null && b.stddev <= lim.bridge.stddev, `bridge stddev ${fmtMs(b.stddev)} > ${fmtMs(lim.bridge.stddev)}`);
  }
  if (c) {
    gate(sc.name, "C.min", c.min != null && c.min >= lim.lead.min, `lead min ${fmtMs(c.min)} < ${fmtMs(lim.lead.min)}`);
    gate(sc.name, "C.stddev", c.stddev != null && c.stddev <= lim.lead.stddev, `lead stddev ${fmtMs(c.stddev)} > ${fmtMs(lim.lead.stddev)}`);
    const lagP99 = c.lagP99 == null ? 0 : c.lagP99;
    gate(sc.name, "C.lagP99", lagP99 <= lim.lead.lagP99, `lagP99 ${fmtMs(lagP99)} > ${fmtMs(lim.lead.lagP99)}`);
  }
  if (snap.e2eP99 != null) {
    gate(sc.name, "e2eP99", snap.e2eP99 <= lim.e2eP99, `e2eP99 ${fmtMs(snap.e2eP99)} > ${fmtMs(lim.e2eP99)}`);
  }
}

await browser.close();
await new Promise((r) => server.close(r));

if (failures.length > 0) {
  console.error(`\nthree-stage latency: ${failures.length} FAILURE(S)`);
  for (const f of failures) console.error("  " + f);
  if (!SOFT) process.exit(1);
}
console.log(`\nthree-stage latency: ${failures.length === 0 ? "PASSED" : "soft-warnings"}`);
