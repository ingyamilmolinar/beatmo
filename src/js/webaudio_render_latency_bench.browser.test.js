/** Off-thread is not on time: a cache miss defers the note past an async render, and the schedule metrics cannot see it.
 * Captures that hidden lateness (deferMs, renderMs, workerQueuePeak) for raw WAV, drum and synth param storms, and
 * Sampler edits → bench-results/render_latency.json. Gated structurally; RENDER_BENCH_STRICT=1 also gates deferAbsMsP90. */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
  waitForAudioReady,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// ── Tunables ──
const NOTE_INTERVAL_MS = Number(process.env.RENDER_BENCH_NOTE_MS || 80);
const DURATION_MS = Number(process.env.RENDER_BENCH_DURATION_MS || 2500);
const SPAM_INTERVAL_MS = Number(process.env.RENDER_BENCH_SPAM_MS || 16);
// Match the real browser sequencer: AudioLookaheadSec=0.06 (runtime_profile.go).
const SCHED_LEAD_SEC = Number(process.env.RENDER_BENCH_LEAD_SEC || 0.06);
const STRICT = process.env.RENDER_BENCH_STRICT === "1";
const DEFER_P90_MAX_MS = Number(process.env.RENDER_BENCH_DEFER_P90_MAX_MS || 15);

const PITCHES = [-12, -7, -5, 0, 4, 7, 11, 12];

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
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof window.playSoundParams === "function");
await page.waitForFunction(() => typeof window.getRenderLatencyMetrics === "function");
await page.waitForFunction(() => typeof window.registerSamplePCM === "function");
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
console.log(`[TEST] AudioContext running; note=${NOTE_INTERVAL_MS}ms spam=${SPAM_INTERVAL_MS}ms lead=${SCHED_LEAD_SEC * 1000}ms dur=${DURATION_MS}ms strict=${STRICT}`);

// A decaying sine registered like a Sampler save: no RENDER mapping, so it never renders or defers.
await page.evaluate(() => {
  const sr = 48000;
  const frames = Math.floor(sr * 0.5);
  const data = new Float32Array(frames);
  for (let i = 0; i < frames; i++) {
    const t = i / sr;
    data[i] = Math.sin(2 * Math.PI * 180 * t) * Math.exp(-6 * t) * 0.8;
  }
  window.registerSamplePCM("bench-wav", new Uint8Array(data.buffer), sr);
});

