/**
 * Browser tests for node sidebar scroll in WASM.
 *
 * Verifies wheel scroll, scrollbar click, scrollbar drag, and that
 * wheel events over the sidebar don't zoom the camera.
 */
import { setupFullWasm, wheelAt, dragMouse, rectCenter, assertValidRect } from "./real_input_test_helpers.js";
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

// Helper: create a node, open sidebar, expand all sections, force draw.
async function openSidebarWithOverflow() {
  await page.evaluate(() => {
    // Add node at (4,4) to avoid default start node conflicts
    addNode?.(4, 4, "regular");
    // Open sidebar programmatically
    openNodeSidebar?.(4, 4);
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const open = await page.evaluate(() => nodeMenuOpen?.());
  if (!open) throw new Error("sidebar did not open");

  // Expand all sections to create overflow
  await page.evaluate(() => {
    sidebarExpandAllSections?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);
}

// Test 1: Wheel scroll changes sidebar offset
console.log("Test 1: Wheel scroll changes sidebar offset");
try {
  await openSidebarWithOverflow();

  const hasScroll = await page.evaluate(() => sidebarHasScroll?.());
  assert(hasScroll, "sidebar should have scroll after expanding all sections");

  const offsetBefore = await page.evaluate(() => sidebarScrollOffset?.());
  const panel = await page.evaluate(() => sidebarPanelRect?.());
  assertValidRect(panel, "sidebarPanelRect");

  // Wheel down inside the sidebar panel
  const cx = panel.x + panel.w / 2;
  const cy = panel.y + panel.h / 2;
  await wheelAt(page, cx, cy, 240); // positive = scroll down
  await page.waitForTimeout(200);

  const offsetAfter = await page.evaluate(() => sidebarScrollOffset?.());
  assert(offsetAfter > offsetBefore, `scroll offset should increase after wheel down: before=${offsetBefore} after=${offsetAfter}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// Close sidebar for next test
await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 2: Scrollbar track click jumps scroll
console.log("Test 2: Scrollbar track click");
try {
  await openSidebarWithOverflow();

  const hasScroll = await page.evaluate(() => sidebarHasScroll?.());
  assert(hasScroll, "sidebar should have scroll");

  const barRect = await page.evaluate(() => sidebarScrollBarRect?.());
  assertValidRect(barRect, "scrollbar bar rect");

  // Click near the bottom of the scrollbar track.
  // Use mouse.down + wait + mouse.up so the game loop sees pressed=true.
  const clickX = barRect.x + barRect.w / 2;
  const clickY = barRect.y + barRect.h * 0.8;
  await page.mouse.move(clickX, clickY);
  await page.mouse.down();
  await page.waitForTimeout(200);
  await page.mouse.up();
  await page.waitForTimeout(200);

  const offset = await page.evaluate(() => sidebarScrollOffset?.());
  assert(offset > 0, `scroll offset should be non-zero after track click: got ${offset}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 3: Scrollbar thumb drag scrolls content
console.log("Test 3: Scrollbar thumb drag");
try {
  await openSidebarWithOverflow();

  const thumbRect = await page.evaluate(() => sidebarScrollThumbRect?.());
  assertValidRect(thumbRect, "scrollbar thumb rect");

  const thumbCenter = rectCenter(thumbRect);
  const offsetBefore = await page.evaluate(() => sidebarScrollOffset?.());

  // Drag thumb downward
  await dragMouse(page, thumbCenter.x, thumbCenter.y, thumbCenter.x, thumbCenter.y + 60, { steps: 8 });
  await page.waitForTimeout(200);

  const offsetAfter = await page.evaluate(() => sidebarScrollOffset?.());
  assert(offsetAfter > offsetBefore, `scroll offset should increase after thumb drag: before=${offsetBefore} after=${offsetAfter}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 4: Wheel over sidebar does NOT zoom camera
console.log("Test 4: Wheel over sidebar doesn't zoom camera");
try {
  await openSidebarWithOverflow();

  const scaleBefore = await page.evaluate(() => camScale?.());
  const panel = await page.evaluate(() => sidebarPanelRect?.());
  assertValidRect(panel, "sidebarPanelRect");

  const cx = panel.x + panel.w / 2;
  const cy = panel.y + panel.h / 2;
  await wheelAt(page, cx, cy, 240);
  await page.waitForTimeout(200);

  const scaleAfter = await page.evaluate(() => camScale?.());
  assert(
    Math.abs(scaleAfter - scaleBefore) < 0.01,
    `camera scale should not change: before=${scaleBefore} after=${scaleAfter}`
  );

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 5: Debug state export returns valid data
console.log("Test 5: Debug state export");
try {
  await openSidebarWithOverflow();

  const state = await page.evaluate(() => sidebarDebugState?.());
  assert(state, "sidebarDebugState should return an object");
  assert(state.open === true, "state.open should be true");
  assert(typeof state.scrollOffset === "number", "state.scrollOffset should be a number");
  assert(typeof state.hasScroll === "boolean", "state.hasScroll should be a boolean");
  assert(typeof state.contentH === "number", "state.contentH should be a number");
  assert(typeof state.viewportH === "number", "state.viewportH should be a number");
  assert(state.contentH > state.viewportH, "contentH should exceed viewportH when scrollable");

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

await page.evaluate(() => closeNodeMenu?.());
await page.waitForTimeout(100);

// Test 6: Mobile touch drag on section header scrolls without toggling
console.log("Test 6: Mobile touch drag on section header doesn't toggle");
try {
  await openSidebarWithOverflow();

  // Check vol section is open (expanded by openSidebarWithOverflow)
  const volOpenBefore = await page.evaluate(() => sidebarSectionOpen?.("vol"));
  assert(volOpenBefore === true, "vol section should be open after expanding all");

  // Get section header rect for "vol"
  const secRect = await page.evaluate(() => sidebarSectionRect?.("vol"));
  assert(secRect, "sidebarSectionRect('vol') should return a rect");
  assertValidRect(secRect, "vol section rect");

  const scrollBefore = await page.evaluate(() => sidebarScrollOffset?.());

  // CDP touch drag on the section header (vertical drag of 40px)
  const { cdpDrag } = await import("./touch_cdp_helpers.js");
  const cx = secRect.x + secRect.w / 2;
  const cy = secRect.y + secRect.h / 2;
  await cdpDrag(page, cx, cy, cx, cy - 40, 8, 30, 100);
  await page.waitForTimeout(300);

  // Section should NOT have toggled
  const volOpenAfter = await page.evaluate(() => sidebarSectionOpen?.("vol"));
  assert(volOpenAfter === volOpenBefore,
    `vol section should not toggle during touch scroll: before=${volOpenBefore} after=${volOpenAfter}`);

  // Scroll offset should have changed from the touch drag
  const scrollAfter = await page.evaluate(() => sidebarScrollOffset?.());
  assert(scrollAfter !== scrollBefore,
    `touch drag should change scroll offset: before=${scrollBefore} after=${scrollAfter}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// Cleanup
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "sidebar_scroll");
await cleanup();

if (allPassed) {
  console.log("\nAll sidebar scroll browser tests passed.");
  process.exit(0);
} else {
  console.log("\nSome sidebar scroll browser tests FAILED.");
  process.exit(1);
}
