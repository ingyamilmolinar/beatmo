/**
 * Resilience guard: a THROW in the re-voice (parameterized) render must NOT
 * permanently silence the instrument (WASM, production build).
 *
 * Root-cause mechanism uncovered while chasing "changing the generator stops the
 * audio": when ensureRenderedSample throws on the re-voice path, processAudioEvent
 * drops the hit and re-renders on every subsequent hit — so a PERSISTENT throw
 * leaves the row dead until reload ("audio instantly stops for that instrument").
 * audio.js now catches a parameterized-render throw and falls back to the
 * instrument's NATIVE renderer (degraded, not dead) + logs LOUDLY.
 *
 * This test forces the failure mode via window.__forceRevoiceRenderThrow and
 * asserts the soloed instrument stays AUDIBLE (native fallback) after the
 * generator change, and that the loud error surfaced.
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/synth_revoice_render_resilience.browser.test.js
 */
import { chromium } from "playwright";
import { buildMainWasm, createServer, waitForGameLoopRelease } from "./real_input_test_helpers.js";

if (!buildMainWasm({ logLevel: "INFO" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const fallbackLogs = [];
page.on("console", (m) => {
  const t = m.text();
  // "threw" (sync ccall) OR "failed" (off-thread render rejected) — both mean
  // the parameterized render fell back to the native renderer.
  if (/parameterized render (threw|failed)/.test(t)) fallbackLogs.push(t);
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
const RMS_MIN = 1e-4;

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof startPlay === "function" && typeof setInstrumentParam === "function" && typeof totalRows === "function", { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, { timeout: 30000 });
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForFunction(() => totalRows() >= 2, { timeout: 15000 });
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
  await page.evaluate(async () => { try { await window.audioReady; } catch (_) {} });
  await page.evaluate(() => startPlay());
  await page.waitForFunction(() => isPlaying() === true, { timeout: 10000 });

  // Solo the instrument so its silence (if any) is the whole mix.
  await page.evaluate((r) => { window.__row = r; for (let i = 0; i < totalRows(); i++) if (i !== r && rowSoloed(i)) toggleSolo(i); if (!rowSoloed(r)) toggleSolo(r); }, row);
  await waitForGameLoopRelease(page);
  if (!(await page.evaluate(() => rowSoloed(window.__row)))) fail("could not solo");

  const base = await measure(1000);
  console.log(`[TEST] baseline (Native, soloed) rms=${base.toFixed(5)}`);
  if (!(base > RMS_MIN)) fail(`no audio at baseline rms=${base}`);

  // ARM the forced re-voice render throw, THEN change the generator off Native.
  // Without the fallback this would permanently silence the instrument.
  await page.evaluate(() => { window.__forceRevoiceRenderThrow = true; });
  await page.evaluate((id) => setInstrumentParam(id, "gen_type", 2), inst); // Saw → re-voice

  const post = await measure(2000);
  const floor = Math.max(RMS_MIN, base * 0.05);
  console.log(`[TEST] after re-voice WITH forced render throw: rms=${post.toFixed(5)} floor=${floor.toFixed(5)} fallbackLogs=${fallbackLogs.length}`);

  if (!(post > floor)) {
    fail(`instrument went SILENT after a re-voice render throw (rms=${post.toFixed(6)} < ${floor.toFixed(6)}) — native fallback did not save it`);
  }
  if (fallbackLogs.length === 0) {
    fail("expected a LOUD '[AUDIOJS] parameterized render threw/failed … falling back to native' error, but none was logged");
  }

  // Disarm and confirm a clean re-voice still works (no permanent damage).
  await page.evaluate(() => { window.__forceRevoiceRenderThrow = false; });
  await page.evaluate((id) => setInstrumentParam(id, "gen_type", 6), inst); // Noise White
  const recovered = await measure(1500);
  console.log(`[TEST] after disarming, clean re-voice rms=${recovered.toFixed(5)}`);
  if (!(recovered > floor)) fail(`instrument did not recover after disarming the forced throw (rms=${recovered.toFixed(6)})`);

  await page.evaluate(() => stopPlay());
} catch (e) {
  fail("exception: " + (e && e.message ? e.message : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (failed) { console.error("[TEST] synth re-voice render resilience: FAIL"); process.exit(1); }
console.log("[TEST] synth re-voice render resilience: PASS (render throw falls back to native, no permanent silence)");
