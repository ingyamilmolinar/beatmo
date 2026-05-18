/**
 * REAL end-to-end Synth-tab audio regression — Phase 5 follow-up.
 *
 * Drives real Playwright mouse events on the actual canvas during real
 * playback (startPlay → sequencer firing notes) to reproduce the user
 * report: "audio stops in WASM after slider interaction". Captures
 * WebAudio output via startOutputCapture and asserts:
 *
 *   1. Baseline playback produces audible non-NaN audio.
 *   2. Real mouse-drag on each of the 8 slider rects (snare's recipe)
 *      doesn't silence the audio.
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
} from "./browser_test_helpers.js";

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
    typeof synthSliderRects === "function" &&
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
  await page.evaluate(() => window.audioReady);

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
  await page.evaluate(() => {
    setEQTab("synth");
    if (typeof forceDraw === "function") forceDraw();
  });
  // Allow a few frames so the EQ panel's Layout runs.
  await page.waitForTimeout(300);

  let rects = await page.evaluate(() => synthSliderRects());
  console.log(`[TEST] synth sliders visible: ${rects.length}`);
  if (rects.length === 0) {
    // Fall back: tab might not yet be active because the row's
    // instrument is still loading. Try forcing another draw.
    await page.evaluate(() => {
      setEQTab("synth");
      if (typeof forceDraw === "function") forceDraw();
    });
    await page.waitForTimeout(500);
    rects = await page.evaluate(() => synthSliderRects());
  }
  if (rects.length === 0) {
    throw new Error("synthSliderRects() returned 0 entries — Synth tab not active or layout did not run");
  }
  console.log(`[TEST] slider names: ${rects.map((r) => r.name).join(", ")}`);

  // Start playback — sequencer fires snare every beat.
  await page.evaluate(() => startPlay());
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

  // Drag each slider through its full range. We do this DURING active
  // playback so the audio capture overlaps with the slider drag.
  for (const r of rects) {
    console.log(`[TEST] dragging ${r.name} (${r.x},${r.y})..(${r.x + r.w},${r.y + r.h})`);

    // Press down at the slider's left edge (slider Value = 0).
    const yMid = canvasBox.y + r.y + r.h / 2;
    const xStart = canvasBox.x + r.x + 2;
    const xEnd = canvasBox.x + r.x + r.w - 2;

    await page.mouse.move(xStart, yMid);
    await page.mouse.down();
    // Sweep across the slider over ~500ms in 30 steps.
    const steps = 30;
    for (let s = 1; s <= steps; s++) {
      const x = xStart + ((xEnd - xStart) * s) / steps;
      await page.mouse.move(x, yMid, { steps: 1 });
      await page.waitForTimeout(15);
    }
    await page.mouse.up();
    // Settle.
    await page.waitForTimeout(150);
  }

  // ─── Post-drag capture ───
  await page.evaluate(() => {
    clearOutputCapture();
    startOutputCapture();
  });
  await page.waitForTimeout(2000); // long enough for several beats
  const postDrag = await page.evaluate(() => Array.from(getOutputCapture()));
  if (hasNonFinite(postDrag)) {
    throw new Error(
      `post-drag capture contains NaN/Inf after dragging all 8 sliders — chain permanently corrupted`,
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
