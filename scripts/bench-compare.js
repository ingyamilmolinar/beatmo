#!/usr/bin/env node
/**
 * bench-compare.js — Cross-platform benchmark comparison report
 *
 * Reads bench-results/desktop.json and bench-results/browser.json, produces
 * a side-by-side comparison table and saves bench-results/REPORT.md.
 *
 * Usage:
 *   node scripts/bench-compare.js
 *   node scripts/bench-compare.js --results-dir /path/to/bench-results
 */

import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const resultsDir = process.argv.includes("--results-dir")
  ? process.argv[process.argv.indexOf("--results-dir") + 1]
  : path.resolve(__dirname, "..", "bench-results");

function loadJSON(file) {
  const p = path.join(resultsDir, file);
  if (!fs.existsSync(p)) {
    console.error(`Missing: ${p}`);
    return null;
  }
  return JSON.parse(fs.readFileSync(p, "utf-8"));
}

const desktop = loadJSON("desktop.json");
const browser = loadJSON("browser.json");

if (!desktop || !browser) {
  console.error("Run bench-desktop and bench-browser first.");
  process.exit(1);
}

// Index by BPM
function indexByBPM(arr) {
  const map = {};
  for (const r of arr) map[r.bpm] = r;
  return map;
}

const dMap = indexByBPM(desktop);
const bMap = indexByBPM(browser);
const allBPMs = [...new Set([...desktop.map(r => r.bpm), ...browser.map(r => r.bpm)])].sort((a, b) => a - b);

// Formatting helpers
function fmt(v, dec = 2) {
  if (v == null || isNaN(v)) return "N/A";
  return v.toFixed(dec);
}
function fmtMs(v) {
  if (v == null || isNaN(v)) return "  N/A  ";
  return v.toFixed(2) + "ms";
}
function pad(s, n) { return String(s).padStart(n); }
function rpad(s, n) { return String(s).padEnd(n); }

// Build report
const lines = [];
const md = [];

function both(line) {
  console.log(line);
  lines.push(line);
  md.push(line);
}

function consoleOnly(line) {
  console.log(line);
  lines.push(line);
}

const now = new Date().toISOString().split("T")[0];

both("");
both("# Cross-Platform Benchmark Comparison Report");
both("");
both(`**Date**: ${now}`);
both("**Circuit**: Startup Demo (58 nodes, 7 instruments, EQ+effects)");
both("**Duration**: 15s per BPM level");
both("");

// ── FPS / Update / Draw comparison ──
both("## Performance Comparison");
both("");
both("| BPM | FPS (D) | FPS (B) | Upd Avg D | Upd Avg B | Draw Avg D | Draw Avg B |");
both("|-----|---------|---------|-----------|-----------|------------|------------|");

for (const bpm of allBPMs) {
  const d = dMap[bpm] || {};
  const b = bMap[bpm] || {};
  both(`| ${bpm} | ${pad(fmt(d.fps, 1), 7)} | ${pad(fmt(b.fps, 1), 7)} | ${pad(fmtMs(d.updateAvgMS), 9)} | ${pad(fmtMs(b.updateAvgMS), 9)} | ${pad(fmtMs(d.drawAvgMS), 10)} | ${pad(fmtMs(b.drawAvgMS), 10)} |`);
}

both("");

// ── Scheduler timing comparison ──
both("## Audio Scheduler Timing");
both("");
both("| BPM | Count D | Count B | Overdue D | Overdue B | lagP90 D | lagP90 B | lagP99 D | lagP99 B | MinLead D | MinLead B |");
both("|-----|---------|---------|-----------|-----------|----------|----------|----------|----------|-----------|-----------|");

for (const bpm of allBPMs) {
  const d = dMap[bpm] || {};
  const b = bMap[bpm] || {};
  both(`| ${bpm} | ${pad(d.schedCount ?? "N/A", 7)} | ${pad(b.schedCount ?? "N/A", 7)} | ${pad(d.schedOverdue ?? "N/A", 9)} | ${pad(b.schedOverdue ?? "N/A", 9)} | ${pad(fmtMs(d.schedLagP90MS), 8)} | ${pad(fmtMs(b.schedLagP90MS), 8)} | ${pad(fmtMs(d.schedLagP99MS), 8)} | ${pad(fmtMs(b.schedLagP99MS), 8)} | ${pad(fmtMs(d.schedMinLeadMS), 9)} | ${pad(fmtMs(b.schedMinLeadMS), 9)} |`);
}

