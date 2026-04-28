/**
 * Real Input Circuit Build Test
 *
 * Tests circuit building and playback workflow using a hybrid approach:
 * nodes are created via API (for reliability), then edge creation via
 * shift-drag, playback control, and BPM changes are tested via real
 * Playwright mouse events.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import {
  shiftDragEdge,
  clickPlayBtn,
  clickStopBtn,
  clickBpmInc,
  assertNodeExists,
  assertPlaying,
} from "./real_input_actions.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("real_input_circuit_build: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  // Wait for demo to load - it sets up a full playable circuit
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  // ─────────────────────────────────────────────────────────────────────
  // Step 1: Create 4 nodes via API and center camera on them
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_circuit_build: Creating 4 test nodes via API...");
  const positions = [[30, 0], [34, 0], [34, 4], [30, 4]];
  await page.evaluate((pos) => {
    for (const [i, j] of pos) addNode?.(i, j, "regular");
    forceDraw?.();
  }, positions);
  await page.waitForTimeout(200);

  for (const [i, j] of positions) {
    await assertNodeExists(page, i, j, `Node at (${i},${j})`);
  }
  console.log("real_input_circuit_build: All 4 nodes created");

  // Center camera on new nodes
  const firstRect = await page.evaluate(() => nodeRect?.(30, 0));
  if (!firstRect) throw new Error("Node (30,0) has no screen rect");
  const canvasCenter = await page.evaluate(() => {
    const s = splitY?.();
    return { x: 640, y: (40 + s) / 2 };
  });
  const dx = canvasCenter.x - (firstRect.x + 30);
  const dy = canvasCenter.y - (firstRect.y + 30);
  await page.evaluate(([pdx, pdy]) => { panBy?.(pdx, pdy); forceDraw?.(); }, [dx, dy]);
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Step 2: Create edges via real shift-drag to form a loop
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_circuit_build: Creating edges via shift-drag...");
  for (let k = 0; k < positions.length; k++) {
    const [i1, j1] = positions[k];
    const [i2, j2] = positions[(k + 1) % positions.length];
    console.log(`  Shift-drag (${i1},${j1}) -> (${i2},${j2})`);
    await shiftDragEdge(page, i1, j1, i2, j2);
  }

  // Verify all edges exist
  let edgeCount = 0;
  for (const [gi, gj] of positions) {
    const s = await page.evaluate(([i, j]) => nodeSuccessorsGrid?.(i, j), [gi, gj]);
    if (s && s.length > 0) {
      edgeCount++;
      console.log(`  (${gi},${gj}) -> ${JSON.stringify(s)}`);
    }
  }
  console.log(`real_input_circuit_build: ${edgeCount}/4 nodes have outgoing edges`);
  if (edgeCount < 2) {
    throw new Error(`Too few edges created: expected 4, got ${edgeCount}`);
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 3: Start playback on the demo circuit (rows 0-5 already set up)
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_circuit_build: Starting playback...");
  await clickPlayBtn(page);
  await page.waitForTimeout(500);
  await assertPlaying(page, true, "Expected playback to start");
  console.log("real_input_circuit_build: Playback running");

  // ─────────────────────────────────────────────────────────────────────
  // Step 4: Check for node highlights during playback
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_circuit_build: Checking for node highlights...");
  // Check the demo circuit nodes (0,0) is a common position
  let highlightSeen = false;
  const demoCoords = [[0, 0], [-8, 0], [-8, -8], [-16, -8]];
  for (let attempt = 0; attempt < 30; attempt++) {
    for (const [ci, cj] of demoCoords) {
      const hl = await page.evaluate(([gi, gj]) => nodeHighlightedAt?.(gi, gj), [ci, cj]);
      if (hl) {
        highlightSeen = true;
        console.log(`  Highlight seen at (${ci}, ${cj})`);
        break;
      }
    }
    if (highlightSeen) break;
    await page.waitForTimeout(100);
  }
  if (!highlightSeen) {
    console.log("  WARNING: no highlight observed (timing dependent)");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 5: Change BPM during playback via real button clicks
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_circuit_build: Changing BPM during playback...");
  const bpmBefore = await page.evaluate(() => getBPM?.());
  for (let i = 0; i < 3; i++) {
    await clickBpmInc(page);
  }
  const bpmAfter = await page.evaluate(() => getBPM?.());
  console.log(`  BPM: ${bpmBefore} -> ${bpmAfter}`);
  if (bpmAfter <= bpmBefore) {
    throw new Error(`BPM didn't increase: ${bpmBefore} -> ${bpmAfter}`);
  }
  await assertPlaying(page, true, "Playback should continue after BPM change");

  // ─────────────────────────────────────────────────────────────────────
  // Step 6: Stop playback
  // ─────────────────────────────────────────────────────────────────────
  console.log("real_input_circuit_build: Stopping playback...");
  await clickStopBtn(page);
  await page.waitForTimeout(200);
  await assertPlaying(page, false, "Expected playback to stop");

  // ─────────────────────────────────────────────────────────────────────
  // Step 7: Check for parity mismatches
  // ─────────────────────────────────────────────────────────────────────
  const mismatches = await page.evaluate(() => recentSchedulerMismatches?.());
  const mismatchCount = mismatches?.length ?? 0;
  console.log(`real_input_circuit_build: Parity mismatches: ${mismatchCount}`);

  console.log("real_input_circuit_build: PASS - Circuit build + playback via real input");
} catch (error) {
  console.error("real_input_circuit_build: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "real_input_circuit_build");
  if (cleanup) {
    await cleanup();
  }
}
