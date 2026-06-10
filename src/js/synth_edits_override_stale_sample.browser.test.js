/**
 * Regression: synth edits must take effect on a recipe-bound builtin even when
 * a STALE persisted sample (legacy destructive Sampler-Save PCM in IndexedDB)
 * exists for that instrument id (WASM, production build).
 *
 * Bug class (user-reported "knobs / stage toggles / Save do nothing on WASM,
 * desktop fine — tested with snare"): pre-descriptor Sampler-Save baked PCM
 * over a builtin and persisted it. On every later browser session,
 * audio.ApplySavedSamples re-registers that PCM (registerSamplePCM →
 * DELETES RENDER['snare'], plants frozen PCM in renderCache) while
 * bindBuiltinInstrumentRecipes restores the snare→drum-snare binding. The
 * Synth tab then shows live knobs (binding intact) and every edit flows
 * through the Go→JS bridge — but _updateInstrumentParams skips cache
 * invalidation for sample instruments (no RENDER[id]) and ensureRenderedSample
 * can't re-render, so the sound NEVER changes. Desktop in the identical state
 * takes the recipe path as soon as params exist, so edits are audible there.
 *
 * Intended semantics (2026-06-03): synths remain synths — a recipe-bound
 * builtin must never be frozen by a stale PCM registration; synth edits win
 * on both platforms.
 *
 * This test seeds the legacy state end-to-end (IndexedDB → reload →
 * ApplySavedSamples) and asserts synth knob edits still re-render audibly.
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/synth_edits_override_stale_sample.browser.test.js
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
page.on("console", (m) => {
  const t = m.text();
  if (/AUDIOJS|SYNTH-|FAIL|error/i.test(t)) { try { console.log("[PAGE]", m.type(), t); } catch (_) {} }
});

let failed = false;
const fail = (msg) => { failed = true; console.error("[FAIL] " + msg); };

const INST = "snare";

const waitForApp = async () => {
  await page.waitForFunction(
    () => typeof setInstrumentParam === "function" &&
          typeof window.idbPutSample === "function" &&
          typeof window.__testCaptureSynthRender === "function" &&
          typeof window.getCachedRenderData === "function",
    { timeout: 30000 },
  );
  await page.mouse.click(200, 200);
  await page.evaluate(async () => { try { await window.audioReady; } catch (_) {} });
};

try {
  // ── Session 1: seed the legacy persisted sample over the builtin ──
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await waitForApp();
  await page.evaluate(async (id) => {
    // A 0.4s 330 Hz sine "chop" — clearly NOT the snare synth render.
    const sr = 48000, frames = Math.floor(sr * 0.4);
    const f32 = new Float32Array(frames);
    for (let i = 0; i < frames; i++) f32[i] = Math.sin(2 * Math.PI * 330 * i / sr) * 0.8;
    await window.idbPutSample(id, sr, new Uint8Array(f32.buffer.slice(0)));
  }, INST);
  const seeded = await page.evaluate(async (id) => {
    const all = await window.idbGetAllSamples();
    return all.some((r) => r.id === id && r.bytes && r.bytes.length > 0);
  }, INST);
  if (!seeded) throw new Error("test setup: failed to seed IndexedDB sample for " + INST);

  // ── Session 2: reload — startup runs ApplySavedSamples against the seed ──
  await page.reload({ waitUntil: "load" });
  await waitForApp();
  // Let the Go bootstrap finish sample rehydration.
  await page.waitForTimeout(500);

  const result = await page.evaluate(async (id) => {
    const summarize = (r) => r ? {
      peak: r.peak, rms: r.rms, tailRMS: r.tailRMS, length: r.length,
      sig: r.head.slice(0, 8).map((v) => v.toFixed(6)).join(","),
    } : null;

    // Synth knob edits — the user gesture that must always be audible on a
    // recipe-bound builtin. decay extremes produce visibly different tails.
    setInstrumentParam(id, "decay", 0.25);
    await new Promise((r) => setTimeout(r, 100));
    const shortDecay = summarize(await window.__testCaptureSynthRender(id));

    setInstrumentParam(id, "decay", 4);
    await new Promise((r) => setTimeout(r, 100));
    const longDecay = summarize(await window.__testCaptureSynthRender(id));

    // What will the NEXT trigger actually play? (renderCache is what
    // processAudioEvent reads.) A frozen 330 Hz sine chop means the edits are
    // audibly dead even if a render could be forced elsewhere.
    const played = window.getCachedRenderData(id);
    let playedPeak = 0;
    if (played) for (let i = 0; i < played.length; i++) { const a = Math.abs(played[i]); if (a > playedPeak) playedPeak = a; }
    return { shortDecay, longDecay, playedLen: played ? played.length : 0, playedPeak };
  }, INST);

  console.log("[TEST] renders:", JSON.stringify(result));

  if (!result.shortDecay || !result.longDecay) {
    fail(
      `synth render unavailable for '${INST}' after a knob edit (short=${JSON.stringify(result.shortDecay)}, ` +
      `long=${JSON.stringify(result.longDecay)}). The stale persisted sample deleted RENDER['${INST}'] — ` +
      `the recipe-bound builtin is frozen and every Synth-tab edit is audibly a no-op.`,
    );
  } else {
    if (!(result.shortDecay.peak > 1e-3) || !(result.longDecay.peak > 1e-3)) {
      fail(`synth renders are silent (short.peak=${result.shortDecay.peak}, long.peak=${result.longDecay.peak})`);
    }
    // decay=0.25 vs decay=4 must differ materially: the long-decay render keeps
    // far more tail energy. Render-level (deterministic), not wall-clock capture.
    const ratio = result.longDecay.tailRMS / Math.max(result.shortDecay.tailRMS, 1e-9);
    if (!(ratio > 1.5)) {
      fail(
        `decay edit did not change the render (tailRMS short=${result.shortDecay.tailRMS}, ` +
        `long=${result.longDecay.tailRMS}, ratio=${ratio.toFixed(3)}) — knob edits are not reaching the synth.`,
      );
    }
    // And the playback cache must hold the (long-decay) SYNTH render, not the
    // frozen 330 Hz chop: same length as the fresh render, not the seeded 19200.
    if (result.playedLen !== result.longDecay.length) {
      fail(
        `playback cache holds a different buffer than the fresh synth render ` +
        `(cache=${result.playedLen} frames, render=${result.longDecay.length} frames) — ` +
        `the next trigger plays the stale sample, not the edited synth.`,
      );
    }
  }
} catch (e) {
  fail("exception: " + (e && e.message ? e.message : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (pageErrors.length) console.log("[TEST] page errors:", pageErrors.slice(0, 10));
if (failed) { console.error("[TEST] synth edits override stale sample: FAIL"); process.exit(1); }
console.log("[TEST] synth edits override stale sample: PASS");
