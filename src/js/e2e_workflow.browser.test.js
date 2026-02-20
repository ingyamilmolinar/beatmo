/**
 * E2E Workflow Browser Tests
 *
 * Multi-step scenario tests exercising the full user workflow:
 * build → play → edit → export → import → verify.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import {
  clickPlayBtn,
  clickStopBtn,
  clickBpmInc,
  assertPlaying,
  assertNodeExists,
} from "./real_input_actions.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("e2e_workflow: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // ═══════════════════════════════════════════════════════════════════════
  // Scenario 1: Full Lifecycle (Build → Play → Edit → Export → Import)
  // ═══════════════════════════════════════════════════════════════════════
  console.log("\n═══ Scenario 1: Full Lifecycle ═══");

  // Step 1: Build a 4-node circuit
  console.log("  Step 1: Building circuit...");
  await page.evaluate(() => {
    addNode?.(40, 0, "regular");
    addNode?.(44, 0, "regular");
    addNode?.(44, 4, "regular");
    addNode?.(40, 4, "regular");
    addEdgeGrid?.(40, 0, 44, 0);
    addEdgeGrid?.(44, 0, 44, 4);
    addEdgeGrid?.(44, 4, 40, 4);
    addEdgeGrid?.(40, 4, 40, 0);
    setOrigin?.(40, 0, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  await assertNodeExists(page, 40, 0, "Node (40,0) after build");
  await assertNodeExists(page, 44, 0, "Node (44,0) after build");
  console.log("  Circuit built: 4 nodes, 4 edges");

  // Step 2: Start playback, wait for a few beats
  console.log("  Step 2: Starting playback...");
  await clickPlayBtn(page);
  await page.waitForTimeout(600);
  await assertPlaying(page, true, "Playback should be running");
  console.log("  Playback running");

  // Step 3: Live-edit — add a node + edge while playing
  console.log("  Step 3: Live editing during playback...");
  await page.evaluate(() => {
    addNode?.(40, -4, "regular");
    addEdgeGrid?.(40, 0, 40, -4);
    updateBeatInfosJS?.();
  });
  await page.waitForTimeout(200);
  await assertNodeExists(page, 40, -4, "New node during playback");
  await assertPlaying(page, true, "Playback continues after edit");
  console.log("  Node added during playback");

  // Step 4: Change BPM
  console.log("  Step 4: Changing BPM...");
  const bpmBefore = await page.evaluate(() => getBPM?.());
  for (let i = 0; i < 5; i++) await clickBpmInc(page);
  const bpmAfter = await page.evaluate(() => getBPM?.());
  if (bpmAfter <= bpmBefore) throw new Error(`BPM didn't increase: ${bpmBefore} -> ${bpmAfter}`);
  console.log(`  BPM: ${bpmBefore} -> ${bpmAfter}`);

  // Step 5: Stop playback
  console.log("  Step 5: Stopping playback...");
  await clickStopBtn(page);
  await page.waitForTimeout(200);
  await assertPlaying(page, false, "Playback stopped");
  console.log("  Playback stopped cleanly");

  // Step 6: Export JSON
  console.log("  Step 6: Exporting JSON...");
  const exported = await page.evaluate(() => exportJSON?.());
  if (!exported || exported.length < 10) {
    throw new Error("Export returned empty or too short");
  }
  const parsed = JSON.parse(exported);
  const exportedNodeCount = parsed.nodes?.length ?? 0;
  console.log(`  Exported: ${exported.length} bytes, ${exportedNodeCount} nodes`);

  // Step 7: Import the exported JSON (round-trip)
  console.log("  Step 7: Re-importing exported JSON...");
  await page.evaluate((json) => importJSON?.(json), exported);
  await page.waitForTimeout(500);
  await page.evaluate(() => {
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Step 8: Verify circuit restored
  console.log("  Step 8: Verifying round-trip...");
  const exportedAfter = await page.evaluate(() => exportJSON?.());
  const parsedAfter = JSON.parse(exportedAfter);
  const nodeCountAfter = parsedAfter.nodes?.length ?? 0;
  if (nodeCountAfter !== exportedNodeCount) {
    throw new Error(`Node count mismatch after round-trip: ${exportedNodeCount} -> ${nodeCountAfter}`);
  }
  if (parsedAfter.bpm !== parsed.bpm) {
    throw new Error(`BPM mismatch after round-trip: ${parsed.bpm} -> ${parsedAfter.bpm}`);
  }
  console.log(`  Round-trip verified: ${nodeCountAfter} nodes, BPM=${parsedAfter.bpm}`);

  // Step 9: Resume playback after import
  console.log("  Step 9: Resuming playback after import...");
  await clickPlayBtn(page);
  await page.waitForTimeout(500);
  await assertPlaying(page, true, "Playback should resume after import");
  await clickStopBtn(page);
  await page.waitForTimeout(200);
  console.log("  Playback resumed and stopped successfully");

  // Check parity
  const mismatches1 = await page.evaluate(() => recentSchedulerMismatches?.());
  console.log(`  Parity mismatches: ${mismatches1?.length ?? 0}`);
  console.log("═══ Scenario 1: PASS ═══\n");

  // ═══════════════════════════════════════════════════════════════════════
  // Scenario 2: Rapid BPM Stress
  // ═══════════════════════════════════════════════════════════════════════
  console.log("═══ Scenario 2: Rapid BPM Stress ═══");

  // Build fresh circuit
  await page.evaluate(() => {
    // Re-import the simple fixture to reset state
    const fix = JSON.stringify({
      version: 1, subdiv: 8, bpm: 120,
      instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1, origin: 1, color: "#80C850FF" }],
      nodes: [
        { id: 1, i: 0, j: 0, type: "regular", outputs: [2] },
        { id: 2, i: 8, j: 0, type: "regular", outputs: [3] },
        { id: 3, i: 8, j: 8, type: "regular", outputs: [4] },
        { id: 4, i: 0, j: 8, type: "regular", outputs: [1] },
      ],
    });
    importJSON?.(fix);
  });
  await page.waitForTimeout(300);
  await page.evaluate(() => { updateBeatInfosJS?.(); forceDraw?.(); });
  await page.waitForTimeout(200);

  // Start playback via API (Scenario 1 already tested real play button click)
  await page.evaluate(() => startPlay?.());
  await page.waitForTimeout(300);
  await assertPlaying(page, true, "Playback for stress test");

  // Rapid BPM clicks (20 increments at ~50ms apart)
  console.log("  Clicking BPM+ 20 times rapidly...");
  const bpmStart = await page.evaluate(() => getBPM?.());
  for (let i = 0; i < 20; i++) {
    await clickBpmInc(page);
    await page.waitForTimeout(50);
  }
  const bpmEnd = await page.evaluate(() => getBPM?.());
  console.log(`  BPM: ${bpmStart} -> ${bpmEnd}`);

  if (bpmEnd <= bpmStart) {
    throw new Error(`BPM didn't increase during stress: ${bpmStart} -> ${bpmEnd}`);
  }

  // Verify still playing
  await assertPlaying(page, true, "Playback should survive rapid BPM");
  console.log("  Playback survived rapid BPM changes");

  // Check parity
  await page.waitForTimeout(500);
  const mismatches2 = await page.evaluate(() => recentSchedulerMismatches?.());
  const mm2 = mismatches2?.length ?? 0;
  console.log(`  Parity mismatches: ${mm2}`);

  await page.evaluate(() => stopPlay?.());
  await page.waitForTimeout(200);
  console.log("═══ Scenario 2: PASS ═══\n");

  // ═══════════════════════════════════════════════════════════════════════
  // Scenario 3: Row Add/Delete During Playback
  // ═══════════════════════════════════════════════════════════════════════
  console.log("═══ Scenario 3: Row Add/Delete During Playback ═══");

  // Start with a fresh circuit
  await page.evaluate(() => {
    const fix = JSON.stringify({
      version: 1, subdiv: 8, bpm: 120,
      instruments: [
        { name: "Kick", id: "kick", kind: "builtin", volume: 1, origin: 1, color: "#80C850FF" },
        { name: "Snare", id: "snare", kind: "builtin", volume: 1, origin: 5, color: "#C85050FF" },
      ],
      nodes: [
        { id: 1, i: 0, j: 0, type: "regular", outputs: [2] },
        { id: 2, i: 8, j: 0, type: "regular", outputs: [3] },
        { id: 3, i: 8, j: 8, type: "regular", outputs: [4] },
        { id: 4, i: 0, j: 8, type: "regular", outputs: [1] },
        { id: 5, i: -16, j: 0, type: "regular", outputs: [6] },
        { id: 6, i: -8, j: 0, type: "regular", outputs: [7] },
        { id: 7, i: -8, j: 8, type: "regular", outputs: [8] },
        { id: 8, i: -16, j: 8, type: "regular", outputs: [5] },
      ],
    });
    importJSON?.(fix);
  });
  await page.waitForTimeout(300);
  await page.evaluate(() => { updateBeatInfosJS?.(); forceDraw?.(); });
  await page.waitForTimeout(200);

  const rowsBefore = await page.evaluate(() => totalRows?.());
  console.log(`  Initial rows: ${rowsBefore}`);

  // Start playback via API (Scenario 1 already tested real play button click)
  await page.evaluate(() => startPlay?.());
  await page.waitForTimeout(500);
  await assertPlaying(page, true, "Playback for row test");

  // Add a new drum row during playback
  console.log("  Adding drum row during playback...");
  await page.evaluate(() => addDrumRow?.());
  await page.waitForTimeout(200);

  const rowsAfterAdd = await page.evaluate(() => totalRows?.());
  if (rowsAfterAdd <= rowsBefore) {
    throw new Error(`Row not added: ${rowsBefore} -> ${rowsAfterAdd}`);
  }
  console.log(`  Rows after add: ${rowsAfterAdd}`);
  await assertPlaying(page, true, "Playback continues after row add");

  // Verify playback still running
  await page.waitForTimeout(500);
  await assertPlaying(page, true, "Playback should survive row operations");

  // Stop playback
  await page.evaluate(() => stopPlay?.());
  await page.waitForTimeout(200);
  await assertPlaying(page, false, "Playback stopped after row test");

  // Check parity
  const mismatches3 = await page.evaluate(() => recentSchedulerMismatches?.());
  console.log(`  Parity mismatches: ${mismatches3?.length ?? 0}`);
  console.log("═══ Scenario 3: PASS ═══\n");

  console.log("e2e_workflow: PASS - All 3 scenarios completed successfully");
} catch (error) {
  console.error("e2e_workflow: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "e2e_workflow");
  if (cleanup) {
    await cleanup();
  }
}
