import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const repoRoot = path.resolve(jsDir, "..", "..");
const GO = resolveGoBinary();

// Build WASM
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
let failed = false;
const errors = [];

// Gate readiness on REAL exports that only register after Game.New() finishes
// installing the full JS bridge. Go's init() (js_bootstrap_wasm.go) installs a
// no-op placeholder `startPlay` BEFORE Game.New() runs, so gating on
// `typeof startPlay === "function"` races ahead of full init and leaves
// resetPerfStats / buildPerfRect undefined. These two are registered in the
// post-init export pass (js_exports_harness.go / js_exports_playback_perf.go),
// so they are a reliable "fully initialized" signal.
async function waitForFullInit(page) {
  await page.waitForFunction(
    () =>
      typeof resetPerfStats === "function" &&
      typeof buildPerfRect === "function" &&
      typeof startPlay === "function" &&
      typeof getAudioScheduleMetrics === "function" &&
      typeof resetAudioScheduleMetrics === "function"
  );
}

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
  console.log(`  ${label}: count=${m.count} overdue=${m.overdue} smallLead=${m.smallLeadCount} minLead=${fmtMs(m.minLead)} avgLag=${fmtMs(m.avgLag)} lagP90=${fmtMs(m.lagP90)} lagP99=${fmtMs(m.lagP99)} maxLag=${fmtMs(m.maxLag)}`);
}

// Minimum scheduler events we require to have observed before judging timing.
// Each scenario runs 8-10s of playback at >=260 BPM across >=1 row, so a
// healthy scheduler emits hundreds of events. A sane floor proves the
// scheduler actually ran rather than stalling silently. Kept conservative so
// it gates on "did real work happen" without being brittle to machine speed.
const MIN_EVENTS = 50;

// ---------------------------------------------------------------------------
// Scenario A: Single-row extreme BPM
// ---------------------------------------------------------------------------
console.log("\n=== Scenario A: Single-row extreme (BPM=300, 1 row, 10s) ===");
{
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await waitForFullInit(page);
  await clearSchedulerMismatches(page);

  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
    buildPerfRect(1, 1);
    forceDraw?.(); forceDraw?.();
    setBPM(300);
    startPlay();
  });

  // Give AudioContext time to unlock, then wait for playback.
  await page.waitForTimeout(10500);

  const m = await page.evaluate(() => getAudioScheduleMetrics?.());
  const stats = await page.evaluate(() => perfStats?.());
  await page.evaluate(() => stopPlay?.());
  await assertNoSchedulerMismatches(page, "Scenario A");

  console.log("  perfStats:", stats);
  printAudioMetrics("audio", m);

  // Hard gates on metrics that POPULATE on a healthy run.
  hardAssert(m && m.count >= MIN_EVENTS, `A: count ${m?.count} < ${MIN_EVENTS} (scheduler stalled?)`);
  hardAssert(m.overdue === 0, `A: ${m.overdue} overdue events (lead<0 — scheduler firing late)`);
  hardAssert(m.smallLeadCount === 0, `A: ${m.smallLeadCount} events with <3ms lead (scheduler cushion too thin)`);
  hardAssert(m.minLead != null && m.minLead * 1000 >= 3, `A: minLead ${fmtMs(m.minLead)} < 3ms`);
  // lagP90/lagP99 are null on a healthy scheduler (no overdue events ⇒ empty
  // lag samples). Keep as WARN-only; they only carry signal when overdue>0.
  softAssert(m.lagP90 == null || m.lagP90 <= 0.008, `A: lagP90 ${fmtMs(m.lagP90)} > 8ms`);
  softAssert(m.lagP99 == null || m.lagP99 <= 0.015, `A: lagP99 ${fmtMs(m.lagP99)} > 15ms`);

  await page.close();
}

