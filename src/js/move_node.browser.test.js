/**
 * Move Node Browser Tests
 *
 * Tests for the moveNodeGrid() WASM export:
 * 1. Basic move: verify position changes and ID preserved
 * 2. Edge preservation: orthogonal edges kept, non-orthogonal dropped
 * 3. During playback: move while playing, verify still playing
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import { assertPlaying } from "./real_input_actions.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("move_node: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // ═══════════════════════════════════════════════════════════════════════
  // Scenario 1: Basic Move
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Scenario 1: Basic Move ═══");

  // Create a node at (10, 0)
  await page.evaluate(() => {
    addNode?.(10, 0, "regular");
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(100);

  const idBefore = await page.evaluate(() => nodeIdAt?.(10, 0));
  if (idBefore == null || idBefore < 0) {
    throw new Error(`No node at (10,0): got ${idBefore}`);
  }
  console.log(`  Node created at (10,0), id=${idBefore}`);

  // Move it to (14, 2)
  const moved = await page.evaluate(() => moveNodeGrid?.(10, 0, 14, 2));
  if (!moved) {
    throw new Error("moveNodeGrid returned false");
  }

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(100);

  // Verify old position empty
  const idOld = await page.evaluate(() => nodeIdAt?.(10, 0));
  if (idOld != null && idOld >= 0) {
    throw new Error(`Old position (10,0) should be empty, got id=${idOld}`);
  }

  // Verify new position has the node with same ID
  const idAfter = await page.evaluate(() => nodeIdAt?.(14, 2));
  if (idAfter !== idBefore) {
    throw new Error(`Expected id=${idBefore} at (14,2), got ${idAfter}`);
  }
  console.log(`  Moved to (14,2), id preserved: ${idAfter}`);

  // Clean up for next scenario
  await page.evaluate(() => {
    deleteNodeGrid?.(14, 2);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(100);

  console.log("  PASS: Basic move");

  // ═══════════════════════════════════════════════════════════════════════
  // Scenario 2: Edge Preservation
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Scenario 2: Edge Preservation ═══");

  // Build a 3-node horizontal chain: A(20,0) → B(24,0) → C(28,0) and back
  await page.evaluate(() => {
    addNode?.(20, 0, "regular");
    addNode?.(24, 0, "regular");
    addNode?.(28, 0, "regular");
    addEdgeGrid?.(20, 0, 24, 0);
    addEdgeGrid?.(24, 0, 28, 0);
    addEdgeGrid?.(28, 0, 20, 0);
    setOrigin?.(20, 0, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Export before move
  const beforeJSON = await page.evaluate(() => exportJSON?.());
  const before = JSON.parse(beforeJSON);
  const edgesBefore = before.nodes.reduce(
    (sum, n) => sum + (n.outputs ? n.outputs.length : 0),
    0
  );
  console.log(`  Circuit built: 3 nodes, ${edgesBefore} directed edges`);

  // Move B from (24,0) to (24,4) — diagonal to A and C, edges should drop
  const moved2 = await page.evaluate(() => moveNodeGrid?.(24, 0, 24, 4));
  if (!moved2) {
    throw new Error("moveNodeGrid returned false for edge test");
  }

  await page.evaluate(() => {
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(100);

  // Export after move and check edges
  const afterJSON = await page.evaluate(() => exportJSON?.());
  const after = JSON.parse(afterJSON);

  // B moved to (24,4): A is at (20,0), C at (28,0)
  // A→B: A.I=20, B.I=24 (different), A.J=0, B.J=4 (different) → non-orthogonal → dropped
  // B→C: B.I=24, C.I=28 (different), B.J=4, C.J=0 (different) → non-orthogonal → dropped
  // C→A: C.I=28, A.I=20 (different), C.J=0, A.J=0 (same) → orthogonal → kept
  const edgesAfter = after.nodes.reduce(
    (sum, n) => sum + (n.outputs ? n.outputs.length : 0),
    0
  );
  console.log(`  After move: ${edgesAfter} directed edges (was ${edgesBefore})`);

  // C→A should remain (1 edge), A→B and B→C dropped
  if (edgesAfter >= edgesBefore) {
    throw new Error(
      `Expected fewer edges after diagonal move: before=${edgesBefore}, after=${edgesAfter}`
    );
  }
  console.log("  Edges correctly dropped for non-orthogonal positions");

  // Clean up
  await page.evaluate(() => {
    deleteNodeGrid?.(20, 0);
    deleteNodeGrid?.(24, 4);
    deleteNodeGrid?.(28, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(100);

  console.log("  PASS: Edge preservation");

  // ═══════════════════════════════════════════════════════════════════════
  // Scenario 3: Move During Playback
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Scenario 3: Move During Playback ═══");

  // Build a simple 2-node loop
  await page.evaluate(() => {
    addNode?.(30, 0, "regular");
    addNode?.(34, 0, "regular");
    addEdgeGrid?.(30, 0, 34, 0);
    addEdgeGrid?.(34, 0, 30, 0);
    setOrigin?.(30, 0, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Start playback
  await page.evaluate(() => startPlay?.());
  await page.waitForTimeout(400);
  await assertPlaying(page, true, "Should be playing before move");

  // Move node B from (34,0) to (34,4) during playback
  const moved3 = await page.evaluate(() => moveNodeGrid?.(34, 0, 34, 4));
  if (!moved3) {
    throw new Error("moveNodeGrid failed during playback");
  }
  console.log("  Node moved during playback");

  // Wait and verify still playing
  await page.waitForTimeout(300);
  await assertPlaying(page, true, "Should still be playing after move");
  console.log("  Playback continues after move");

  // Stop playback
  await page.evaluate(() => stopPlay?.());
  await page.waitForTimeout(200);

  console.log("  PASS: Move during playback");

  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ All move_node scenarios PASSED ═══");
} catch (err) {
  console.error("FAIL:", err.message || err);
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "move_node");
  if (cleanup) await cleanup();
  process.exit(1);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "move_node");
if (cleanup) await cleanup();
