/** A param change never kills an instrument carrying sample PCM: a shipped instrument re-renders as a synth with the new
 * params; a pure user sample keeps its PCM, its only audio, untouched. */
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
page.on("console", (m) => { try { console.log("[PAGE]", m.type(), m.text()); } catch (_) {} });

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof setInstrumentParam === "function" && typeof window.registerSamplePCM === "function" && typeof window.playSound === "function", null, { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 30000 });
  await page.mouse.click(200, 200);
  await waitForAudioReady(page, { allowReject: true });
  await page.waitForFunction(() => window.__audioCtx ? window.__audioCtx.state === "running" : true, null, { timeout: 10000 }).catch(() => {});

  const INST = "snare";

  // Measured off the rendered PCM, not the live capture, whose ScriptProcessor underruns under headless SwiftShader and
  // drops a one-shot's transient; the render is deterministic.
  const result = await page.evaluate(async (id) => {
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    const sr = 44100, frames = Math.floor(sr * 0.3);
    // An audible decaying 220 Hz tone as the instrument's "sample".
    const f32 = new Float32Array(frames);
    for (let i = 0; i < frames; i++) f32[i] = Math.sin(2 * Math.PI * 220 * i / sr) * Math.exp(-3 * i / frames);
    const u8 = new Uint8Array(f32.buffer.slice(0));

    // Deletes RENDER[id] and stores the PCM in renderCache.
    window.registerSamplePCM(id, u8, sr);

    // The registered PCM if present, else a forced synth render.
    const peakOf = async () => {
      let d = window.getCachedRenderData(id);
      if (!d || !d.length) { await window.ensureSynthSample(id); d = window.getCachedRenderData(id); }
      if (!d || !d.length) return 0;
      let peak = 0; for (let i = 0; i < d.length; i++) { const a = Math.abs(d[i]); if (a > peak) peak = a; }
      return peak;
    };

    const before = await peakOf();
    // Change the generator param on the SAMPLE instrument (the user gesture).
    setInstrumentParam(id, "amp_decay", 2);
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

  // No synth to re-render: its PCM must play on bit-identically.
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
    // setInstrumentBlock's sample guard.
    const s = window.__instrumentBlockForTest("kick");
    window.setInstrumentBlock(id, s.block, s.seconds, s.amp, s.melodic, s.gateTail, s.fx);
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
