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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
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

const BPM_LEVELS = [120, 200, 240, 300];
const DURATION_SEC = 15;
const results = [];

for (const bpm of BPM_LEVELS) {
  console.log(`\n=== Startup Demo @ BPM=${bpm} (${DURATION_SEC}s) ===`);

  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function", { timeout: 30000 });
  await clearSchedulerMismatches(page);

  // Unlock AudioContext + settle caches
  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(500);

  // Reset metrics, set BPM, start playback
  await page.evaluate((b) => {
    resetPerfStats?.();
    resetAudioScheduleMetrics?.();
    setBPM(b);
    startPlay();
  }, bpm);

  // Wait for playback duration
  await page.waitForTimeout(DURATION_SEC * 1000 + 500);

  // Collect metrics
  const m = await page.evaluate(() => getAudioScheduleMetrics?.());
  const stats = await page.evaluate(() => perfStats?.());
  const rows = await page.evaluate(() => totalRows?.());
  await page.evaluate(() => stopPlay?.());
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
  });

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

// Timing assertions (tightened for sequential execution with exclusive CPU)
for (const r of results) {
  hardAssert(r.overdue === 0, `BPM=${r.bpm}: ${r.overdue} overdue events`);
  hardAssert(r.lagP90 == null || r.lagP90 <= 0.010,
    `BPM=${r.bpm}: lagP90 ${fmtMs(r.lagP90)} > 10ms`);
  hardAssert(r.lagP99 == null || r.lagP99 <= 0.020,
    `BPM=${r.bpm}: lagP99 ${fmtMs(r.lagP99)} > 20ms`);
}

// FPS floor and update time ceiling (BPM-scaled)
for (const r of results) {
  const fpsFloor = r.bpm <= 200 ? 20 : (r.bpm <= 240 ? 18 : 12);
  hardAssert(r.fpsAvg >= fpsFloor,
    `BPM=${r.bpm}: fpsAvg ${r.fpsAvg.toFixed(1)} below floor ${fpsFloor}`);
  const updateMax = r.bpm <= 200 ? 2.5 : 3.0;
  hardAssert(r.updateAvgMS <= updateMax,
    `BPM=${r.bpm}: updateAvgMS ${r.updateAvgMS.toFixed(3)}ms exceeded ${updateMax}ms`);
}

// ---------------------------------------------------------------------------
// Save machine-readable JSON (matches desktop bench-results/desktop.json schema)
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
}));

const resultsDir = path.resolve(jsDir, "../../bench-results");
fs.mkdirSync(resultsDir, { recursive: true });
fs.writeFileSync(path.join(resultsDir, "browser.json"), JSON.stringify(jsonResults, null, 2) + "\n");
console.log(`\nResults saved to bench-results/browser.json`);

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
