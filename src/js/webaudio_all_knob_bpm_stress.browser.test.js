/**
 * ALL-knob / all-settings live-edit stress during REAL sequencer playback,
 * across multiple BPMs (WASM).
 *
 * WHY THIS TEST EXISTS (coverage gap it closes)
 * ---------------------------------------------
 * The existing live-edit perf gates only spam a SINGLE param:
 *   - webaudio_synth_param_stress:  osc_type (synth) / GainDB (sampler),
 *     timer-driven notes — no sequencer, no BPM dimension.
 *   - webaudio_render_latency_bench: osc_type storms, timer-driven.
 *   - instrument_params_playback_stability: pitch on a drum.
 * Nothing exercised the OTHER ~50 user-facing synth knobs (filter, envelope,
 * FM, LFO, burst, unison, post…), none of the live-mutate mixer surfaces
 * (insert-FX params, EQ bands, HPF/LPF, sends, volume/pan) were ever edited
 * DURING playback with latency assertions, and no live-edit test ran under a
 * real sequencer at different tempos.
 *
 * WHAT THIS TEST DOES
 * -------------------
 * Builds a 3-row circuit (buildPerfRect) with a melodic waveguide voice
 * (cello), the heaviest additive/unison voice (organ-church) and a
 * config-kick drum (dnb-kick — the "Punch knob hang" instrument), starts REAL
 * playback, and for each BPM in ALL_KNOB_BPMS runs two spam phases:
 *
 *   1. synth-knobs: round-robin over EVERY user-facing ParamDef of every row
 *      instrument (enumerated live from synthRecipeCatalog(), group!=="hidden"
 *      — so new knobs are covered automatically, no drift), one set per
 *      instrument per 16 ms tick, values alternating within [min,max].
 *   2. mixer-fx: round-robin live edits over insert-FX params (from
 *      insertEffectCatalog()), EQ band gains, HPF/LPF, channel volume/pan,
 *      delay/reverb sends, row + master volume.
 *
 * Optionally repeats the synth-knobs phase under 4x CDP CPU throttle
 * (mobile-class; ALL_KNOB_THROTTLE=0 to skip).
 *
 * GATES
 * -----
 * PRIMARY (machine-independent, structural):
 *   - every visible knob of every instrument was actually written (>=1 pass)
 *   - the sequencer really advanced (currentBeat) at every BPM
 *   - mainThreadRenders === 0 — no heavy C render may run synchronously on
 *     the main thread during playback
 *   - overdue === 0 — no audio event scheduled into the past
 * SECONDARY (behavioural, env-tunable):
 *   - deferAbsMsP90 <= ALL_KNOB_DEFER_P90_MAX_MS (notes land on-grid)
 *   - worst event-loop gap <= ALL_KNOB_MAX_GAP_MS
 *
 * CHARTER: WebAudio scheduling + WASM bridge behavior under load — JS-only
 * surface (Go cannot observe AudioContext timing or the render worker).
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

// ── Tunables (env-overridable) ──
const BPMS = (process.env.ALL_KNOB_BPMS || "60 174 300").split(/\s+/).map(Number);
const PHASE_MS = Number(process.env.ALL_KNOB_PHASE_MS || 2600);
const SPAM_INTERVAL_MS = Number(process.env.ALL_KNOB_SPAM_MS || 16); // ~60 Hz
// maxGap must absorb one-off Go-WASM GC pauses (measured ~230–300 ms roaming
// across phases at 180 knob-writes/s — each write clones + marshals the full
// ~1088-entry modular block; see the delta-push improvement note in the
// header). Sustained jank is caught by the much tighter p99 gate instead.
// RATCHET both down once param pushes become deltas.
const MAX_GAP_MS = Number(process.env.ALL_KNOB_MAX_GAP_MS || 350);
const P99_GAP_MS = Number(process.env.ALL_KNOB_P99_GAP_MS || 30);
const DEFER_P90_MAX_MS = Number(process.env.ALL_KNOB_DEFER_P90_MAX_MS || 15);
const THROTTLE_RATE = Number(process.env.ALL_KNOB_THROTTLE || 4); // 0 = skip
const THROTTLE_MAX_GAP_MS = Number(process.env.ALL_KNOB_THROTTLE_MAX_GAP_MS || 350);
const THROTTLE_P99_GAP_MS = Number(process.env.ALL_KNOB_THROTTLE_P99_GAP_MS || 60);
const THROTTLE_DEFER_P90_MAX_MS = Number(process.env.ALL_KNOB_THROTTLE_DEFER_P90_MAX_MS || 15);
// ALL_KNOB_SOLO=<instrument-id>: restrict the synth-knob spam to one
// instrument (realistic one-finger drag) instead of all rows at once.
const SOLO_INSTRUMENT = process.env.ALL_KNOB_SOLO || "";

// Rows: melodic waveguide / heaviest additive-unison / config-kick drum.
const ROW_INSTRUMENTS = ["cello", "organ-church", "dnb-kick"];

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
await page.waitForFunction(() => typeof window.buildPerfRect === "function");
await page.waitForFunction(() => typeof window.getRenderLatencyMetrics === "function");
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

// ── Circuit + instruments + one insert-FX slot per row (params spammed live) ──
const setup = await page.evaluate(async (ROW_INSTRUMENTS) => {
  buildPerfRect(ROW_INSTRUMENTS.length, 2);
  for (let r = 0; r < ROW_INSTRUMENTS.length; r++) setRowInstrument(r, ROW_INSTRUMENTS[r]);
  // Give warm-on-switch a moment before the storm starts.
  await new Promise((res) => setTimeout(res, 600));

  // Enumerate every USER-FACING knob per instrument from the live registry —
  // group "hidden" is engine plumbing, everything else renders in the Synth tab.
  const cat = synthRecipeCatalog();
  const knobs = {};
  for (const id of ROW_INSTRUMENTS) {
    const rid = recipeForInstrument(id);
    const params = (cat[rid] && cat[rid].params) || [];
    knobs[id] = params
      .filter((d) => d.group !== "hidden")
      .map((d) => ({ name: d.name, min: d.min, max: d.max, def: d.default }));
  }

  // One insert effect per row so setInsertEffectParam has a live target.
  const fxCat = insertEffectCatalog();
  const fxTypes = Object.keys(fxCat).sort();
  const fx = {};
  ROW_INSTRUMENTS.forEach((id, i) => {
    const type = fxTypes[i % fxTypes.length];
    const slot = addInsertEffect(id, type);
    fx[id] = { slot, type, params: fxCat[type].map((p) => ({ name: p.name, min: p.min, max: p.max })) };
  });
  return { knobs: Object.fromEntries(Object.entries(knobs).map(([k, v]) => [k, v.length])), fx: Object.fromEntries(Object.entries(fx).map(([k, v]) => [k, v.type])), fxTypes };
}, ROW_INSTRUMENTS);
console.log(`[TEST] knobs per instrument: ${JSON.stringify(setup.knobs)} insert-fx: ${JSON.stringify(setup.fx)}`);
for (const [id, n] of Object.entries(setup.knobs)) {
  if (!n || n < 5) throw new Error(`instrument ${id} exposes only ${n} visible knobs — enumeration broken`);
}

// runPhase spams live edits at SPAM_INTERVAL_MS while the sequencer plays.
// mode: "synth-knobs" | "mixer-fx". Returns metrics.
async function runPhase(mode, bpm, phaseOpts = {}) {
  return await page.evaluate(async ({ mode, bpm, ROW_INSTRUMENTS, PHASE_MS, SPAM_INTERVAL_MS, SOLO }) => {
    setBPM(bpm);
    window.__audioMetrics = { renders: {}, cacheHits: {} };
    if (window.resetRenderLatencyMetrics) window.resetRenderLatencyMetrics();
    if (window.resetAudioScheduleMetrics) window.resetAudioScheduleMetrics();

    const cat = synthRecipeCatalog();
    const insts = ROW_INSTRUMENTS.map((id) => {
      const rid = recipeForInstrument(id);
      const defs = ((cat[rid] && cat[rid].params) || []).filter((d) => d.group !== "hidden");
      return { id, defs, idx: 0, visits: 0 };
    });
    const fxCat = insertEffectCatalog();
    const fxState = ROW_INSTRUMENTS.map((id, i) => {
      const effects = getInsertEffects(id) || [];
      const slot = effects.length - 1;
      const type = effects.length ? effects[effects.length - 1].type : null;
      const params = type ? fxCat[type] : [];
      return { id, slot, params, idx: 0 };
    });

    const beat0 = currentBeat();
    const gaps = [];
    let last = performance.now();
    const sampler = setInterval(() => { const n = performance.now(); gaps.push(n - last); last = n; }, 8);

    let tick = 0;
    const touched = insts.map(() => new Set());
    const spam = setInterval(() => {
      tick++;
      if (mode === "synth-knobs") {
        for (let r = 0; r < insts.length; r++) {
          const s = insts[r];
          if (SOLO && s.id !== SOLO) continue;
          if (!s.defs.length) continue;
          const d = s.defs[s.idx % s.defs.length];
          const pass = Math.floor(s.idx / s.defs.length);
          const span = d.max - d.min;
          // Alternate between two in-range values so the no-op coalesce
          // layers never swallow the write; snap near-integer ranges.
          let v = d.min + (pass % 2 === 0 ? 0.3 : 0.75) * span;
          if (span <= 16 && Number.isInteger(d.min) && Number.isInteger(d.max)) {
            v = d.min + ((pass + s.idx) % (span + 1));
          }
          setInstrumentParam(s.id, d.name, v);
          touched[r].add(d.name);
          s.idx++;
        }
      } else {
        const id = ROW_INSTRUMENTS[tick % ROW_INSTRUMENTS.length];
        const row = tick % ROW_INSTRUMENTS.length;
        const ph = tick % 8;
        const f = fxState[row];
        switch (ph) {
          case 0: if (f.slot >= 0 && f.params.length) { const p = f.params[f.idx++ % f.params.length]; setInsertEffectParam(id, f.slot, p.name, p.min + ((f.idx % 2) ? 0.7 : 0.3) * (p.max - p.min)); } break;
          case 1: setEQBandGain(id, tick % 10, (tick % 2 ? 4 : -4)); break;
          case 2: setEQHPF(true, 40 + (tick % 5) * 100); break;
          case 3: setEQLPF(true, 2000 + (tick % 5) * 2000); break;
          case 4: window.setChannelVolume(id, 0.4 + 0.1 * (tick % 5)); window.setChannelPan(id, ((tick % 5) - 2) / 2.5); break;
          case 5: window.setDelaySend(id, 0.1 * (tick % 6)); break;
          case 6: window.setReverbSend(id, 0.1 * (tick % 6)); break;
          case 7: setRowVolume(row, 0.5 + 0.1 * (tick % 4)); setMasterVolume(0.5 + 0.1 * (tick % 4)); break;
        }
      }
    }, SPAM_INTERVAL_MS);

    await new Promise((r) => setTimeout(r, PHASE_MS));
    clearInterval(spam); clearInterval(sampler);
    await new Promise((r) => setTimeout(r, 300)); // let final renders flush

    gaps.sort((a, b) => a - b);
    const q = (p) => (gaps.length ? gaps[Math.min(gaps.length - 1, Math.floor(p * gaps.length))] : 0);
    const am = window.getAudioScheduleMetrics ? window.getAudioScheduleMetrics() : {};
    const rl = window.getRenderLatencyMetrics ? window.getRenderLatencyMetrics() : {};
    const met = window.__audioMetrics || {};
    const sum = (m) => Object.values(m || {}).reduce((a, b) => a + b, 0);
    return {
      beatAdvanced: currentBeat() - beat0,
      knobsTouched: touched.map((s) => s.size),
      knobsTotal: insts.map((s) => s.defs.length),
      renders: sum(met.renders),
      workerRenders: sum(met.workerRenders),
      mainThreadRenders: sum(met.mainThreadRenders),
      maxGapMs: gaps.length ? gaps[gaps.length - 1] : 0,
      p99GapMs: q(0.99),
      p90GapMs: q(0.90),
      deferredPlays: rl.deferredPlays || 0,
      deferAbsMsP90: rl.deferAbsMsP90 || 0,
      deferMsAvg: rl.deferMsAvg || 0,
      deferMsMin: rl.deferMsMin || 0,
      deferMsMax: rl.deferMsMax || 0,
      stalePlays: rl.stalePlays || 0,
      neighborPlays: rl.neighborPlays || 0,
      renderMsP90: rl.renderMsP90 || 0,
      workerQueuePeak: rl.workerQueuePeak || 0,
      audio: { count: am.count, overdue: am.overdue, smallLead: am.smallLeadCount },
    };
  }, { mode, bpm, ROW_INSTRUMENTS, PHASE_MS, SPAM_INTERVAL_MS, SOLO: phaseOpts.solo ?? SOLO_INSTRUMENT });
}

const failures = [];

function evaluatePhase(label, m, opts = {}) {
  const maxGap = opts.maxGapMs ?? MAX_GAP_MS;
  const p99Gap = opts.p99GapMs ?? P99_GAP_MS;
  const deferP90 = opts.deferP90Max ?? DEFER_P90_MAX_MS;
  console.log(
    `[TEST] ${label}: beat+${m.beatAdvanced} audio=${JSON.stringify(m.audio)} renders=${m.renders} (worker=${m.workerRenders} main=${m.mainThreadRenders}) ` +
    `defer=${m.deferredPlays} (absP90 ${m.deferAbsMsP90}ms avg ${m.deferMsAvg}ms min ${m.deferMsMin}ms max ${m.deferMsMax}ms) stale=${m.stalePlays} nbr=${m.neighborPlays} renderP90=${m.renderMsP90}ms qPeak=${m.workerQueuePeak} ` +
    `maxGap=${m.maxGapMs.toFixed(1)}ms p99Gap=${m.p99GapMs.toFixed(1)}ms p90Gap=${m.p90GapMs.toFixed(1)}ms knobs=${JSON.stringify(m.knobsTouched)}/${JSON.stringify(m.knobsTotal)}`,
  );
  if (opts.infoOnly) return; // saturation-documenting scenario: log, don't gate
  // Structural: real playback happened.
  if (m.beatAdvanced <= 0) failures.push(`${label}: sequencer did not advance (beat+${m.beatAdvanced}) — not a playback test`);
  if (!m.audio || !(m.audio.count > 0)) failures.push(`${label}: no audio events were scheduled during the phase`);
  // Structural: full knob coverage (synth phase only).
  if (opts.requireAllKnobs) {
    m.knobsTouched.forEach((n, i) => {
      if (SOLO_INSTRUMENT && ROW_INSTRUMENTS[i] !== SOLO_INSTRUMENT) return;
      if (n < m.knobsTotal[i]) failures.push(`${label}: instrument ${ROW_INSTRUMENTS[i]} — only ${n}/${m.knobsTotal[i]} knobs written; increase ALL_KNOB_PHASE_MS`);
    });
  }
  // PRIMARY machine-independent gates.
  if (m.mainThreadRenders > 0) failures.push(`${label}: ${m.mainThreadRenders} render(s) ran on the MAIN THREAD during playback (must be 0 — each blocks UI + audio scheduler)`);
  if (m.audio && m.audio.overdue > 0) failures.push(`${label}: ${m.audio.overdue} overdue audio event(s) scheduled in the past`);
  // SECONDARY behavioural gates.
  if (m.deferAbsMsP90 > deferP90) failures.push(`${label}: deferAbsMsP90 ${m.deferAbsMsP90}ms > ${deferP90}ms — notes not landing on-grid under edit load`);
  if (m.maxGapMs > maxGap) failures.push(`${label}: worst event-loop gap ${m.maxGapMs.toFixed(1)}ms exceeds ${maxGap}ms ceiling`);
  if (m.p99GapMs > p99Gap) failures.push(`${label}: p99 event-loop gap ${m.p99GapMs.toFixed(1)}ms exceeds ${p99Gap}ms — sustained main-thread jank`);
}

// ── Main matrix: BPM × {synth-knobs, mixer-fx} under real playback ──
await page.evaluate(() => startPlay());
await page.waitForTimeout(300);

for (const bpm of BPMS) {
  const synth = await runPhase("synth-knobs", bpm);
  evaluatePhase(`bpm=${bpm} synth-knobs`, synth, { requireAllKnobs: true });
  const mixer = await runPhase("mixer-fx", bpm);
  evaluatePhase(`bpm=${bpm} mixer-fx`, mixer);
}

// ── Mobile-class CPU throttle scenarios (4x) at mid BPM ──
// (a) GATED realistic mobile contract: ONE instrument's knobs dragged (a
//     finger can only touch one knob) — must stay on-grid, no main-thread
//     renders, bounded jank. This is the "synth edits are real-time on
//     mobile WASM" guarantee.
// (b) INFO-ONLY saturation probe: all 3 instruments spammed at once under
//     throttle. Currently saturates the render pipeline (deferAbsMsP90
//     measured ~715 ms); logged for tracking, not gated.
if (THROTTLE_RATE > 1) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: THROTTLE_RATE });
  // Throttle-onset warm-up (results discarded): engaging CDP throttling
  // mid-session makes Chrome recalibrate timers / re-tier JIT, and the first
  // spam phase after onset eats a one-off 100-700ms defer transient that a
  // real always-slow device never sees (verified: whichever scenario runs
  // first is bad, the second is clean regardless of scenario). Measure
  // steady-state throttled editing, not the emulator transient.
  await page.evaluate(() => setBPM(174));
  await runPhase("synth-knobs", 174, { solo: ROW_INSTRUMENTS[0] });
  const solo = await runPhase("synth-knobs", 174, { solo: ROW_INSTRUMENTS[0] });
  const storm = await runPhase("synth-knobs", 174);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: 1 });
  evaluatePhase(`throttle=${THROTTLE_RATE}x bpm=174 solo-knob-drag (${ROW_INSTRUMENTS[0]})`, solo, {
    maxGapMs: THROTTLE_MAX_GAP_MS,
    p99GapMs: THROTTLE_P99_GAP_MS,
    deferP90Max: THROTTLE_DEFER_P90_MAX_MS,
  });
  evaluatePhase(`throttle=${THROTTLE_RATE}x bpm=174 3-instrument storm (INFO)`, storm, { infoOnly: true });
}

await page.evaluate(() => stopPlay());

if (pageErrors.length > 0) {
  failures.push(`page emitted ${pageErrors.length} error(s): ${pageErrors.join(" | ")}`);
}

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "webaudio_all_knob_bpm_stress",
  );
}
await browser.close();
server.close();

if (failures.length > 0) {
  console.error("\nwebaudio_all_knob_bpm_stress FAILED:\n  - " + failures.join("\n  - "));
  process.exit(1);
}
console.log("\nwebaudio_all_knob_bpm_stress browser test PASSED");
