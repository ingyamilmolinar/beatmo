/**
 * Resilience guard: a THROW in the re-voice render must NOT permanently
 * silence the instrument, and must NEVER swap in a different voice (WASM,
 * production build).
 *
 * Root-cause mechanism uncovered while chasing "changing the generator stops the
 * audio": when ensureRenderedSample throws on the re-voice path, processAudioEvent
 * drops the hit and re-renders on every subsequent hit — so a PERSISTENT throw
 * leaves the row dead until reload ("audio instantly stops for that instrument").
 * A failed render throws LOUDLY and leaves the instrument's previous (param-
 * stale) record in the cache, which keeps playing on time. There is no fallback
 * voice: the old "native fallback" rendered the bare identity block — a 220 Hz
 * blip in place of the instrument — once the engine lost its built-in defaults.
 *
 * This test forces the failure mode via window.__forceRevoiceRenderThrow and
 * asserts the soloed instrument stays AUDIBLE (its previous buffer) after the
 * param change, that the loud error surfaced, and that no substitute voice was
 * rendered. It then disarms the
 * throw and asserts a clean re-voice still renders audibly — that one is
 * checked at the RENDER level rather than acoustically; see the comment on
 * the disarm phase for why live RMS is unmeasurable there under parallel load.
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/synth_revoice_render_resilience.browser.test.js
 */
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
// A re-voice re-render runs on the synth WORKER thread. Under parallel CI load a
// fresh render can outlast a single measure window, and while it is pending
// processAudioEvent DROPS the hit (degraded, not dead — the whole point of this
// test), so a fixed window can read EXACT ZERO even though the instrument DOES
// recover a beat later. Poll short windows until audio returns (or the budget
// runs out) so we assert "does it come back", not "did it come back within
// exactly 1.5 s under contention". A genuine permanent-silence regression still
// fails — it just takes the full budget to do so.
//
// `extra` (optional) is an additional predicate that must ALSO hold before we
// return early. The forced-throw phase passes `() => failureLogs.length > 0`:
// the failing re-render (and its loud error log) runs async, and audio can
// read above floor from hits scheduled BEFORE it — so exiting on audio alone
// races the log. Gating the early return on the log couples the exit to the
// failure actually having happened. A real regression still fails: no audio →
// `post > floor` fails; no failure → the `failureLogs.length === 0` check fires.
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

  // Baseline audibility must poll, not sample a fixed window. Under
  // make test-real's 4-way parallel browser load the WASM game loop is starved
  // hard enough that the sequencer stalls for >300ms against a 4ms tick (see the
  // measureUntil comment above and project_revoice_resilience_acoustic_flake), so
  // a single 1s window can read exact zero even though the soloed instrument IS
  // audible a beat later — the post-revoice measurement below already uses this
  // same resilient poll for exactly that reason. Assert "is it audible", not
  // "was it audible within one second under contention". A real dead-instrument
  // regression still fails: it never crosses RMS_MIN across the full 8s budget.
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

  // Disarm and confirm a clean re-voice still works (no permanent damage).
  //
  // This phase asserts on the RENDER, not on live RMS, and deliberately so.
  // Live RMS cannot measure anything by this point in the run: under
  // `make test-real`'s 4-way parallel browser load the WASM game loop is
  // starved hard enough that the sequencer stops emitting (CI shows
  // seqFire.max≈350ms against a 4ms tick, plus a PARITY emitdrift breach), and
  // the mix reads EXACT 0.000000 for 10+ seconds with playing=true and the tap
  // still receiving frames. That silence is NOT this test's regression: a
  // control run that changes NO params at all measures 22 of 24 windows below
  // this same floor. So an acoustic threshold that survived the contention
  // would have to accept 0 — i.e. have no teeth at all.
  //
  // __testCaptureSynthRender renders on demand and inspects the resulting
  // buffer, which proves "the forced throw left no permanent damage" exactly
  // and with no dependency on real-time scheduling. It is the same helper the
  // other synth-audibility suites assert with (synth_generator_audible,
  // modular_synth_audible, synth_param_extremes, …).
  //
  // Playback is stopped BEFORE the capture so ensureRenderedSample is allowed
  // its synchronous main-thread fallback — during playback it refuses to block
  // and throws "render deferred", which a loaded render worker could otherwise
  // trigger. isPlaybackActive() lags the last scheduled note by ~0.5s, hence
  // the settle wait.
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
  // The disarmed re-voice must render on its own: a failure here would mean the
  // render is broken independently of the (now disarmed) forced throw.
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
