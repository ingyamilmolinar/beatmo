/**
 * EXACT production reproduction of the user's report (WASM, full game):
 *
 *   click Play → solo Snare → open Synth tab → change the generator → audio dies.
 *
 * Unlike the earlier tests this uses the production index.html + main.wasm, taps
 * the real AudioContext destination, and drives REAL canvas input (solo button
 * click + a real mouse WHEEL over the generator control) — not the
 * setInstrumentParam export. It measures the master-output RMS in a window AFTER
 * the change and fails if the audio goes silent. The snare is SOLOED, so if
 * re-voicing silences the snare, ALL audio dies — matching "the audio dies".
 *
 * POP-UP-CONTROLS MODEL: the synth control's horizontal rotary drag was removed —
 * a tap now opens the value-wheel popup, and an in-place value change comes from
 * the mouse wheel over the control (synthControlHitAdapter.OnWheel). This test
 * drives that wheel; see the value-change block below.
 *
 * The generator knob is the gen-slot `gen1_wave` knob (0=Sine default, 1=Saw, 2=Square, 3=Triangle). `osc_type` only exists on the unified modular
 * recipe, kept here as a fallback in case the row is rebound.
 *
 * CHIP-STRIP REDESIGN: the Synth tab now renders one compact chip per pipeline
 * stage (VOICE·OSC·FM·PITCH·LFO·BURST·ENVELOPE·FILTER·POST) with exactly ONE
 * stage expanded into a detail pane. synthControlRects() still returns one entry
 * per wired knob, but knobs of collapsed (non-selected) stages have ZERO-SIZE
 * rects (w=0,h=0) — only the selected stage's knobs are interactable. The
 * `gen1_wave` knob lives in the VOICE stage; osc_type lives in
 * the OSC stage. We call selectSynthSection() to open the owning stage, then
 * filter for the knob with a non-zero rect.
 */
import { chromium } from "playwright";
import {
  buildMainWasm, createServer, clickUntilStateChanges,
  rectCenter, assertValidRect, waitForGameLoopRelease,
} from "./real_input_test_helpers.js";
import { waitForAudioReady } from "./browser_test_helpers.js";
import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";

