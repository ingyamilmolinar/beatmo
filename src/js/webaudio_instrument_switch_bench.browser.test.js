/** The cold-start cost the moment a row switches to a never-rendered instrument during playback ("clicks, then laggy
 * for seconds"): melodic voices are cached per pitch, so a row firing N pitches pays N cold renders. Scenarios: warm
 * baseline, cold melodic (also 4×), cold single-key, cold cello. Recorded to bench-results/instrument_switch.json, not gated. */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  waitForAudioReady,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// ── Tunables ──
const NOTE_INTERVAL_MS = Number(process.env.SWITCH_BENCH_NOTE_MS || 90);
const DURATION_MS = Number(process.env.SWITCH_BENCH_DURATION_MS || 3000);
const SCHED_LEAD_SEC = Number(process.env.SWITCH_BENCH_LEAD_SEC || 0.06);

// Several distinct pitches turn one cold render into a per-pitch storm.
const PITCHES = [-12, -8, -5, 0, 3, 7, 12, 15, 19, 24];

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, "").split("?")[0]);
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (err) => { pageErrors.push(String(err)); console.log("[PAGE-ERROR]", String(err)); });
page.on("console", (msg) => {
  try {
    const t = msg.text();
    if (msg.type() === "error" || t.startsWith("[TEST]")) console.log("[PAGE]", msg.type(), t);
  } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.playSoundParams === "function");
await page.waitForFunction(() => typeof window.getRenderLatencyMetrics === "function");
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await waitForAudioReady(page);

// Real gesture unlocks + CREATES the AudioContext.
await page.mouse.click(20, 20);
await page.waitForTimeout(200);
await page.evaluate(async () => {
  if (window.__audioCtx && window.__audioCtx.state === "suspended") await window.__audioCtx.resume();
});
const ctxState = await page.evaluate(() => window.__audioCtx && window.__audioCtx.state);
if (ctxState !== "running") {
  throw new Error(`AudioContext not running (state=${ctxState}); cannot bench`);
}
console.log(`[TEST] AudioContext running; note=${NOTE_INTERVAL_MS}ms lead=${SCHED_LEAD_SEC * 1000}ms dur=${DURATION_MS}ms pitches=${PITCHES.length}`);

// The row's output after a switch: the target across the pitch set at a fixed cadence with the real lead, not pre-warmed
// unless cfg.prewarm.
async function runScenario(cfg) {
  return await page.evaluate(async ({ cfg, NOTE_INTERVAL_MS, DURATION_MS, SCHED_LEAD_SEC, PITCHES }) => {
    const ctx = window.__audioCtx;
    const id = cfg.id;

    // Fully evicted so every run starts genuinely cold.
    window.__audioMetrics = { renders: {}, cacheHits: {} };
    try { resetInstrumentParams(id); } catch (_) {}
    if (window.__evictRenderCacheForTest) {
      window.__evictRenderCacheForTest(id);
    }

    if (cfg.prewarm) {
      for (const p of PITCHES) window.playSoundParams(id, 0.0, p, 1.0, ctx.currentTime + 0.05);
      await new Promise((r) => setTimeout(r, 1600));
    }

    window.resetRenderLatencyMetrics();
    if (window.resetAudioScheduleMetrics) window.resetAudioScheduleMetrics();
    if (window.resetPerfStats) window.resetPerfStats();

    let notes = 0, pi = 0;
    const gaps = [];
    let last = performance.now();
    const sampler = setInterval(() => { const n = performance.now(); gaps.push(n - last); last = n; }, 8);
    const noteTimer = setInterval(() => {
      const when = ctx.currentTime + SCHED_LEAD_SEC;
      const pitch = PITCHES[pi++ % PITCHES.length];
      window.playSoundParams(id, 0.7, pitch, 1.0, when);
      notes++;
    }, NOTE_INTERVAL_MS);

    await new Promise((r) => setTimeout(r, DURATION_MS));
    clearInterval(noteTimer); clearInterval(sampler);
    await new Promise((r) => setTimeout(r, 600)); // let deferred renders land

    gaps.sort((a, b) => a - b);
    const q = (p) => (gaps.length ? gaps[Math.min(gaps.length - 1, Math.floor(p * gaps.length))] : 0);
    const rl = window.getRenderLatencyMetrics();
    const am = window.getAudioScheduleMetrics ? window.getAudioScheduleMetrics() : {};
    return {
      notes,
      renders: rl.renders,
      workerRenders: rl.workerRenders,
      mainThreadRenders: rl.mainThreadRenders,
      renderMsAvg: rl.renderMsAvg,
      renderMsP90: rl.renderMsP90,
      renderMsMax: rl.renderMsMax,
      deferredPlays: rl.deferredPlays,
      neighborPlays: rl.neighborPlays,
      stalePlays: rl.stalePlays,
      deferMsAvg: rl.deferMsAvg,
      deferMsMin: rl.deferMsMin,
      deferMsMax: rl.deferMsMax,
      deferAbsMsP90: rl.deferAbsMsP90,
      workerQueuePeak: rl.workerQueuePeak,
      maxGapMs: gaps.length ? gaps[gaps.length - 1] : 0,
      p90GapMs: q(0.90),
      audio: {
        count: am.count, overdue: am.overdue, smallLead: am.smallLeadCount,
        minLead: am.minLead, avgLead: am.avgLead,
      },
    };
  }, { cfg, NOTE_INTERVAL_MS, DURATION_MS, SCHED_LEAD_SEC, PITCHES });
}

const SCENARIOS = [
  { name: "warm-baseline",   id: "organ-church", prewarm: true },
  { name: "cold-melodic",    id: "organ-church", prewarm: false },
  { name: "cold-melodic-4x", id: "organ-church", prewarm: false, cpuThrottle: 4 },
  { name: "cold-single",     id: "fm-bass",      prewarm: false },
  { name: "cold-cello",      id: "cello",        prewarm: false },
];

const cdp = await page.context().newCDPSession(page);
const results = {};
for (const cfg of SCENARIOS) {
  if (cfg.cpuThrottle) await cdp.send("Emulation.setCPUThrottlingRate", { rate: cfg.cpuThrottle });
  results[cfg.name] = await runScenario(cfg);
  if (cfg.cpuThrottle) await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
}

// ── Report ──
const fmt = (v) => (v == null ? "-" : (typeof v === "number" ? v.toFixed(1) : String(v)));
console.log("\n[TEST] scenario         notes rndr defer nbr stale  deferMs avg/min/max (absP90)   render avg/p90/max ms  qPeak maxGap");
for (const [name, r] of Object.entries(results)) {
  console.log(
    `[TEST] ${name.padEnd(16)} ${String(r.notes).padStart(5)} ${String(r.renders).padStart(4)} ${String(r.deferredPlays).padStart(5)} ${String(r.neighborPlays).padStart(3)} ${String(r.stalePlays).padStart(5)}  ` +
    `${fmt(r.deferMsAvg)}/${fmt(r.deferMsMin)}/${fmt(r.deferMsMax)} (${fmt(r.deferAbsMsP90)})`.padEnd(31) +
    `${fmt(r.renderMsAvg)}/${fmt(r.renderMsP90)}/${fmt(r.renderMsMax)}`.padEnd(23) +
    `${String(r.workerQueuePeak).padStart(5)} ${fmt(r.maxGapMs)}ms`
  );
}

const outDir = path.resolve(jsDir, "../../bench-results");
fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(
  path.join(outDir, "instrument_switch.json"),
  JSON.stringify({
    noteIntervalMS: NOTE_INTERVAL_MS, schedLeadSec: SCHED_LEAD_SEC,
    durationMS: DURATION_MS, pitches: PITCHES, scenarios: results,
  }, null, 2),
);
console.log(`[TEST] wrote ${path.join(outDir, "instrument_switch.json")}`);

await browser.close();
server.close();

if (pageErrors.length > 0) {
  console.error("\nwebaudio_instrument_switch_bench page errors:\n  - " + pageErrors.join("\n  - "));
  process.exit(1);
}
console.log("\nwebaudio_instrument_switch_bench browser test PASSED");
