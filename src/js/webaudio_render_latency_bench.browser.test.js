/**
 * Render-latency bench: raw WAV vs synth vs synth-under-param-storm (WASM).
 *
 * WHAT THIS BENCH MEASURES (and why it exists)
 * --------------------------------------------
 * webaudio_synth_param_stress.browser.test.js proved the heavy C voice renders
 * run OFF the main thread — but off-thread is not the same as ON TIME. When a
 * trigger misses the render cache, processAudioEvent DROPS the event's
 * musically-intended `when`, awaits the async worker render (~30–40 ms for a
 * 2 s physical-model voice, serialized on ONE worker), and replays the note at
 * a fresh currentTime. The note starts LATE by (worker queue + render +
 * re-flush) — and the existing schedule metrics can't see it, because they
 * observe the REWRITTEN `when` (healthy lead) rather than the intended one.
 * During a live param edit every push evicts the whole per-pitch cache, so
 * EVERY note pays that hidden lateness → the audible "laggy / not real time".
 *
 * This bench captures the hidden number via the renderLatencyMetrics counters
 * in audio.js (getRenderLatencyMetrics):
 *   - deferMs avg/min/max (signed), deferAbsMsP90 — actual start minus
 *     intended `when` for deferred plays. Positive = late (render+queue ate
 *     more than the lead); negative = early (the replay drops the intended
 *     lead and fires at now+minLead). Both are off-grid jitter.
 *   - renderMs avg/p90/max    — per-voice render cost as experienced by the
 *     trigger path (worker round-trip incl. queue wait + main-thread post)
 *   - workerQueuePeak         — render-request pileup on the single worker
 * plus the existing schedule metrics and event-loop gap sampling.
 *
 * SCENARIOS (all through the identical enqueue→processAudioEvent path, with
 * the browser sequencer's real scheduling lead):
 *   raw-wav       — registerSamplePCM buffer (pure sample playback baseline)
 *   drum-storm    — snare + 60 Hz param spam (single cache key, short render)
 *   synth-warm    — cello (2 s bowed-string physical model), pitches
 *                   pre-warmed, no edits → steady-state cache hits
 *   synth-storm   — cello + 60 Hz osc_type spam across 8 pitches (the
 *                   melodic per-pitch re-render storm)
 *   sampler-storm — cello + 60 Hz updateSampleEdit spam (same evict path)
 *
 * OUTPUT: a comparison table on stdout + bench-results/render_latency.json
 * (schema: { scenarios: { name: metrics } }) so regressions are trackable.
 *
 * GATES (structural, machine-independent; latency numbers are recorded, not
 * gated, unless RENDER_BENCH_STRICT=1):
 *   - raw-wav must have zero renders and zero deferred plays
 *   - synth-warm must have zero deferred plays in the measure window
 *   - no page errors
 *   - RENDER_BENCH_STRICT=1 additionally gates deferAbsMsP90 for the storm
 *     scenarios at RENDER_BENCH_DEFER_P90_MAX_MS (default 15) — the
 *     real-time acceptance bar for the param-storm fix.
 */

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
await page.evaluate(async () => { await window.audioReady; });

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

// Install the raw-WAV baseline instrument: 0.5 s decaying sine burst PCM,
// registered exactly like a Sampler-tab save (renderCache entry, no RENDER
// mapping → playback never renders, never defers).
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

// runScenario plays notes at a fixed cadence with the sequencer's real lead,
// optionally spamming a param, and snapshots all latency metrics.
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
  // Dense cadence ≈ chords / several melodic rows on the same grid: render
  // requests arrive faster than one render completes, so the single worker's
  // queue builds and deferMs drifts late instead of early.
  { name: "synth-storm-dense", id: "cello", pitched: true, spam: "synth", prewarm: true, noteMs: 40 },
  { name: "sampler-storm", id: "cello", pitched: true, spam: "sampler", prewarm: true },
  // Mobile-class device simulation: 4× CPU throttle pushes the per-voice
  // render past the scheduling lead, so deferred notes land LATE (the
  // user-reported real-world lag) instead of early.
  { name: "synth-storm-4x", id: "cello", pitched: true, spam: "synth", prewarm: true, cpuThrottle: 4 },
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
    `${String(r.workerQueuePeak).padStart(5)} ${fmt(r.maxGapMs)}ms`
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
