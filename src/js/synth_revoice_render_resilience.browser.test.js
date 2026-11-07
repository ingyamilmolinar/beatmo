/** A THROW in a re-voice render must neither silence the instrument for good nor swap in another voice: the error is loud
 * and the previous record keeps playing on time (a "fallback" once played a 220 Hz blip). Forced via
 * __forceRevoiceRenderThrow; disarmed, a clean re-voice must render. */
import { chromium } from "playwright";
import { buildMainWasm, createServer, waitForGameLoopRelease } from "./real_input_test_helpers.js";
import { waitForAudioReady } from "./browser_test_helpers.js";

if (!buildMainWasm({ logLevel: "INFO" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const failureLogs = [];    // the loud "render failed" error
const substituteLogs = []; // any sign of a fallback voice — must stay empty
page.on("console", (m) => {
  const t = m.text();
  if (/\[AUDIOJS\] render failed for/.test(t)) failureLogs.push(t);
  if (/falling back to native|parameterized render (threw|failed)/.test(t)) substituteLogs.push(t);
  try { console.log("[PAGE]", m.type(), t); } catch (_) {}
});

// Tap the real AudioContext destination with a windowed RMS accumulator.
await page.addInitScript(() => {
  const RealAC = window.AudioContext || window.webkitAudioContext;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(1024, 1, 1);
      window.__rmsAccum = 0; window.__frames = 0;
      sp.addEventListener("audioprocess", (e) => {
        const d = e.inputBuffer.getChannelData(0);
        for (let i = 0; i < d.length; i++) window.__rmsAccum += d[i] * d[i];
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
  await page.evaluate(() => { window.__rmsAccum = 0; window.__frames = 0; });
  await page.waitForTimeout(ms);
  return page.evaluate(() => (window.__frames ? Math.sqrt(window.__rmsAccum / window.__frames) : 0));
};
// While a re-render is pending the hit is DROPPED (degraded, not dead), so short windows are polled until audio returns.
// `extra` must also hold: audio from earlier hits can read above floor before the failing render has even logged.
const measureUntil = async (floor, budgetMs, win = 500, extra = null) => {
  let best = 0;
  for (let waited = 0; waited < budgetMs; waited += win) {
    const r = await measure(win);
    if (r > best) best = r;
    if (r > floor && (!extra || extra())) return r;
  }
  return best;
};
const RMS_MIN = 1e-4;

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof startPlay === "function" && typeof setInstrumentParam === "function" && typeof totalRows === "function", null, { timeout: 30000 });
  await page.waitForFunction(() => typeof window.__testCaptureSynthRender === "function", null, { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 30000 });
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForFunction(() => totalRows() >= 2, null, { timeout: 15000 });
  await waitForGameLoopRelease(page);

  // Find a bespoke drum row.
  const nRows = await page.evaluate(() => totalRows());
  let row = -1, inst = "";
  for (let i = 0; i < nRows; i++) {
    const id = await page.evaluate((r) => rowInstrument(r), i);
    if (id) { row = i; inst = id; break; }
  }
  if (row < 0) throw new Error("no instrument row");
  console.log(`[TEST] using row=${row} inst=${inst}`);

  await page.mouse.click(200, 200);
  await waitForAudioReady(page, { allowReject: true });
  await page.evaluate(() => startPlay());
  await page.waitForFunction(() => isPlaying() === true, null, { timeout: 10000 });

  // Solo the instrument so its silence (if any) is the whole mix.
  await page.evaluate((r) => { window.__row = r; for (let i = 0; i < totalRows(); i++) if (i !== r && rowSoloed(i)) toggleSolo(i); if (!rowSoloed(r)) toggleSolo(r); }, row);
  await waitForGameLoopRelease(page);
  if (!(await page.evaluate(() => rowSoloed(window.__row)))) fail("could not solo");

  // Polled: under parallel load the sequencer stalls long enough that one window can read exact zero.
  const base = await measureUntil(RMS_MIN, 8000, 500);
  console.log(`[TEST] baseline (Native, soloed) rms=${base.toFixed(5)}`);
  if (!(base > RMS_MIN)) fail(`no audio at baseline rms=${base}`);
  const floor = Math.max(RMS_MIN, base * 0.05);

  // ARM the forced re-voice render throw, THEN edit a param (re-voice).
  await page.evaluate(() => { window.__forceRevoiceRenderThrow = true; });
  await page.evaluate((id) => setInstrumentParam(id, "osc_type", 1), inst); // Saw → re-voice

  const post = await measureUntil(floor, 8000, 500, () => failureLogs.length > 0);
  console.log(`[TEST] after re-voice WITH forced render throw: rms=${post.toFixed(5)} floor=${floor.toFixed(5)} failureLogs=${failureLogs.length} substituteLogs=${substituteLogs.length}`);

  if (!(post > floor)) {
    fail(`instrument went SILENT after a re-voice render throw (rms=${post.toFixed(6)} < ${floor.toFixed(6)}) — the previous buffer did not keep playing`);
  }
  if (failureLogs.length === 0) {
    fail("expected a LOUD '[AUDIOJS] render failed for …' error, but none was logged");
  }
  if (substituteLogs.length > 0) {
    fail(`a failed render swapped in a substitute voice: ${substituteLogs[0]}`);
  }

  // Asserted on the RENDER: by now parallel load can stop the sequencer outright (a control run with no edits reads
  // silence too), so an acoustic floor would need to accept 0. Playback stops first so the render may take its synchronous
  // fallback; isPlaybackActive() lags the last note by ~0.5 s.
  await page.evaluate(() => { window.__forceRevoiceRenderThrow = false; });
  await page.evaluate((id) => setInstrumentParam(id, "osc_type", 5), inst); // Noise White
  await page.evaluate(() => stopPlay());
  await page.waitForTimeout(800);
  const failuresBefore = failureLogs.length;
  const rec = await page.evaluate((id) => window.__testCaptureSynthRender(id), inst);
  console.log(`[TEST] after disarming, clean re-voice render peak=${rec ? rec.peak.toFixed(5) : "null"} rms=${rec ? rec.rms.toFixed(5) : "null"}`);
  if (!rec || !(rec.peak > 0)) {
    fail(`instrument did not recover after disarming the forced throw — the clean re-voice render is ${rec ? `SILENT (peak=${rec.peak})` : "unavailable"}`);
  }
  // A failure here is independent of the disarmed throw.
  if (failureLogs.length > failuresBefore || substituteLogs.length > 0) {
    fail("a DISARMED re-voice still failed (or substituted a voice) — the render is broken on its own");
  }
} catch (e) {
  fail("exception: " + (e && e.message ? e.message : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (failed) { console.error("[TEST] synth re-voice render resilience: FAIL"); process.exit(1); }
console.log("[TEST] synth re-voice render resilience: PASS (a render throw keeps the previous buffer, no substitute voice, no permanent silence)");
