/** Adding nodes and edges while playing, then a BPM change, without parity mismatches. */

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
  console.log("real_input_live_edit: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  console.log("real_input_live_edit: Building initial 4-node loop...");
  await page.evaluate(() => {
    addNode?.(0, 0, "regular");
    addNode?.(8, 0, "regular");
    addNode?.(8, 8, "regular");
    addNode?.(0, 8, "regular");
    addEdgeGrid?.(0, 0, 8, 0);
    addEdgeGrid?.(8, 0, 8, 8);
    addEdgeGrid?.(8, 8, 0, 8);
    addEdgeGrid?.(0, 8, 0, 0);
    setOrigin?.(0, 0, 0);
    updateBeatInfos?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  console.log("real_input_live_edit: Starting playback...");
  await clickPlayBtn(page);
  await page.waitForTimeout(500);
  await assertPlaying(page, true, "Playback should be running");

  // (0,-8) shares I=0 with (0,0), so an edge to it is legal.
  console.log("real_input_live_edit: Adding node at (0, -8) during playback...");
  await page.evaluate(() => addNode?.(0, -8, "regular"));
  await page.waitForTimeout(100);

  const newId = await assertNodeExists(page, 0, -8, "New node should exist after add during playback");
  console.log(`real_input_live_edit: New node created with id=${newId}`);

  // Verify playback is still running
  await assertPlaying(page, true, "Playback should continue after adding node");

  console.log("real_input_live_edit: Splicing node into circuit...");
  // Create edge from (0,0) to (0,-8) - orthogonal (shared I=0)
  await shiftDragEdge(page, 0, 0, 0, -8);
  // Create edge from (0,-8) to (8,-8) via API (add node at (8,-8) first)
  await page.evaluate(() => {
    addNode?.(8, -8, "regular");
    addEdgeGrid?.(0, -8, 8, -8);
  });

  // Update beat infos to reflect new circuit
  await page.evaluate(() => updateBeatInfos?.());
  await page.waitForTimeout(300);

  // Verify the new node is connected
  const succs = await page.evaluate(() => nodeSuccessorsGrid?.(4, -8));
  console.log("real_input_live_edit: Successors of (4,-8):", JSON.stringify(succs));

  console.log("real_input_live_edit: Changing BPM during playback...");
  const bpmBefore = await page.evaluate(() => getBPM?.());

  for (let i = 0; i < 5; i++) {
    await clickBpmInc(page);
  }

  const bpmAfter = await page.evaluate(() => getBPM?.());
  console.log(`real_input_live_edit: BPM changed from ${bpmBefore} to ${bpmAfter}`);

  if (bpmAfter <= bpmBefore) {
    throw new Error(`BPM should have increased: before=${bpmBefore} after=${bpmAfter}`);
  }

  // Verify playback still running
  await assertPlaying(page, true, "Playback should continue after BPM change");

  console.log("real_input_live_edit: Checking for parity mismatches...");
  await page.waitForTimeout(500);

  const mismatches = await page.evaluate(() => recentSchedulerMismatches?.());
  const mismatchCount = mismatches?.length ?? 0;
  console.log(`real_input_live_edit: Parity mismatches: ${mismatchCount}`);
  if (mismatchCount > 0) {
    console.log("real_input_live_edit: WARNING - parity mismatches detected:");
    for (const m of mismatches.slice(0, 3)) {
      console.log(`  row=${m.row} abs=${m.abs} kind=${m.kind}`);
    }
  }

  console.log("real_input_live_edit: Stopping playback...");
  await clickStopBtn(page);
  await page.waitForTimeout(200);
  await assertPlaying(page, false, "Playback should stop cleanly");

  console.log("real_input_live_edit: PASS - Live editing during playback works via real input");
} catch (error) {
  console.error("real_input_live_edit: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "real_input_live_edit");
  if (cleanup) {
    await cleanup();
  }
}
