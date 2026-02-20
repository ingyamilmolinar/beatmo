// mobile_speaker_routing.browser.test.js
//
// Verifies iOS speaker routing fixes: silent <audio> element, audioSession API,
// visibilitychange recovery, and suspend/resume resilience.
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/mobile_speaker_routing.browser.test.js

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { cdpTap } from "./touch_cdp_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
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

const iPhone = devices['iPhone 12 landscape'];
let exitCode = 0;
let browser;
try {
  // ========================================================================
  // Scenario 1: Silent audio element created on gesture unlock
  // Launch WITHOUT --autoplay-policy=no-user-gesture-required to test
  // that the silent <audio> element is created during the unlock gesture.
  // ========================================================================
  console.log("--- Scenario 1: Silent audio element created on gesture unlock ---");
  {
    const b = await chromium.launch({
      args: [
        // No autoplay policy override — enforce restrictions.
      ],
    });
    const context = await b.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    // Wait for WASM and audio.js exports to be ready.
    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof resumeAudio === "function" &&
      typeof __hasSilentAudioEl === "function" &&
      typeof __silentAudioElState === "function"
    );

    // Before any gesture, the silent audio element should NOT exist.
    const beforeGesture = await page.evaluate(() => __hasSilentAudioEl());
    console.log(`  Silent audio element before gesture: ${beforeGesture}`);
    if (beforeGesture) throw new Error("Scenario 1 FAIL: silent audio element exists before gesture");

    // Tap the canvas to trigger the unlock listeners.
    const canvasRect = await page.evaluate(() => {
      const c = document.querySelector('canvas');
      if (!c) return null;
      const r = c.getBoundingClientRect();
      return { x: r.left, y: r.top, w: r.width, h: r.height };
    });
    if (canvasRect) {
      await cdpTap(page, canvasRect.w / 2, canvasRect.h / 2);
    } else {
      await page.evaluate(() => {
        document.dispatchEvent(new Event("touchstart", { bubbles: true }));
        document.dispatchEvent(new Event("touchend", { bubbles: true }));
      });
    }
    await page.waitForTimeout(500);

    // After gesture, the silent audio element should exist.
    const afterGesture = await page.evaluate(() => __hasSilentAudioEl());
    console.log(`  Silent audio element after gesture: ${afterGesture}`);
    if (!afterGesture) throw new Error("Scenario 1 FAIL: silent audio element NOT created after gesture");

    // Verify element attributes.
    const elState = await page.evaluate(() => __silentAudioElState());
    console.log(`  Element state: loop=${elState.loop}, volume=${elState.volume}, src=${elState.src}`);
    if (!elState.loop) throw new Error("Scenario 1 FAIL: silent audio element loop is false");
    if (!elState.src) throw new Error("Scenario 1 FAIL: silent audio element has no src");
    if (elState.volume > 0.1) throw new Error(`Scenario 1 FAIL: volume too high (${elState.volume})`);

    // Verify the <audio> element exists in the DOM.
    const audioInDOM = await page.evaluate(() => !!document.querySelector('audio'));
    console.log(`  <audio> element in DOM: ${audioInDOM}`);
    if (!audioInDOM) throw new Error("Scenario 1 FAIL: <audio> element not found in DOM");

    console.log("  PASS");
    await b.close();
  }

  // ========================================================================
  // Scenario 2: audioSession.type set to 'playback'
  // Verifies that configureAudioSession() runs without error. Chromium
  // doesn't support navigator.audioSession, so we verify graceful fallback.
  // ========================================================================
  console.log("\n--- Scenario 2: audioSession.type set to 'playback' ---");
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof __audioSessionType === "function" &&
      typeof resumeAudio === "function"
    );

    // Check audioSession type — Chromium doesn't support this API,
    // so we expect null (graceful no-op).
    const sessionType = await page.evaluate(() => __audioSessionType());
    console.log(`  audioSession.type: ${sessionType}`);

    // The key assertion: no errors were thrown during configureAudioSession().
    // On Safari 16.4+ this would return 'playback'; on Chromium it's null.
    if (sessionType !== null && sessionType !== 'playback') {
      throw new Error(`Scenario 2 FAIL: unexpected audioSession.type '${sessionType}'`);
    }

    // Trigger a gesture and check again.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    const afterGesture = await page.evaluate(() => __audioSessionType());
    console.log(`  audioSession.type after gesture: ${afterGesture}`);
    if (afterGesture !== null && afterGesture !== 'playback') {
      throw new Error(`Scenario 2 FAIL: unexpected audioSession.type after gesture '${afterGesture}'`);
    }

    console.log("  PASS");
    await context.close();
  }

  // ========================================================================
  // Scenario 3: Audio plays with speaker routing fix active
  // Mobile emulation WITH autoplay bypass. Build circuit, start playback,
  // verify output AND that the silent audio element was created.
  // ========================================================================
  console.log("\n--- Scenario 3: Audio plays with speaker routing fix active ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function" &&
      typeof addNode === "function" &&
      typeof addEdgeGrid === "function" &&
      typeof updateBeatInfosJS === "function" &&
      typeof startPlay === "function" &&
      typeof stopPlay === "function" &&
      typeof __hasSilentAudioEl === "function"
    );

    // Unlock audio context via gesture.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(100);

    // Build a simple circuit: two nodes in a loop.
    await page.evaluate(() => {
      addNode?.(0, 0, "regular");
      addNode?.(4, 0, "regular");
      addEdgeGrid?.(0, 0, 4, 0);
      addEdgeGrid?.(4, 0, 0, 0);
      updateBeatInfosJS?.();
      forceDraw?.();
    });
    await page.waitForTimeout(200);

    // Enable output capture and start playback.
    const s3 = await page.evaluate(async () => {
      enableOutputCapture?.();
      startOutputCapture?.();
      startPlay?.();
      await new Promise((r) => setTimeout(r, 1500));
      stopPlay?.();
      await new Promise((r) => setTimeout(r, 200));
      const captured = Array.from(stopOutputCapture?.() || []);

      let peak = 0;
      let sum = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        sum += captured[i] * captured[i];
      }
      const rms = captured.length > 0 ? Math.sqrt(sum / captured.length) : 0;
      const hasSilent = __hasSilentAudioEl?.();
      return { samples: captured.length, peak, rms, hasSilentEl: hasSilent };
    });

    console.log(`  Samples: ${s3.samples}, Peak: ${s3.peak.toFixed(6)}, RMS: ${s3.rms.toFixed(6)}`);
    console.log(`  Silent audio element active: ${s3.hasSilentEl}`);
    if (s3.samples === 0) throw new Error("Scenario 3 FAIL: no samples captured");
    if (s3.rms < 0.0001) throw new Error(`Scenario 3 FAIL: no audio energy (RMS=${s3.rms})`);
    if (!s3.hasSilentEl) throw new Error("Scenario 3 FAIL: silent audio element not created");
    console.log("  PASS");

    await context.close();
  }

  // ========================================================================
  // Scenario 4: Page visibility change resumes audio
  // Start playback, suspend context programmatically, dispatch
  // visibilitychange, verify context resumes and audio output recovers.
  // ========================================================================
  console.log("\n--- Scenario 4: Page visibility change resumes audio ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function" &&
      typeof resumeAudio === "function"
    );

    // Unlock audio context.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    await page.evaluate(async () => {
      await window.audioReady;
      await ensureSynthSample?.("kick");
      await ensureSynthSample?.("snare");
    });

    const s4 = await page.evaluate(async () => {
      const ctx = window.__audioCtx;
      if (!ctx) return { error: "no AudioContext" };

      // Verify context is running.
      if (ctx.state !== 'running') {
        return { error: `expected running, got ${ctx.state}` };
      }

      // Play a sound to prove audio works before suspend.
      // Use software-side recordSamples capture instead of
      // ScriptProcessorNode-based output capture.  The deprecated
      // ScriptProcessorNode is unreliable in headless Chromium after
      // AudioContext suspend/resume cycles.
      window.__samples = [];
      window.__captureSamples = true;
      await playSound("kick", 1.0);
      window.__captureSamples = false;
      const beforeCapture = window.__samples;
      let beforeRMS = 0;
      for (let i = 0; i < beforeCapture.length; i++) {
        beforeRMS += beforeCapture[i] * beforeCapture[i];
      }
      beforeRMS = beforeCapture.length > 0 ? Math.sqrt(beforeRMS / beforeCapture.length) : 0;

      // Suspend context to simulate backgrounding.
      await ctx.suspend();
      const suspendedState = ctx.state;

      // Dispatch visibilitychange to trigger recovery handler.
      // The handler checks document.visibilityState, which we can't override,
      // but in headless it returns 'visible', so the handler will fire.
      document.dispatchEvent(new Event('visibilitychange'));

      // Wait for resume to take effect.
      await new Promise((r) => setTimeout(r, 500));
      const afterVisState = ctx.state;

      // Play another sound after recovery.
      window.__samples = [];
      window.__captureSamples = true;
      await playSound("snare", 1.0);
      window.__captureSamples = false;
      const afterCapture = window.__samples;
      let afterRMS = 0;
      for (let i = 0; i < afterCapture.length; i++) {
        afterRMS += afterCapture[i] * afterCapture[i];
      }
      afterRMS = afterCapture.length > 0 ? Math.sqrt(afterRMS / afterCapture.length) : 0;

      return {
        beforeRMS,
        suspendedState,
        afterVisState,
        afterSamples: afterCapture.length,
        afterRMS,
      };
    });

    if (s4.error) throw new Error(`Scenario 4 FAIL: ${s4.error}`);
    console.log(`  Before suspend RMS: ${s4.beforeRMS.toFixed(6)}`);
    console.log(`  State after suspend(): ${s4.suspendedState}`);
    console.log(`  State after visibilitychange: ${s4.afterVisState}`);
    console.log(`  After recovery: ${s4.afterSamples} samples, RMS: ${s4.afterRMS.toFixed(6)}`);

    if (s4.suspendedState !== 'suspended') {
      throw new Error(`Scenario 4 FAIL: context not suspended after suspend() (${s4.suspendedState})`);
    }
    if (s4.afterVisState !== 'running') {
      throw new Error(`Scenario 4 FAIL: context not resumed after visibilitychange (${s4.afterVisState})`);
    }
    if (s4.afterSamples === 0) throw new Error("Scenario 4 FAIL: no samples after recovery");
    if (s4.afterRMS < 0.0001) throw new Error(`Scenario 4 FAIL: no audio after recovery (RMS=${s4.afterRMS})`);
    console.log("  PASS");

    await context.close();
  }

  // ========================================================================
  // Scenario 5: Multiple suspend/resume cycles
  // Start playback, then do 3 cycles of suspend → resumeAudio → verify.
  // ========================================================================
  console.log("\n--- Scenario 5: Multiple suspend/resume cycles ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function" &&
      typeof resumeAudio === "function"
    );

    // Unlock audio context.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    await page.evaluate(async () => {
      await window.audioReady;
      await ensureSynthSample?.("kick");
    });

    const s5 = await page.evaluate(async () => {
      const ctx = window.__audioCtx;
      if (!ctx) return { error: "no AudioContext" };
      if (ctx.state !== 'running') return { error: `initial state ${ctx.state}` };

      const results = [];

      for (let cycle = 0; cycle < 3; cycle++) {
        // Suspend.
        await ctx.suspend();
        if (ctx.state !== 'suspended') {
          return { error: `cycle ${cycle}: expected suspended, got ${ctx.state}` };
        }

        // Resume via resumeAudio (the function Go calls).
        resumeAudio?.();
        // Wait for resume to complete.
        await new Promise((r) => setTimeout(r, 300));

        const stateAfterResume = ctx.state;

        // Use software-side recordSamples capture instead of
        // ScriptProcessorNode-based output capture.  The deprecated
        // ScriptProcessorNode is unreliable in headless Chromium under
        // CPU contention (parallel tests), producing all-zero buffers.
        // recordSamples captures synchronously inside playSound(), so
        // it's deterministic regardless of audio thread scheduling.
        window.__samples = [];
        window.__captureSamples = true;
        await playSound("kick", 1.0);
        window.__captureSamples = false;
        const captured = window.__samples;

        let rms = 0;
        for (let i = 0; i < captured.length; i++) {
          rms += captured[i] * captured[i];
        }
        rms = captured.length > 0 ? Math.sqrt(rms / captured.length) : 0;

        results.push({ cycle, state: stateAfterResume, samples: captured.length, rms });
      }

      return { results };
    });

    if (s5.error) throw new Error(`Scenario 5 FAIL: ${s5.error}`);

    for (const r of s5.results) {
      console.log(`  Cycle ${r.cycle}: state=${r.state}, samples=${r.samples}, RMS=${r.rms.toFixed(6)}`);
      if (r.state !== 'running') {
        throw new Error(`Scenario 5 FAIL: cycle ${r.cycle} state is ${r.state}, expected running`);
      }
      if (r.samples === 0) {
        throw new Error(`Scenario 5 FAIL: cycle ${r.cycle} no samples captured`);
      }
      if (r.rms < 0.0001) {
        throw new Error(`Scenario 5 FAIL: cycle ${r.cycle} no audio energy (RMS=${r.rms})`);
      }
    }
    console.log("  PASS");

    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_speaker_routing");
    await context.close();
  }

  console.log("\nAll mobile speaker routing tests passed.");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
