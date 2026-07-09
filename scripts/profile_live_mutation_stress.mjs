// profile_live_mutation_stress.mjs
//
// Live-mutation audio-jitter profiler for the WASM build. Extends
// scripts/profile_fx_chain_playback.mjs (which only mutates the circuit BEFORE
// playback) to drive the mutations users actually perform DURING playback:
// BPM changes, node-logic/graph edits, insert-effect add/remove, synth-param
// knob turns, and EQ band moves. It answers two questions:
//
//   1. BASELINE — is plain playback (no mutations, no FX) already choppy?
//      Measured by Stage A seqFireLate, draw cost split (grid vs drum), audio
//      clock drift, and queue latency while just playing the startup demo.
//
//   2. WHICH live mutation hurts? — during a second playback window we fire one
//      mutation type per "tick" on a rotating schedule and tag each per-second
//      sample with the mutation that ran in it, so a draw/Stage-A/qlat spike
//      can be attributed to BPM vs graph-edit vs FX-add vs synth vs EQ.
//
// Rich per-second metrics (beyond the FX profiler): drawGridMS / drawDrumMS
// split, rowsRepaints / rowCacheFull / gridTileBlits / imagesAllocatedTotal
// deltas (the Ebiten draw-churn the prior investigation fingered), heap + GC.
//
// CAVEAT (carried from the prior investigation): headless Chromium uses
// SwiftShader, which inflates Ebiten draw ~4-5x and whose audio thread is too
// fast to underrun — so absolute drift/click numbers do NOT match real HW. The
// RELATIVE attribution (which phase / which mutation spikes draw + Stage A)
// holds on any platform. For deterministic audio-thread cost use
// scripts/bench_audio_node_cost.mjs instead.
//
// Run:
//   WASM_PREBUILT=1 GO=$(pwd)/.tools/go/bin/go node scripts/profile_live_mutation_stress.mjs
// Env:
//   MUT_BASELINE_SECS=14   plain-playback baseline window
//   MUT_LIVE_SECS=24       live-mutation window
//   MUT_BPM=200            base tempo
//   MUT_INTERVAL_MS=700    gap between live mutations
//   MUT_PROFILE=1          capture a CDP main-thread CPU profile per phase

import { chromium } from "../src/js/node_modules/playwright/index.mjs";
import path from "node:path";
import http from "node:http";
import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(__dirname, "..");
const jsroot = path.join(ROOT, "src/js");

const BASELINE_SECS = Number(process.env.MUT_BASELINE_SECS ?? 14);
const LIVE_SECS = Number(process.env.MUT_LIVE_SECS ?? 24);
const BPM = Number(process.env.MUT_BPM ?? 200);
const INTERVAL_MS = Number(process.env.MUT_INTERVAL_MS ?? 700);
const DO_PROFILE = process.env.MUT_PROFILE !== "0";

const GO = process.env.GO;
if (!GO) { console.error("Set GO=/path/to/.tools/go/bin/go"); process.exit(2); }

const ts = new Date().toISOString().replace(/[:.]/g, "-").replace("T", "_").slice(0, 19);
const outdir = path.join(ROOT, "bench-results", `live-mutation-${ts}`);
fs.mkdirSync(outdir, { recursive: true });
console.log(`[live-mut] outdir=${outdir}`);

const wasmPath = path.join(jsroot, "main.wasm");
if (process.env.WASM_PREBUILT !== "1" || !fs.existsSync(wasmPath)) {
  console.log("[live-mut] building main.wasm ...");
  const b = spawnSync(GO, ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", wasmPath, "./cmd"], {
    cwd: path.join(ROOT, "src/go"),
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  });
  if (b.status !== 0) { console.error("wasm build failed"); process.exit(1); }
}

