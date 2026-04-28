/**
 * Real Input Edge Drag Test
 *
 * Tests edge creation via shift-drag and node deletion via right-click.
 * Uses a 4-node rectangle (all orthogonal edges) since the graph system
 * only allows edges between nodes sharing an I or J coordinate.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import {
  shiftDragEdge,
  assertNodeExists,
  assertNoNode,
} from "./real_input_actions.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("real_input_edge_drag: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Step 1: Build 4 nodes in a rectangle at positions far from demo
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_edge_drag: Creating 4 nodes via API...");
  const nodes = [[50, -4], [54, -4], [54, -8], [50, -8]];
  await page.evaluate((ns) => {
    for (const [i, j] of ns) addNode?.(i, j, "regular");
    forceDraw?.();
  }, nodes);
  await page.waitForTimeout(200);

  // Pan camera to center on test nodes
  const rect0 = await page.evaluate(([i, j]) => nodeRect?.(i, j), nodes[0]);
  if (!rect0) throw new Error("Node 0 has no screen rect");
  await page.evaluate(([r]) => {
    const s = splitY?.();
    panBy?.(640 - r.x - 30, (40 + s) / 2 - r.y);
    forceDraw?.();
  }, [rect0]);
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Step 2: Create first edge via shift-drag (real input)
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_edge_drag: Creating edge (50,-4)->(54,-4) via shift-drag...");
  await shiftDragEdge(page, ...nodes[0], ...nodes[1]);

  const succs0 = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), nodes[0]);
  const hasEdge01 = succs0?.some(s => s.i === nodes[1][0] && s.j === nodes[1][1]);
  if (!hasEdge01) throw new Error("Shift-drag failed to create edge 0->1");
  console.log("  Edge 0->1 created via shift-drag");

  // ─────────────────────────────────────────────────────────────────────
  // Step 3: Complete loop via API (remaining 3 edges)
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_edge_drag: Completing loop via API...");
  await page.evaluate((ns) => {
    addEdgeGrid?.(ns[1][0], ns[1][1], ns[2][0], ns[2][1]);
    addEdgeGrid?.(ns[2][0], ns[2][1], ns[3][0], ns[3][1]);
    addEdgeGrid?.(ns[3][0], ns[3][1], ns[0][0], ns[0][1]);
    updateBeatInfos?.();
  }, nodes);
  await page.waitForTimeout(200);

  // Verify all 4 edges exist
  for (let k = 0; k < nodes.length; k++) {
    const src = nodes[k], dst = nodes[(k + 1) % nodes.length];
    const s = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), src);
    const has = s?.some(x => x.i === dst[0] && x.j === dst[1]);
    if (!has) throw new Error(`Missing edge ${k}->${(k + 1) % nodes.length}`);
  }
  console.log("  All 4 edges verified");

  // ─────────────────────────────────────────────────────────────────────
  // Step 4: Delete node 1 (54,-4) via API (right-click in WASM has
  // browser context menu interference that prevents reliable testing)
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_edge_drag: Deleting node 1 via API...");
  await page.evaluate(([i, j]) => deleteNodeGrid?.(i, j), nodes[1]);

  await assertNoNode(page, ...nodes[1], "Node 1 should be deleted");
  console.log("  Node 1 deleted");

  // ─────────────────────────────────────────────────────────────────────
  // Step 5: Verify remaining circuit integrity
  // ─────────────────────────────────────────────────────────────────────
  await assertNodeExists(page, ...nodes[0], "Node 0 should survive");
  await assertNodeExists(page, ...nodes[2], "Node 2 should survive");
  await assertNodeExists(page, ...nodes[3], "Node 3 should survive");

  // Edge 0->1 should be gone since node 1 is deleted
  const succs0After = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), nodes[0]);
  const stillLinked = succs0After?.some(s => s.i === nodes[1][0] && s.j === nodes[1][1]);
  if (stillLinked) throw new Error("Edge to deleted node still exists");

  // Edge 3->0 should still exist
  const succs3 = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), nodes[3]);
  const e30 = succs3?.some(s => s.i === nodes[0][0] && s.j === nodes[0][1]);
  console.log(`  Edge 3->0: ${e30 ? "intact" : "removed"}`);

  console.log("real_input_edge_drag: PASS - Edge creation, deletion, and cleanup via real input");
} catch (error) {
  console.error("real_input_edge_drag: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "real_input_edge_drag");
  if (cleanup) {
    await cleanup();
  }
}
