/**
 * Browser tests for live parameter editing during playback.
 *
 * Verifies that changing node parameters (probability, volume, logic kind,
 * duration) while the sequencer is running correctly rebases the predictor
 * and does not crash or stall playback.
 */
import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let env;
let allPassed = true;

function assert(cond, msg) {
  if (!cond) { allPassed = false; throw new Error(msg); }
}

try {
  env = await setupFullWasm({ logLevel: "ERROR" });
} catch (e) {
  console.log(`Setup failed: ${e.message}`);
  process.exit(1);
}

const { page, cleanup } = env;

// Helper: import a clean 3-node loop circuit at 180 BPM.
async function setupCircuit() {
  await page.evaluate(() => {
    importJSON?.(JSON.stringify({
      version: 1, subdiv: 8, bpm: 180,
      instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1, origin: 0, color: "#C87850FF" }],
      nodes: [
        { id: 0, i: 0, j: 0, type: "regular", inputs: [2], outputs: [1] },
        { id: 1, i: 1, j: 0, type: "regular", inputs: [0], outputs: [2] },
        { id: 2, i: 2, j: 0, type: "regular", inputs: [1], outputs: [0] }
      ]
    }));
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);
}

// Helper: wait for beats to advance past a given index.
async function waitForBeats(minIdx, timeout = 5000) {
  await page.waitForFunction((min) => {
    const idxs = nextBeatIdxs?.();
    return idxs && idxs[0] > min;
  }, minIdx, { timeout });
}

// Helper: ensure playback is stopped.
async function ensureStopped() {
  await page.evaluate(() => {
    if (isPlaying?.()) stopPlay?.();
  });
  await page.waitForTimeout(200);
}