const server = http.createServer((req, res) => {
  const fp = path.join(jsroot, req.url === "/" ? "index.html" : req.url.replace(/^\//, ""));
  if (!fs.existsSync(fp)) { res.writeHead(404); res.end(); return; }
  const ct = { ".html": "text/html", ".js": "application/javascript", ".wasm": "application/wasm", ".json": "application/json" }[path.extname(fp)] || "application/octet-stream";
  res.writeHead(200, { "Content-Type": ct });
  fs.createReadStream(fp).pipe(res);
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

// Snapshot of the cumulative perf counters we track as per-second deltas.
// IIFE string: page.evaluate(string) evaluates the expression, so an IIFE
// returns the object (a bare `() => ({...})` string would return the function).
const sampleExpr = `(() => ({
  wall: performance.now(),
  audio: window.__audioCtx ? window.__audioCtx.currentTime : null,
  perf: perfStats(),
}))()`;

function deltaRow(sec, cur, prev, tag) {
  const wallDelta = (cur.wall - prev.wall) / 1000;
  const audioDelta = prev.audio != null && cur.audio != null ? cur.audio - prev.audio : null;
  const drift = audioDelta != null && wallDelta > 0 ? (wallDelta - audioDelta) / wallDelta : null;
  const p = cur.perf, pp = prev.perf;
  return {
    sec,
    tag: tag || "",
    driftPct: drift != null ? +(drift * 100).toFixed(2) : null,
    drawAvg: +(p.drawAvgMS ?? 0).toFixed(1),
    drawMax: +(p.drawMaxMS ?? 0).toFixed(1),
    drawGrid: +(p.drawGridMS ?? 0).toFixed(1),
    drawDrum: +(p.drawDrumMS ?? 0).toFixed(1),
    updMax: +(p.updateMaxMS ?? 0).toFixed(1),
    qlatMax: +(p.audioQLatMax ?? 0).toFixed(2),
    repaints: (p.rowsRepaints ?? 0) - (pp.rowsRepaints ?? 0),
    cacheFull: (p.rowCacheFull ?? 0) - (pp.rowCacheFull ?? 0),
    tileBlits: Math.round((p.gridTileBlits ?? 0) - (pp.gridTileBlits ?? 0)),
    imgAlloc: Math.round((p.imagesAllocatedTotal ?? 0) - (pp.imagesAllocatedTotal ?? 0)),
    heapMB: Math.round((p.heapAllocKB ?? 0) / 1024),
    gor: p.goroutines,
  };
}

function summarizeFinal(f) {
  const s = f.sched || {}, t = f.threeStage || {};
  return {
    schedCount: s.count, overdue: s.overdue, smallLead: s.smallLeadCount,
    minLeadMs: s.minLead != null ? +(s.minLead * 1000).toFixed(2) : null,
    qlatMaxMs: f.perf?.audioQLatMax,
    drawAvgMs: +(f.perf?.drawAvgMS ?? 0).toFixed(2), drawMaxMs: +(f.perf?.drawMaxMS ?? 0).toFixed(2),
    stageA_p99Ms: t.seqFireLate?.p99 != null ? +(t.seqFireLate.p99 * 1000).toFixed(1) : null,
    stageA_maxMs: t.seqFireLate?.max != null ? +(t.seqFireLate.max * 1000).toFixed(1) : null,
    stageC_maxLagMs: t.lead?.maxLag != null ? +(t.lead.maxLag * 1000).toFixed(2) : null,
  };
}

function topSelfTime(profile, topN = 18) {
  if (!profile || !profile.nodes) return [];
  const selfByNode = new Map();
  for (const n of profile.nodes) {
    const cf = n.callFrame;
    const name = (cf.functionName || "(anonymous)") + (cf.url ? ` @ ${cf.url.split("/").pop()}:${cf.lineNumber}` : "");
    selfByNode.set(name, (selfByNode.get(name) || 0) + (n.hitCount || 0));
  }
  const total = profile.samples ? profile.samples.length : [...selfByNode.values()].reduce((a, b) => a + b, 0);
  const intervalUs = profile.timeDeltas?.length ? profile.timeDeltas.reduce((a, b) => a + b, 0) / profile.timeDeltas.length : 200;
  return [...selfByNode.entries()]
    .map(([name, hits]) => ({ name, pct: +((hits / total) * 100).toFixed(2), ms: +((hits * intervalUs) / 1000).toFixed(1) }))
    .sort((a, b) => b.pct - a.pct).slice(0, topN);
}

function printRun(label, rows, final, top) {
  console.log(`\n=== ${label} ===`);
  console.log("sec  tag        drift% drawAvg drawMax drawGrid drawDrum updMax qlat repaint cFull blits imgAl heapMB gor");
  for (const r of rows) {
    console.log(
      `${String(r.sec).padStart(3)}  ${String(r.tag).padEnd(10)} ${String(r.driftPct).padStart(6)} ${String(r.drawAvg).padStart(7)} ${String(r.drawMax).padStart(7)} ${String(r.drawGrid).padStart(8)} ${String(r.drawDrum).padStart(8)} ${String(r.updMax).padStart(6)} ${String(r.qlatMax).padStart(4)} ${String(r.repaints).padStart(7)} ${String(r.cacheFull).padStart(5)} ${String(r.tileBlits).padStart(5)} ${String(r.imgAlloc).padStart(5)} ${String(r.heapMB).padStart(6)} ${String(r.gor).padStart(3)}`,
    );
  }
  console.log("final:", JSON.stringify(final));
  if (top) { console.log("top self-time (main thread):"); for (const t of top) console.log(`  ${String(t.pct).padStart(5)}%  ${t.ms}ms  ${t.name}`); }
}

const browser = await chromium.launch({ headless: true, args: ["--autoplay-policy=no-user-gesture-required", "--enable-precise-memory-info"] });
const page = await browser.newPage();
const cdp = await page.context().newCDPSession(page);
await cdp.send("Profiler.enable");
const consoleLines = [];
page.on("console", (m) => consoleLines.push(m.text()));

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function" && typeof exportJSON === "function");
await page.waitForTimeout(400);

// Discover circuit topology for live edits: instrument ids + a free roaming
// cell + an anchor node to attach edges to.
const topo = await page.evaluate(() => {
  const doc = JSON.parse(exportJSON());
  const ids = (doc.instruments || []).map((i) => i.id).filter((x) => x && x !== "master");
  // Occupied cells from node grid coords.
  const occ = new Set((doc.nodes || []).map((n) => `${n.i},${n.j}`));
  let anchor = null;
  for (const n of doc.nodes || []) { if (n.i != null && n.j != null) { anchor = { i: n.i, j: n.j }; break; } }
  return { ids, anchor, occupied: [...occ] };
});
console.log(`[live-mut] instruments=${JSON.stringify(topo.ids)} anchor=${JSON.stringify(topo.anchor)} nodes=${topo.occupied.length}`);

async function playPhase(label, secs, mutate, withProfile) {
  if (withProfile) { await cdp.send("Profiler.setSamplingInterval", { interval: 200 }); await cdp.send("Profiler.start"); }
  const t0 = await page.evaluate((bpm) => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
    resetPerfStats?.(); resetAudioScheduleMetrics?.(); resetThreeStageLatency?.();
    setBPM(bpm); startPlay();
    return null;
  }, BPM);
  let prev = await page.evaluate(sampleExpr);
  const rows = [];
  let mutTimer = 0;
  for (let s = 0; s < secs; s++) {
    // Within each 1s window, fire mutations on the INTERVAL_MS cadence.
    let tagThisSec = "";
    const slices = Math.max(1, Math.round(1000 / INTERVAL_MS));
    for (let k = 0; k < slices; k++) {
      await page.waitForTimeout(Math.round(1000 / slices));
      if (mutate) { const t = await mutate(mutTimer++); if (t) tagThisSec = t; }
    }
    const cur = await page.evaluate(sampleExpr);
    rows.push(deltaRow(s + 1, cur, prev, tagThisSec));
    prev = cur;
  }
  const final = await page.evaluate(() => ({
    perf: perfStats(),
    sched: getAudioScheduleMetrics(),
    threeStage: typeof getThreeStageLatency === "function" ? getThreeStageLatency() : null,
  }));
  await page.evaluate(() => stopPlay());
  let profile = null;
  if (withProfile) profile = (await cdp.send("Profiler.stop")).profile;
  return { rows, final: summarizeFinal(final), top: profile ? topSelfTime(profile) : null, profile };
}

const report = { ts, bpm: BPM, baselineSecs: BASELINE_SECS, liveSecs: LIVE_SECS, intervalMs: INTERVAL_MS, phases: {} };

// ── Phase 1: baseline plain playback, no mutations ──
console.log(`\n[live-mut] BASELINE plain playback (${BASELINE_SECS}s @ ${BPM}bpm)...`);
const baseline = await playPhase("baseline", BASELINE_SECS, null, DO_PROFILE);
report.phases.baseline = { rows: baseline.rows, final: baseline.final, top: baseline.top };
printRun("baseline (no mutations)", baseline.rows, baseline.final, baseline.top);
if (baseline.profile) fs.writeFileSync(path.join(outdir, "baseline.cpuprofile"), JSON.stringify(baseline.profile));

// ── Phase 2: live mutations during playback ──
// Rotating mutation schedule. Each returns a short tag for the per-second log.
const MUT_TYPES = ["bpm", "graph", "fx", "synth", "eq"];
const synthKeys = ["brightness", "drive", "body"];
let roamN = 0, lastRoamCell = null, lastFxByInst = {};
async function liveMutate(n) {
  const kind = MUT_TYPES[n % MUT_TYPES.length];
  await page.evaluate(({ kind, n, ids, anchor, synthKeys, BPM }) => {
    switch (kind) {
      case "bpm":
        setBPM(BPM + ((n % 5) * 16)); // sweep 200..264
        break;
      case "graph": {
        // Roam a free cell far from the demo: add a node + edge from anchor,
        // delete the previous one → bounded graph churn → predictor rebuild.
        const i = 6 + (n % 7), j = 9 + ((n >> 1) % 5);
        if (window.__lastRoam) deleteNodeGrid(window.__lastRoam.i, window.__lastRoam.j);
        addNode(i, j, "regular");
        if (anchor) addEdgeGrid(anchor.i, anchor.j, i, j);
        setNodeLogicGrid(i, j, (n % 2 ? "every_n" : "probability"), 2 + (n % 3), 0.5);
        window.__lastRoam = { i, j };
        break;
      }
      case "fx": {
        const id = ids[n % ids.length];
        const pool = ["distortion", "delay", "reverb", "chorus", "filter", "compressor"];
        window.__fxByInst = window.__fxByInst || {};
        // Remove prior slot on this inst to keep chain bounded, then add one.
        if (window.__fxByInst[id] != null) removeInsertEffect(id, window.__fxByInst[id]);
        const slot = addInsertEffect(id, pool[n % pool.length]);
        window.__fxByInst[id] = slot;
        break;
      }
      case "synth": {
        const id = ids[n % ids.length];
        const k = synthKeys[n % synthKeys.length];
        setInstrumentParam(id, k, (n % 10) / 10);
        break;
      }
      case "eq":
        setEQBandGain("main", n % 10, ((n % 7) - 3) * 2);
        break;
    }
  }, { kind, n, ids: topo.ids, anchor: topo.anchor, synthKeys, BPM });
  return kind;
}

console.log(`\n[live-mut] LIVE MUTATIONS during playback (${LIVE_SECS}s, one mutation / ${INTERVAL_MS}ms)...`);
const live = await playPhase("live", LIVE_SECS, liveMutate, DO_PROFILE);
report.phases.live = { rows: live.rows, final: live.final, top: live.top };
printRun("live mutations", live.rows, live.final, live.top);
if (live.profile) fs.writeFileSync(path.join(outdir, "live.cpuprofile"), JSON.stringify(live.profile));

// Per-mutation-type aggregation: mean draw/Stage proxies grouped by tag.
const byTag = {};
for (const r of live.rows) {
  if (!r.tag) continue;
  (byTag[r.tag] = byTag[r.tag] || []).push(r);
}
const tagAgg = {};
for (const [tag, rs] of Object.entries(byTag)) {
  const mean = (f) => +(rs.reduce((a, r) => a + (r[f] || 0), 0) / rs.length).toFixed(1);
  const max = (f) => Math.max(...rs.map((r) => r[f] || 0));
  tagAgg[tag] = { n: rs.length, drawAvg: mean("drawAvg"), drawMax: max("drawMax"), qlatMax: max("qlatMax"), repaints: mean("repaints"), tileBlits: mean("tileBlits"), imgAlloc: mean("imgAlloc"), driftMax: max("driftPct") };
}
console.log("\n=== per-mutation-type aggregation (live phase) ===");
console.log(JSON.stringify(tagAgg, null, 2));
report.phases.live.tagAgg = tagAgg;

fs.writeFileSync(path.join(outdir, "report.json"), JSON.stringify(report, null, 2));
fs.writeFileSync(path.join(outdir, "browser_console.log"), consoleLines.join("\n"));
await browser.close();
server.close();
console.log(`\n[live-mut] wrote ${path.join(outdir, "report.json")}`);
