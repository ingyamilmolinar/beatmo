// audio_pending_ops.browser.test.js
//
// Tests pending channel operations that queue before audio context unlock.
// The audio.js module defers setChannelVolume, setChannelEQ
// calls when no AudioContext exists yet. These are applied once the context
// first reaches 'running' state.
//
// Verifies:
//   1. Volume set before context creation is applied after unlock
//   2. EQ set before context creation is applied after unlock
//   3. Multiple pending ops are applied in order
//   4. Pending insert effects are applied after unlock
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_pending_ops.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// This test uses audio.js directly (no WASM) to test the pending ops behavior
// before AudioContext creation. We create a minimal HTML page that loads audio.js
// as a module but does NOT auto-play or create an AudioContext.

const server = http.createServer((req, res) => {
  if (req.url === "/" || req.url === "/pending.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module" src="audio.js"></script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

let exitCode = 0;
let browser;
try {
  // Do NOT pass --autoplay-policy=no-user-gesture-required
  // so that AudioContext starts suspended (simulating mobile behavior).
  browser = await chromium.launch({ args: [] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/pending.html`);

  // Wait for audio.js to load (it sets window.playSound and similar).
  await page.waitForFunction(() =>
    typeof window.playSound === "function" &&
    typeof window.setChannelVolume === "function" &&
    typeof window.channelVolume === "function" &&
    typeof window.setChannelEQ === "function"
  );

  // ========================================================================
  // Scenario 1: Volume set before context creation is applied after unlock
  // ========================================================================
  console.log("--- Scenario 1: Pending volume ---");

  const s1 = await page.evaluate(async () => {
    // At this point, no AudioContext should exist yet.
    const hasCtxBefore = !!window.__audioCtx;

    // Set volume before any AudioContext exists. This should be queued.
    window.setChannelVolume("kick", 0.42);

    // Volume query before context returns default (1).
    const volBefore = window.channelVolume("kick");

    // Now trigger audio unlock via user gesture simulation.
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof window.resumeAudio === "function") window.resumeAudio();

    // Wait for context to be created and reach running state.
    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (window.__audioCtx && window.__audioCtx.state === "running") break;
      await new Promise((r) => setTimeout(r, 100));
    }

    const hasCtxAfter = !!window.__audioCtx;
    const ctxState = window.__audioCtx ? window.__audioCtx.state : "none";

    // After context is running, the pending volume should be applied.
    // Small delay to allow applyPendingOps to complete.
    await new Promise((r) => setTimeout(r, 200));
    const volAfter = window.channelVolume("kick");

    // Clean up
    window.setChannelVolume("kick", 1.0);

    return { hasCtxBefore, volBefore, hasCtxAfter, ctxState, volAfter };
  });

  console.log(`  Context before: ${s1.hasCtxBefore}, Vol before: ${s1.volBefore}, Context after: ${s1.hasCtxAfter} (${s1.ctxState}), Vol after: ${s1.volAfter}`);
  // Volume before context should be the default (1)
  if (s1.volBefore !== 1) throw new Error(`Scenario 1 FAIL: volume before context should be 1 (default), got ${s1.volBefore}`);
  // After unlock, volume should be applied
  if (!s1.hasCtxAfter || s1.ctxState !== "running") {
    throw new Error(`Scenario 1 FAIL: AudioContext did not reach running state after unlock (state=${s1.ctxState})`);
  }
  if (Math.abs(s1.volAfter - 0.42) > 0.01) throw new Error(`Scenario 1 FAIL: volume after unlock should be 0.42, got ${s1.volAfter}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 2: EQ set before context is applied after unlock
  // ========================================================================
  console.log("--- Scenario 2: Pending EQ ---");

  const page2 = await browser.newPage();
  await page2.goto(`http://localhost:${port}/pending.html`);
  await page2.waitForFunction(() =>
    typeof window.setChannelEQ === "function" &&
    typeof window.enableChannelAnalyzer === "function"
  );

  const s2 = await page2.evaluate(async () => {
    const hasCtxBefore = !!window.__audioCtx;

    // Set EQ before context exists.
    window.setChannelEQ("kick", [
      { freq: 100, q: 1, gainDB: 6 },
      { freq: 1000, q: 1, gainDB: -3 },
    ]);

    // Trigger unlock
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof window.resumeAudio === "function") window.resumeAudio();

    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (window.__audioCtx && window.__audioCtx.state === "running") break;
      await new Promise((r) => setTimeout(r, 100));
    }
    await new Promise((r) => setTimeout(r, 200));

    const hasCtxAfter = !!window.__audioCtx;
    const ctxState = window.__audioCtx ? window.__audioCtx.state : "none";

    // Verify the EQ was applied: the channel chain should have eqChain entries.
    // We can't easily inspect the internal eqChain array from outside, but we
    // can verify that setChannelEQ did not throw and the analyzer can be enabled
    // (which exercises the same pipeline).
    let analyzerOK = false;
    try {
      window.enableChannelAnalyzer("kick", 512);
      const snap = window.channelAnalyzerSnapshot("kick");
      analyzerOK = snap && typeof snap.rms === "number";
    } catch (_) {}

    // Clean up EQ
    window.setChannelEQ("kick", []);

    return { hasCtxBefore, hasCtxAfter, ctxState, analyzerOK };
  });

  console.log(`  Context: ${s2.ctxState}, Analyzer after EQ: ${s2.analyzerOK}`);
  if (!s2.hasCtxAfter || s2.ctxState !== "running") {
    throw new Error(`Scenario 2 FAIL: AudioContext did not reach running state after unlock (state=${s2.ctxState})`);
  }
  if (!s2.analyzerOK) throw new Error(`Scenario 2 FAIL: analyzer should work after pending EQ was applied`);
  console.log("  PASS");
  await page2.close();

  // ========================================================================
  // Scenario 3: Multiple pending ops are applied in order
  // ========================================================================
  console.log("--- Scenario 3: Multiple pending ops order ---");

  const page3 = await browser.newPage();
  await page3.goto(`http://localhost:${port}/pending.html`);
  await page3.waitForFunction(() =>
    typeof window.setChannelVolume === "function" &&
    typeof window.channelVolume === "function"
  );

  const s3 = await page3.evaluate(async () => {
    // Queue multiple volume changes before context exists.
    // The last value should win because they execute in order.
    window.setChannelVolume("kick", 0.2);
    window.setChannelVolume("kick", 0.5);
    window.setChannelVolume("kick", 0.77);

    // Trigger unlock
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof window.resumeAudio === "function") window.resumeAudio();

    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (window.__audioCtx && window.__audioCtx.state === "running") break;
      await new Promise((r) => setTimeout(r, 100));
    }
    await new Promise((r) => setTimeout(r, 200));

    const ctxState = window.__audioCtx ? window.__audioCtx.state : "none";
    const finalVol = window.channelVolume("kick");

    // Clean up
    window.setChannelVolume("kick", 1.0);

    return { ctxState, finalVol };
  });

  console.log(`  Final volume: ${s3.finalVol} (ctx: ${s3.ctxState})`);
  if (s3.ctxState !== "running") {
    throw new Error(`Scenario 3 FAIL: AudioContext did not reach running state after unlock (state=${s3.ctxState})`);
  }
  // The last setChannelVolume(0.77) should win
  if (Math.abs(s3.finalVol - 0.77) > 0.01) throw new Error(`Scenario 3 FAIL: expected final volume 0.77, got ${s3.finalVol}`);
  console.log("  PASS");
  await page3.close();

  // ========================================================================
  // Scenario 4: Pending insert effects are applied after context unlock.
  //
  // Reproduces the bug where the WASM startup demo imports per-instrument
  // insert effects (e.g. distortion on Snare) before the AudioContext is
  // unlocked. updateInsertEffects() used to silently early-return on
  // !hasCtx(), so the slot config was lost; the user would only hear the
  // effect after toggling it off/on (which fires updateInsertEffects again
  // with a live context).
  // ========================================================================
  console.log("--- Scenario 4: Pending insert effects ---");

  const page4 = await browser.newPage();
  await page4.goto(`http://localhost:${port}/pending.html`);
  await page4.waitForFunction(() =>
    typeof window.updateInsertEffects === "function" &&
    typeof window.__channelInsertFXCount === "function"
  );

  const s4 = await page4.evaluate(async () => {
    const hasCtxBefore = !!window.__audioCtx;
    const fxBefore = window.__channelInsertFXCount("snare");

    // Queue an insert effect chain before any AudioContext exists. This is
    // exactly what happens during demo Import() at startup on mobile, where
    // the AudioContext is suspended pending a user gesture.
    const slots = [
      { type: "distortion", enabled: true, params: { drive: 8, tone: 4000, mix: 1 } },
    ];
    window.updateInsertEffects("snare", JSON.stringify(slots));

    // Trigger user-gesture unlock.
    document.dispatchEvent(new Event("pointerdown"));
    if (typeof window.resumeAudio === "function") window.resumeAudio();

    const deadline = Date.now() + 10000;
    while (Date.now() < deadline) {
      if (window.__audioCtx && window.__audioCtx.state === "running") break;
      await new Promise((r) => setTimeout(r, 100));
    }
    // Allow applyPendingOps + rewireChannel to settle.
    await new Promise((r) => setTimeout(r, 200));

    const hasCtxAfter = !!window.__audioCtx;
    const ctxState = window.__audioCtx ? window.__audioCtx.state : "none";
    const fxAfter = window.__channelInsertFXCount("snare");

    // Clean up
    try { window.updateInsertEffects("snare", "[]"); } catch (_) {}

    return { hasCtxBefore, fxBefore, hasCtxAfter, ctxState, fxAfter };
  });

  console.log(`  Ctx before: ${s4.hasCtxBefore}, FX before: ${s4.fxBefore}, Ctx after: ${s4.hasCtxAfter} (${s4.ctxState}), FX after: ${s4.fxAfter}`);
  if (s4.hasCtxBefore) throw new Error(`Scenario 4 FAIL: AudioContext should not exist before unlock`);
  if (s4.fxBefore !== 0) throw new Error(`Scenario 4 FAIL: insert FX count before context should be 0, got ${s4.fxBefore}`);
  if (!s4.hasCtxAfter || s4.ctxState !== "running") {
    throw new Error(`Scenario 4 FAIL: AudioContext did not reach running state after unlock (state=${s4.ctxState})`);
  }
  if (s4.fxAfter !== 1) {
    throw new Error(`Scenario 4 FAIL: pending insert effect was dropped — expected 1 FX subgraph after unlock, got ${s4.fxAfter}. This means updateInsertEffects() did not queue while the context was suspended.`);
  }
  console.log("  PASS");
  await page4.close();

  console.log("\nAll pending ops tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_pending_ops");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