// ---------------------------------------------------------------------------
// Scenario B: Multi-row stress
// ---------------------------------------------------------------------------
console.log("\n=== Scenario B: Multi-row stress (BPM=280, 6 rows, 10s) ===");
{
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await waitForFullInit(page);
  await clearSchedulerMismatches(page);

  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
    buildPerfRect(6, 1);
    forceDraw?.(); forceDraw?.();
    setBPM(280);
    startPlay();
  });

  await page.waitForTimeout(10500);

  const m = await page.evaluate(() => getAudioScheduleMetrics?.());
  const stats = await page.evaluate(() => perfStats?.());
  await page.evaluate(() => stopPlay?.());
  await assertNoSchedulerMismatches(page, "Scenario B");

  console.log("  perfStats:", stats);
  printAudioMetrics("audio", m);

  // Hard gates on metrics that POPULATE on a healthy run.
  hardAssert(m && m.count >= MIN_EVENTS, `B: count ${m?.count} < ${MIN_EVENTS} (scheduler stalled?)`);
  hardAssert(m.overdue === 0, `B: ${m.overdue} overdue events (lead<0 — scheduler firing late)`);
  hardAssert(m.smallLeadCount === 0, `B: ${m.smallLeadCount} events with <3ms lead (scheduler cushion too thin)`);
  hardAssert(m.minLead != null && m.minLead * 1000 >= 3, `B: minLead ${fmtMs(m.minLead)} < 3ms`);
  // null-on-healthy lag percentiles: WARN-only.
  softAssert(m.lagP90 == null || m.lagP90 <= 0.010, `B: lagP90 ${fmtMs(m.lagP90)} > 10ms`);
  softAssert(m.lagP99 == null || m.lagP99 <= 0.020, `B: lagP99 ${fmtMs(m.lagP99)} > 20ms`);
  softAssert(m.avgLag == null || m.avgLag <= 0.005, `B: avgLag ${fmtMs(m.avgLag)} > 5ms`);

  await page.close();
}

// ---------------------------------------------------------------------------
// Scenario C: Real project extreme BPM
// ---------------------------------------------------------------------------
console.log("\n=== Scenario C: Real project extreme (BPM=260, beatmo.json, 8s) ===");
{
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await waitForFullInit(page);
  await clearSchedulerMismatches(page);

  const projectJson = fs.readFileSync(path.join(repoRoot, "src", "go", "internal", "assets", "beatmo_project_fixture.json"), "utf8");

  await page.evaluate(({ json }) => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
    importJSON?.(json);
    forceDraw?.(); forceDraw?.();
    setBPM?.(260);
  }, { json: projectJson });

  // Settle after import
  await page.waitForTimeout(500);

  await page.evaluate(() => {
    resetAudioScheduleMetrics?.();
    startPlay?.();
  });

  await page.waitForTimeout(8000);

  const m = await page.evaluate(() => getAudioScheduleMetrics?.());
  const stats = await page.evaluate(() => perfStats?.());
  await page.evaluate(() => stopPlay?.());
  await assertNoSchedulerMismatches(page, "Scenario C");

  console.log("  perfStats:", stats);
  printAudioMetrics("audio", m);

  // Hard gates on metrics that POPULATE on a healthy run.
  hardAssert(m && m.count >= MIN_EVENTS, `C: count ${m?.count} < ${MIN_EVENTS} (scheduler stalled?)`);
  hardAssert(m.overdue === 0, `C: ${m.overdue} overdue events (lead<0 — scheduler firing late)`);
  hardAssert(m.smallLeadCount === 0, `C: ${m.smallLeadCount} events with <3ms lead (scheduler cushion too thin)`);
  hardAssert(m.minLead != null && m.minLead * 1000 >= 3, `C: minLead ${fmtMs(m.minLead)} < 3ms`);
  // null-on-healthy lag percentiles: WARN-only.
  softAssert(m.lagP90 == null || m.lagP90 <= 0.010, `C: lagP90 ${fmtMs(m.lagP90)} > 10ms`);
  softAssert(m.lagP99 == null || m.lagP99 <= 0.020, `C: lagP99 ${fmtMs(m.lagP99)} > 20ms`);

  await page.close();
}

