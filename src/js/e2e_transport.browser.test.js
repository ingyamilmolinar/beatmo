/**
 * Transport Controls Real Input Test
 *
 * Tests that Play/Stop buttons and BPM controls work via real canvas clicks.
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 *
 * The Play/Stop/BPM scenario bodies live in
 * src/js/scenarios/e2e_transport.js so the same assertions can run via
 * test_runner_overlay.js on a real phone (`make serve-lan`). The user-tap
 * reproduction scenario is exposed only via the in-page runner — it requires
 * a real finger and cannot be automated through Playwright in CI.
 */
import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

async function runInPage(name) {
  return await page.evaluate(async (scenarioName) => {
    const m = await import("/scenarios/e2e_transport.js");
    const fn = m["scenario" + scenarioName];
    if (typeof fn !== "function") throw new Error("missing scenario fn: " + scenarioName);
    return await fn();
  }, name);
}

try {
  console.log("transport_real: Setting up full WASM environment...");
  ({ page, cleanup } = await setupFullWasm());

  // Ensure default playback path exists so playBtn-driven scenarios actually fire audio events.
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // CI runs the synthetic-input scenarios. The user-tap reproduction
  // scenario is skipped here — it lives in scenarios/e2e_transport.js and
  // is invoked by test_runner_overlay.js on a real device.
  const cases = [
    ["PlayButtonSyntheticClick",  "Test 1: Play button"],
    ["StopButtonSyntheticClick",  "Test 2: Stop button"],
    ["BpmIncrementSynthetic",     "Test 3: BPM increment"],
  ];

  for (const [key, label] of cases) {
    console.log(`transport_real: ${label}`);
    const r = await runInPage(key);
    console.log(`  ${JSON.stringify(r)}`);
    if (!r.pass) throw new Error(`${label} FAIL: ${r.reason || "unknown"}`);
    console.log("  PASS");
  }

  console.log("transport_real: PASS - All transport controls work via real canvas clicks");
} catch (error) {
  console.error("transport_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "transport_real");
  if (cleanup) {
    await cleanup();
  }
}