// The sequencer's real lead, optionally spamming a param.
async function runScenario(cfg) {
  return await page.evaluate(async ({ cfg, NOTE_INTERVAL_MS, DURATION_MS, SPAM_INTERVAL_MS, SCHED_LEAD_SEC, PITCHES }) => {
    const ctx = window.__audioCtx;
    const id = cfg.id;
    window.__audioMetrics = { renders: {}, cacheHits: {} };
    try { resetInstrumentParams(id); } catch (_) {}
    try { window.updateSampleEdit(id, JSON.stringify({})); } catch (_) {}
    window.resetRenderLatencyMetrics();
    if (window.resetAudioScheduleMetrics) window.resetAudioScheduleMetrics();
    if (window.resetPerfStats) window.resetPerfStats();

    // Pre-warm: render every pitch once so warm scenarios measure steady state.
    if (cfg.prewarm) {
      for (const p of PITCHES) {
        window.playSoundParams(id, 0.0, p, 1.0, ctx.currentTime + 0.05);
      }
      await new Promise((r) => setTimeout(r, 1200));
      window.resetRenderLatencyMetrics();
      if (window.resetAudioScheduleMetrics) window.resetAudioScheduleMetrics();
    }

    let notes = 0, pi = 0, sp = 0;
    let paramPushes = 0;
    const gaps = [];
    let last = performance.now();
    const sampler = setInterval(() => { const n = performance.now(); gaps.push(n - last); last = n; }, 8);
    const noteTimer = setInterval(() => {
      const when = ctx.currentTime + SCHED_LEAD_SEC;
      const pitch = cfg.pitched ? PITCHES[pi++ % PITCHES.length] : 0;
      window.playSoundParams(id, 0.7, pitch, 1.0, when);
      notes++;
    }, cfg.noteMs || NOTE_INTERVAL_MS);

    let spamTimer = null;
    if (cfg.spam === "synth") {
      spamTimer = setInterval(() => {
        sp = (sp + 1) % 5;
        setInstrumentParam(id, "osc_type", sp);
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "sampler") {
      spamTimer = setInterval(() => {
        sp = (sp + 1) % 7;
        window.updateSampleEdit(id, JSON.stringify({ GainDB: -3 + sp }));
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "toggle") {
      // Cyclic enable/disable of one stage — 2 alternating param hashes.
      spamTimer = setInterval(() => {
        sp = (sp + 1) % 2;
        setInstrumentParam(id, cfg.toggleParam || "filter_enabled", sp);
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "toggle-multi") {
      // Rotate on/off across several stages → many distinct hashes.
      const stages = ["filter_enabled", "lfo_enabled", "filtenv_enabled"];
      let k = 0;
      spamTimer = setInterval(() => {
        const st = stages[k % stages.length];
        setInstrumentParam(id, st, (k >> 1) & 1 ? 0 : 1); // vary value too
        k++;
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "ctrl-sweep") {
      // Monotone sweep of a control on an ENABLED stage (worst for LRU key).
      let v = 0;
      spamTimer = setInterval(() => {
        v = (v + 0.03) % 1.0;
        setInstrumentParam(id, cfg.ctrlParam || "filter_cutoff", v);
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "ctrl-cyclic") {
      // Triangle sweep — values revisit, so the cache warms.
      let v = 0, dir = 1;
      spamTimer = setInterval(() => {
        v += dir * 0.05; if (v >= 1) { v = 1; dir = -1; } else if (v <= 0) { v = 0; dir = 1; }
        setInstrumentParam(id, cfg.ctrlParam || "lfo_rate", v);
        paramPushes++;
      }, SPAM_INTERVAL_MS);
    } else if (cfg.spam === "combined") {
      // Toggle a stage AND sweep its control together (the realistic storm).
      let v = 0, t = 0, cyc = 0;
      spamTimer = setInterval(() => {
        t = (t + 1) % 8;
        if (t === 0) { setInstrumentParam(id, "filter_enabled", cyc & 1 ? 0 : 1); cyc++; }
        v = (v + 0.04) % 1.0;
        setInstrumentParam(id, "filter_cutoff", v);
        paramPushes += (t === 0 ? 2 : 1);
      }, SPAM_INTERVAL_MS);
    }

    await new Promise((r) => setTimeout(r, DURATION_MS));
    clearInterval(noteTimer); if (spamTimer) clearInterval(spamTimer); clearInterval(sampler);
    await new Promise((r) => setTimeout(r, 400)); // let deferred plays land

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
      deferMsAvg: rl.deferMsAvg,
      deferMsMin: rl.deferMsMin,
      deferMsMax: rl.deferMsMax,
      deferAbsMsP90: rl.deferAbsMsP90,
      workerQueuePeak: rl.workerQueuePeak,
      stalePlays: rl.stalePlays,
      neighborPlays: rl.neighborPlays,
      maxGapMs: gaps.length ? gaps[gaps.length - 1] : 0,
      p90GapMs: q(0.90),
      paramPushes,
      audio: {
        count: am.count, overdue: am.overdue, smallLead: am.smallLeadCount,
        minLead: am.minLead, avgLead: am.avgLead,
      },
    };
  }, { cfg, NOTE_INTERVAL_MS, DURATION_MS, SPAM_INTERVAL_MS, SCHED_LEAD_SEC, PITCHES });
}

const SCENARIOS = [
  { name: "raw-wav", id: "bench-wav", pitched: true, spam: null, prewarm: false },
  { name: "drum-storm", id: "snare", pitched: false, spam: "synth", prewarm: true },
  { name: "synth-warm", id: "cello", pitched: true, spam: null, prewarm: true },
  { name: "synth-storm", id: "cello", pitched: true, spam: "synth", prewarm: true },
  // Requests outpace one render, so the queue builds and deferMs drifts late.
  { name: "synth-storm-dense", id: "cello", pitched: true, spam: "synth", prewarm: true, noteMs: 40 },
  { name: "sampler-storm", id: "cello", pitched: true, spam: "sampler", prewarm: true },
  // 4× throttle pushes a render past the lead, so deferred notes land LATE, the real-world lag.
  { name: "synth-storm-4x", id: "cello", pitched: true, spam: "synth", prewarm: true, cpuThrottle: 4 },
  { name: "stage-toggle-storm", id: "cello", pitched: true, spam: "toggle", prewarm: true, toggleParam: "filter_enabled" },
  { name: "stage-toggle-multi", id: "cello", pitched: true, spam: "toggle-multi", prewarm: true },
  { name: "enabled-ctrl-sweep", id: "cello", pitched: true, spam: "ctrl-sweep", prewarm: true, ctrlParam: "filter_cutoff" },
  { name: "enabled-ctrl-cyclic", id: "cello", pitched: true, spam: "ctrl-cyclic", prewarm: true, ctrlParam: "lfo_rate" },
  { name: "combined-toggle+ctrl", id: "cello", pitched: true, spam: "combined", prewarm: true },
];

const cdp = await page.context().newCDPSession(page);

const results = {};
for (const cfg of SCENARIOS) {
  if (cfg.cpuThrottle) {
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: cfg.cpuThrottle });
  }
  results[cfg.name] = await runScenario(cfg);
  if (cfg.cpuThrottle) {
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
  }
}

// ── Report ──
const fmt = (v) => (v == null ? "-" : (typeof v === "number" ? v.toFixed(1) : String(v)));
console.log("\n[TEST] scenario        notes renders defer stale nbr  deferMs avg/min/max (absP90)   render avg/p90/max ms  qPeak maxGap");
for (const [name, r] of Object.entries(results)) {
  console.log(
    `[TEST] ${name.padEnd(15)} ${String(r.notes).padStart(5)} ${String(r.renders).padStart(7)} ${String(r.deferredPlays).padStart(5)} ${String(r.stalePlays).padStart(5)} ${String(r.neighborPlays).padStart(3)}  ` +
    `${fmt(r.deferMsAvg)}/${fmt(r.deferMsMin)}/${fmt(r.deferMsMax)} (${fmt(r.deferAbsMsP90)})`.padEnd(31) +
    `${fmt(r.renderMsAvg)}/${fmt(r.renderMsP90)}/${fmt(r.renderMsMax)}`.padEnd(23) +
    `${String(r.workerQueuePeak).padStart(5)} ${fmt(r.maxGapMs)}ms  pushes=${r.paramPushes ?? 0}`
  );
}

// Persist for bench tracking.
const outDir = path.resolve(jsDir, "../../bench-results");
fs.mkdirSync(outDir, { recursive: true });
fs.writeFileSync(
  path.join(outDir, "render_latency.json"),
  JSON.stringify({
    noteIntervalMS: NOTE_INTERVAL_MS, spamIntervalMS: SPAM_INTERVAL_MS,
    schedLeadSec: SCHED_LEAD_SEC, durationMS: DURATION_MS,
    scenarios: results,
  }, null, 2),
);
console.log(`[TEST] wrote ${path.join(outDir, "render_latency.json")}`);

// ── Gates ──
const failures = [];
const raw = results["raw-wav"];
if (raw.renders !== 0 || raw.deferredPlays !== 0) {
  failures.push(`raw-wav: expected pure sample playback (renders=0 defers=0), got renders=${raw.renders} defers=${raw.deferredPlays}`);
}
const warm = results["synth-warm"];
if (warm.deferredPlays !== 0) {
  failures.push(`synth-warm: cache should stay warm without edits, got ${warm.deferredPlays} deferred plays (renders=${warm.renders})`);
}
if (STRICT) {
  for (const name of ["drum-storm", "synth-storm", "synth-storm-dense", "sampler-storm"]) {
    const r = results[name];
    if ((r.deferAbsMsP90 || 0) > DEFER_P90_MAX_MS) {
      failures.push(`${name}: deferAbsMsP90 ${r.deferAbsMsP90.toFixed(1)}ms exceeds real-time bar ${DEFER_P90_MAX_MS}ms`);
    }
    if (r.mainThreadRenders > 0) {
      failures.push(`${name}: ${r.mainThreadRenders} render(s) on the main thread`);
    }
  }
}
if (pageErrors.length > 0) {
  failures.push(`page emitted ${pageErrors.length} error(s): ${pageErrors.join(" | ")}`);
}

// They must actually spam params and keep renders off-thread.
for (const name of ["stage-toggle-storm", "stage-toggle-multi", "enabled-ctrl-sweep", "enabled-ctrl-cyclic", "combined-toggle+ctrl"]) {
  const r = results[name];
  if (!r) { failures.push(`${name}: scenario missing from results`); continue; }
  if ((r.paramPushes || 0) <= 0) failures.push(`${name}: expected paramPushes>0, got ${r.paramPushes}`);
  if ((r.mainThreadRenders || 0) > 0) failures.push(`${name}: ${r.mainThreadRenders} render(s) on the main thread (must stay off-thread)`);
}

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "webaudio_render_latency_bench",
  );
}
await browser.close();
server.close();

if (failures.length > 0) {
  console.error("\nwebaudio_render_latency_bench FAILED:\n  - " + failures.join("\n  - "));
  process.exit(1);
}
console.log("\nwebaudio_render_latency_bench browser test PASSED");