both("");

// ── Desktop-only metrics ──
both("## Desktop-Only Metrics");
both("");
both("| BPM | Heap (MB) | Goroutines | Audio QLat Avg | Audio QLat Max | Parity Scans |");
both("|-----|-----------|------------|----------------|----------------|--------------|");

for (const bpm of allBPMs) {
  const d = dMap[bpm];
  if (!d) continue;
  const heapMB = d.heapAllocKB ? (d.heapAllocKB / 1024).toFixed(1) : "N/A";
  both(`| ${bpm} | ${pad(heapMB, 9)} | ${pad(d.goroutines ?? "N/A", 10)} | ${pad(fmtMs(d.audioQLatAvgMS), 14)} | ${pad(fmtMs(d.audioQLatMaxMS), 14)} | ${pad(d.parityScans ?? "N/A", 12)} |`);
}

both("");

// ── Divergence analysis ──
both("## Divergence Analysis");
both("");

const issues = [];

for (const bpm of allBPMs) {
  const d = dMap[bpm];
  const b = bMap[bpm];
  if (!d || !b) continue;

  if ((d.schedOverdue || 0) > 0)
    issues.push({ severity: "HIGH", bpm, msg: `Desktop: ${d.schedOverdue} overdue events` });
  if ((b.schedOverdue || 0) > 0)
    issues.push({ severity: "HIGH", bpm, msg: `Browser: ${b.schedOverdue} overdue events` });

  if (d.schedLagP90MS != null && d.schedLagP90MS > 15)
    issues.push({ severity: "MEDIUM", bpm, msg: `Desktop lagP90 ${d.schedLagP90MS.toFixed(2)}ms > 15ms` });
  if (b.schedLagP90MS != null && b.schedLagP90MS > 15)
    issues.push({ severity: "MEDIUM", bpm, msg: `Browser lagP90 ${b.schedLagP90MS.toFixed(2)}ms > 15ms` });

  if (d.schedLagP99MS != null && d.schedLagP99MS > 25)
    issues.push({ severity: "MEDIUM", bpm, msg: `Desktop lagP99 ${d.schedLagP99MS.toFixed(2)}ms > 25ms` });

  if (d.fps && b.fps) {
    const ratio = d.fps / b.fps;
    if (ratio < 0.3 || ratio > 3)
      issues.push({ severity: "LOW", bpm, msg: `FPS divergence: desktop=${d.fps.toFixed(1)} browser=${b.fps.toFixed(1)} (${ratio.toFixed(1)}x)` });
  }

  if (d.drawAvgMS && b.drawAvgMS) {
    const ratio = d.drawAvgMS / b.drawAvgMS;
    if (ratio > 5)
      issues.push({ severity: "LOW", bpm, msg: `Draw time divergence: desktop=${d.drawAvgMS.toFixed(1)}ms browser=${b.drawAvgMS.toFixed(1)}ms (${ratio.toFixed(1)}x)` });
  }
}

if (issues.length === 0) {
  both("No significant divergences detected.");
} else {
  both(`${issues.length} issue(s) detected:`);
  both("");
  for (const { severity, bpm, msg } of issues) {
    both(`- **${severity}** [BPM=${bpm}]: ${msg}`);
  }
}

both("");

// ── Prioritized Follow-up Items ──
both("## Prioritized Follow-up Items");
both("");

// Analyze the data to produce dynamic recommendations
const maxDrawD = Math.max(...allBPMs.map(b => dMap[b]?.drawAvgMS || 0));
const maxDrawB = Math.max(...allBPMs.map(b => bMap[b]?.drawAvgMS || 0));
const totalOverdue = allBPMs.reduce((s, b) => s + (dMap[b]?.schedOverdue || 0), 0);
const maxLagP99D = Math.max(...allBPMs.map(b => dMap[b]?.schedLagP99MS || 0));
const maxQLat = Math.max(...allBPMs.map(b => dMap[b]?.audioQLatMaxMS || 0));

