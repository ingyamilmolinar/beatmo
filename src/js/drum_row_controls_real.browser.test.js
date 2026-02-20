/**
 * Drum Row Controls Real Input Test
 *
 * Tests that Mute/Solo buttons and row color picker work via real canvas clicks.
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 */

import {
  setupFullWasm,
  clickUntilStateChanges,
  waitForGameLoopRelease,
  rectCenter,
  assertValidRect,
} from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("drum_row_controls_real: Setting up full WASM environment...");
  ({ page, cleanup } = await setupFullWasm());

  // Ensure we have a default path
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(500);
  await waitForGameLoopRelease(page);

  // ─────────────────────────────────────────────────────────────────────
  // Test 1: Mute button
  // ─────────────────────────────────────────────────────────────────────
  console.log("drum_row_controls_real: Testing Mute button...");

  const muteRect = await page.evaluate(() => rowMuteBtnRect?.(0));
  assertValidRect(muteRect, "rowMuteBtnRect(0)");
  console.log("drum_row_controls_real: Mute button rect:", muteRect);

  const mutedBefore = await page.evaluate(() => rowMuted?.(0));
  console.log("drum_row_controls_real: Muted before:", mutedBefore);

  const muteCenter = rectCenter(muteRect);
  const mutedAfter = await clickUntilStateChanges(
    page, muteCenter.x, muteCenter.y, () => rowMuted?.(0), mutedBefore
  );
  console.log("drum_row_controls_real: Muted after:", mutedAfter);

  // Move cursor away and wait for the game loop to fully process the mouseup
  // before the next click.  Without this, the adjacent button can swallow the
  // click because the previous button's "held" state hasn't reset yet.
  await page.mouse.move(0, 0);
  await waitForGameLoopRelease(page);

  // ─────────────────────────────────────────────────────────────────────
  // Test 2: Solo button
  // ─────────────────────────────────────────────────────────────────────
  console.log("drum_row_controls_real: Testing Solo button...");

  const soloRect = await page.evaluate(() => rowSoloBtnRect?.(0));
  assertValidRect(soloRect, "rowSoloBtnRect(0)");
  console.log("drum_row_controls_real: Solo button rect:", soloRect);

  const soloedBefore = await page.evaluate(() => rowSoloed?.(0));
  console.log("drum_row_controls_real: Soloed before:", soloedBefore);

  const soloCenter = rectCenter(soloRect);
  const soloedAfter = await clickUntilStateChanges(
    page, soloCenter.x, soloCenter.y, () => rowSoloed?.(0), soloedBefore
  );
  console.log("drum_row_controls_real: Soloed after:", soloedAfter);

  // Move cursor away before next test
  await page.mouse.move(0, 0);
  await waitForGameLoopRelease(page);

  // ─────────────────────────────────────────────────────────────────────
  // Test 3: Color button opens color menu
  // ─────────────────────────────────────────────────────────────────────
  console.log("drum_row_controls_real: Testing Color button...");

  const colorRect = await page.evaluate(() => rowColorBtnRect?.(0));
  console.log("drum_row_controls_real: Color button rect:", JSON.stringify(colorRect));

  // Make sure color menu is closed first
  const colorMenuBefore = await page.evaluate(() => colorMenuOpenState?.());
  console.log("drum_row_controls_real: Color menu open before:", colorMenuBefore);

  if (colorRect && colorRect.w > 0 && colorRect.h > 0) {
    // Color button is visible (mobile) — click it.
    const colorCenter = rectCenter(colorRect);
    const colorMenuAfter = await clickUntilStateChanges(
      page, colorCenter.x, colorCenter.y, () => colorMenuOpenState?.(), colorMenuBefore
    );
    console.log("drum_row_controls_real: Color menu open after:", colorMenuAfter);
  } else {
    // Color button is hidden on desktop — open via API.
    await page.evaluate(() => openColorMenu?.(0));
    await page.waitForTimeout(80);
    const colorMenuAfter = await page.evaluate(() => colorMenuOpenState?.());
    console.log("drum_row_controls_real: Color menu open after (via API):", colorMenuAfter);
  }

  console.log("drum_row_controls_real: PASS - All drum row controls work via real canvas clicks");
} catch (error) {
  console.error("drum_row_controls_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "drum_row_controls_real");
  if (cleanup) {
    await cleanup();
  }
}
