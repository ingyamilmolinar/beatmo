// profile_chain_tab.mjs
//
// Focused profiler for the "audio is choppy/laggy while the Chain (Chn) tab is
// open" report. The experiment isolates a SINGLE variable: which audio-panel
// tab is active. The circuit, BPM, playback duration, and everything else are
// held identical across variants, so any difference in per-frame Draw cost and
// Stage-A sequencer lateness is attributable to the active tab's renderer.
//
// On the single cooperatively-scheduled WASM thread, Ebiten Draw and the
// sequencer goroutine share one thread, so a tab whose Draw is expensive
// starves the sequencer → late audio → choppy. We therefore compare drawAvg/
// drawMax AND Stage-A seqFireLate per tab, and capture a CDP main-thread CPU
// profile while the Chain tab is active to see WHICH draw function dominates.
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node scripts/profile_chain_tab.mjs
// Env:
//   PROFILE_SECS=12   playback window per tab
//   PROFILE_BPM=160   tempo
//   TABS=wave,chain   comma list of tabs to compare (default below)
//   PROFILE_TAB=chain which tab gets the CPU profile (default chain)

import { chromium } from "../src/js/node_modules/playwright/index.mjs";
import path from "node:path";
import http from "node:http";
import fs from "node:fs";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const ROOT = path.join(__dirname, "..");
const jsroot = path.join(ROOT, "src/js");

const SECS = Number(process.env.PROFILE_SECS ?? 12);
const BPM = Number(process.env.PROFILE_BPM ?? 160);
const TABS = (process.env.TABS ?? "wave,spectrum,levels,chain,synth").split(",").map((s) => s.trim()).filter(Boolean);
const PROFILE_TAB = process.env.PROFILE_TAB ?? "chain";

const GO = process.env.GO;
if (!GO) { console.error("Set GO=/path/to/.tools/go/bin/go"); process.exit(2); }

const ts = new Date().toISOString().replace(/[:.]/g, "-").replace("T", "_").slice(0, 19);
const outdir = path.join(ROOT, "bench-results", `chain-tab-profile-${ts}`);
fs.mkdirSync(outdir, { recursive: true });
console.log(`[chain-profile] outdir=${outdir}`);