if (!buildMainWasm({ logLevel: "INFO" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
page.on("console", (m) => { const t = m.text(); if (m.type() === "error") pageErrors.push("console.error: " + t); try { console.log("[PAGE]", m.type(), t); } catch (_) {} });

// Tap the real AudioContext destination; windowed accumulators we reset per phase.
await page.addInitScript(() => {
  const RealAC = window.AudioContext || window.webkitAudioContext;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(1024, 1, 1);
      window.__rmsAccum = 0; window.__nonZero = 0; window.__frames = 0;
      sp.addEventListener("audioprocess", (e) => {
        const d = e.inputBuffer.getChannelData(0);
        for (let i = 0; i < d.length; i++) { window.__rmsAccum += d[i] * d[i]; if (d[i] !== 0) window.__nonZero++; }
        window.__frames += d.length;
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC; window.webkitAudioContext = TestAC;
});

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };
const measure = async (ms) => {
  await page.evaluate(() => { window.__rmsAccum = 0; window.__nonZero = 0; window.__frames = 0; });
  await page.waitForTimeout(ms);
  return page.evaluate(() => ({ frames: window.__frames || 0, nonZero: window.__nonZero || 0, rms: window.__frames ? Math.sqrt(window.__rmsAccum / window.__frames) : 0 }));
};
const RMS_MIN = 1e-4;

// A purpose-built circuit (never the startup demo, which is hand-edited): a
// kick loop and a snare loop, so soloing the snare silences the kick and a
// snare-silencing bug presents as "all audio dies".
const loop = (base, j) => [0, 4, 8, 12].map((i, k) => ({ id: base + k, i, j, type: "regular", outputs: [base + ((k + 1) % 4)] }));
const CIRCUIT = JSON.stringify({
  schema_version: "0.0.1", subdiv: 8, bpm: 160,
  instruments: [
    { name: "Kick", id: "kick", kind: "builtin", volume: 1.0, origin: 1, color: "#C87850FF" },
    { name: "Snare", id: "snare", kind: "builtin", volume: 1.0, origin: 5, color: "#50A0C8FF" },
  ],
  nodes: [...loop(1, 0), ...loop(5, 2)],
});

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof startPlay === "function" && typeof rowSoloBtnRect === "function" && typeof synthControlRects === "function", null, { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 30000 });
  await page.evaluate((j) => { importJSON(j); forceDraw?.(); }, CIRCUIT);
  await page.waitForFunction(() => typeof totalRows === "function" && totalRows() === 2, null, { timeout: 15000 });
  await page.waitForTimeout(300);
  await waitForGameLoopRelease(page);

  const abi = await page.evaluate(() => window.__synthABI || null);
  console.log("[TEST] __synthABI =", JSON.stringify(abi));
  if (abi && abi.stale) fail("synth_param_abi.gen.js STALE — re-voiced instruments will be silent.");

  // Locate the Snare row dynamically (don't assume an index).
  const nRows = await page.evaluate(() => totalRows());
  let snareRow = -1;
  for (let i = 0; i < nRows; i++) {
    const inst = await page.evaluate((r) => (typeof rowInstrument === "function" ? rowInstrument(r) : ""), i);
    if (inst === "snare") { snareRow = i; break; }
  }
  console.log(`[TEST] rows=${nRows} snareRow=${snareRow}`);
  if (snareRow < 0) throw new Error("the imported circuit has no snare row");

  // Unlock audio with a real gesture, then Play (ignored until the import's
  // warm-up gate opens).
  await page.mouse.click(200, 200);
  await waitForAudioReady(page, { allowReject: true });
  await startPlaybackWhenReady(page);

  // Solo the Snare. We try the real solo-button click first (faithful), and
  // fall back to toggleSolo() — same solo STATE — if the desktop layout doesn't
  // expose that row's button rect. The solo state (only snare audible) is what
  // makes a snare-silencing bug present as "all audio dies".
  await page.evaluate((r) => { window.__snareRow = r; }, snareRow);
  const soloRect = await page.evaluate(() => rowSoloBtnRect(window.__snareRow));
  if (soloRect && soloRect.w > 0 && soloRect.h > 0) {
    const sc = rectCenter(soloRect);
    await clickUntilStateChanges(page, sc.x, sc.y, () => rowSoloed(window.__snareRow), false);
    await page.mouse.move(0, 0);
  } else {
    await page.evaluate(() => toggleSolo(window.__snareRow));
  }
  await waitForGameLoopRelease(page);
  const soloed = await page.evaluate(() => rowSoloed(window.__snareRow));
  console.log("[TEST] snare soloed =", soloed);
  if (soloed !== true) fail("could not solo the snare");

  // Make sure the Synth tab edits the snare (solo auto-selects it; be explicit).
  await page.evaluate(() => { if (typeof setEQChannel === "function") setEQChannel("snare"); });

  // Baseline: snare soloed + playing → master output must be non-silent.
  const base = await measure(1500);
  console.log(`[TEST] baseline (snare soloed, Native) frames=${base.frames} nonZero=${base.nonZero} rms=${base.rms.toFixed(6)}`);
  if (!(base.rms > RMS_MIN)) fail(`no audio with snare soloed BEFORE the change (rms=${base.rms}) — solo/setup issue.`);

  // Open the Synth tab (the soloed snare becomes the edited instrument).
  await page.evaluate(() => { if (typeof setEQTab === "function") setEQTab("synth"); else if (typeof setActiveEQTab === "function") setActiveEQTab("synth"); });
  await page.waitForTimeout(400);
  await waitForGameLoopRelease(page);

  // CHIP-STRIP: the generator knob (gen1_wave / osc_type) lives in a collapsed
  // stage by default unless that stage happens to be the first knobbed enabled
  // stage. Open its owning stage so its rect is laid out (non-zero). gen1_wave
  // → VOICE, osc_type → OSC. We try VOICE first (drum recipe default), and if
  // the row is the modular voice the OSC fallback path handles osc_type.
  await page.evaluate(() => { if (typeof selectSynthSection === "function") selectSynthSection("VOICE"); });
  await page.waitForTimeout(200);
  await waitForGameLoopRelease(page);

  // Find the generator knob so we can wheel it (Sine → another waveform). Only
  // knobs with a non-zero rect belong to the selected (expanded) stage; collapsed
  // stages report w=0/h=0 and are not interactable.
  let knobs = await page.evaluate(() => (typeof synthControlRects === "function" ? synthControlRects() : []));
  const laidOut = (k) => k && k.w > 0 && k.h > 0;
  // The snare (synth-modular-snare) exposes its generator waveform as the
  // gen-slot knob gen1_wave.
  const GEN_KNOBS = ["gen1_wave"];
  let gen = knobs.find((k) => GEN_KNOBS.includes(k.name) && laidOut(k));
  if (!gen) {
    // Modular voice fallback: osc_type lives in the OSC stage.
    await page.evaluate(() => { if (typeof selectSynthSection === "function") selectSynthSection("OSC"); });
    await page.waitForTimeout(200);
    await waitForGameLoopRelease(page);
    knobs = await page.evaluate(() => (typeof synthControlRects === "function" ? synthControlRects() : []));
    gen = knobs.find((k) => k.name === "osc_type" && laidOut(k));
  }
  console.log("[TEST] synth knob names =", JSON.stringify(knobs.map((k) => k.name)));
  if (!gen) { fail("no laid-out generator knob (gen1_wave/osc_type) on the Synth tab — solo did not target snare, owning stage not selected, or tab not open."); }
  else {
    const genName = gen.name;
    const cx = Math.round(gen.x + gen.w / 2), cy = Math.round(gen.y + gen.h / 2);
    const paramsBefore = await page.evaluate(() => getInstrumentParams("snare") || {});
    const before = paramsBefore[genName] ?? 0;
    // ARMED-CONTROL MODEL: a click arms the Synth control and the arrow keys
    // step its value (Up = next value; armedControlKeys claims them), Enter
    // accepts. The earlier wheel-over-control nudge no longer exists — five
    // wheel notches changed no knob at all, which left this test failing on
    // the gesture before it ever reached the regression it guards. Two Up
    // presses move the enum Sine 0 → Square 2, crossing a waveform boundary
    // one step at a time (the window a mid-gesture schema rebind would fire
    // in). Still REAL canvas + keyboard input, not the setInstrumentParam export.
    await page.mouse.click(cx, cy);
    await waitForGameLoopRelease(page);
    for (let i = 0; i < 2; i++) { await page.keyboard.press("ArrowUp"); await page.waitForTimeout(80); }
    await page.keyboard.press("Enter");
    await waitForGameLoopRelease(page);
    const after = await page.evaluate((n) => (getInstrumentParams("snare") || {})[n] ?? 0, genName);
    console.log(`[TEST] ${genName} via real input: ${before} -> ${after}`);
    if (after === before) fail(`the generator wheel did not change snare ${genName} (${before} -> ${after}) — control not hit / not targeting snare.`);

    // ROOT-CAUSE GUARD: a generator change must touch ONLY the generator param.
    // The historical bug: crossing gen_type Native→waveform swapped the synth
    // tab's bespoke schema to the 33-knob modular pipeline mid-gesture; the
    // handler (captured/indexed) then drove a foreign modular param (fm_op1_ratio,
    // and in the worst case gain/osc_enabled/amp_decay, which silence the
    // re-voiced voice until reload). Schema rebinds can still come from any
    // source, so the guard stays. Go unit guards:
    // TestSynthKnobAdapter_PropagateIsIndexSafe /
    // TestSynthKnobAdapter_PropagateWritesStableParam
    // (synth_knob_adapter_index_safe_test.go).
    // getInstrumentParams returns the instrument's whole document (every
    // value), so compare it before and after: only the generator may move.
    const params = await page.evaluate(() => getInstrumentParams("snare") || {});
    const foreign = [...new Set([...Object.keys(paramsBefore), ...Object.keys(params)])]
      .filter((k) => k !== genName && paramsBefore[k] !== params[k]);
    console.log(`[TEST] snare params changed by the generator change: ${JSON.stringify(Object.fromEntries(foreign.concat(genName).map((k) => [k, [paramsBefore[k], params[k]]])))}`);
    if (foreign.length) {
      fail(`generator change scrambled foreign param(s) [${foreign.join(", ")}] — a generator change must touch only ${genName} (mid-gesture schema swap regression).`);
    }

    // THE REPRO: measure master output AFTER the generator change. Leave the
    // Synth tab FIRST: under software WebGL (headless CI Chromium) the Synth
    // tab is the heaviest draw in the app and its per-frame cost starves the
    // WebAudio rendering pipeline — master output collapses ~100x while the
    // tab is open and recovers the moment it closes (verified: tab open/close
    // toggles the collapse with NO graph or param change; in-graph
    // AnalyserNodes see the same collapse, so it is not a tap artifact). The
    // regression this test guards — a scrambled foreign param silencing the
    // re-voiced snare until reload — persists across tab switches, so
    // measuring on the EQ tab still catches it without the env-induced
    // false positive.
    await page.evaluate(() => { if (typeof setEQTab === "function") setEQTab("eq"); else if (typeof setActiveEQTab === "function") setActiveEQTab("eq"); });
    await page.waitForTimeout(400);
    await waitForGameLoopRelease(page);
    // Use a RELATIVE floor (5% of the soloed-snare baseline) as well as the
    // absolute one — a stale ABI collapses RMS ~250x (0.057 → 0.0002), which is
    // silence even though the residual fade transient technically clears the
    // abs floor.
    const post = await measure(2500);
    const relFloor = Math.max(RMS_MIN, base.rms * 0.05);
    console.log(`[TEST] AFTER generator change frames=${post.frames} nonZero=${post.nonZero} rms=${post.rms.toFixed(6)} (floor=${relFloor.toFixed(6)})`);
    if (!(post.rms > relFloor)) {
      fail(`AUDIO DIED after changing the soloed snare's generator (rms=${post.rms.toFixed(6)} < ${relFloor.toFixed(6)}, ${(base.rms / Math.max(post.rms, 1e-9)).toFixed(0)}x drop). Reproduces the user report.`);
    }
  }
} catch (e) {
  fail("exception: " + (e && e.message ? e.message : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (pageErrors.length) console.log("[TEST] page errors:", pageErrors.slice(0, 10));
if (failed || pageErrors.length) { console.error("[TEST] synth generator solo real-input: FAIL"); process.exit(1); }
console.log("[TEST] synth generator solo real-input: PASS");
