/**
 * Harness B — real-UI synth stage-toggle bench (WASM, full main.wasm).
 *
 * Reproduces the user-reported crackle: while playing, real clicks on the
 * detail-header enable pill (via synthEnablePillRect) and real ParamCard
 * wheel-popup drags on an enabled stage, WITH THE SYNTH TAB OPEN AND DRAWING.
 * The eqControl run applies the IDENTICAL logical param churn (toggle
 * filter_enabled + sweep filter_cutoff) but via the bridge (setInstrumentParam)
 * while the cheap EQ tab draws — a sound single-variable control that isolates
 * Synth-tab draw cost (project_synth_tab_audio_starvation). See the comment
 * above runUIStorm for why bridge churn, not frozen-coordinate clicks, is used.
 *
 * Structural gates only (software-GL amplifies draw cost — wall-clock is
 * recorded, not gated):
 *   - no page errors; AudioContext running
 *   - toggles>0 and knobDrags>0 for the synth run (the storm actually ran)
 *   - mainThreadRenders==0 for the synth run (off-thread voice rendering
 *     must hold even under stage-toggle cache eviction)
 *   - audio recovers after the storm (RMS on a quiet tab > silence floor)
 *     — proves starvation, not a permanently dead instrument
 * Records: RMS, FPS, drawMaxMS, audioDrops, overdue, gaps, render metrics.
 */
import { chromium } from "playwright";
import fs from "fs";
import path from "path";
import {
  buildMainWasm, createServer, dragMouse, waitForGameLoopRelease,
} from "./real_input_test_helpers.js";
import { fileURLToPath } from "url";

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

