/**
 * Browser tests for node sidebar interaction in WASM.
 *
 * Verifies open/close lifecycle, parameter adjustments (volume, pitch,
 * duration), logic kind changes, section expand/collapse, panel rect
 * validity, and switching between nodes.
 */
import { setupFullWasm, assertValidRect } from "./real_input_test_helpers.js";
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

// Test 1: Open/Close lifecycle
console.log("Test 1: Open/Close lifecycle");
try {
  await page.evaluate(() => {
    addNode?.(4, 4, "regular");
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const isOpen = await page.evaluate(() => nodeMenuOpen?.());
  assert(isOpen === true, `sidebar should be open after openNodeSidebar: got ${isOpen}`);

  const menuNodeId = await page.evaluate(() => nodeMenuNodeId?.());
  const expectedId = await page.evaluate(() => nodeIdAt?.(4, 4));
  assert(menuNodeId === expectedId, `nodeMenuNodeId should match nodeIdAt(4,4): got ${menuNodeId}, expected ${expectedId}`);

  await page.evaluate(() => {
    closeNodeMenu?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const isOpenAfter = await page.evaluate(() => nodeMenuOpen?.());
  assert(isOpenAfter === false, `sidebar should be closed after closeNodeMenu: got ${isOpenAfter}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 2: Volume adjustment
console.log("Test 2: Volume adjustment via sidebar");
try {
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const volBefore = await page.evaluate(() => nodeParams?.(4, 4)?.volume);
  assert(typeof volBefore === "number", `initial volume should be a number: got ${volBefore}`);

  await page.evaluate(() => {
    nodeMenuAction?.("vol+");
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const volAfter = await page.evaluate(() => nodeParams?.(4, 4)?.volume);
  assert(volAfter > volBefore - 0.001, `volume should increase after vol+: before=${volBefore} after=${volAfter}`);
  assert(Math.abs(volAfter - volBefore - 0.1) < 0.05, `volume should increase by ~0.1: delta=${volAfter - volBefore}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 3: Pitch adjustment
console.log("Test 3: Pitch adjustment via sidebar");
try {
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const pitBefore = await page.evaluate(() => nodeParams?.(4, 4)?.pitch);
  assert(typeof pitBefore === "number", `initial pitch should be a number: got ${pitBefore}`);

  await page.evaluate(() => {
    nodeMenuAction?.("pit+");
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const pitAfter = await page.evaluate(() => nodeParams?.(4, 4)?.pitch);
  assert(pitAfter > pitBefore, `pitch should increase after pit+: before=${pitBefore} after=${pitAfter}`);
  assert(Math.abs(pitAfter - pitBefore - 1) < 0.5, `pitch should increase by ~1: delta=${pitAfter - pitBefore}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 4: Duration adjustment
console.log("Test 4: Duration adjustment via sidebar");
try {
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const durBefore = await page.evaluate(() => nodeParams?.(4, 4)?.duration);
  assert(typeof durBefore === "number", `initial duration should be a number: got ${durBefore}`);

  await page.evaluate(() => {
    nodeMenuAction?.("dur+");
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const durAfter = await page.evaluate(() => nodeParams?.(4, 4)?.duration);
  assert(durAfter > durBefore - 0.001, `duration should increase after dur+: before=${durBefore} after=${durAfter}`);
  assert(Math.abs(durAfter - durBefore - 0.1) < 0.05, `duration should increase by ~0.1: delta=${durAfter - durBefore}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 5: Logic kind change via setNodeLogicGrid + sidebar verification
console.log("Test 5: Logic kind change to probability");
try {
  // Set logic via direct API (the sidebar dropdown requires multiple game loop
  // frames to wire buttons — setNodeLogicGrid is the reliable programmatic path)
  await page.evaluate(() => {
    setNodeLogicGrid?.(4, 4, "probability", 0, 0.5);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const params = await page.evaluate(() => nodeParams?.(4, 4));
  assert(params, "nodeParams should return an object");
  assert(params.logicKind === "probability", `logicKind should be 'probability': got '${params.logicKind}'`);
  assert(params.logicP > 0, `logicP should be > 0 after setting probability: got ${params.logicP}`);

  // Now open sidebar and verify the sidebar reflects the change
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const debugState = await page.evaluate(() => sidebarDebugState?.());
  assert(debugState && debugState.open === true, "sidebar should be open to show probability node");

  // Verify lp+/lp- buttons exist (they only appear for probability logic)
  const lpRect = await page.evaluate(() => nodeMenuRect?.("lp+"));
  assert(lpRect !== null && lpRect !== undefined, "lp+ button should exist for probability logic");

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 6: Section expand/collapse
console.log("Test 6: Section expand/collapse");
try {
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // Sections should be collapsed by default
  const volBefore = await page.evaluate(() => sidebarSectionOpen?.("vol"));
  assert(volBefore === false, `vol section should be closed initially: got ${volBefore}`);

  // Expand all sections
  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const volAfter = await page.evaluate(() => sidebarSectionOpen?.("vol"));
  assert(volAfter === true, `vol section should be open after expandAll: got ${volAfter}`);

  const pitAfter = await page.evaluate(() => sidebarSectionOpen?.("pit"));
  assert(pitAfter === true, `pit section should be open after expandAll: got ${pitAfter}`);

  const durAfter = await page.evaluate(() => sidebarSectionOpen?.("dur"));
  assert(durAfter === true, `dur section should be open after expandAll: got ${durAfter}`);

  const logicAfter = await page.evaluate(() => sidebarSectionOpen?.("logic"));
  assert(logicAfter === true, `logic section should be open after expandAll: got ${logicAfter}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 7: Panel rect validity
console.log("Test 7: Panel rect validity");
try {
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const panel = await page.evaluate(() => sidebarPanelRect?.());
  assert(panel !== null && panel !== undefined, "sidebarPanelRect should return non-null");
  assertValidRect(panel, "sidebarPanelRect");
  assert(panel.w > 0, `panel width should be positive: got ${panel.w}`);
  assert(panel.h > 0, `panel height should be positive: got ${panel.h}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 8: Switch nodes
console.log("Test 8: Switch between nodes in sidebar");
try {
  // Add a second node at (6,6)
  await page.evaluate(() => {
    addNode?.(6, 6, "regular");
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const id44 = await page.evaluate(() => nodeIdAt?.(4, 4));
  const id66 = await page.evaluate(() => nodeIdAt?.(6, 6));
  assert(id44 !== undefined && id44 >= 0, `node at (4,4) should exist: got ${id44}`);
  assert(id66 !== undefined && id66 >= 0, `node at (6,6) should exist: got ${id66}`);
  assert(id44 !== id66, `nodes at (4,4) and (6,6) should have different IDs: ${id44} vs ${id66}`);

  // Open sidebar for (4,4)
  await page.evaluate(() => {
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const menuId1 = await page.evaluate(() => nodeMenuNodeId?.());
  assert(menuId1 === id44, `sidebar should show node (4,4): got ${menuId1}, expected ${id44}`);

  // Switch to (6,6)
  await page.evaluate(() => {
    openNodeSidebar?.(6, 6);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const menuId2 = await page.evaluate(() => nodeMenuNodeId?.());
  assert(menuId2 === id66, `sidebar should show node (6,6): got ${menuId2}, expected ${id66}`);

  const stillOpen = await page.evaluate(() => nodeMenuOpen?.());
  assert(stillOpen === true, "sidebar should remain open after switching nodes");

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// Cleanup
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "sidebar_interaction");
await cleanup();

if (allPassed) {
  console.log("\nAll sidebar interaction browser tests passed.");
  process.exit(0);
} else {
  console.log("\nSome sidebar interaction browser tests FAILED.");
  process.exit(1);
}
