/**
 * REAL end-to-end Synth-tab audio regression — Phase 5 follow-up.
 *
 * Drives real Playwright mouse events on the actual canvas during real
 * playback (startPlay → sequencer firing notes) to reproduce the user
 * report: "audio stops in WASM after slider interaction". Captures
 * WebAudio output via startOutputCapture and asserts:
 *
 *   1. Baseline playback produces audible non-NaN audio.
 *   2. Real input on each laid-out control (snare's recipe), stage by
 *      stage, changes values and doesn't silence the audio.
 *   3. After the drag, audio continues without BiquadFilterNode warnings
 *      or page errors.
 *
 * Unlike instrument_params_e2e_audio.browser.test.js (which drives
 * setInstrumentParam from JS), this test goes through the actual UI
 * pipeline: synthSliderHitAdapter.OnDrag → propagateSynthSliderValue →
 * audio.SetInstrumentParam → platformInstrumentParamsChanged →
 * window.updateInstrumentParams → renderCache.delete →
 * ensureRenderedSample → render_X_p. If any link in the chain is broken,
 * the assertions catch it.
 *
 * The Synth tab drills into one pipeline STAGE at a time and lays out only
 * that stage's controls; each control is edited the way a user does today —
 * click to ARM it, arrow keys to step the value, Enter to accept (the earlier
 * rotary/horizontal drags were removed). synthControlRects() lists every
 * binding, so the test drills each stage and edits what it actually shows.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
  waitForAudioReady,
} from "./browser_test_helpers.js";

import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";
const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const pageErrors = [];
const biquadWarnings = [];
let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage({ viewport: { width: 1280, height: 800 } });
  page.on("pageerror", (err) => {
    pageErrors.push(String(err));
    console.log("[PAGE-ERROR]", String(err));
  });
  page.on("console", (msg) => {
    const txt = msg.text();
    if (msg.type() === "warning" && /BiquadFilterNode/i.test(txt)) {
      biquadWarnings.push(txt);
    }
    // Echo any warning/error for diagnosis.
    if (msg.type() === "error" || msg.type() === "warning") {
      console.log("[PAGE]", msg.type(), txt);
    }
  });

  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof startPlay === "function" &&
    typeof stopPlay === "function" &&
    typeof setEQTab === "function" &&
    typeof synthControlRects === "function" &&
    typeof startOutputCapture === "function" &&
    typeof getOutputCapture === "function" &&
    typeof clearOutputCapture === "function"
  );


  // Unlock the audio context.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 },
  );
  await waitForAudioReady(page);

  // Helpers.
  const peakOf = (arr) => {
    let p = 0;
    for (const v of arr) {
      const a = Math.abs(v);
      if (a > p) p = a;
    }
    return p;
  };
  const hasNonFinite = (arr) => {
    for (const v of arr) if (!Number.isFinite(v)) return true;
    return false;
  };

  // Switch to Synth tab. Force a layout so the slider rects are
  // populated (forceDraw triggers Update/Draw, which calls Layout).
  // The Synth view edits ONE instrument and is blocked while the channel is
  // Master (where the app starts) — select the snare's channel first.
  await page.evaluate(() => {
    setEQChannel("snare");
    setEQTab("synth");
    if (typeof forceDraw === "function") forceDraw();
  });
  // Allow a few frames so the EQ panel's Layout runs.
  await page.waitForTimeout(300);

  let rects = await page.evaluate(() => synthControlRects());
  console.log(`[TEST] synth knobs visible: ${rects.length}`);
  if (rects.length === 0) {
    // Fall back: tab might not yet be active because the row's
    // instrument is still loading. Try forcing another draw.
    await page.evaluate(() => {
      setEQTab("synth");
      if (typeof forceDraw === "function") forceDraw();
    });
    await page.waitForTimeout(500);
    rects = await page.evaluate(() => synthControlRects());
  }
  if (rects.length === 0) {
    throw new Error("synthControlRects() returned 0 entries — Synth tab not active or layout did not run");
  }
  console.log(`[TEST] knob names: ${rects.map((r) => r.name).join(", ")}`);

  // Start playback — sequencer fires snare every beat.
  await startPlaybackWhenReady(page); // Play is ignored while the warm-up gate is closed
  await page.waitForTimeout(800);

  // Baseline capture during playback (no slider activity).
  await page.evaluate(() => {
    clearOutputCapture();
    startOutputCapture();
  });
  await page.waitForTimeout(1500);
  const baseline = await page.evaluate(() => Array.from(getOutputCapture()));
  if (hasNonFinite(baseline)) {
    throw new Error("baseline contains NaN/Inf BEFORE any slider drag");
  }
  const basePeak = peakOf(baseline);
  console.log(`[TEST] baseline peak=${basePeak.toFixed(4)}`);
  if (basePeak < 0.001) {
    throw new Error(`baseline is silent (peak=${basePeak}). Sequencer not playing.`);
  }

  // ─── Real mouse drag on each slider — this is the key reproduction step ───
  // Get canvas screen position once (it's static during the test).
  const canvasBox = await page.evaluate(() => {
    const canvas = document.querySelector("canvas");
    if (!canvas) return null;
    const r = canvas.getBoundingClientRect();
    return { x: r.x, y: r.y, width: r.width, height: r.height };
  });
  if (!canvasBox) throw new Error("no canvas in DOM");
  console.log(`[TEST] canvas: ${canvasBox.width}×${canvasBox.height} at (${canvasBox.x},${canvasBox.y})`);

  // Drag each knob through its full range. We do this DURING active playback so
  // the audio capture overlaps with the drag. CHIP-STRIP: each knob lives in a
  // collapsed stage unless its owning stage is selected, so we select the
  // owning stage, re-fetch synthControlRects(), and pick the laid-out (w>0,h>0)
  // rect before dragging. The names come from the union of all known params.
  // Drill into each pipeline STAGE once and edit every control it actually lays
  // out. (synthControlRects() lists every binding of the recipe — over a
  // thousand on a modular voice, most of them hidden gen-slot plumbing — so
  // walking it name by name waited out a timeout on each one that never shows.)
  const STAGES = ["VOICE", "OSC", "FM", "PITCH", "LFO", "BURST", "ENVELOPE", "FILTER", "POST", "FORMANT", "ENSEMBLE"];
  const targets = [];
  const seen = new Set();
  for (const st of STAGES) {
    const ok = await page.evaluate((s) => (typeof selectSynthSection === "function" ? selectSynthSection(s) : false), st);
    if (!ok) continue;
    await page.evaluate(() => { if (typeof forceDraw === "function") forceDraw(); });
    await page.waitForTimeout(200);
    const laid = await page.evaluate(() => synthControlRects().filter((k) => k.w > 0 && k.h > 0).map((k) => k.name));
    for (const n of laid) if (!seen.has(n)) { seen.add(n); targets.push([st, n]); }
  }
  console.log(`[TEST] laid-out synth controls: ${targets.map(([st, n]) => `${st}:${n}`).join(", ")}`);
  let dragged = 0;
  let changed = 0;
  for (const [stage, name] of targets) {
    await page.evaluate((s) => selectSynthSection(s), stage);
    await page.evaluate(() => { if (typeof forceDraw === "function") forceDraw(); });
    await page.waitForTimeout(120);
    const r = await page.evaluate((n) => synthControlRects().find((kk) => kk.name === n && kk.w > 0 && kk.h > 0) || null, name);
    if (!r) {
      console.log(`[TEST] skipping ${name}: not laid out after re-selecting ${stage}`);
      continue;
    }
    console.log(`[TEST] editing ${r.name} [${stage}] (${r.x},${r.y})..(${r.x + r.w},${r.y + r.h})`);

    // ARMED-CONTROL model: a click ARMS the control in place, the arrow keys
    // step its value (armedControlKeys), Enter accepts. The horizontal drag
    // this test was written for no longer edits anything (synthControlHitAdapter
    // OnDrag is a no-op), so the old sweep moved no value at all.
    const before = await page.evaluate((n) => (getInstrumentParams("snare") || {})[n], r.name);
    await page.mouse.click(canvasBox.x + r.x + r.w / 2, canvasBox.y + r.y + r.h / 2);
    await page.waitForTimeout(40);
    for (let k = 0; k < 3; k++) { await page.keyboard.press("ArrowUp"); await page.waitForTimeout(15); }
    await page.keyboard.press("Enter");
    const after = await page.evaluate((n) => (getInstrumentParams("snare") || {})[n], r.name);
    if (after === before) {
      console.log(`[TEST] ${r.name}: value unchanged by real input (at its max, or not steppable)`);
    } else {
      changed++;
    }
    dragged++;
    // Settle.
    await page.waitForTimeout(150);
  }
  if (dragged === 0) {
    throw new Error("edited 0 controls — stage selection produced no laid-out rects");
  }
  if (changed === 0) {
    throw new Error(`real input changed NO value across ${dragged} controls — the edit gesture is not reaching the synth`);
  }
  console.log(`[TEST] edited ${dragged} control(s) across their owning stages; ${changed} value(s) changed`);

  // ─── Post-drag capture ───
  await page.evaluate(() => {
    clearOutputCapture();
    startOutputCapture();
  });
  await page.waitForTimeout(2000); // long enough for several beats
  const postDrag = await page.evaluate(() => Array.from(getOutputCapture()));
  if (hasNonFinite(postDrag)) {
    throw new Error(
      `post-drag capture contains NaN/Inf after dragging all knobs — chain permanently corrupted`,
    );
  }
  const postPeak = peakOf(postDrag);
  console.log(`[TEST] post-drag peak=${postPeak.toFixed(4)} (baseline ${basePeak.toFixed(4)})`);
  if (postPeak < 0.001) {
    throw new Error(
      `BUG: audio stopped after slider drag — post-drag peak=${postPeak} (baseline=${basePeak}). ` +
      `This is the user-reported "audio stops in WASM" symptom.`,
    );
  }

  await page.evaluate(() => stopPlay());

  // Surface any errors / warnings collected during the drags.
  if (pageErrors.length > 0) {
    throw new Error(`page emitted ${pageErrors.length} errors during slider drags:\n  ${pageErrors.join("\n  ")}`);
  }
  if (biquadWarnings.length > 0) {
    throw new Error(
      `${biquadWarnings.length} BiquadFilterNode warning(s) during slider drag.\nSample: ${biquadWarnings[0]}`,
    );
  }

  console.log("[TEST] Real-mouse-drag e2e test passed.");

  if (isCoverageEnabled()) {
    await flushCoverage(
      page,
      new URL("../../coverage/browser-raw", import.meta.url).pathname,
      "instrument_params_e2e_real_drag",
    );
  }
} catch (err) {
  console.error("instrument_params_e2e_real_drag FAILED:", err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

console.log(exitCode === 0 ? "instrument_params_e2e_real_drag browser test PASSED" : "instrument_params_e2e_real_drag browser test FAILED");
process.exit(exitCode);