// ---------------------------------------------------------------------------
// Scenario D: BPM ramp 120 → 320
// ---------------------------------------------------------------------------
console.log("\n=== Scenario D: BPM ramp (120 -> 320, 4 rows, 2s/step) ===");
{
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await waitForFullInit(page);
  await clearSchedulerMismatches(page);

  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
    buildPerfRect(4, 1);
    forceDraw?.(); forceDraw?.();
  });

  const rampResults = [];
  const bpmStart = 120;
  const bpmEnd = 320;
  const bpmStep = 20;
  const stepDurationMs = 2000;

  for (let bpm = bpmStart; bpm <= bpmEnd; bpm += bpmStep) {
    await page.evaluate((b) => {
      resetAudioScheduleMetrics?.();
      resetPerfStats?.();
      setBPM(b);
      if (!isPlaying?.()) startPlay();
    }, bpm);

    await page.waitForTimeout(stepDurationMs);

    const m = await page.evaluate(() => getAudioScheduleMetrics?.());
    const stats = await page.evaluate(() => perfStats?.());

    const entry = {
      bpm,
      count: m?.count ?? 0,
      overdue: m?.overdue ?? 0,
      smallLeadCount: m?.smallLeadCount ?? 0,
      lagP90: m?.lagP90,
      lagP99: m?.lagP99,
      avgLag: m?.avgLag,
      minLead: m?.minLead,
      fpsAvg: stats?.fpsAvg,
      updateAvgMS: stats?.updateAvgMS,
    };
    rampResults.push(entry);
  }

  await page.evaluate(() => stopPlay?.());
  await assertNoSchedulerMismatches(page, "Scenario D");

  // Print ramp table
  console.log("\n  BPM  | count | overdue | lagP90    | lagP99    | avgLag    | minLead   | FPS   | updateMS");
  console.log("  -----|-------|---------|-----------|-----------|-----------|-----------|-------|--------");
  for (const r of rampResults) {
    console.log(
      `  ${String(r.bpm).padStart(4)} | ${String(r.count).padStart(5)} | ${String(r.overdue).padStart(7)} | ` +
      `${fmtMs(r.lagP90).padStart(9)} | ${fmtMs(r.lagP99).padStart(9)} | ${fmtMs(r.avgLag).padStart(9)} | ` +
      `${fmtMs(r.minLead).padStart(9)} | ${(r.fpsAvg ?? 0).toFixed(1).padStart(5)} | ${(r.updateAvgMS ?? 0).toFixed(2)}`
    );
  }

  // Find breakpoint: first BPM where lagP99 exceeds 20ms
  const breakpoint = rampResults.find(r => r.lagP99 != null && r.lagP99 > 0.020);
  if (breakpoint) {
    console.log(`\n  BPM breakpoint (lagP99 > 20ms): ${breakpoint.bpm}`);
  } else {
    console.log("\n  No breakpoint found up to BPM=320");
  }

  // Each ramp step runs only 2s with a 0.5s scheduler warmup, so far fewer
  // events accrue per step than the long single-BPM scenarios. Use a lower
  // per-step floor that still proves the scheduler ran at each BPM.
  const MIN_EVENTS_PER_STEP = 10;

  // Hard gates: timing must hold across the whole sweep up to BPM=280.
  // Gate on metrics that POPULATE on a healthy scheduler.
  for (const r of rampResults) {
    if (r.bpm > 280) continue;
    hardAssert(r.count >= MIN_EVENTS_PER_STEP,
      `D: count ${r.count} < ${MIN_EVENTS_PER_STEP} at BPM=${r.bpm} (scheduler stalled?)`);
    hardAssert(r.overdue === 0,
      `D: ${r.overdue} overdue events at BPM=${r.bpm} (lead<0 — scheduler firing late)`);
    hardAssert(r.smallLeadCount === 0,
      `D: ${r.smallLeadCount} events with <3ms lead at BPM=${r.bpm} (cushion too thin)`);
    hardAssert(r.minLead != null && r.minLead * 1000 >= 3,
      `D: minLead ${fmtMs(r.minLead)} < 3ms at BPM=${r.bpm}`);
    // null-on-healthy lag percentiles: WARN-only.
    softAssert(r.lagP90 == null || r.lagP90 <= 0.010,
      `D: lagP90 at BPM=${r.bpm} is ${fmtMs(r.lagP90)} > 10ms`);
    softAssert(r.lagP99 == null || r.lagP99 <= 0.020,
      `D: lagP99 at BPM=${r.bpm} is ${fmtMs(r.lagP99)} > 20ms`);
  }

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_timing_stress");
  await page.close();
}

// ---------------------------------------------------------------------------
// Summary
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

console.log("\naudio_timing_stress: all scenarios passed");
