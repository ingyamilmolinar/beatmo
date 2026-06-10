/**
 * Long-session WASM heap soak — reproduces the production OOM scenario the
 * Go stub soak cannot reach.
 *
 *   The user-reported crash:
 *     row 1 solo on  → play started → ~1 s later
 *     runtime: out of memory: cannot allocate 4194304-byte block (2130444288 in use)
 *
 *   Stack: ui.Game.Draw → drawDrumPane → DrumView.Draw → DrumViewTree.Draw →
 *          EQPanelZone.Draw → drawSynthTab → drawSynthSectionCard →
 *          Knob.Draw → drawArc → vector.Path.AppendVerticesAndIndices…
 *
 *   The 40 KB allocation that failed inside drawArc is collateral damage —
 *   2.13 GB of Go-heap retention was already in place by the time the draw
 *   call ran. The retained data lives in a path the `-tags test` stub does
 *   not exercise (native voice_cache, WASM analyzer_wasm.go bridge, etc.),
 *   so this browser test is the only place the leak can surface.
 *
 *   Scenario A — Solo + play + synth tab + param drag
 *     1. Build a 4-beat kick pattern.
 *     2. Switch the EQ panel to the Synth tab so the production Draw stack
 *        is active. CHIP-STRIP: the Synth tab now renders a pipeline chip
 *        strip with one stage expanded; the soak also churns the selected
 *        stage (selectSynthSection cycling VOICE↔ENVELOPE) so the expand/
 *        collapse + per-stage knob layout path is exercised under load.
 *     3. Solo row 1 (mirrors the user's log line).
 *     4. Start playback.
 *     5. For 90 s, cycle audio.SetInstrumentParam('kick', 'pitch', ...) at
 *        ~10 Hz to mimic a synth-knob drag, while cycling the expanded chip.
 *     6. Read memSizes() and analyzerBridgeStats() periodically. Assert:
 *          - Final Go heapAlloc growth < 512 MB.
 *          - usedJSHeapSize stays below 1.5 GB (production crashed at 2.13 GB;
 *            1.5 GB gives a 30 % cushion so we trip on a regression long
 *            before the browser would actually OOM).
 *          - Analyzer bridge reads/sample stay below 20 — that's the per-Draw
 *            allocation rate the WASM single-threaded GC can tolerate.
 *          - timelineArchives + paritySeqDecisions rows stay bounded.
 *
 *   This test is justified under the JS Test Charter (CLAUDE.md §"JS Test
 *   Charter — what the JS suite owns"): item #1 (WebAudio behaviour) and #6
 *   (visual regression on a real canvas) cannot be reached from Go, and the
 *   crash itself only manifests against the WASM linear-memory ceiling.
 */

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

// Allow shorter runs for local iteration via env vars.
const SOAK_SECONDS = parseInt(process.env.SOAK_SECONDS || "90", 10);
const PARAM_HZ = parseFloat(process.env.SOAK_PARAM_HZ || "10");
const SAMPLE_EVERY_MS = parseInt(process.env.SOAK_SAMPLE_MS || "5000", 10);

// Bounds. Tightened post-Phase-A-C: forced GC pacing in heapProbeTick
// (runtime_profile.ForceGCInterval=30s on WASM) makes Go heap oscillate
// around the live-data baseline (~50 MB) instead of growing linearly
// to the 2 GB WASM ceiling. 75 s soak now lands at go.alloc ~50 MB,
// js.used ~260 MB with gcCount >= 2; the new bounds catch any
// regression that re-introduces GC starvation. The previous bounds
// (1.5 GB JS, 512 MB Go) would have passed even with the OOM-bound
// growth pattern observed on 2026-05-17 (gc=0, 1.18 MB/s linear
// growth) — tightening here is how we ratchet the regression net.
const JS_HEAP_BOUND = 800 * 1024 * 1024;
const GO_HEAP_GROWTH_BOUND = 96 * 1024 * 1024;
const ANALYZER_READS_PER_SAMPLE_BOUND = 250_000;
const PARITY_DECISIONS_PER_ROW_BOUND = 4096;
// GC effectiveness invariant — single-threaded WASM Go GC starves
// under sustained playback unless forced; gcCount==0 at the end of a
// 75 s soak is the pre-fix signature. forcedGCRuns asserts the
// trigger path actually fired (some WASM runtime versions don't
// advance NumGC after every runtime.GC() call — the counter is the
// authoritative signal that our pacing executed).
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
    // performance.memory.usedJSHeapSize is only exposed in Chromium with
    // --enable-precise-memory-info; without it the value rounds to 100 MB
    // granularity and the soak can't detect sub-100MB regressions.
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
  // Silence noisy INFO console output but surface ERROR + warnings so a
  // failure printout includes browser-side context.
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

// Build a 4-beat kick pattern then open the Synth tab BEFORE play starts,
// so the very first Draw goes through the production OOM stack. Solo row
// 1 to mirror the user's "row 1 solo on" log line. resetPerfStats /
// resetAnalyzerBridgeStats wipe the cumulative counters so the per-sample
// arithmetic below measures only this soak window.
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

await page.evaluate(() => { startPlay(); });
// Let playback settle before the first sample so we don't capture the
// startup transient.
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

// Drive the param drag at PARAM_HZ for SOAK_SECONDS, taking memory samples
// every SAMPLE_EVERY_MS. Two params so the manager.params map exits its
// degenerate one-key state, mirroring a real user knob session.
const dragIntervalMs = Math.max(1, Math.floor(1000 / PARAM_HZ));
const totalIterations = Math.floor((SOAK_SECONDS * 1000) / dragIntervalMs);
const samplesEveryIter = Math.max(1, Math.floor(SAMPLE_EVERY_MS / dragIntervalMs));
let it = 0;
const startTs = Date.now();
for (it = 0; it < totalIterations; it++) {
  const t = it / PARAM_HZ;
  const pitch = -12 + 24 * Math.sin(t / 17);
  const decay = 0.25 + 0.75 * Math.sin(t / 41);
  // page.evaluate per iteration is slower than batching, but the goal is
  // to provoke the same Go-side per-Set work the user's drag triggers —
  // batching would let WASM coalesce and hide the regression. CHIP-STRIP:
  // also churn the expanded stage (VOICE↔ENVELOPE) every ~20 iterations so
  // the chip expand/collapse + per-stage knob (re)layout path runs under the
  // OOM Draw stack. Guarded so older builds without the export still run.
  const stage = (it % 40) < 20 ? "VOICE" : "ENVELOPE";
  await page.evaluate(({ p, d, s }) => {
    setInstrumentParam("kick", "pitch", p);
    setInstrumentParam("kick", "decay", d);
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

// Final sample taken after stopping playback so any pending highlight /
// scheduler retention drops out of the picture.
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
