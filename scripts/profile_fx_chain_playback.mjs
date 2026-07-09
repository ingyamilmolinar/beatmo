// profile_fx_chain_playback.mjs
//
// Diagnostic profiler for the "choppy/inconsistent audio under heavy FX load"
// investigation (see src/js/webaudio_fx_chain_stress.browser.test.js, which is
// the failing reproducer). The stress test fails INTERMITTENTLY — clicks and
// queue-latency spikes appear on some runs, not all. This script gathers the
// evidence to localize that transient:
//
//   1. Builds the heavy circuit (every instrument: 3 insert effects, shaped
//      EQ + HPF + LPF, delay/reverb sends, all modular synth stages incl. the
//      default-OFF PITCH/LFO/BURST modulators).
//   2. Plays for PROFILE_SECS, sampling a TIME SERIES every second:
//        - wall-clock vs AudioContext.currentTime  → per-second audio-thread
//          realtime drift (the smoking gun for render-thread underrun)
//        - perfStats: drawAvg/drawMax, updateMax, audioQLatMax, GC counters,
//          heapAlloc, goroutines
//        - getAudioScheduleMetrics history (per-second lead/overdue)
//        - getThreeStageLatency (Stage A seqFire / B bridge / C lead+lag)
//   3. Captures a CDP main-thread CPU profile across the whole window and
//      aggregates SELF time by function so we can see whether Draw, the
//      Go→JS bridge, or GC dominates the main thread.
//   4. (Optional) A/B sweep: re-run the same circuit with components removed
//      one at a time (PROFILE_AB=1) to attribute drift/clicks to a component.
//
// CDP's CPU profiler samples the MAIN thread only — AudioWorklet render
// threads (the per-channel insert-FX worklets + the recording-capture tap)
// are NOT in the profile. The per-second drift series is the proxy for
// audio-thread health: when the worklets can't render realtime, ctx.currentTime
// falls behind wall-clock and that second's drift spikes.
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node scripts/profile_fx_chain_playback.mjs
// Env:
//   PROFILE_SECS=25     playback window
//   PROFILE_BPM=200     tempo
//   PROFILE_AB=1        also run the component-isolation A/B sweep
//   PROFILE_RECORD=1    also run the recording capture tap during the main run

import { chromium } from "../src/js/node_modules/playwright/index.mjs";
import path from "node:path";
import http from "node:http";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(__dirname, "..");
const jsroot = path.join(ROOT, "src/js");

const SECS = Number(process.env.PROFILE_SECS ?? 25);
const BPM = Number(process.env.PROFILE_BPM ?? 200);
const AB = process.env.PROFILE_AB === "1";
const RECORD = process.env.PROFILE_RECORD === "1";

const GO = process.env.GO;
if (!GO) {
  console.error("Set GO=/path/to/.tools/go/bin/go");
  process.exit(2);
}

const ts = new Date().toISOString().replace(/[:.]/g, "-").replace("T", "_").slice(0, 19);
const outdir = path.join(ROOT, "bench-results", `fx-chain-profile-${ts}`);
fs.mkdirSync(outdir, { recursive: true });
console.log(`[profile] outdir=${outdir}`);

// Build WASM if missing.
const { spawnSync } = await import("node:child_process");
const wasmPath = path.join(jsroot, "main.wasm");
if (process.env.WASM_PREBUILT !== "1" || !fs.existsSync(wasmPath)) {
  console.log("[profile] building main.wasm ...");
  const b = spawnSync(GO, ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", wasmPath, "./cmd"], {
    cwd: path.join(ROOT, "src/go"),
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  });
  if (b.status !== 0) { console.error("wasm build failed"); process.exit(1); }
}

