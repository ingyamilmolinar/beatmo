/** Real clicks on the stage enable pill and real armed-control edits while playing with the Synth tab DRAWING, against an
 * identical bridge-driven churn on the cheap EQ tab: the delta isolates Synth-tab draw cost. Gated structurally (no
 * errors, the storm ran, no main-thread renders, audio recovers); wall-clock is only recorded. */
import { chromium } from "playwright";
import fs from "fs";
import path from "path";
import {
  buildMainWasm, createServer, waitForGameLoopRelease,
} from "./real_input_test_helpers.js";
import { fileURLToPath } from "url";
import { waitForAudioReady } from "./browser_test_helpers.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));

// ── Tunables (env-overridable) ──
const DURATION_MS = Number(process.env.UI_STORM_DURATION_MS || 6000);
const SPAM_INTERVAL_MS = Number(process.env.UI_STORM_INTERVAL_MS || 300);
const RMS_MIN = 1e-3;

if (!buildMainWasm({ logLevel: "INFO" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
page.on("console", (m) => { const t = m.text(); if (m.type() === "error") pageErrors.push("console.error: " + t); try { console.log("[PAGE]", m.type(), t); } catch (_) {} });

// Windowed accumulators reset per sample window.
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

// sampleRMS: reset the accumulator, wait `ms`, return the RMS of that window.
const sampleRMS = async (ms) => {
  await page.evaluate(() => { window.__rmsAccum = 0; window.__nonZero = 0; window.__frames = 0; });
  await page.waitForTimeout(ms);
  return page.evaluate(() => (window.__frames ? Math.sqrt(window.__rmsAccum / window.__frames) : 0));
};

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof startPlay === "function"
    && typeof setEQTab === "function"
    && typeof selectSynthSection === "function"
    && typeof synthEnablePillRect === "function"
    && typeof synthControlRects === "function"
    && typeof setRowInstrument === "function"
    && typeof setEQChannel === "function", null, { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 30000 });
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForFunction(() => typeof totalRows === "function" && totalRows() >= 1, null, { timeout: 15000 });
  await page.waitForTimeout(300);
  await waitForGameLoopRelease(page);

  // Unlock audio with a real gesture (creates the AudioContext).
  await page.mouse.click(200, 200);
  await waitForAudioReady(page, { allowReject: true });

  // A heavy melodic per-pitch voice.
  await page.evaluate(() => setRowInstrument(0, "cello"));
  // setRowInstrument does not change which instrument the Synth tab edits.
  await page.evaluate(() => setEQChannel("cello")); // present per the waitForFunction guard above
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForTimeout(200);
  await waitForGameLoopRelease(page);

  await page.evaluate(() => startPlay());
  await page.waitForFunction(() => typeof isPlaying === "function" && isPlaying() === true, null, { timeout: 10000 });

  const ctxState = await page.evaluate(() => window.__audioCtx && window.__audioCtx.state);
  console.log("[TEST] AudioContext state =", ctxState);
  if (ctxState !== "running") fail(`AudioContext not running (state=${ctxState})`);

  // Same load either way (25 % toggles, 75 % cutoff sweeps): "synth" opens FILTER and uses REAL input on the drawn tab (the
  // gated run); "eq" drives the bridge while the EQ tab draws, since the Synth panel's frozen coordinates hit unrelated EQ
  // controls there.
  async function runUIStorm(tabName) {
    await page.evaluate((t) => setEQTab(t), tabName);
    await page.waitForTimeout(250);
    await waitForGameLoopRelease(page);
    if (tabName === "synth") {
      const selectResult = await page.evaluate(() => selectSynthSection("FILTER"));
      await page.waitForTimeout(150);
      await waitForGameLoopRelease(page);

      // A zero-input run would fail the gates with no clue why.
      const diagPill = await page.evaluate(() => (typeof synthEnablePillRect === "function" ? synthEnablePillRect() : { x: 0, y: 0, w: 0, h: 0 }));
      const diagKnobs = await page.evaluate(() => (typeof synthControlRects === "function" ? synthControlRects() : []));
      const diagChips = await page.evaluate(() => (typeof synthChipRects === "function" ? synthChipRects() : []));
      const pillEmpty = !diagPill || diagPill.w <= 0 || diagPill.h <= 0;
      const noCutoffKnob = !(diagKnobs || []).some((k) => k && k.w > 0 && k.h > 0 && /cutoff/i.test(k.name || ""));
      if (pillEmpty && noCutoffKnob) {
        fail(
          "synth setup produced zero input targets — FILTER section not laid out for the edited instrument.\n"
          + `  selectSynthSection("FILTER") returned: ${JSON.stringify(selectResult)}\n`
          + `  synthChipRects() names: ${JSON.stringify((diagChips || []).map((c) => c && c.name))}\n`
          + `  synthControlRects() names: ${JSON.stringify((diagKnobs || []).map((k) => k && k.name))}\n`
          + `  synthEnablePillRect(): ${JSON.stringify(diagPill)}`,
        );
      }
    }

    await page.evaluate(() => {
      if (typeof resetPerfStats === "function") resetPerfStats();
      if (typeof resetAudioScheduleMetrics === "function") resetAudioScheduleMetrics();
      if (typeof resetRenderLatencyMetrics === "function") resetRenderLatencyMetrics();
      // A blocked main thread shows as a large gap between 8 ms ticks.
      window.__uiGaps = [];
      window.__uiGapLast = performance.now();
      window.__uiGapTimer = setInterval(() => {
        const n = performance.now();
        window.__uiGaps.push(n - window.__uiGapLast);
        window.__uiGapLast = n;
      }, 8);
    });

    let toggles = 0, knobDrags = 0, ctlSweep = 0;
    const rmsWindows = [];
    const ticks = Math.max(1, Math.floor(DURATION_MS / SPAM_INTERVAL_MS));
    for (let i = 0; i < ticks; i++) {
      await page.evaluate(() => { window.__rmsAccum = 0; window.__frames = 0; });
      if (tabName !== "synth") {
        // The bridge, never frozen-coordinate clicks, which once muted master and read as "audio dead".
        if (i % 4 === 0) {
          await page.evaluate(() => { window.__ctlTog = (window.__ctlTog | 0) ^ 1; setInstrumentParam("cello", "filter_enabled", window.__ctlTog); });
          toggles++;
        } else {
          ctlSweep = (ctlSweep + 0.05) % 1.0;
          await page.evaluate((v) => setInstrumentParam("cello", "filter_cutoff", v), ctlSweep);
          knobDrags++;
        }
      } else if (i % 4 === 0) {
        const pill = await page.evaluate(() => (typeof synthEnablePillRect === "function" ? synthEnablePillRect() : { x: 0, y: 0, w: 0, h: 0 }));
        if (pill && pill.w > 0 && pill.h > 0) {
          await page.mouse.click(Math.round(pill.x + pill.w / 2), Math.round(pill.y + pill.h / 2));
          toggles++;
        }
      } else {
        // A click ARMS the control, arrows step it, Enter accepts; an edit counts only if the param moved, so input that did
        // nothing cannot pass.
        const knobs = await page.evaluate(() => (typeof synthControlRects === "function" ? synthControlRects() : []));
        const knob = (knobs || []).find((k) => k && k.w > 0 && k.h > 0 && /cutoff/i.test(k.name || ""));
        if (knob) {
          const kcx = Math.round(knob.x + knob.w / 2), kcy = Math.round(knob.y + knob.h / 2);
          const before = await page.evaluate((n) => (getInstrumentParams("cello") || {})[n], knob.name);
          await page.mouse.click(kcx, kcy);
          await page.waitForTimeout(80);
          const key = (i % 8 < 4) ? "ArrowUp" : "ArrowDown"; // alternate to avoid clamping at an extreme
          await page.keyboard.press(key);
          await page.keyboard.press(key);
          await page.keyboard.press("Enter");
          await page.waitForTimeout(60);
          await waitForGameLoopRelease(page);
          const after = await page.evaluate((n) => (getInstrumentParams("cello") || {})[n], knob.name);
          if (after !== before) knobDrags++;
        }
      }
      await page.waitForTimeout(SPAM_INTERVAL_MS);
      const win = await page.evaluate(() => ({ accum: window.__rmsAccum || 0, frames: window.__frames || 0 }));
      rmsWindows.push(win.frames > 0 ? Math.sqrt(win.accum / win.frames) : 0);
    }

    const gaps = await page.evaluate(() => { clearInterval(window.__uiGapTimer); return window.__uiGaps || []; });
    const perf = await page.evaluate(() => (typeof perfStats === "function" ? perfStats() : {}));
    const sched = await page.evaluate(() => (typeof getAudioScheduleMetrics === "function" ? getAudioScheduleMetrics() : {}));
    const render = await page.evaluate(() => (typeof getRenderLatencyMetrics === "function" ? getRenderLatencyMetrics() : {}));

    const rmsAvg = rmsWindows.length ? rmsWindows.reduce((a, b) => a + b, 0) / rmsWindows.length : 0;
    const rmsMin = rmsWindows.length ? Math.min(...rmsWindows) : 0;
    const maxGapMs = gaps.length ? Math.max(...gaps) : 0;

    return {
      rmsAvg, rmsMin,
      fpsAvg: perf.fpsAvg ?? 0,
      drawMaxMS: perf.drawMaxMS ?? 0,
      audioDrops: perf.audioDrops ?? 0,
      overdue: sched.overdue ?? 0,
      smallLead: sched.smallLeadCount ?? 0,
      maxGapMs,
      renders: render.renders ?? 0,
      mainThreadRenders: render.mainThreadRenders ?? 0,
      deferAbsMsP90: render.deferAbsMsP90 ?? 0,
      toggles, knobDrags,
    };
  }

  console.log("[TEST] progress: entering synth storm");
  const synthResult = await runUIStorm("synth");

  // Recovery after the real-input storm proves starvation, not a dead instrument; measured before the control run.
  await page.evaluate(() => setEQTab("eq"));
  await page.waitForTimeout(600);
  const recoverRMS = await sampleRMS(400);
  console.log(`[TEST] recovery RMS (EQ tab, post-synth-storm) = ${recoverRMS.toFixed(6)} (floor=${RMS_MIN})`);

  console.log("[TEST] progress: entering eqControl storm");
  const eqControlResult = await runUIStorm("eq");
  console.log("[TEST] progress: storms complete");
  const results = { synth: synthResult, eqControl: eqControlResult };

  const failures = [];
  if ((results.synth.toggles || 0) <= 0) failures.push("synth: no stage toggles fired");
  if ((results.synth.knobDrags || 0) <= 0) failures.push("synth: no knob drags fired");
  if ((results.synth.mainThreadRenders || 0) > 0) failures.push(`synth: ${results.synth.mainThreadRenders} main-thread renders`);

  const outDir = path.resolve(jsDir, "../../bench-results");
  fs.mkdirSync(outDir, { recursive: true });
  fs.writeFileSync(path.join(outDir, "synth_stage_toggle_ui.json"), JSON.stringify(results, null, 2));
  console.log(`[TEST] wrote ${path.join(outDir, "synth_stage_toggle_ui.json")}`);
  console.log("[TEST] tab      rmsAvg rmsMin fpsAvg drawMax drops overdue maxGap toggles drags renders mtRenders");
  for (const [tab, r] of Object.entries(results)) {
    console.log(`[TEST] ${tab.padEnd(8)} ${r.rmsAvg?.toFixed?.(4)} ${r.rmsMin?.toFixed?.(4)} ${r.fpsAvg?.toFixed?.(1)} ${r.drawMaxMS?.toFixed?.(1)} ${r.audioDrops} ${r.overdue} ${r.maxGapMs?.toFixed?.(1)} ${r.toggles} ${r.knobDrags} ${r.renders} ${r.mainThreadRenders}`);
  }

  for (const f of failures) fail(f);
  if (recoverRMS < RMS_MIN) fail(`audio did not recover after real-input storm (RMS ${recoverRMS}) — instrument may be dead, not merely starved`);
} catch (e) {
  fail("exception: " + (e && e.stack ? e.stack : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (pageErrors.length) console.log("[TEST] page errors:", pageErrors.slice(0, 10));
if (failed || pageErrors.length) { console.error("[TEST] synth stage-toggle UI bench: FAIL"); process.exit(1); }
console.log("[TEST] synth stage-toggle UI bench: PASS");
