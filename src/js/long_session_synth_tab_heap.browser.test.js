/** The production OOM only shows against the WASM linear-memory ceiling: solo, play, the Synth tab open, a knob dragged
 * at ~10 Hz for the soak. Bounded: Go heap growth, JS heap, analyzer bridge reads per sample, and the timeline / parity
 * stores. */

import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Allow shorter runs for local iteration via env vars.
const SOAK_SECONDS = parseInt(process.env.SOAK_SECONDS || "90", 10);
const PARAM_HZ = parseFloat(process.env.SOAK_PARAM_HZ || "10");
const SAMPLE_EVERY_MS = parseInt(process.env.SOAK_SAMPLE_MS || "5000", 10);

// Forced GC pacing keeps the Go heap near its live baseline; these bounds sit far below the old OOM growth, so a
// regression that brings back GC starvation trips them.
const JS_HEAP_BOUND = 800 * 1024 * 1024;
const GO_HEAP_GROWTH_BOUND = 96 * 1024 * 1024;
const ANALYZER_READS_PER_SAMPLE_BOUND = 250_000;
const PARITY_DECISIONS_PER_ROW_BOUND = 4096;
// Single-threaded WASM Go GC starves under playback unless forced; forcedGCRuns is authoritative because some runtimes
// do not advance NumGC on every runtime.GC().
const GC_COUNT_BOUND_MIN = 2;
const FORCED_GC_RUNS_BOUND_MIN = 2;

// ─── WASM build ─────────────────────────────────────────────────────────
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

// ─── HTTP server ────────────────────────────────────────────────────────
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

const browser = await chromium.launch({
  args: [
    "--autoplay-policy=no-user-gesture-required",
    // Without it usedJSHeapSize rounds to 100 MB and sub-100 MB regressions are invisible.
    "--enable-precise-memory-info",
  ],
});

let failed = false;
const errors = [];
function softAssert(cond, msg) {
  if (!cond) { console.warn(`  WARN: ${msg}`); errors.push(msg); }
}
function hardAssert(cond, msg) {
  if (!cond) { failed = true; errors.push(msg); throw new Error(msg); }
}

// ─── Page setup ─────────────────────────────────────────────────────────
async function newSoakPage() {
  const page = await browser.newPage();
  // Errors and warnings stay, so a failure carries browser-side context.
  page.on("console", (msg) => {
    const t = msg.type();
    if (t === "error" || t === "warning") {
      console.log(`  [browser ${t}] ${msg.text()}`);
    }
  });
  page.on("pageerror", (err) => {
    console.log(`  [pageerror] ${err.message}`);
    errors.push(`pageerror: ${err.message}`);
    failed = true;
  });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof startPlay === "function" &&
    typeof memSizes === "function" &&
    typeof setEQTab === "function" &&
    typeof setInstrumentParam === "function" &&
    typeof toggleSolo === "function" &&
    typeof buildPerfRect === "function"
  );
  // Unlock audio (autoplay policy) via a dispatched gesture.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof resumeAudio === "function") resumeAudio();
  });
  return page;
}

console.log("\n=== Scenario A: 90s soak — solo + play + synth tab + param drag ===");
console.log(`  config: SOAK_SECONDS=${SOAK_SECONDS} PARAM_HZ=${PARAM_HZ} SAMPLE_EVERY_MS=${SAMPLE_EVERY_MS}`);

const page = await newSoakPage();

// The Synth tab opens BEFORE play, so the very first Draw goes through the OOM stack; counters reset so samples
// measure only the soak.
await page.evaluate(() => {
  resetPerfStats?.();
  resetAnalyzerBridgeStats?.();
  buildPerfRect(2, 2); // 2 rows so toggleSolo(1) is valid
  setBPM(120);
  setEQTab("synth");
  toggleSolo(1);
  forceDraw?.();
  forceDraw?.();
});

await startPlaybackWhenReady(page); // Play is ignored while the warm-up gate is closed
// Skip the startup transient.
await page.waitForTimeout(500);

const samples = [];
async function sample(tag) {
  const s = await page.evaluate(() => {
    const m = memSizes();
    const a = analyzerBridgeStats();
    return {
      heapAlloc: m.heapAlloc,
      heapSys: m.heapSys,
      sys: m.sys,
      totalAlloc: m.totalAlloc,
      gcCount: m.gcCount,
      forcedGCRuns: m.forcedGCRuns ?? 0,
      frame: m.frame ?? 0,
      parityAudio: m.parityAudio,
      paritySeqDecisions: m.paritySeqDecisions,
      timelineArchives: m.timelineArchives,
      bridgeCalls: a.calls,
      bridgeReads: a.elementReads,
      usedJSHeapSize: performance.memory ? performance.memory.usedJSHeapSize : 0,
      totalJSHeapSize: performance.memory ? performance.memory.totalJSHeapSize : 0,
    };
  });
  s.tag = tag;
  s.ts = Date.now();
  samples.push(s);
  console.log(
    `  sample[${tag}] go.alloc=${(s.heapAlloc / 1024 / 1024).toFixed(1)}MB ` +
    `go.sys=${(s.sys / 1024 / 1024).toFixed(1)}MB ` +
    `js.used=${(s.usedJSHeapSize / 1024 / 1024).toFixed(1)}MB ` +
    `gc=${s.gcCount} forcedGC=${s.forcedGCRuns} frame=${s.frame} bridge.reads=${s.bridgeReads}`
  );
  return s;
}

await sample("baseline");

