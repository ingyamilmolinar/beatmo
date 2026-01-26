/**
 * Camera Real Input Test
 *
 * Tests that camera zoom (wheel) works via real canvas events.
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 */

import {
  setupFullWasm,
  wheelAt,
} from "./real_input_test_helpers.js";

let cleanup;

try {
  console.log("camera_real: Setting up full WASM environment...");
  const { page, cleanup: cleanupFn } = await setupFullWasm();
  cleanup = cleanupFn;

  // Ensure we have a default path
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Get the grid pane area (above the splitter)
  const splitY = await page.evaluate(() => splitY?.());
  const gridCenterX = 400;
  const gridCenterY = Math.min(splitY ? splitY / 2 : 150, 150);

  console.log("camera_real: Grid center for tests:", { gridCenterX, gridCenterY, splitY });

  // ─────────────────────────────────────────────────────────────────────
  // Test: Camera zoom (wheel)
  // ─────────────────────────────────────────────────────────────────────
  console.log("camera_real: Testing camera zoom...");

  const scaleBefore = await page.evaluate(() => camScale?.());
  console.log("camera_real: Camera scale before:", scaleBefore);

  // Scroll to zoom in (negative deltaY = scroll up = zoom in)
  for (let i = 0; i < 5; i++) {
    await wheelAt(page, gridCenterX, gridCenterY, -120);
    await page.waitForTimeout(50);
  }

  const scaleAfterZoomIn = await page.evaluate(() => camScale?.());
  console.log("camera_real: Camera scale after zoom in:", scaleAfterZoomIn);

  if (scaleAfterZoomIn <= scaleBefore) {
    throw new Error(
      `Camera zoom in did NOT increase scale.\n` +
        `Scale before: ${scaleBefore}\n` +
        `Scale after: ${scaleAfterZoomIn}`
    );
  }

  // Scroll to zoom out (positive deltaY = scroll down = zoom out)
  for (let i = 0; i < 10; i++) {
    await wheelAt(page, gridCenterX, gridCenterY, 120);
    await page.waitForTimeout(50);
  }

  const scaleAfterZoomOut = await page.evaluate(() => camScale?.());
  console.log("camera_real: Camera scale after zoom out:", scaleAfterZoomOut);

  if (scaleAfterZoomOut >= scaleAfterZoomIn) {
    throw new Error(
      `Camera zoom out did NOT decrease scale.\n` +
        `Scale before zoom out: ${scaleAfterZoomIn}\n` +
        `Scale after zoom out: ${scaleAfterZoomOut}`
    );
  }

  console.log("camera_real: PASS - Camera zoom works via real canvas events");
} catch (error) {
  console.error("camera_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (cleanup) {
    await cleanup();
  }
}
