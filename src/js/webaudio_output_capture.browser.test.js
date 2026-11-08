// webaudio_output_capture.browser.test.js
//
// Tests the output capture API for recording audio pipeline output.
//
// Scenario bodies live in src/js/scenarios/webaudio_output_capture.js so the
// same assertions can also run inside test_runner.html on a real phone via
// the LAN serve mode (`make serve-lan`). This file is the Playwright shell
// that drives them in CI.
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/webaudio_output_capture.browser.test.js

import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled, waitForAudioReady } from "./browser_test_helpers.js";

let exitCode = 0;
let cleanup = null;
try {
  const env = await setupFullWasm({ headless: true });
  cleanup = env.cleanup;
  const { page } = env;

  await page.waitForFunction(() =>
    typeof playSound === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof getOutputCapture === "function" &&
    typeof getOutputCaptureStats === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof resetOutputCaptureNode === "function"
  );

  // A capture enabled before the audio-thread tap's module loads must still be
  // on the audio thread (the ScriptProcessor fallback reorders blocks when the
  // main thread is late). audio.js alone on a page: no AudioContext exists yet.
  const fresh = await env.browser.newPage();
  await fresh.goto(`http://localhost:${env.port}/audio.js`);
  const first = await fresh.evaluate(async () => {
    await import("/audio.js");
    const hadContext = !!window.__audioCtx;
    await window.enableOutputCapture();
    return { hadContext, stats: window.getOutputCaptureStats() };
  });
  await fresh.close();
  console.log(`--- Capture enabled before the tap module loads ---\n  ${JSON.stringify(first)}`);
  if (first.hadContext) throw new Error("precondition: an AudioContext existed before the first enableOutputCapture");
  if (!first.stats.hasNode || !first.stats.audioThread) {
    throw new Error("an early enableOutputCapture fell back to the main-thread ScriptProcessor");
  }
  console.log("  PASS");

  // Unlock audio context (synthetic gesture is enough for headless Chromium).
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await waitForAudioReady(page);

  // Dynamic-import the scenario module (the dev server already serves any
  // /src/js/scenarios/*.js file as application/javascript). One round trip
  // per scenario keeps each in its own evaluate so failures are isolated.
  async function runScenario(name) {
    return await page.evaluate(async (scenarioName) => {
      const m = await import("/scenarios/webaudio_output_capture.js");
      const fn = m["scenario" + scenarioName];
      if (typeof fn !== "function") throw new Error("missing scenario fn: " + scenarioName);
      return await fn();
    }, name);
  }

  const cases = [
    ["CaptureRoundtrip",  "Scenario 1: Capture round-trip"],
    ["MidStreamGrowth",   "Scenario 2: Mid-stream capture growth"],
    ["CaptureStats",      "Scenario 3: Capture stats"],
    ["ClearBuffer",       "Scenario 4: Clear capture buffer"],
    ["ResetNode",         "Scenario 5: Reset capture node"],
  ];

  for (const [key, label] of cases) {
    console.log(`--- ${label} ---`);
    const r = await runScenario(key);
    console.log(`  ${JSON.stringify(r)}`);
    if (!r.pass) throw new Error(`${label} FAIL: ${r.reason || "unknown"}`);
    console.log("  PASS");
  }

  console.log("\nAll output capture tests passed.");
  if (isCoverageEnabled()) {
    await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_output_capture");
  }
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (cleanup) {
    try { await cleanup(); } catch (e) { console.warn("cleanup error:", e && e.message ? e.message : e); }
  }
}

process.exit(exitCode);
