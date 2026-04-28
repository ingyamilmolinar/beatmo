/**
 * Node Click Real Input Test
 *
 * Tests that grid node clicks work via real canvas events in WASM.
 *
 * The game's click state machine in handleEditor() (game_input_editor.go)
 * requires two separate game-loop ticks:
 *   Tick N   (mouse down): left=true, leftPrev=false  → pendingClick=true
 *   Tick N+1 (mouse up):   left=false, leftPrev=true  → nodeMenuOpen=true
 *
 * Under CPU contention (parallel browser tests), requestAnimationFrame ticks
 * can be delayed beyond short hold times, so we use generous hold durations
 * and poll for the expected state change with retries.
 *
 * IMPORTANT: The target node must be well above the splitter's grab zone,
 * otherwise the splitter captures the mouse-down event. After importing
 * a demo, pan the camera so the target node sits near the grid pane center.
 */

import {
  setupFullWasm,
  rectCenter,
  assertValidRect,
} from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

/**
 * Pan the camera so the node at grid (i, j) is centered vertically in the
 * grid pane (between gridTopOffset=40 and splitY). Re-draws and returns
 * the updated node rect.
 */
async function centerNodeInGridPane(page, i, j) {
  const splitYVal = await page.evaluate(() => splitY?.()) ?? 360;
  const gridTop = 40; // desktopTopOffset constant
  const targetY = Math.round((gridTop + splitYVal) / 2);

  const nr = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
  if (!nr) throw new Error(`No node at (${i}, ${j})`);
  const currentCenter = nr.y + nr.h / 2;
  const dy = currentCenter - targetY;

  if (Math.abs(dy) > 5) {
    await page.evaluate(([pdx, pdy]) => {
      panBy?.(pdx, pdy);
      forceDraw?.();
    }, [0, -dy]);
    await page.waitForTimeout(200);
  }

  const updated = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
  if (!updated) throw new Error(`Node at (${i}, ${j}) not visible after pan`);
  return updated;
}

/**
 * Click a node and wait for the node menu to open.
 * Uses hold-and-poll: holds mouse down until the game loop registers
 * pendingClick, then releases and waits for nodeMenuOpen. Retries the
 * full cycle up to maxRetries times.
 */
async function clickNodeAndWaitForMenu(page, x, y, maxRetries = 3) {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    // Close any existing menu before each attempt
    await page.evaluate(() => closeNodeMenu?.());
    await page.waitForTimeout(100);

    // Hold mouse down with generous duration (≥200ms recommended)
    await page.mouse.move(x, y);
    await page.mouse.down();

    // Poll for pendingClick (set on the tick that sees mousedown)
    let sawPending = false;
    for (let i = 0; i < 20; i++) {
      await page.waitForTimeout(50);
      const st = await page.evaluate(() => debugGridInputState?.());
      if (st?.pendingClick) {
        sawPending = true;
        break;
      }
      // If splitter captured instead, abort this attempt
      if (st?.splitDragging) {
        console.log(`  [attempt ${attempt + 1}] splitter captured the click, aborting`);
        break;
      }
    }

    // Release
    await page.mouse.up();

    if (!sawPending) {
      await page.waitForTimeout(200);
      continue;
    }

    // Poll for nodeMenuOpen (set on the tick after mouse-up)
    try {
      await page.waitForFunction(
        () => debugGridInputState?.()?.nodeMenuOpen === true,
        { timeout: 2000 },
      );
      return; // Success
    } catch {
      console.log(`  [attempt ${attempt + 1}] nodeMenuOpen did not become true`);
    }
  }
  throw new Error(
    `Node menu did not open after ${maxRetries} click attempts at (${x}, ${y})`,
  );
}

let cleanup;
let page;

try {
  console.log("node_click_real: Setting up full WASM environment...");
  ({ page, cleanup } = await setupFullWasm());

  // Ensure demo circuit with nodes
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  // Pan the target node to the center of the grid pane so it's well
  // above the splitter's grab zone.
  const nodeR = await centerNodeInGridPane(page, 0, 0);
  assertValidRect(nodeR, "nodeRect(0,0)");
  const center = rectCenter(nodeR);
  console.log(`node_click_real: Node at (0,0) screen rect: ${JSON.stringify(nodeR)}, center: (${center.x}, ${center.y})`);

  const sy = await page.evaluate(() => splitY?.());
  console.log(`node_click_real: splitY=${sy}, node center y=${center.y}`);
  if (center.y >= (sy - 10)) {
    throw new Error(`Node center (y=${center.y}) too close to splitter (y=${sy})`);
  }

  // ─────────────────────────────────────────────────────────────────────
  // Test 1: Click with fastPath=true (default WASM)
  // ─────────────────────────────────────────────────────────────────────
  console.log("\n--- Test 1: Click with fastPath=true ---");
  await page.evaluate(() => setFastPath?.(true));

  await clickNodeAndWaitForMenu(page, center.x, center.y);

  const st1 = await page.evaluate(() => debugGridInputState?.());
  if (!st1?.nodeMenuOpen) {
    throw new Error(`Test 1 failed: nodeMenuOpen=${st1?.nodeMenuOpen}`);
  }
  console.log("node_click_real: Test 1 PASS (fastPath=true, nodeMenuOpen=true)");

  // ─────────────────────────────────────────────────────────────────────
  // Test 2: Click with fastPath=false
  // ─────────────────────────────────────────────────────────────────────
  console.log("\n--- Test 2: Click with fastPath=false ---");
  await page.evaluate(() => {
    closeNodeMenu?.();
    setFastPath?.(false);
  });
  await page.waitForTimeout(100);

  await clickNodeAndWaitForMenu(page, center.x, center.y);

  const st2 = await page.evaluate(() => debugGridInputState?.());
  if (!st2?.nodeMenuOpen) {
    throw new Error(`Test 2 failed: nodeMenuOpen=${st2?.nodeMenuOpen}`);
  }
  console.log("node_click_real: Test 2 PASS (fastPath=false, nodeMenuOpen=true)");

  console.log("\nnode_click_real: PASS - Grid node clicks work correctly");
} catch (error) {
  console.error("node_click_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "node_click_real");
  if (cleanup) await cleanup();
}
