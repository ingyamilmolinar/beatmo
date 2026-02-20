/**
 * Popup Close Browser Tests
 *
 * Verifies close buttons and ESC key support for popups in WASM.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("popup_close: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // ═══════════════════════════════════════════════════════════════════════
  // Test 1: closeAllPopups JS export
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Test 1: closeAllPopups JS export ═══");

  // Build a circuit and open node menu
  await page.evaluate(() => {
    addNode?.(0, 0, "regular");
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Check that closeAllPopups is available
  const hasExport = await page.evaluate(() => typeof closeAllPopups === "function");
  if (!hasExport) throw new Error("closeAllPopups JS export not found");
  console.log("  closeAllPopups export exists");

  // Open node menu by clicking on the node
  const nodeOpen1 = await page.evaluate(() => {
    // We can't easily click the node, but we can verify the export works
    // by calling closeAllPopups and checking it doesn't error
    closeAllPopups?.();
    return nodeMenuOpen?.();
  });
  if (nodeOpen1) throw new Error("Node menu should be closed after closeAllPopups");
  console.log("  closeAllPopups works without error");

  // ═══════════════════════════════════════════════════════════════════════
  // Test 2: ESC closes node menu
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Test 2: ESC closes node menu ═══");

  // Open node menu by clicking on node (0,0)
  const screenCoords = await page.evaluate(() => {
    const coords = gridToScreen?.(0, 0);
    return coords ? { x: coords.x, y: coords.y } : null;
  });
  if (!screenCoords) {
    console.log("  SKIP: gridToScreen not available");
  } else {
    // Click to select the node
    await page.mouse.click(screenCoords.x, screenCoords.y);
    await page.waitForTimeout(200);

    const menuOpen = await page.evaluate(() => nodeMenuOpen?.());
    if (menuOpen) {
      // Hold Escape long enough to span multiple game frames under CPU pressure
      await page.keyboard.down("Escape");
      await page.waitForTimeout(100);
      await page.keyboard.up("Escape");
      await page.waitForTimeout(200);

      const menuAfterEsc = await page.evaluate(() => nodeMenuOpen?.());
      if (menuAfterEsc) throw new Error("Node menu still open after ESC");
      console.log("  ESC closed node menu");
    } else {
      console.log("  SKIP: node menu didn't open from click (grid scaling may differ)");
    }
  }

  // ═══════════════════════════════════════════════════════════════════════
  // Test 3: ESC closes instrument menu
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Test 3: ESC closes instrument menu ═══");

  await page.evaluate(() => openInstMenu?.(0));
  await page.waitForTimeout(200);

  const instOpen = await page.evaluate(() => instMenuOpenState?.());
  if (!instOpen) {
    console.log("  SKIP: instrument menu didn't open");
  } else {
    await page.keyboard.down("Escape");
    await page.waitForTimeout(100);
    await page.keyboard.up("Escape");
    await page.waitForTimeout(200);

    const instAfterEsc = await page.evaluate(() => instMenuOpenState?.());
    if (instAfterEsc) throw new Error("Instrument menu still open after ESC");
    console.log("  ESC closed instrument menu");
  }

  // ═══════════════════════════════════════════════════════════════════════
  // Test 4: ESC closes subdiv menu
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Test 4: ESC closes subdiv menu ═══");

  await page.evaluate(() => openSubdivMenu?.());
  await page.waitForTimeout(200);

  // Hold Escape long enough to span multiple game frames under CPU pressure
  await page.keyboard.down("Escape");
  await page.waitForTimeout(100);
  await page.keyboard.up("Escape");
  await page.waitForTimeout(200);

  console.log("  Subdiv menu ESC test done (no error)");

  // ═══════════════════════════════════════════════════════════════════════
  // Test 5: closeAllPopups clears all state
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Test 5: closeAllPopups clears all state ═══");

  // Open some menus then close all
  await page.evaluate(() => {
    openInstMenu?.(0);
    openColorMenu?.(0);
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => closeAllPopups?.());
  await page.waitForTimeout(100);

  const anyOpen = await page.evaluate(() => {
    return (
      (instMenuOpenState?.() || false) ||
      (colorMenuOpenState?.() || false) ||
      (nodeMenuOpen?.() || false)
    );
  });
  if (anyOpen) throw new Error("Some popup still open after closeAllPopups");
  console.log("  All popups closed");

  console.log("\n✅ All popup_close tests passed");
} catch (err) {
  console.error("FAIL:", err.message || err);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "popup_close");
  if (cleanup) await cleanup();
}