// Two params, so the map leaves its degenerate one-key state.
const dragIntervalMs = Math.max(1, Math.floor(1000 / PARAM_HZ));
const totalIterations = Math.floor((SOAK_SECONDS * 1000) / dragIntervalMs);
const samplesEveryIter = Math.max(1, Math.floor(SAMPLE_EVERY_MS / dragIntervalMs));
let it = 0;
const startTs = Date.now();
for (it = 0; it < totalIterations; it++) {
  const t = it / PARAM_HZ;
  const pitch = -12 + 24 * Math.sin(t / 17);
  const decay = 0.25 + 0.75 * Math.sin(t / 41);
  // One evaluate per iteration, not batched: batching lets WASM coalesce and hides the regression. The drilled stage
  // alternates too, so per-stage layout runs under the same stack.
  const stage = (it % 40) < 20 ? "VOICE" : "ENVELOPE";
  await page.evaluate(({ p, d, s }) => {
    setInstrumentParam("kick", "pitch", p);
    setInstrumentParam("kick", "amp_decay", d);
    if (typeof selectSynthSection === "function") selectSynthSection(s);
  }, { p: pitch, d: decay, s: stage });
  if (it > 0 && it % samplesEveryIter === 0) {
    await sample(`t+${Math.floor((Date.now() - startTs) / 1000)}s`);
    // Catch a runaway before the browser actually OOMs.
    const last = samples[samples.length - 1];
    if (last.usedJSHeapSize > JS_HEAP_BOUND) {
      console.log(`  EARLY ABORT: js.used=${last.usedJSHeapSize} exceeded bound during soak`);
      break;
    }
  }
}

// After stopping, so pending highlight and scheduler retention drop out.
await page.evaluate(() => { stopPlay?.(); });
await page.waitForTimeout(500);
await sample("post-stop");

// ─── Assertions ─────────────────────────────────────────────────────────
const baseline = samples[0];
const finalS = samples[samples.length - 1];
const goGrowth = finalS.heapAlloc - baseline.heapAlloc;
const jsUsed = finalS.usedJSHeapSize;
const elapsedSec = (finalS.ts - baseline.ts) / 1000;
const readsRate = (finalS.bridgeReads - baseline.bridgeReads) / Math.max(1, samples.length - 1);

console.log(`\n  summary:`);
console.log(`    elapsed:               ${elapsedSec.toFixed(1)}s`);
console.log(`    go heap growth:        ${(goGrowth / 1024 / 1024).toFixed(1)}MB (bound=${GO_HEAP_GROWTH_BOUND / 1024 / 1024}MB)`);
console.log(`    js heap used:          ${(jsUsed / 1024 / 1024).toFixed(1)}MB (bound=${JS_HEAP_BOUND / 1024 / 1024}MB)`);
console.log(`    analyzer reads/sample: ${readsRate.toFixed(0)} (bound=${ANALYZER_READS_PER_SAMPLE_BOUND})`);
console.log(`    paritySeqDecisions:    ${JSON.stringify(finalS.paritySeqDecisions)}`);
console.log(`    timelineArchives:      ${JSON.stringify(finalS.timelineArchives)}`);

hardAssert(jsUsed < JS_HEAP_BOUND,
  `usedJSHeapSize=${jsUsed} exceeded bound=${JS_HEAP_BOUND}; ` +
  `this is the WASM linear-memory regression the user hit. ` +
  `Inspect samples + analyzerBridgeStats output for the suspect path.`);
hardAssert(goGrowth < GO_HEAP_GROWTH_BOUND,
  `Go heap grew by ${goGrowth} bytes during ${elapsedSec.toFixed(0)}s soak ` +
  `(bound=${GO_HEAP_GROWTH_BOUND}); a Go-side retain leak is the most likely culprit. ` +
  `Compare baseline vs final samples in the trace above to localize the structure that grew.`);
hardAssert(finalS.gcCount >= GC_COUNT_BOUND_MIN,
  `gcCount=${finalS.gcCount} expected >=${GC_COUNT_BOUND_MIN} over ${elapsedSec.toFixed(0)}s soak. ` +
  `Forced-GC pacing in heapProbeTick (runtime_profile.ForceGCInterval=30s on WASM) ` +
  `is the root-cause fix for WASM linear-memory exhaustion; if this assertion ` +
  `fails the optimisation regressed and the heap will grow linearly to the 2 GB ceiling.`);
hardAssert(finalS.forcedGCRuns >= FORCED_GC_RUNS_BOUND_MIN,
  `forcedGCRuns=${finalS.forcedGCRuns} expected >=${FORCED_GC_RUNS_BOUND_MIN}; ` +
  `the heap-probe trigger path never executed. Either ForceGCInterval is 0 (regression in ` +
  `browserRuntimeProfile builder) or g.frame stayed at 0 (Update loop never ran).`);
softAssert(readsRate < ANALYZER_READS_PER_SAMPLE_BOUND,
  `analyzer bridge averaged ${readsRate.toFixed(0)} reads per sample (bound=${ANALYZER_READS_PER_SAMPLE_BOUND}); ` +
  `per-Draw ChannelAnalyzerSnapshot churn is high enough that WASM GC may not keep up.`);

// Per-row decisions bound — mirrors the Go soak's paritySeqDecisionsPerRowBound.
for (const [rowKey, count] of Object.entries(finalS.paritySeqDecisions || {})) {
  softAssert(count <= PARITY_DECISIONS_PER_ROW_BOUND,
    `paritySeqDecisions[${rowKey}]=${count} exceeds bound=${PARITY_DECISIONS_PER_ROW_BOUND}`);
}

await page.close();
await browser.close();
server.close();

if (failed || errors.length > 0) {
  console.error("\nFAILED:");
  for (const e of errors) console.error("  -", e);
  process.exit(1);
}
console.log("\nOK");
process.exit(0);