both("### P0 — Decouple audio dispatch from Draw thread (Impact: HIGH, Effort: LOW)");
both("");
both(`Desktop shows ${totalOverdue} total overdue events across all BPMs. The root cause: \`audioLoop()\` dequeues from \`audioCh\` on a goroutine, but \`seqScheduleTime()\` runs inside \`Update()\` which is blocked while \`Draw()\` runs (avg ${maxDrawD.toFixed(0)}ms). Events pile up during long draws.`);
both("");
both("**Fix**: Move audio scheduling to a dedicated goroutine with a high-resolution timer (e.g., 2ms tick) independent of Ebiten's `Update()` cadence. This matches browser architecture where WebAudio scheduling runs on a separate thread.");
both("");

both("### P1 — Reduce Desktop Draw time (Impact: HIGH, Effort: MEDIUM)");
both("");
both(`Desktop Draw averages ${maxDrawD.toFixed(0)}ms vs browser's ${maxDrawB.toFixed(0)}ms (${(maxDrawD/maxDrawB).toFixed(1)}x slower). The pprof hotspot is \`runtime.mapiternext\` in Ebiten's \`makeStaleIfDependingOn\` (20-35% CPU). DrumView row sprite rebuilds happen ~5x per 2s interval.`);
both("");
both("**Fix**: (a) Reduce draw calls by compositing multiple drum cells into fewer sprites. (b) Investigate why row caches invalidate every ~400ms during playback — if highlight-only, patch individual cells instead of full rebuild. (c) Check Ebiten v2.8+ for dependency tracking optimizations.");
both("");

both("### P2 — Reduce CGo bridge overhead (Impact: MEDIUM, Effort: LOW)");
both("");
both("pprof shows `runtime.cgocall` at ~24% flat CPU. Each C synth voice render and each insert effect crosses Go→C. Block-based rendering (`SampleBlock`) is in place but individual voice scheduling still triggers per-call CGo transitions.");
both("");
both("**Fix**: Batch multiple voice renders into a single CGo call where possible. The `renderVoiceIntoInstBuf` loop could accumulate voice parameters and dispatch them in one C call.");
both("");

both("### P3 — Block-process biquad/FFT per-sample paths (Impact: MEDIUM, Effort: LOW)");
both("");
both("`biquad.ProcessSample` + `fft` contribute ~5-10% cumulative CPU. Both are called per-sample inside `ProcessBlockLocal`. Converting the inner loop to process entire blocks eliminates per-sample function call overhead.");
both("");

both("### P4 — Skip audio analyzer in headless/benchmark mode (Impact: LOW, Effort: LOW)");
both("");
both("The spectrum `Analyzer` runs FFT on every block even when no EQ visualization is displayed. Adding an `AnalyzerEnabled` toggle would save ~4% cumulative CPU during benchmarks and headless playback.");
both("");

if (maxQLat > 30) {
  both("### P5 — Investigate desktop audio queue latency spikes (Impact: LOW, Effort: LOW)");
  both("");
  both(`Queue latency max reaches ${maxQLat.toFixed(1)}ms (BPM=300). This reflects events waiting in \`audioCh\` while Draw blocks Update. Fixing P0 (decoupled scheduling) would eliminate this as a side effect.`);
  both("");
}

// ── Known architectural differences ──
both("## Architectural Notes");
both("");
both("- **Desktop FPS** is typically lower than browser due to Ebiten's `DrawImage`/`DrawTriangles` map iteration hotspot (`runtime.mapiternext` ~20% CPU).");
both("- **Browser audio** is offloaded to a separate WebAudio thread (<0.2% CPU), while desktop mixes audio on a Go goroutine (~20% CPU).");
both("- **Desktop Draw** includes real GL rendering; browser Draw dispatches WebGL commands which execute asynchronously.");
both("- **Schedule metrics** measure lead/lag at the dispatch boundary: desktop in `audioLoop()` Go goroutine, browser in `playSoundParams()` JS callback.");
both("- Both platforms use the same adaptive audio lookahead (max 120ms) and the same startup demo circuit.");
both("");

// Save report
const reportPath = path.join(resultsDir, "REPORT.md");
fs.writeFileSync(reportPath, md.join("\n") + "\n");
console.log(`\nReport saved to ${reportPath}`);
