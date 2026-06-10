/**
 * Regression: changing a synth param (any Synth-tab knob) on an
 * instrument carrying sample PCM must NOT silence it (WASM, production build).
 *
 * Original bug ("play → solo → synth tab → drag the oscillator knob → that
 * instrument's audio instantly stops, dead until reload"):
 * `registerSamplePCM(id)` converts an instrument to a sample by DELETING
 * RENDER[id] and storing its only PCM copy in renderCache[id]. The param-change
 * handler `_updateInstrumentParams` then did `renderCache.delete(id)`
 * UNCONDITIONALLY — for a sample that throws away the only audio, and
 * processAudioEvent can't re-render it (no RENDER[id]) → permanent silence.
 *
 * Current semantics ("synths remain synths", 2026-06-03):
 *  - FACTORY instrument (has RENDER_DEFAULTS, e.g. "snare"): a synth edit
 *    RESTORES the factory render and re-renders with the new params — desktop
 *    precedence (recipe path wins once params exist). Audible, never frozen.
 *    Deep coverage: synth_edits_override_stale_sample.browser.test.js.
 *  - PURE user sample (no RENDER_DEFAULTS, e.g. "user.sample.x"): the PCM is
 *    the only audio — a param edit must leave its buffer playing untouched.
 *
 * Both scenarios assert the same invariant: a param change NEVER kills the
 * audio.
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/synth_sample_param_change_keeps_audio.browser.test.js
 */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

if (!buildMainWasm({ logLevel: "INFO" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
page.on("console", (m) => { try { console.log("[PAGE]", m.type(), m.text()); } catch (_) {} });

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof setInstrumentParam === "function" && typeof window.registerSamplePCM === "function" && typeof window.playSound === "function", { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, { timeout: 30000 });
  await page.mouse.click(200, 200);
  await page.evaluate(async () => { try { await window.audioReady; } catch (_) {} });
  await page.waitForFunction(() => window.__audioCtx ? window.__audioCtx.state === "running" : true, { timeout: 10000 }).catch(() => {});

  const INST = "snare";

  // Drive the whole experiment inside the page: register a PCM sample, then
  // change synth params, asserting the instrument stays AUDIBLE at every step.
  //
  // "Audible" is measured off the RENDERED / registered PCM buffer
  // (getCachedRenderData + ensureSynthSample), NOT the live AudioContext output
  // capture. The ScriptProcessor capture node underruns non-deterministically
  // under single-CPU headless SwiftShader and drops the loud transient of a
  // short decaying one-shot — even a fresh, full-amplitude sample reads as
  // ~silent and varies ~30x run-to-run (see project_synth_tab_audio_starvation).
  // The C DSP render is deterministic and env-independent, so it faithfully
  // proves the production invariant ("a param change never silences the
  // instrument") that the param-change handler in _updateInstrumentParams owns.
  const result = await page.evaluate(async (id) => {
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const sr = 44100, frames = Math.floor(sr * 0.3);
    // An audible decaying 220 Hz tone as the instrument's "sample".
    const f32 = new Float32Array(frames);
    for (let i = 0; i < frames; i++) f32[i] = Math.sin(2 * Math.PI * 220 * i / sr) * Math.exp(-3 * i / frames);
    const u8 = new Uint8Array(f32.buffer.slice(0));

    // Convert the instrument to a SAMPLE (this is what the app does for some
    // default instruments) — deletes RENDER[id], stores PCM in renderCache.
    window.registerSamplePCM(id, u8, sr);

    // Peak of the audio the instrument WOULD play: the registered PCM if present,
    // else a freshly forced synth render (the factory-restore path after a param
    // edit clears renderCache and re-binds RENDER[id]).
    const peakOf = async () => {
      let d = window.getCachedRenderData(id);
      if (!d || !d.length) { await window.ensureSynthSample(id); d = window.getCachedRenderData(id); }
      if (!d || !d.length) return 0;
      let peak = 0; for (let i = 0; i < d.length; i++) { const a = Math.abs(d[i]); if (a > peak) peak = a; }
      return peak;
    };

    const before = await peakOf();
    // Change the generator param on the SAMPLE instrument (the user gesture).
    setInstrumentParam(id, "decay", 2);
    await sleep(50);
    const after = await peakOf();
    // And a second param to be thorough.
    setInstrumentParam(id, "drive", 0.6);
    await sleep(50);
    const after2 = await peakOf();
    return { before, after, after2 };
  }, INST);

  console.log(`[TEST] sample-instrument render peaks: before=${result.before.toFixed(5)} afterDecay=${result.after.toFixed(5)} afterDrive=${result.after2.toFixed(5)}`);
  if (!(result.before > 1e-3)) fail(`sample instrument was not audible to begin with (peak=${result.before}) — test setup issue`);
  if (!(result.after > 1e-3)) fail(`sample instrument went SILENT after a synth-param change (peak=${result.after}). Reproduces "changing the generator kills the audio".`);
  if (!(result.after2 > 1e-3)) fail(`sample instrument went SILENT after a second param change (peak=${result.after2}).`);

  // Scenario 2: a PURE user sample (no factory render) — the protection
  // branch. Its PCM is the only audio; a param edit must leave it playing
  // bit-identically (there is no synth to re-render).
  const userResult = await page.evaluate(async () => {
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const id = "user.sample.protection-check";
    const sr = 44100, frames = Math.floor(sr * 0.3);
    const f32 = new Float32Array(frames);
    for (let i = 0; i < frames; i++) f32[i] = Math.sin(2 * Math.PI * 440 * i / sr) * Math.exp(-3 * i / frames);
    window.registerSamplePCM(id, new Uint8Array(f32.buffer.slice(0)), sr);
    const sig = () => {
      const d = window.getCachedRenderData(id);
      return d && d.length ? `${d.length}:${d[100]}:${d[1000]}` : String(d);
    };
    const sigBefore = sig();
    // Param edits arrive via the same bridge handler; with no RENDER_DEFAULTS
    // for this id the PCM must survive untouched.
    window.updateInstrumentParams(id, JSON.stringify({ drive: 0.5, decay: 4 }));
    await sleep(50);
    const sigAfter = sig();
    return { sigBefore, sigAfter, intact: sigBefore === sigAfter && sigBefore !== "undefined" };
  });
  console.log(`[TEST] pure user sample PCM intact after edits: ${userResult.intact} (${userResult.sigBefore} → ${userResult.sigAfter})`);
  if (!userResult.intact) fail(`pure user sample PCM was dropped/changed by a param edit (${userResult.sigBefore} → ${userResult.sigAfter}) — the silence-protection branch regressed`);
} catch (e) {
  fail("exception: " + (e && e.message ? e.message : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (pageErrors.length) console.log("[TEST] page errors:", pageErrors.slice(0, 10));
if (failed || pageErrors.length) { console.error("[TEST] sample param-change keeps audio: FAIL"); process.exit(1); }
console.log("[TEST] sample param-change keeps audio: PASS");
