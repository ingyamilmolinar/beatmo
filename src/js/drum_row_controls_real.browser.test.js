/**
 * Drum Row Controls Real Input Test
 *
 * Tests that Mute/Solo buttons and row color picker work via real canvas clicks.
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 */

import {
  setupFullWasm,
  clickAndHold,
  rectCenter,
  assertValidRect,
} from "./real_input_test_helpers.js";

let cleanup;

try {
  console.log("drum_row_controls_real: Setting up full WASM environment...");
  const { page, cleanup: cleanupFn } = await setupFullWasm();
  cleanup = cleanupFn;

  // Ensure we have a default path
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

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
  await clickAndHold(page, muteCenter.x, muteCenter.y, 50);
  await page.waitForTimeout(100);

  const mutedAfter = await page.evaluate(() => rowMuted?.(0));
  console.log("drum_row_controls_real: Muted after:", mutedAfter);

  if (mutedAfter === mutedBefore) {
    throw new Error("Mute button did NOT toggle mute state.");
  }

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
  await clickAndHold(page, soloCenter.x, soloCenter.y, 50);
  await page.waitForTimeout(100);

  const soloedAfter = await page.evaluate(() => rowSoloed?.(0));
  console.log("drum_row_controls_real: Soloed after:", soloedAfter);

  if (soloedAfter === soloedBefore) {
    throw new Error("Solo button did NOT toggle solo state.");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Test 3: Color button opens color menu
  // ─────────────────────────────────────────────────────────────────────
  console.log("drum_row_controls_real: Testing Color button...");

  const colorRect = await page.evaluate(() => rowColorBtnRect?.(0));
  assertValidRect(colorRect, "rowColorBtnRect(0)");
  console.log("drum_row_controls_real: Color button rect:", colorRect);

  // Make sure color menu is closed first
  const colorMenuBefore = await page.evaluate(() => colorMenuOpenState?.());
  console.log("drum_row_controls_real: Color menu open before:", colorMenuBefore);

  const colorCenter = rectCenter(colorRect);
  await clickAndHold(page, colorCenter.x, colorCenter.y, 50);
  await page.waitForTimeout(100);

  const colorMenuAfter = await page.evaluate(() => colorMenuOpenState?.());
  console.log("drum_row_controls_real: Color menu open after:", colorMenuAfter);

  if (colorMenuAfter === colorMenuBefore) {
    throw new Error("Color button did NOT toggle color menu.");
  }

  console.log("drum_row_controls_real: PASS - All drum row controls work via real canvas clicks");
} catch (error) {
  console.error("drum_row_controls_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (cleanup) {
    await cleanup();
  }
}
