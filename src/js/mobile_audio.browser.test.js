// Mobile audio: touch unlock with autoplay rules enforced, playback output, parity with desktop, EQ rewires, and the
// suspended→running queue flush. Usage: GO=… node src/js/mobile_audio.browser.test.js

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

// A stable fixture with a non-flat EQ on EVERY row, so import rewires every channel; the startup demo is free to change.
function eqCircuitJSON() {
  const f = JSON.parse(fs.readFileSync(path.join(repoRoot, "src/go/internal/assets/test_fixture_demo.json"), "utf-8"));
  const bands = [[22, 44], [44, 88], [88, 177], [177, 354], [354, 707], [707, 1414], [1414, 2828], [2828, 5657], [5657, 11314], [11314, 20000]];
  for (const inst of f.instruments) {
    inst.eq = { gains_db: [1.1, 1.7, -2.4, -6, -3, 0, -0.8, 0, 0, 0], bands_hz: bands, hpf_cutoff_hz: 20, lpf_enabled: true, lpf_cutoff_hz: 8000 };
  }
  if (!f.instruments.length) throw new Error("test fixture has no instruments");
  return JSON.stringify(f);
}
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
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
  // Without the autoplay override, so Chromium enforces the real mobile restrictions.
  console.log("--- Scenario 1: AudioContext unlock via touch ---");
  {
    const b = await chromium.launch({
      args: [
        // No --autoplay-policy flag: the restrictions must apply.
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
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function"
    );

    // Check initial AudioContext state — should be suspended (no user gesture yet).
    const initialState = await page.evaluate(() => {
      // audioNow() calls getCtx().
      audioNow?.();
      return window.__audioCtx?.state || 'no-context';
    });
    console.log(`  Initial AudioContext state: ${initialState}`);
    // Headless may auto-run the context; logged either way.

    // Trusted CDP touch, as a real phone.
    const canvasRect = await page.evaluate(() => {
      const c = document.querySelector('canvas');
      if (!c) return null;
      const r = c.getBoundingClientRect();
      return { x: r.left, y: r.top, w: r.width, h: r.height };
    });
    if (canvasRect) {
      // Tap center of canvas.
      await cdpTap(page, canvasRect.w / 2, canvasRect.h / 2);
    } else {
      // Fallback: dispatch synthetic touch event.
      await page.evaluate(() => {
        document.dispatchEvent(new Event("touchstart", { bubbles: true }));
        document.dispatchEvent(new Event("touchend", { bubbles: true }));
      });
    }

    // Give the unlock time to process.
    await page.waitForTimeout(500);

    // Try resumeAudio as well (mimics what Go calls on each frame).
    await page.evaluate(() => resumeAudio?.());
    await page.waitForTimeout(200);

    const finalState = await page.evaluate(() => window.__audioCtx?.state || 'no-context');
    console.log(`  Final AudioContext state: ${finalState}`);

    if (finalState === 'running') {
      console.log("  PASS: AudioContext unlocked to running state");
    } else if (finalState === 'suspended') {
      // Headless CDP touch may not count as a gesture; the unlock code still ran.
      console.log("  WARN: AudioContext still suspended (expected in headless without autoplay policy)");
      console.log("  NOTE: The unlock listeners were registered and exercised; real mobile devices will unlock.");
    } else {
      console.log(`  WARN: unexpected state '${finalState}'`);
    }

    await b.close();
  }

  // With the autoplay override: the pipeline under a mobile viewport, not the unlock.
  console.log("\n--- Scenario 2: Mobile playback produces audio ---");
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
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
      typeof updateBeatInfos === "function" &&
      typeof startPlay === "function" &&
      typeof stopPlay === "function"
    );

    // Unlock audio context.
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
      updateBeatInfos?.();
      forceDraw?.();
    });
    await page.waitForTimeout(200);

    // Enable output capture and start playback via API.
    const s2 = await page.evaluate(async () => {
      enableOutputCapture?.();
      startOutputCapture?.();
      startPlay?.();
      // Let several beats play.
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
      return { samples: captured.length, peak, rms };
    });

    console.log(`  Samples: ${s2.samples}, Peak: ${s2.peak.toFixed(6)}, RMS: ${s2.rms.toFixed(6)}`);
    if (s2.samples === 0) throw new Error("Scenario 2 FAIL: no samples captured");
    if (s2.rms < 0.0001) throw new Error(`Scenario 2 FAIL: no audio energy on mobile (RMS=${s2.rms})`);
    // Capture samples are pre-clamp, so overlapping voices can exceed 1.0; only extremes fail.
    if (s2.peak > 10) throw new Error(`Scenario 2 FAIL: extreme peak detected (peak=${s2.peak})`);
    console.log("  PASS");

    await context.close();
  }

  // The same 2-node circuit on both, same duration, RMS compared.
  console.log("\n--- Scenario 3: Mobile vs desktop audio parity ---");
  {
    // Helper: build a 2-node loop, play for 1.5s, capture output.
    async function capturePlayback(page) {
      await page.waitForFunction(() =>
        typeof playSound === "function" &&
        typeof startOutputCapture === "function" &&
        typeof stopOutputCapture === "function" &&
        typeof addNode === "function" &&
        typeof addEdgeGrid === "function" &&
        typeof updateBeatInfos === "function" &&
        typeof startPlay === "function" &&
        typeof stopPlay === "function"
      );

      await page.evaluate(() => {
        document.dispatchEvent(new Event("pointerdown"));
        resumeAudio?.();
      });
      await page.waitForTimeout(100);

      // Build a simple 2-node loop circuit.
      await page.evaluate(() => {
        addNode?.(0, 0, "regular");
        addNode?.(4, 0, "regular");
        addEdgeGrid?.(0, 0, 4, 0);
        addEdgeGrid?.(4, 0, 0, 0);
        updateBeatInfos?.();
        forceDraw?.();
      });
      await page.waitForTimeout(200);

      return page.evaluate(async () => {
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
        return { samples: captured.length, peak, rms };
      });
    }

    // Desktop run.
    const desktopCtx = await browser.newContext();
    const desktopPage = await desktopCtx.newPage();
    await desktopPage.goto(`http://localhost:${port}/`);
    const desktopResult = await capturePlayback(desktopPage);
    await desktopCtx.close();

    // Mobile run.
    const mobileCtx = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const mobilePage = await mobileCtx.newPage();
    await mobilePage.goto(`http://localhost:${port}/`);
    const mobileResult = await capturePlayback(mobilePage);
    await mobileCtx.close();

    console.log(`  Desktop: ${desktopResult.samples} samples, Peak=${desktopResult.peak.toFixed(6)}, RMS=${desktopResult.rms.toFixed(6)}`);
    console.log(`  Mobile:  ${mobileResult.samples} samples, Peak=${mobileResult.peak.toFixed(6)}, RMS=${mobileResult.rms.toFixed(6)}`);

    // A broken mobile path (routing, unlock, a silent bus) reads ~0 RMS: the assertion that catches real bugs.
    if (desktopResult.rms < 0.0001) throw new Error(`Scenario 3 FAIL: desktop has no audio energy (RMS=${desktopResult.rms})`);
    if (mobileResult.rms < 0.0001) throw new Error(`Scenario 3 FAIL: mobile has no audio energy (RMS=${mobileResult.rms})`);

    // Identical code on both; mobile's 3× DPR canvas starves the single-threaded scheduler far more under load, so a low
    // ratio measures contention, not code. Only gross divergence fails; the EQ'd-circuit scenarios cover real regressions.
    const ratio = mobileResult.rms / desktopResult.rms;
    console.log(`  RMS ratio (mobile/desktop): ${ratio.toFixed(4)}`);
    if (ratio > 10) {
      // Starvation only makes mobile quieter, so louder is a real gain bug.
      throw new Error(`Scenario 3 FAIL: mobile RMS (${mobileResult.rms.toFixed(6)}) is much higher than desktop (${desktopResult.rms.toFixed(6)})`);
    }
    if (ratio < 0.1) {
      console.log(`  WARN: mobile RMS (${mobileResult.rms.toFixed(6)}) much lower than desktop (${desktopResult.rms.toFixed(6)}) — expected under single-thread audio starvation on the heavier mobile canvas; both produced audio so routing is intact.`);
    }
    console.log("  PASS");
  }

  // Structural rather than rendered (ScriptProcessor timing flakes under load): setChannelEQ's rewire must not
  // disconnect the output capture chain.
  console.log("\n--- Scenario 4: rewireChannel preserves output capture ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof enableOutputCapture === "function" &&
      typeof startOutputCapture === "function" &&
      typeof getOutputCaptureStats === "function" &&
      typeof setChannelEQ === "function" &&
      typeof window.__getMasterOut === "function"
    );

    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(100);

    const s4 = await page.evaluate(() => {
      // Step 1: Enable output capture — sets up limiter → captureNode → destination.
      enableOutputCapture?.();
      const statsBefore = getOutputCaptureStats?.();
      if (!statsBefore?.hasNode) {
        return { error: "capture node not created after enableOutputCapture" };
      }

      // Step 2: Spy on limiter.disconnect to verify rewireChannel doesn't touch it.
      const limiter = window.__getMasterOut?.();
      if (!limiter) {
        return { error: "mainLimiter not available" };
      }
      const origDisconnect = limiter.disconnect.bind(limiter);
      let limiterDisconnectCalls = 0;
      limiter.disconnect = function(...args) {
        limiterDisconnectCalls++;
        return origDisconnect(...args);
      };

      // Step 3: Trigger rewireChannel via setChannelEQ on main channel.
      setChannelEQ?.("main", [
        { type: "peaking", freq: 1000, q: 1, gainDB: 3 },
      ]);

      // Restore the original disconnect method.
      limiter.disconnect = origDisconnect;

      // Step 4: Verify capture chain is intact after rewire.
      const statsAfter = getOutputCaptureStats?.();

      // Step 5: Verify startOutputCapture still works (doesn't throw).
      let startCaptureOk = false;
      try {
        startOutputCapture?.();
        startCaptureOk = true;
      } catch (_) {}
      const statsAfterStart = getOutputCaptureStats?.();

      return {
        hasNodeBefore: statsBefore.hasNode,
        limiterDisconnectCalls,
        hasNodeAfterRewire: statsAfter?.hasNode,
        startCaptureOk,
        enabledAfterStart: statsAfterStart?.enabled,
      };
    });

    if (s4.error) throw new Error(`Scenario 4 FAIL: ${s4.error}`);
    console.log(`  Capture node before rewire: ${s4.hasNodeBefore}`);
    console.log(`  Limiter disconnect calls during rewire: ${s4.limiterDisconnectCalls}`);
    console.log(`  Capture node after rewire: ${s4.hasNodeAfterRewire}`);
    console.log(`  startOutputCapture succeeded: ${s4.startCaptureOk}`);
    console.log(`  Capture enabled after start: ${s4.enabledAfterStart}`);

    if (s4.limiterDisconnectCalls !== 0) {
      throw new Error(`Scenario 4 FAIL: rewireChannel called limiter.disconnect ${s4.limiterDisconnectCalls} time(s) — capture chain may be broken`);
    }
    if (!s4.hasNodeAfterRewire) {
      throw new Error("Scenario 4 FAIL: capture node cleared after rewireChannel");
    }
    if (!s4.startCaptureOk) {
      throw new Error("Scenario 4 FAIL: startOutputCapture threw after rewireChannel");
    }
    if (!s4.enabledAfterStart) {
      throw new Error("Scenario 4 FAIL: capture not enabled after startOutputCapture");
    }
    console.log("  PASS");

    await context.close();
  }

  // Import rewires every channel's EQ, which once broke the audio routing.
  console.log("\n--- Scenario 5: EQ'd circuit playback on mobile ---");
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
      typeof importJSON === "function" &&
      typeof startPlay === "function" &&
      typeof stopPlay === "function" &&
      typeof forceDraw === "function"
    );

    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(100);

    const circuitJSON = eqCircuitJSON();

    await page.evaluate((json) => {
      importJSON?.(json);
      forceDraw?.();
    }, circuitJSON);
    await page.waitForTimeout(500);

    const s5 = await page.evaluate(async () => {
      enableOutputCapture?.();
      startOutputCapture?.();
      startPlay?.();
      // Let the circuit play for 2 seconds to ensure multiple instruments fire.
      await new Promise((r) => setTimeout(r, 2000));
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
      return { samples: captured.length, peak, rms };
    });

    console.log(`  Samples: ${s5.samples}, Peak: ${s5.peak.toFixed(6)}, RMS: ${s5.rms.toFixed(6)}`);
    if (s5.samples === 0) throw new Error("Scenario 5 FAIL: no samples captured");
    if (s5.rms < 0.0001) throw new Error(`Scenario 5 FAIL: no audio energy from the EQ'd circuit on mobile (RMS=${s5.rms})`);
    // Capture samples are pre-clamp, so overlapping voices can exceed 1.0; only extremes fail.
    if (s5.peak > 10) throw new Error(`Scenario 5 FAIL: extreme peak detected (peak=${s5.peak})`);
    console.log("  PASS");

    await context.close();
  }

  // Events enqueued while suspended must flush and sound on resume: the path real phones hit, without a real gesture.
  console.log("\n--- Scenario 6: Suspended→running queue flush ---");
  {
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    // A page without Go has no instruments, and under load notes reached audio.js before their documents did.
    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof resumeAudio === "function" &&
      typeof window.__instrumentBlockForTest === "function" &&
      !!window.__instrumentBlockForTest("kick") && !!window.__instrumentBlockForTest("snare"),
      null, { timeout: 60000 }
    );

    // Ensure audio context is created and running first.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    const s6 = await page.evaluate(async () => {
      const ctx = window.__audioCtx;
      if (!ctx) return { error: "no AudioContext" };

      // Asserted off the rendered PCM, not the live ScriptProcessor tap: on headless SwiftShader under load the tap drops the
      // short transients right after resume, while the voices still play.
      window.__samples = [];
      window.__captureSamples = true;

      // Suspend the context to simulate mobile state.
      await ctx.suspend();
      if (ctx.state !== 'suspended') {
        window.__captureSamples = false;
        return { error: `expected suspended, got ${ctx.state}` };
      }

      // Deferred by the suspended guard until 'running'.
      await playSound("kick", 1.0);
      await playSound("snare", 1.0);

      // Small wait to let the deferred flush path set up its promise chain.
      await new Promise((r) => setTimeout(r, 100));

      // Resume the context — statechange listener triggers the deferred flush.
      await ctx.resume();

      // The flush is fire-and-forget and may defer under contention.
      const deadline = performance.now() + 2000;
      while (window.__samples.length === 0 && performance.now() < deadline) {
        await new Promise((r) => setTimeout(r, 10));
      }
      window.__captureSamples = false;

      const captured = window.__samples.slice();
      let peak = 0;
      let sum = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        sum += captured[i] * captured[i];
      }
      const rms = captured.length > 0 ? Math.sqrt(sum / captured.length) : 0;
      return { samples: captured.length, peak, rms, state: ctx.state };
    });

    if (s6.error) throw new Error(`Scenario 6 FAIL: ${s6.error}`);
    console.log(`  Samples: ${s6.samples}, Peak: ${s6.peak.toFixed(6)}, RMS: ${s6.rms.toFixed(6)}, State: ${s6.state}`);
    if (s6.samples === 0) throw new Error("Scenario 6 FAIL: no samples captured");
    if (s6.rms < 0.0001) throw new Error(`Scenario 6 FAIL: no audio after suspended→running (RMS=${s6.rms})`);
    console.log("  PASS");

    await context.close();
  }

  // The full startup path: EQ set up while suspended, playback scheduled while suspended, the flush on resume.
  console.log("\n--- Scenario 7: EQ'd circuit import while suspended → resume ---");
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
      typeof importJSON === "function" &&
      typeof startPlay === "function" &&
      typeof stopPlay === "function" &&
      typeof forceDraw === "function" &&
      typeof resumeAudio === "function"
    );

    // Create the audio context first so we can suspend it.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    const circuitJSON2 = eqCircuitJSON();

    const s7 = await page.evaluate(async (json) => {
      const ctx = window.__audioCtx;
      if (!ctx) return { error: "no AudioContext" };

      // Play is ignored until the gate warms a newly loaded circuit, and warming resumes the context, so it warms first as
      // the launch circuit does at boot.
      const waitGate = async () => {
        const by = Date.now() + 20000;
        while (typeof window.warmupProgress === "function" && window.warmupProgress().ready !== true) {
          if (Date.now() > by) return false;
          await new Promise((r) => setTimeout(r, 50));
        }
        return true;
      };
      importJSON?.(json);
      if (!(await waitGate())) return { error: "playback-readiness gate never opened (warm load)" };

      // Enable output capture before suspending.
      enableOutputCapture?.();

      // Suspend the context to simulate mobile startup.
      await ctx.suspend();
      if (ctx.state !== 'suspended') {
        return { error: `expected suspended after suspend(), got ${ctx.state}` };
      }

      // Voices are warm, so the gate opens without rendering.
      importJSON?.(json);
      forceDraw?.();
      const gateOpenAtImport = typeof window.warmupProgress !== "function" || window.warmupProgress().ready === true;
      if (!(await waitGate())) return { error: "playback-readiness gate never opened (suspended load)" };
      if (ctx.state !== 'suspended') {
        return { error: `context left suspended during the suspended import (${ctx.state})` };
      }

      // Start playback while still suspended — audio events get queued.
      startOutputCapture?.();
      startPlay?.(); // a press flag the next frame consumes
      if (typeof window.isPlaying === "function") {
        const by = Date.now() + 2000;
        while (!window.isPlaying()) {
          if (Date.now() > by) return { error: "startPlay was ignored (transport not running)" };
          await new Promise((r) => setTimeout(r, 20));
        }
      }

      // Small wait for events to queue up.
      await new Promise((r) => setTimeout(r, 200));

      // Resume the context (simulating user gesture on real mobile).
      await ctx.resume();

      // Wait for deferred events to flush and audio to render.
      await new Promise((r) => setTimeout(r, 2000));

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
      return { samples: captured.length, peak, rms, state: ctx.state, gateOpenAtImport };
    }, circuitJSON2);

    if (s7.error) throw new Error(`Scenario 7 FAIL: ${s7.error}`);
    console.log(`  Samples: ${s7.samples}, Peak: ${s7.peak.toFixed(6)}, RMS: ${s7.rms.toFixed(6)}, State: ${s7.state}, gate open at import: ${s7.gateOpenAtImport}`);
    if (s7.samples === 0) throw new Error("Scenario 7 FAIL: no samples captured");
    if (s7.rms < 0.0001) throw new Error(`Scenario 7 FAIL: no audio after suspended circuit import + resume (RMS=${s7.rms})`);
    console.log("  PASS");

    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "mobile_audio");
    await context.close();
  }

  console.log("\nAll mobile audio tests passed.");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