const server = http.createServer((req, res) => {
  let fp = path.join(jsroot, req.url === "/" ? "index.html" : req.url.replace(/^\//, ""));
  if (!fs.existsSync(fp)) { res.writeHead(404); res.end(); return; }
  const ct = { ".html": "text/html", ".js": "application/javascript", ".wasm": "application/wasm", ".json": "application/json" }[path.extname(fp)] || "application/octet-stream";
  res.writeHead(200, { "Content-Type": ct });
  fs.createReadStream(fp).pipe(res);
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

// Circuit builder injected into the page. components controls A/B variants.
const buildCircuit = `(components) => {
  const doc = JSON.parse(exportJSON());
  const fxPool = [
    ["distortion","delay","reverb"], ["chorus","filter","compressor"],
    ["phaser","flanger","tremolo"], ["bitcrusher","tape","limiter"],
  ];
  const eqGains = [3,-2,2,-1,1,2,-2,2,-3,2];
  doc.instruments = (doc.instruments||[]).map((inst,i) => {
    const out = Object.assign({}, inst);
    if (components.fx) out.effects = fxPool[i%fxPool.length].map(type => ({type, enabled:true}));
    if (components.eq) out.eq = { gains_db: eqGains, hpf_enabled:true, hpf_cutoff_hz:60, lpf_enabled:true, lpf_cutoff_hz:12000 };
    if (components.sends) { out.pan = (i%2?-1:1)*0.4; out.delay_send=0.25; out.reverb_send=0.25; }
    if (components.synth) out.synth_params = Object.assign({}, inst.synth_params||{}, {
      osc_enabled:1, fm_enabled:1, env_enabled:1, filter_enabled:1, drive_enabled:1, post_enabled:1,
      pitchenv_enabled:1, pitchenv_amt:0.4, pitchenv_decay:0.3,
      lfo_enabled:1, lfo_rate:4, lfo_depth:0.3, burst_enabled:1, burst_sharp:0.5,
    });
    return out;
  });
  if (components.eq) doc.eq = Object.assign({}, doc.eq||{}, { gains_db: eqGains });
  return importJSON(JSON.stringify(doc));
}`;

async function runVariant(page, cdp, label, components, withProfile) {
  const importErr = await page.evaluate(`(${buildCircuit})(${JSON.stringify(components)})`);
  if (importErr !== "") throw new Error(`[${label}] import failed: ${importErr}`);
  await page.evaluate(() => { forceDraw?.(); forceDraw?.(); });
  await page.waitForTimeout(600);

  if (withProfile) { await cdp.send("Profiler.setSamplingInterval", { interval: 200 }); await cdp.send("Profiler.start"); }

  const t0 = await page.evaluate((bpm) => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
    resetPerfStats?.(); resetAudioScheduleMetrics?.(); resetThreeStageLatency?.();
    setBPM(bpm); startPlay();
    return { wall: performance.now(), audio: window.__audioCtx ? window.__audioCtx.currentTime : null };
  }, BPM);

  const series = [];
  let prev = t0;
  for (let s = 0; s < SECS; s++) {
    await page.waitForTimeout(1000);
    const sample = await page.evaluate(() => ({
      wall: performance.now(),
      audio: window.__audioCtx ? window.__audioCtx.currentTime : null,
      perf: perfStats(),
    }));
    const wallDelta = (sample.wall - prev.wall) / 1000;
    const audioDelta = prev.audio != null && sample.audio != null ? sample.audio - prev.audio : null;
    const drift = audioDelta != null ? (wallDelta - audioDelta) / wallDelta : null;
    series.push({
      sec: s + 1,
      driftPct: drift != null ? +(drift * 100).toFixed(2) : null,
      drawAvg: +sample.perf.drawAvgMS?.toFixed(1),
      drawMax: +sample.perf.drawMaxMS?.toFixed(1),
      updMax: +sample.perf.updateMaxMS?.toFixed(1),
      qlatMax: +sample.perf.audioQLatMax?.toFixed(2),
      heapMB: Math.round((sample.perf.heapAllocKB || 0) / 1024),
      gor: sample.perf.goroutines,
    });
    prev = sample;
  }

  const final = await page.evaluate(() => ({
    perf: perfStats(),
    sched: getAudioScheduleMetrics(),
    threeStage: typeof getThreeStageLatency === "function" ? getThreeStageLatency() : null,
  }));
  await page.evaluate(() => stopPlay());

  let profile = null;
  if (withProfile) { profile = (await cdp.send("Profiler.stop")).profile; }
  return { label, components, series, final, profile };
}

// Aggregate a CDP CPU profile into self-time-by-function (top N).
function topSelfTime(profile, topN = 30) {
  if (!profile || !profile.nodes) return [];
  const selfByNode = new Map();
  const idToNode = new Map();
  for (const n of profile.nodes) idToNode.set(n.id, n);
  // hitCount = samples whose leaf is this node = self time.
  for (const n of profile.nodes) {
    const cf = n.callFrame;
    const name = (cf.functionName || "(anonymous)") + (cf.url ? ` @ ${cf.url.split("/").pop()}:${cf.lineNumber}` : "");
    selfByNode.set(name, (selfByNode.get(name) || 0) + (n.hitCount || 0));
  }
  const totalSamples = profile.samples ? profile.samples.length : [...selfByNode.values()].reduce((a, b) => a + b, 0);
  const intervalUs = profile.timeDeltas && profile.timeDeltas.length
    ? profile.timeDeltas.reduce((a, b) => a + b, 0) / profile.timeDeltas.length
    : 200;
  return [...selfByNode.entries()]
    .map(([name, hits]) => ({ name, hits, pct: +((hits / totalSamples) * 100).toFixed(2), ms: +((hits * intervalUs) / 1000).toFixed(1) }))
    .sort((a, b) => b.hits - a.hits)
    .slice(0, topN);
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

const report = { ts, bpm: BPM, secs: SECS, runs: [] };

// Main heavy run with CPU profile.
const heavy = { fx: true, eq: true, sends: true, synth: true };
console.log(`[profile] HEAVY run (${SECS}s @ ${BPM}bpm, profiled)...`);
const heavyRun = await runVariant(page, cdp, "heavy", heavy, true);
const heavyTop = topSelfTime(heavyRun.profile);
fs.writeFileSync(path.join(outdir, "heavy_cpuprofile.cpuprofile"), JSON.stringify(heavyRun.profile));
report.runs.push({ label: "heavy", series: heavyRun.series, final: summarizeFinal(heavyRun.final), topSelfTime: heavyTop });
printRun(heavyRun, heavyTop);

if (AB) {
  const variants = [
    ["baseline", { fx: false, eq: false, sends: false, synth: false }],
    ["fx-only", { fx: true, eq: false, sends: false, synth: false }],
    ["eq-only", { fx: false, eq: true, sends: false, synth: false }],
    ["synth-only", { fx: false, eq: false, sends: false, synth: true }],
    ["sends-only", { fx: false, eq: false, sends: true, synth: false }],
  ];
  for (const [label, comp] of variants) {
    console.log(`[profile] A/B ${label}...`);
    const run = await runVariant(page, cdp, label, comp, false);
    report.runs.push({ label, series: run.series, final: summarizeFinal(run.final) });
    printRun(run, null);
  }
}

fs.writeFileSync(path.join(outdir, "report.json"), JSON.stringify(report, null, 2));
fs.writeFileSync(path.join(outdir, "browser_console.log"), consoleLines.join("\n"));
await browser.close();
server.close();
console.log(`\n[profile] wrote ${path.join(outdir, "report.json")} and heavy_cpuprofile.cpuprofile`);

function summarizeFinal(f) {
  const s = f.sched || {}, t = f.threeStage || {};
  return {
    schedCount: s.count, overdue: s.overdue, smallLead: s.smallLeadCount, minLeadMs: round(s.minLead, 1000),
    qlatMaxMs: f.perf?.audioQLatMax,
    drawAvgMs: round(f.perf?.drawAvgMS, 1, 1), drawMaxMs: round(f.perf?.drawMaxMS, 1, 1),
    stageA_p99Ms: t.seqFireLate ? round(t.seqFireLate.p99, 1000) : null,
    stageA_maxMs: t.seqFireLate ? round(t.seqFireLate.max, 1000) : null,
    stageB_p99Ms: t.bridge ? round(t.bridge.p99, 1000) : null,
    stageB_maxMs: t.bridge ? round(t.bridge.max, 1000) : null,
    stageC_maxLagMs: t.lead ? round(t.lead.maxLag, 1000) : null,
  };
}
function round(v, scale = 1, dp = 2) { return v == null ? null : +(v * scale).toFixed(dp); }
function printRun(run, top) {
  console.log(`\n=== ${run.label} ===`);
  console.log("sec  drift%  drawAvg drawMax updMax qlatMax heapMB gor");
  for (const r of run.series) {
    console.log(`${String(r.sec).padStart(3)}  ${String(r.driftPct).padStart(6)}  ${String(r.drawAvg).padStart(7)} ${String(r.drawMax).padStart(7)} ${String(r.updMax).padStart(6)} ${String(r.qlatMax).padStart(7)} ${String(r.heapMB).padStart(6)} ${String(r.gor).padStart(3)}`);
  }
  console.log("final:", JSON.stringify(summarizeFinal(run.final)));
  if (top) {
    console.log("top self-time (main thread):");
    for (const t of top.slice(0, 18)) console.log(`  ${String(t.pct).padStart(5)}%  ${t.ms}ms  ${t.name}`);
  }
}
