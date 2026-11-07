/** The production page and REAL input: Play, solo the snare, open the Synth tab, change gen1_wave (click to arm, arrow
 * keys), then the master RMS must not collapse. Soloed, a snare-silencing bug kills ALL audio. */
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

// Purpose-built, never the hand-edited startup demo: soloing the snare silences the kick.
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

  // Play is ignored until the import's warm-up gate opens.
  await page.mouse.click(200, 200);
  await waitForAudioReady(page, { allowReject: true });
  await startPlaybackWhenReady(page);

  // The real solo button first; toggleSolo() (same state) if the layout exposes no rect for it.
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

  // Open the owning stage so its knob is laid out: VOICE for gen1_wave, OSC for osc_type.
  await page.evaluate(() => { if (typeof selectSynthSection === "function") selectSynthSection("VOICE"); });
  await page.waitForTimeout(200);
  await waitForGameLoopRelease(page);

  // Only the open stage's knobs have a non-zero rect.
  let knobs = await page.evaluate(() => (typeof synthControlRects === "function" ? synthControlRects() : []));
  const laidOut = (k) => k && k.w > 0 && k.h > 0;
  // The snare's generator is the gen-slot knob.
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
    // A click ARMS the control, Up steps it, Enter accepts; two Ups cross a waveform boundary one step at a time, where a
    // mid-gesture schema rebind would fire. Real canvas and keyboard input.
    await page.mouse.click(cx, cy);
    await waitForGameLoopRelease(page);
    for (let i = 0; i < 2; i++) { await page.keyboard.press("ArrowUp"); await page.waitForTimeout(80); }
    await page.keyboard.press("Enter");
    await waitForGameLoopRelease(page);
    const after = await page.evaluate((n) => (getInstrumentParams("snare") || {})[n] ?? 0, genName);
    console.log(`[TEST] ${genName} via real input: ${before} -> ${after}`);
    if (after === before) fail(`the generator wheel did not change snare ${genName} (${before} -> ${after}) — control not hit / not targeting snare.`);

    // A generator change must touch ONLY the generator: a schema rebind mid-gesture once drove foreign params that silenced
    // the voice until reload. Only the generator may move between the two documents.
    const params = await page.evaluate(() => getInstrumentParams("snare") || {});
    const foreign = [...new Set([...Object.keys(paramsBefore), ...Object.keys(params)])]
      .filter((k) => k !== genName && paramsBefore[k] !== params[k]);
    console.log(`[TEST] snare params changed by the generator change: ${JSON.stringify(Object.fromEntries(foreign.concat(genName).map((k) => [k, [paramsBefore[k], params[k]]])))}`);
    if (foreign.length) {
      fail(`generator change scrambled foreign param(s) [${foreign.join(", ")}] — a generator change must touch only ${genName} (mid-gesture schema swap regression).`);
    }

    // Measured on the EQ tab: under software GL the Synth tab's draw starves WebAudio while it is open. The regression
    // this guards persists across tabs, so it is still caught.
    await page.evaluate(() => { if (typeof setEQTab === "function") setEQTab("eq"); else if (typeof setActiveEQTab === "function") setActiveEQTab("eq"); });
    await page.waitForTimeout(400);
    await waitForGameLoopRelease(page);
    // Relative too: a stale ABI collapses RMS ~250× while a fade transient clears the absolute floor.
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
