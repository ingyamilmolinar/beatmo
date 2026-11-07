/** Synths remain synths: a stale persisted sample (old destructive Sampler-Save PCM) re-registered at startup froze the
 * browser's synth, so knobs, stages and Save did nothing while desktop kept responding. Seeds that state end to end
 * (IndexedDB → reload) and asserts knob edits still re-render audibly. */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";
import { waitForAudioReady } from "./browser_test_helpers.js";

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
          typeof window.getCachedRenderData === "function", null,
    { timeout: 30000 },
  );
  await page.mouse.click(200, 200);
  await waitForAudioReady(page, { allowReject: true });
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

    // The FINISH fade ([0, 1], 1 = no fade): any live value proves the edit reaches the render past the stale sample.
    const DECAY = "post_decay";
    setInstrumentParam(id, DECAY, 0.25);
    await new Promise((r) => setTimeout(r, 100));
    const shortDecay = summarize(await window.__testCaptureSynthRender(id));

    setInstrumentParam(id, DECAY, 1);
    await new Promise((r) => setTimeout(r, 100));
    const longDecay = summarize(await window.__testCaptureSynthRender(id));

    // renderCache is what the next trigger plays; a frozen 330 Hz chop means the edits are dead.
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
    // Render-level, deterministic: the long-decay render keeps far more tail.
    const ratio = result.longDecay.tailRMS / Math.max(result.shortDecay.tailRMS, 1e-9);
    if (!(ratio > 1.5)) {
      fail(
        `decay edit did not change the render (tailRMS short=${result.shortDecay.tailRMS}, ` +
        `long=${result.longDecay.tailRMS}, ratio=${ratio.toFixed(3)}) — knob edits are not reaching the synth.`,
      );
    }
    // The cache must hold the synth render, not the seeded 19200-sample chop.
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