// Tap the real AudioContext destination (mirrors synth_generator_solo_realinput):
// windowed accumulators (__rmsAccum/__nonZero/__frames) we reset per sample window.
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
    && typeof setEQChannel === "function", { timeout: 30000 });
  await page.waitForFunction(() => window.audioReady !== undefined, { timeout: 30000 });
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForFunction(() => typeof totalRows === "function" && totalRows() >= 1, { timeout: 15000 });
  await page.waitForTimeout(300);
  await waitForGameLoopRelease(page);

  // Unlock audio with a real gesture (creates the AudioContext).
  await page.mouse.click(200, 200);
  await page.evaluate(async () => { try { await window.audioReady; } catch (_) {} });

  // Put a heavy melodic (modular, per-pitch-rendered) voice on row 0 so the
  // Synth tab edits a heavy voice — cello (see AGENTS.md instrument catalog /
  // project_voice_choir_instruments memory).
  await page.evaluate(() => setRowInstrument(0, "cello"));
  // Make sure the Synth tab edits cello (row 0), not whatever channel was
  // previously selected — setRowInstrument alone does not change the
  // edited-instrument selection (see synth_generator_solo_realinput's
  // "Make sure the Synth tab edits the snare" comment for the same pattern).
  await page.evaluate(() => setEQChannel("cello")); // present per the waitForFunction guard above
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForTimeout(200);
  await waitForGameLoopRelease(page);

  await page.evaluate(() => startPlay());
  await page.waitForFunction(() => typeof isPlaying === "function" && isPlaying() === true, { timeout: 10000 });

  const ctxState = await page.evaluate(() => window.__audioCtx && window.__audioCtx.state);
  console.log("[TEST] AudioContext state =", ctxState);
  if (ctxState !== "running") fail(`AudioContext not running (state=${ctxState})`);

  // runUIStorm(tabName): switches to `tabName` and, for DURATION_MS at
  // SPAM_INTERVAL_MS cadence, applies the same logical param load (25% ticks
  // toggle filter_enabled, 75% ticks sweep filter_cutoff) via one of two input
  // paths chosen by `tabName`:
  //
  //   "synth" — the FAITHFUL reproduction. Opens the FILTER stage, then uses
  //     REAL pointer input on the drawn Synth tab: click the centre of
  //     synthEnablePillRect() to toggle, and for the cutoff knob use the
  //     ParamCard wheel-popup flow (click the card → locate the popup via
  //     subjectRectJS("synth_wheel") → vertical dragMouse → Escape to close so
  //     it never lingers into the next tick). This is what we gate.
  //
  //   "eq" — the DRAW-COST CONTROL. Applies the identical churn via the BRIDGE
  //     (setInstrumentParam) while the cheap EQ tab draws, with NO mouse input.
  //     Bridge (not "click the synth panel's frozen screen coordinates on the
  //     EQ tab") is deliberate: those coordinates map to unrelated EQ controls
  //     when the EQ tab is the one drawn, so clicking them corrupts audio state
  //     (an earlier version muted master → a false "audio dead" reading). Same
  //     mutation load, only the drawn tab differs → the synth-vs-eqControl
  //     delta isolates Synth-tab draw cost. See the tabName !== "synth" branch.
  async function runUIStorm(tabName) {
    await page.evaluate((t) => setEQTab(t), tabName);
    await page.waitForTimeout(250);
    await waitForGameLoopRelease(page);
    if (tabName === "synth") {
      const selectResult = await page.evaluate(() => selectSynthSection("FILTER"));
      await page.waitForTimeout(150);
      await waitForGameLoopRelease(page);

      // Fail loudly (with diagnostics) instead of silently recording a
      // zero-input run: if neither the enable pill nor a cutoff knob laid
      // out, the edited instrument / section selection didn't take, and the
      // toggles>0 / knobDrags>0 gates below would fail with no clue why.
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
      // In-page event-loop gap sampler (mirrors webaudio_synth_param_stress):
      // a blocked main thread (heavy Synth-tab draw) shows up as a large gap
      // between 8ms ticks.
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
        // Draw-cost control: identical param churn via the BRIDGE while the
        // cheap EQ tab draws. NO frozen-coordinate clicks — clicking the synth
        // panel's last-laid-out pill/knob screen coordinates on the EQ tab
        // would hit unrelated EQ controls and corrupt audio state (an earlier
        // version did exactly that and muted master → a false "audio dead"
        // recovery reading). Same logical load (toggle filter_enabled + sweep
        // filter_cutoff) as the synth run, so the synth-vs-eqControl delta
        // isolates Synth-tab DRAW cost, not the mutation itself.
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
        // ParamCard conversion (see synthKnobHitAdapter in synth_panel_zone.go):
        // knobs no longer support in-place rotary/horizontal drag —
        // OnDrag/OnRelease are literal no-ops. A press instead OPENS a modal
        // vertical "barrel" wheel popup (SubjectSynthWheel) that owns the
        // drag. Real interaction: click the card to open the popup, locate
        // its actual on-screen rect via subjectRectJS("synth_wheel") (the
        // same rect the screenshot harness crops to), drag VERTICALLY inside
        // it to spin the value, then Escape to cancel+close — so the popup
        // never lingers open into the next tick and swallows later clicks
        // (that caused a hang/instability in an earlier iteration of this
        // bench).
        const knobs = await page.evaluate(() => (typeof synthControlRects === "function" ? synthControlRects() : []));
        const knob = (knobs || []).find((k) => k && k.w > 0 && k.h > 0 && /cutoff/i.test(k.name || ""));
        if (knob) {
          const kcx = Math.round(knob.x + knob.w / 2), kcy = Math.round(knob.y + knob.h / 2);
          await page.mouse.click(kcx, kcy);
          await page.waitForTimeout(150);
          const wheel = await page.evaluate(() => (typeof subjectRectJS === "function" ? subjectRectJS("synth_wheel") : { visible: false }));
          if (wheel && wheel.visible && wheel.w > 0 && wheel.h > 0) {
            const wcx = Math.round(wheel.x + wheel.w / 2), wcy = Math.round(wheel.y + wheel.h / 2);
            const dir = (i % 8 < 4) ? -1 : 1; // alternate to avoid clamping at an extreme
            await dragMouse(page, wcx, wcy, wcx, wcy + dir * 40, { steps: 6 });
            knobDrags++;
          }
          await page.keyboard.press("Escape");
          await page.waitForTimeout(100);
          await waitForGameLoopRelease(page);
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

  // Recovery probe: after the FAITHFUL real-input storm, leave to the cheap EQ
  // tab, let audio settle, sample RMS. A recovering level proves the storm
  // STARVED audio (recovers once the heavy Synth-tab draw stops) rather than
  // permanently silencing the toggled instrument. Measured here — before the
  // bridge-churn control run — so it reflects the real-input storm's aftermath.
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