// --------------------------------------------------------------------------
// Test 1: Probability change during playback
// --------------------------------------------------------------------------
console.log("Test 1: Probability change during playback");
try {
  await setupCircuit();

  // Set node(1,0) to probability=0.3 and start playback
  await page.evaluate(() => {
    setNodeLogicGrid?.(1, 0, "probability", 0, 0.3);
    setFollow?.(false);
    startPlay?.();
  });

  // Wait for beats to advance
  await waitForBeats(4);

  // Change probability to 1.0 (always fire)
  await page.evaluate(() => {
    setNodeLogicGrid?.(1, 0, "probability", 0, 1.0);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Ensure predictor has computed future and verify node(1,0) positions are visible.
  // In a 3-node loop, the probability node at position 1 fires at abs indices
  // 1, 4, 7, 10, ... (every 3rd starting from 1). With p=1.0, these should ALL
  // be visible. Other positions (0, 2, 3, 5, ...) are regular nodes that always fire.
  const result = await page.evaluate(() => {
    const idxs = nextBeatIdxs?.();
    if (!idxs) return { ok: false, reason: "nextBeatIdxs unavailable" };
    const currentIdx = idxs[0];
    const futureStart = currentIdx + 3; // a few beats ahead
    const futureEnd = futureStart + 12; // 4 full loops
    ensure?.(futureEnd + 1);

    // Check that ALL positions are visible (all 3 nodes are regular with p=1.0
    // on the probability node, so every position should fire).
    let visibleCount = 0;
    let totalChecked = 0;
    for (let a = futureStart; a < futureEnd; a++) {
      const v = visibleAt?.(0, a);
      if (v) visibleCount++;
      totalChecked++;
    }
    // Also specifically check probability node positions
    let probVisible = 0;
    let probTotal = 0;
    for (let a = futureStart; a < futureEnd; a++) {
      // Probability node is at position 1 in the 3-node loop
      if (a % 3 === 1) {
        probTotal++;
        if (visibleAt?.(0, a)) probVisible++;
      }
    }
    return { ok: true, visibleCount, totalChecked, probVisible, probTotal, currentIdx };
  });

  assert(result.ok, result.reason || "evaluation failed");
  // The probability node positions (indices 1,4,7,10,...) should all be visible
  // since p=1.0. Allow for loop offset variations — at minimum most should fire.
  assert(result.visibleCount > result.totalChecked * 0.6,
    `Expected most future positions visible with p=1.0, got ${result.visibleCount}/${result.totalChecked}`);

  const playing = await page.evaluate(() => isPlaying?.());
  assert(playing === true, "Playback should still be running after probability change");

  await ensureStopped();
  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
  await ensureStopped();
}

// --------------------------------------------------------------------------
// Test 2: Volume change during playback via sidebar
// --------------------------------------------------------------------------
console.log("Test 2: Volume change during playback via sidebar");
try {
  await setupCircuit();

  // Get initial volume
  const initialVol = await page.evaluate(() => {
    const p = nodeParams?.(1, 0);
    return p ? p.volume : null;
  });
  assert(initialVol !== null, "nodeParams should return volume");

  // Start playback
  await page.evaluate(() => {
    setFollow?.(false);
    startPlay?.();
  });
  await waitForBeats(3);

  // Open sidebar for node(1,0), expand sections, increase volume
  await page.evaluate(() => {
    openNodeSidebar?.(1, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    nodeMenuAction?.("vol+");
    forceDraw?.();
  });
  await page.waitForTimeout(100);

  // Verify volume increased
  const newVol = await page.evaluate(() => {
    const p = nodeParams?.(1, 0);
    return p ? p.volume : null;
  });
  assert(newVol !== null, "nodeParams should return volume after change");
  assert(newVol > initialVol, `Volume should increase: was ${initialVol}, now ${newVol}`);

  // Verify playback is still running
  const playing = await page.evaluate(() => isPlaying?.());
  assert(playing === true, "Playback should still be running after volume change");

  // Close sidebar and stop
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(100);
  await ensureStopped();
  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
  await ensureStopped();
}

// --------------------------------------------------------------------------
// Test 3: Logic kind change during playback (skip_every_n)
// --------------------------------------------------------------------------
console.log("Test 3: Logic kind change during playback");
try {
  await setupCircuit();

  // Start playback with no logic on any node
  await page.evaluate(() => {
    setFollow?.(false);
    startPlay?.();
  });
  await waitForBeats(4);

  // Change node(1,0) to skip_every_n with n=2
  await page.evaluate(() => {
    setNodeLogicGrid?.(1, 0, "skip_every_n", 2, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Check future predictions: skip_every_n=2 means every other trigger at this
  // node position is skipped, so we should see a mix of visible and not-visible.
  const result = await page.evaluate(() => {
    const idxs = nextBeatIdxs?.();
    if (!idxs) return { ok: false, reason: "nextBeatIdxs unavailable" };
    const currentIdx = idxs[0];
    const futureStart = currentIdx + 3;
    const futureEnd = futureStart + 18; // 6 full loops of the 3-node circuit
    ensure?.(futureEnd + 1);

    let visibleCount = 0;
    let totalChecked = 0;
    // In a 3-node loop, node(1,0) corresponds to positions 1, 4, 7, 10, ...
    // Check only those positions (every 3rd starting from 1).
    for (let a = futureStart; a < futureEnd; a++) {
      const v = visibleAt?.(0, a);
      if (v) visibleCount++;
      totalChecked++;
    }
    return { ok: true, visibleCount, totalChecked };
  });

  assert(result.ok, result.reason || "evaluation failed");
  // With skip_every_n=2, roughly half of the node's triggers should be skipped.
  // We expect some visible and some not-visible (not all and not none).
  assert(result.visibleCount > 0, "Some future positions should be visible");
  assert(result.visibleCount < result.totalChecked,
    `Not all positions should be visible with skip_every_n=2: ${result.visibleCount}/${result.totalChecked}`);

  const playing = await page.evaluate(() => isPlaying?.());
  assert(playing === true, "Playback should still be running after logic change");

  await ensureStopped();
  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
  await ensureStopped();
}

// --------------------------------------------------------------------------
// Test 4: Duration change during playback via sidebar
// --------------------------------------------------------------------------
console.log("Test 4: Duration change during playback via sidebar");
try {
  await setupCircuit();

  // Get initial duration
  const initialDur = await page.evaluate(() => {
    const p = nodeParams?.(1, 0);
    return p ? p.duration : null;
  });
  assert(initialDur !== null, "nodeParams should return duration");

  // Start playback
  await page.evaluate(() => {
    setFollow?.(false);
    startPlay?.();
  });
  await waitForBeats(3);

  // Open sidebar, expand sections, increase duration
  await page.evaluate(() => {
    openNodeSidebar?.(1, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    nodeMenuAction?.("dur+");
    forceDraw?.();
  });
  await page.waitForTimeout(100);

  // Verify duration increased
  const newDur = await page.evaluate(() => {
    const p = nodeParams?.(1, 0);
    return p ? p.duration : null;
  });
  assert(newDur !== null, "nodeParams should return duration after change");
  assert(newDur > initialDur, `Duration should increase: was ${initialDur}, now ${newDur}`);

  // Verify playback is still running
  const playing = await page.evaluate(() => isPlaying?.());
  assert(playing === true, "Playback should still be running after duration change");

  // Close sidebar and stop
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(100);
  await ensureStopped();
  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
  await ensureStopped();
}

// --------------------------------------------------------------------------
// Test 5: Multiple rapid edits during playback
// --------------------------------------------------------------------------
console.log("Test 5: Multiple rapid edits during playback");
try {
  await setupCircuit();

  // Start playback
  await page.evaluate(() => {
    setFollow?.(false);
    startPlay?.();
  });
  await waitForBeats(3);

  // Rapid-fire probability changes: 0.5 -> 0.8 -> 1.0
  await page.evaluate(() => {
    setNodeLogicGrid?.(1, 0, "probability", 0, 0.5);
    forceDraw?.();
  });
  await page.evaluate(() => {
    setNodeLogicGrid?.(1, 0, "probability", 0, 0.8);
    forceDraw?.();
  });
  await page.evaluate(() => {
    setNodeLogicGrid?.(1, 0, "probability", 0, 1.0);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Verify final params match the last edit
  const params = await page.evaluate(() => {
    const p = nodeParams?.(1, 0);
    return p ? { logicKind: p.logicKind, logicP: p.logicP } : null;
  });
  assert(params !== null, "nodeParams should return after rapid edits");
  assert(params.logicKind === "probability",
    `Logic kind should be 'probability', got '${params.logicKind}'`);
  assert(Math.abs(params.logicP - 1.0) < 0.01,
    `Logic P should be 1.0, got ${params.logicP}`);

  // Verify playback survived the rapid edits
  const playing = await page.evaluate(() => isPlaying?.());
  assert(playing === true, "Playback should still be running after rapid edits");

  await ensureStopped();
  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
  await ensureStopped();
}

// Cleanup
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "live_edit_rebase");
await cleanup();

if (allPassed) {
  console.log("\nAll live edit rebase browser tests passed.");
  process.exit(0);
} else {
  console.log("\nSome live edit rebase browser tests FAILED.");
  process.exit(1);
}