const wasmPath = path.join(jsroot, "main.wasm");
if (process.env.WASM_PREBUILT !== "1" || !fs.existsSync(wasmPath)) {
  console.log("[chain-profile] building main.wasm ...");
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

async function runTab(page, cdp, tab, withProfile) {
  // Switch to the tab BEFORE measuring; force a couple of draws so its layout
  // settles and the analyzer dispatcher enables the right analyzers.
  await page.evaluate((t) => { setEQTab?.(t); forceDraw?.(); forceDraw?.(); }, tab);
  await page.waitForTimeout(500);

  if (withProfile) { await cdp.send("Profiler.setSamplingInterval", { interval: 150 }); await cdp.send("Profiler.start"); }

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
    const drift = audioDelta != null && wallDelta > 0 ? (wallDelta - audioDelta) / wallDelta : null;
    const frameDelta = (sample.perf.frames || 0) - (prev.perf?.frames || 0);
    const callDelta = (sample.perf.drawCalls || 0) - (prev.perf?.drawCalls || 0);
    const callsPerFrame = frameDelta > 0 ? Math.round(callDelta / frameDelta) : null;
    series.push({
      sec: s + 1,
      driftPct: drift != null ? +(drift * 100).toFixed(2) : null,
      drawAvg: +sample.perf.drawAvgMS?.toFixed(1),
      drawMax: +sample.perf.drawMaxMS?.toFixed(1),
      callsPF: callsPerFrame,
      updMax: +sample.perf.updateMaxMS?.toFixed(1),
      qlatMax: +sample.perf.audioQLatMax?.toFixed(2),
      heapMB: Math.round((sample.perf.heapAllocKB || 0) / 1024),
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
  return { tab, series, final, profile };
}

function topSelfTime(profile, topN = 30) {
  if (!profile || !profile.nodes) return [];
  const selfByNode = new Map();
  for (const n of profile.nodes) {
    const cf = n.callFrame;
    const name = (cf.functionName || "(anonymous)") + (cf.url ? ` @ ${cf.url.split("/").pop()}:${cf.lineNumber}` : "");
    selfByNode.set(name, (selfByNode.get(name) || 0) + (n.hitCount || 0));
  }
  const totalSamples = profile.samples ? profile.samples.length : [...selfByNode.values()].reduce((a, b) => a + b, 0);
  const intervalUs = profile.timeDeltas && profile.timeDeltas.length
    ? profile.timeDeltas.reduce((a, b) => a + b, 0) / profile.timeDeltas.length : 150;
  return [...selfByNode.entries()]
    .map(([name, hits]) => ({ name, hits, pct: +((hits / totalSamples) * 100).toFixed(2), ms: +((hits * intervalUs) / 1000).toFixed(1) }))
    .sort((a, b) => b.hits - a.hits)
    .slice(0, topN);
}

function summarizeFinal(f) {
  const s = f.sched || {}, t = f.threeStage || {};
  const round = (v, scale = 1, dp = 2) => (v == null ? null : +(v * scale).toFixed(dp));
  return {
    schedCount: s.count, overdue: s.overdue, smallLead: s.smallLeadCount,
    qlatMaxMs: f.perf?.audioQLatMax,
    drawAvgMs: round(f.perf?.drawAvgMS, 1, 1), drawMaxMs: round(f.perf?.drawMaxMS, 1, 1),
    stageA_p99Ms: t.seqFireLate ? round(t.seqFireLate.p99, 1000) : null,
    stageA_maxMs: t.seqFireLate ? round(t.seqFireLate.max, 1000) : null,
  };
}

const browser = await chromium.launch({ headless: true, args: ["--autoplay-policy=no-user-gesture-required", "--enable-precise-memory-info"] });
const page = await browser.newPage();
const cdp = await page.context().newCDPSession(page);
await cdp.send("Profiler.enable");
const consoleLines = [];
page.on("console", (m) => consoleLines.push(m.text()));

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function" && typeof setEQTab === "function");
await page.waitForTimeout(400);

const report = { ts, bpm: BPM, secs: SECS, tabs: TABS, runs: [] };
const summary = [];
for (const tab of TABS) {
  const withProfile = tab === PROFILE_TAB;
  console.log(`[chain-profile] tab=${tab} (${SECS}s @ ${BPM}bpm${withProfile ? ", profiled" : ""})...`);
  const run = await runTab(page, cdp, tab, withProfile);
  const fin = summarizeFinal(run.final);
  let top = null;
  if (run.profile) {
    top = topSelfTime(run.profile);
    fs.writeFileSync(path.join(outdir, `${tab}_cpuprofile.cpuprofile`), JSON.stringify(run.profile));
  }
  report.runs.push({ tab, series: run.series, final: fin, topSelfTime: top });
  summary.push({ tab, ...fin });
  // Median draw-calls-per-frame over the warm seconds (drop sec1 warmup).
  const cpf = run.series.map((r) => r.callsPF).filter((v) => v != null).sort((a, b) => a - b);
  const medCallsPF = cpf.length ? cpf[Math.floor(cpf.length / 2)] : null;
  fin.callsPerFrame = medCallsPF;
  summary[summary.length - 1].callsPerFrame = medCallsPF;
  console.log(`\n=== ${tab} ===`);
  console.log("sec  drift%  drawAvg drawMax callsPF updMax qlatMax heapMB");
  for (const r of run.series) {
    console.log(`${String(r.sec).padStart(3)}  ${String(r.driftPct).padStart(6)}  ${String(r.drawAvg).padStart(7)} ${String(r.drawMax).padStart(7)} ${String(r.callsPF).padStart(7)} ${String(r.updMax).padStart(6)} ${String(r.qlatMax).padStart(7)} ${String(r.heapMB).padStart(6)}`);
  }
  console.log("final:", JSON.stringify(fin));
  if (top) {
    console.log("top self-time (main thread, chain tab):");
    for (const t of top.slice(0, 20)) console.log(`  ${String(t.pct).padStart(5)}%  ${t.ms}ms  ${t.name}`);
  }
}

console.log("\n=== PER-TAB SUMMARY ===");
console.log("tab        drawAvg drawMax stageA_p99 stageA_max qlatMax overdue");
for (const s of summary) {
  console.log(`${s.tab.padEnd(10)} ${String(s.drawAvgMs).padStart(7)} ${String(s.drawMaxMs).padStart(7)} ${String(s.stageA_p99Ms).padStart(10)} ${String(s.stageA_maxMs).padStart(10)} ${String(s.qlatMaxMs).padStart(7)} ${String(s.overdue).padStart(7)}`);
}

fs.writeFileSync(path.join(outdir, "report.json"), JSON.stringify(report, null, 2));
fs.writeFileSync(path.join(outdir, "browser_console.log"), consoleLines.join("\n"));
await browser.close();
server.close();
console.log(`\n[chain-profile] wrote ${path.join(outdir, "report.json")}`);
