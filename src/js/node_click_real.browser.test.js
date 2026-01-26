/**
 * Node Click Real Input Test
 *
 * Tests that grid node clicks work via real canvas events in WASM.
 *
 * BUG: Grid node clicks don't work in WASM because `fastPath` is enabled by
 * default, which SKIPS the grid input handler (handleEditor).
 *
 * Location: src/go/internal/ui/game_update.go:129-133
 *   if !fastPath && !g.blocksAt(mx, my) {
 *       g.handleEditor()   // THIS IS SKIPPED ON WASM
 *   } else {
 *       g.leftPrev = left  // Only updates state tracking, no click processing
 *   }
 *
 * This test demonstrates the bug by:
 * 1. Showing that fastPath=true (default on WASM) causes node clicks to fail
 * 2. Showing that fastPath=false makes node clicks work
 */

import {
  setupFullWasm,
  clickAndHold,
  rectCenter,
  assertValidRect,
} from "./real_input_test_helpers.js";

let cleanup;

try {
  console.log("node_click_real: Setting up full WASM environment...");
  const { page, cleanup: cleanupFn } = await setupFullWasm();
  cleanup = cleanupFn;

  // Ensure we have a default path with nodes to click
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  // ─────────────────────────────────────────────────────────────────────
  // Log initial state
  // ─────────────────────────────────────────────────────────────────────
  const initialState = await page.evaluate(() => debugGridInputState?.());
  console.log("node_click_real: Initial state:");
  console.log("  - fastPath:", initialState?.fastPath);
  console.log("  - nodeMenuOpen:", initialState?.nodeMenuOpen);
  console.log("  - pendingClick:", initialState?.pendingClick);
  console.log("  - clickNodeId:", initialState?.clickNodeId);

  // Verify fastPath is enabled (this is the bug condition)
  if (!initialState?.fastPath) {
    console.log("node_click_real: WARNING - fastPath is unexpectedly false.");
    console.log("node_click_real: This test expects fastPath=true to demonstrate the bug.");
  }

  // Get the node rect for node at (0, 0)
  const nodeRect = await page.evaluate(() => nodeRect?.(0, 0));
  if (!nodeRect) {
    throw new Error("No node found at (0, 0) - ensureDefaultPath may have failed");
  }
  assertValidRect(nodeRect, "nodeRect(0,0)");
  console.log("node_click_real: Node rect at (0,0):", nodeRect);

  const center = rectCenter(nodeRect);
  console.log("node_click_real: Node center:", center);

  // Close any open menu first
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(100);

  // ─────────────────────────────────────────────────────────────────────
  // Test 1: Verify bug exists (fastPath=true, click fails)
  // ─────────────────────────────────────────────────────────────────────
  console.log("\n--- Test 1: Click with fastPath=true (default WASM) ---");

  // Ensure fastPath is enabled (should already be true on WASM)
  await page.evaluate(() => setFastPath?.(true));
  await page.waitForTimeout(50);

  const fastPathBefore = await page.evaluate(() => getFastPath?.());
  console.log("node_click_real: fastPath before click:", fastPathBefore);

  // Click on the node
  console.log("node_click_real: Clicking at", center.x, center.y);
  await clickAndHold(page, center.x, center.y, 100);

  // Wait for WASM to process the click
  await page.waitForTimeout(200);

  // Check state after click
  const stateAfterClick1 = await page.evaluate(() => debugGridInputState?.());
  console.log("node_click_real: Post-click state (fastPath=true):");
  console.log("  - fastPath:", stateAfterClick1?.fastPath);
  console.log("  - nodeMenuOpen:", stateAfterClick1?.nodeMenuOpen);
  console.log("  - pendingClick:", stateAfterClick1?.pendingClick);
  console.log("  - clickNodeId:", stateAfterClick1?.clickNodeId);
  console.log("  - leftPrev:", stateAfterClick1?.leftPrev);
  console.log("  - selectedNodeId:", stateAfterClick1?.selectedNodeId);


  // ─────────────────────────────────────────────────────────────────────
  // Test 2: Verify fix works (fastPath=false, click works)
  // ─────────────────────────────────────────────────────────────────────
  console.log("\n--- Test 2: Click with fastPath=false ---");

  // Close any menu from previous test
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(100);

  // Disable fastPath
  await page.evaluate(() => setFastPath?.(false));
  await page.waitForTimeout(50);

  const fastPathAfterDisable = await page.evaluate(() => getFastPath?.());
  console.log("node_click_real: fastPath after disable:", fastPathAfterDisable);

  // Click on the node again
  console.log("node_click_real: Clicking at", center.x, center.y);
  await clickAndHold(page, center.x, center.y, 100);

  // Wait for WASM to process the click
  await page.waitForTimeout(200);

  // Check state after click
  const stateAfterClick2 = await page.evaluate(() => debugGridInputState?.());
  console.log("node_click_real: Post-click state (fastPath=false):");
  console.log("  - fastPath:", stateAfterClick2?.fastPath);
  console.log("  - nodeMenuOpen:", stateAfterClick2?.nodeMenuOpen);
  console.log("  - pendingClick:", stateAfterClick2?.pendingClick);
  console.log("  - clickNodeId:", stateAfterClick2?.clickNodeId);
  console.log("  - leftPrev:", stateAfterClick2?.leftPrev);
  console.log("  - selectedNodeId:", stateAfterClick2?.selectedNodeId);

  // Verify results
  const bugExists = !stateAfterClick1?.nodeMenuOpen;
  const fixWorks = stateAfterClick2?.nodeMenuOpen === true;

  if (bugExists) {
    throw new Error(
      `Grid node click failed with fastPath=true: nodeMenuOpen=${stateAfterClick1?.nodeMenuOpen}`
    );
  }

  if (!fixWorks) {
    throw new Error(
      `Grid node click failed with fastPath=false: nodeMenuOpen=${stateAfterClick2?.nodeMenuOpen}`
    );
  }

  console.log("node_click_real: PASS - Grid node clicks work correctly");
} catch (error) {
  console.error("node_click_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (cleanup) {
    await cleanup();
  }
}
