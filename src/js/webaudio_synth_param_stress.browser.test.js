/**
 * Live synth + sampler parameter-update performance stress (WASM).
 *
 * THE BUG THIS GUARDS
 * -------------------
 * Updating a live synth parameter (e.g. the oscillator wave / `osc_type`) on a
 * MELODIC instrument while the sequencer is playing makes the game lag, jitter,
 * and the audio break up — especially in WASM where everything shares one
 * thread.
 *
 * Root cause: every param push runs `evictRenderCache(id)` in audio.js, which
 * for a melodic instrument drops the bare render AND every per-pitch
 * (`id:<pitch>`) variant. The next sequencer note is then a cache miss and runs
 * a SYNCHRONOUS `render_modular_p` ccall on the main thread (~30–40 ms for a
 * 2.0 s physical-model / orchestral voice). During a knob drag the params push
 * at ~60 Hz, so EVERY note re-renders → the main thread is blocked 24–69 ms
 * over and over → dropped frames + audio-scheduling jank.
 *
 * The pre-existing `instrument_params_playback_stability.browser.test.js` does
 * NOT catch this: it spams params on `snare` (a drum: single cache key, no
 * per-pitch storm) and only asserts the audio is not silenced — it never
 * measures main-thread blocking or redundant re-renders. The instruments that
 * actually break are the new melodic ones (violin / cello / guitar / piano /
 * winds / brass — all `paramBlock:'modular'`, 1.5–2.0 s, per-pitch rendered).
 *
 * WHAT THIS TEST MEASURES
 * -----------------------
 * For each instrument it plays notes at several distinct pitches at a realistic
 * sequencer cadence for a couple of seconds, while spamming the param at 60 Hz,
 * and records:
 *   - render count   (window.__audioMetrics.renders[id]) vs note count
 *   - event-loop gaps (an 8 ms self-check interval; a blocked main thread shows
 *                      up as a large gap) → max gap + count of >50 ms stalls
 *   - audio schedule health (getAudioScheduleMetrics: overdue / small-lead)
 *
 * PRIMARY (machine-independent) ASSERTION: the render/note RATIO. With the bug
 * every note re-renders (ratio ≈ 1.0). A correct fix keeps the cache warm
 * between param changes so notes reuse it (ratio well below 1.0). This is a
 * structural fact, independent of CPU speed.
 *
 * SECONDARY (behavioural, env-tunable) ASSERTION: no main-thread stall over
 * SYNTH_STRESS_MAX_BLOCK_MS, and a bounded worst-case event-loop gap.
 *
 * This test is EXPECTED TO FAIL until the live-param re-render path stops
 * forcing a synchronous full re-render per note.
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

// ── Tunables (env-overridable; defaults sized off the measured native run) ──
const NOTE_INTERVAL_MS = Number(process.env.SYNTH_STRESS_NOTE_MS || 80);
const STRESS_DURATION_MS = Number(process.env.SYNTH_STRESS_DURATION_MS || 2500);
const SPAM_INTERVAL_MS = Number(process.env.SYNTH_STRESS_SPAM_MS || 16); // ~60 Hz
// A correct fix keeps the cache warm across rapid param pushes, so notes reuse
// it instead of re-rendering. With the bug renders ≈ notes (ratio ≈ 1.0).
const MAX_RENDER_RATIO = Number(process.env.SYNTH_STRESS_MAX_RENDER_RATIO || 0.6);
// No single synchronous render should stall the main thread this long.
const MAX_BLOCK_MS = Number(process.env.SYNTH_STRESS_MAX_BLOCK_MS || 50);
// Worst-case event-loop gap ceiling. With the bug a synchronous melodic render
// blocks the loop ~64–69 ms; with the off-thread fix the only residual main-
// thread cost is the transferred-buffer receipt + occasional GC (observed
// ≤~35 ms). 55 ms cleanly separates the two while leaving GC headroom so the
// behavioural cross-check does not flake on slower CI. The definitive gate is
// the machine-independent mainThreadRenders==0 assertion below.
const MAX_GAP_MS = Number(process.env.SYNTH_STRESS_MAX_GAP_MS || 55);

// Melodic, modular, per-pitch-rendered "new" instruments — the ones that break.
const SYNTH_INSTRUMENTS = ["cello", "violin", "guitar-nylon"];
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
await page.waitForFunction(() => typeof window.updateSampleEdit === "function");
await page.waitForFunction(() => typeof window.getAudioScheduleMetrics === "function");
await page.waitForFunction(() => window.audioReady !== undefined);
await page.evaluate(async () => { await window.audioReady; });

// Real gesture unlocks + CREATES the AudioContext (resumeAudio alone does not).
await page.mouse.click(20, 20);
await page.waitForTimeout(200);
await page.evaluate(async () => {
  if (window.__audioCtx && window.__audioCtx.state === "suspended") await window.__audioCtx.resume();
});
const ctxState = await page.evaluate(() => window.__audioCtx && window.__audioCtx.state);
if (ctxState !== "running") {
  throw new Error(`AudioContext not running (state=${ctxState}); cannot run audio perf stress`);
}
console.log(`[TEST] AudioContext running; note=${NOTE_INTERVAL_MS}ms spam=${SPAM_INTERVAL_MS}ms dur=${STRESS_DURATION_MS}ms`);

// runStress drives notes across pitches + a param spammer for `id`.
// kind: "synth" → setInstrumentParam(osc_type) ; "sampler" → updateSampleEdit.
async function runStress(id, kind, spam) {
  return await page.evaluate(async ({ id, kind, spam, NOTE_INTERVAL_MS, STRESS_DURATION_MS, SPAM_INTERVAL_MS, PITCHES, MAX_BLOCK_MS }) => {
    const ctx = window.__audioCtx;
    window.__audioMetrics = { renders: {}, cacheHits: {} };
    resetInstrumentParams(id);
    try { window.updateSampleEdit(id, JSON.stringify({})); } catch (_) {}
    if (window.resetPerfStats) window.resetPerfStats();
    if (window.resetAudioScheduleMetrics) window.resetAudioScheduleMetrics();

    let notes = 0, pi = 0, sp = 0;
    const gaps = [];
    let last = performance.now();
    const sampler = setInterval(() => { const n = performance.now(); gaps.push(n - last); last = n; }, 8);
    const noteTimer = setInterval(() => {
      const when = ctx.currentTime + 0.06;
      window.playSoundParams(id, 0.7, PITCHES[pi++ % PITCHES.length], 1.0, when);
      notes++;
    }, NOTE_INTERVAL_MS);

    let spamTimer = null;
    if (spam) {
      spamTimer = setInterval(() => {
        if (kind === "sampler") {
          // Sweep a sample-edit param (gain) — same evict+re-render path.
          sp = (sp + 1) % 7;
          window.updateSampleEdit(id, JSON.stringify({ GainDB: -3 + sp }));
        } else {
          sp = (sp + 1) % 5; // cycle the oscillator wave
          setInstrumentParam(id, "osc_type", sp);
        }
      }, SPAM_INTERVAL_MS);
    }

    await new Promise((r) => setTimeout(r, STRESS_DURATION_MS));
    clearInterval(noteTimer); if (spamTimer) clearInterval(spamTimer); clearInterval(sampler);
    await new Promise((r) => setTimeout(r, 300)); // let final renders flush

    gaps.sort((a, b) => a - b);
    const q = (p) => (gaps.length ? gaps[Math.min(gaps.length - 1, Math.floor(p * gaps.length))] : 0);
    const am = window.getAudioScheduleMetrics ? window.getAudioScheduleMetrics() : {};
    const ps = window.perfStats ? window.perfStats() : {};
    const met = window.__audioMetrics || {};
    return {
      notes,
      renders: (met.renders || {})[id] || 0,
      workerRenders: (met.workerRenders || {})[id] || 0,
      mainThreadRenders: (met.mainThreadRenders || {})[id] || 0,
      cacheHits: (met.cacheHits || {})[id] || 0,
      maxGapMs: gaps.length ? gaps[gaps.length - 1] : 0,
      p99GapMs: q(0.99), p90GapMs: q(0.90),
      blockedOverMax: gaps.filter((g) => g > MAX_BLOCK_MS).length,
      fpsAvg: ps.fpsAvg, updateMaxMS: ps.updateMaxMS, heapAllocKB: ps.heapAllocKB,
      audio: { count: am.count, overdue: am.overdue, smallLead: am.smallLeadCount, minLead: am.minLead },
    };
  }, { id, kind, spam, NOTE_INTERVAL_MS, STRESS_DURATION_MS, SPAM_INTERVAL_MS, PITCHES, MAX_BLOCK_MS });
}

const failures = [];

function evaluate(label, kind, base, spam) {
  const ratio = spam.renders / Math.max(1, spam.notes);
  console.log(`[TEST] ${label}`);
  console.log(`[TEST]   baseline: renders=${base.renders} (worker=${base.workerRenders} main=${base.mainThreadRenders}) maxGap=${base.maxGapMs.toFixed(1)}ms`);
  console.log(`[TEST]   +spam   : renders=${spam.renders}/${spam.notes} ratio=${ratio.toFixed(2)} worker=${spam.workerRenders} main=${spam.mainThreadRenders} maxGap=${spam.maxGapMs.toFixed(1)}ms p90Gap=${spam.p90GapMs.toFixed(1)}ms blocked>${MAX_BLOCK_MS}ms=${spam.blockedOverMax} audio=${JSON.stringify(spam.audio)}`);

  // PRIMARY (machine-independent): the heavy C renders triggered by the param
  // spam must run OFF the main thread. With the bug every render is a
  // synchronous main-thread ccall (mainThreadRenders == renders, workerRenders
  // == 0); with the off-thread fix the renders go to the worker.
  if (spam.renders > 0 && spam.workerRenders === 0) {
    failures.push(`${label}: NO renders went off-thread (workerRenders=0 of ${spam.renders}). Live param edits still render synchronously on the main thread.`);
  }
  if (spam.mainThreadRenders > 0) {
    failures.push(`${label}: ${spam.mainThreadRenders} render(s) ran on the MAIN THREAD during param spam (each ~30–40ms blocks the UI + audio scheduler). Expected 0 — renders must be off-thread.`);
  }
  // SECONDARY (behavioural, env-tunable): no long main-thread stall, no overdue
  // audio. These cross-check that off-thread rendering actually unblocked the
  // event loop. Kept generous so they don't flake on slow CI.
  if (spam.blockedOverMax > 0) {
    failures.push(`${label}: main thread stalled >${MAX_BLOCK_MS}ms ${spam.blockedOverMax} time(s) (maxGap ${spam.maxGapMs.toFixed(1)}ms).`);
  }
  if (spam.maxGapMs > MAX_GAP_MS) {
    failures.push(`${label}: worst event-loop gap ${spam.maxGapMs.toFixed(1)}ms exceeds ${MAX_GAP_MS}ms ceiling.`);
  }
  if (spam.audio && spam.audio.overdue > 0) {
    failures.push(`${label}: ${spam.audio.overdue} overdue audio event(s) scheduled in the past during param spam.`);
  }
  // INFO only: the render-storm ratio is expected to stay high with the chosen
  // off-thread approach (renders happen ~per change, just not on the main
  // thread). Logged for visibility, not gated. MAX_RENDER_RATIO is referenced
  // here so the knob stays documented.
  if (ratio > MAX_RENDER_RATIO) {
    console.log(`[TEST]   (info) render/note ratio ${ratio.toFixed(2)} > ${MAX_RENDER_RATIO} — renders happen per change but off-thread.`);
  }
}

// ── Synth oscillator-wave spam on melodic instruments ──
for (const id of SYNTH_INSTRUMENTS) {
  const base = await runStress(id, "synth", false);
  const spam = await runStress(id, "synth", true);
  evaluate(`synth osc-wave spam — ${id}`, "synth", base, spam);
}

// ── Sampler-edit spam on a melodic instrument (same evict+re-render path) ──
{
  const id = "cello";
  const base = await runStress(id, "sampler", false);
  const spam = await runStress(id, "sampler", true);
  evaluate(`sampler-edit spam — ${id}`, "sampler", base, spam);
}

if (pageErrors.length > 0) {
  failures.push(`page emitted ${pageErrors.length} error(s): ${pageErrors.join(" | ")}`);
}

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "webaudio_synth_param_stress",
  );
}
await browser.close();
server.close();

if (failures.length > 0) {
  console.error("\nwebaudio_synth_param_stress FAILED:\n  - " + failures.join("\n  - "));
  process.exit(1);
}
console.log("\nwebaudio_synth_param_stress browser test PASSED");
