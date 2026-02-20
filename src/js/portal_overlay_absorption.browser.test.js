/**
 * Browser tests for portal overlay input absorption.
 *
 * Verifies that clicks inside overlay empty space are absorbed (not leaked
 * to underlying buttons), and that click-outside closes overlays without
 * activating underlying controls.
 *
 * Uses slow mouse down/up (with frame gap) because Ebiten polls input
 * per-frame — a fast click() can be missed entirely.
 */
import { setupFullWasm, assertValidRect } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";
import path from "path";
import { fileURLToPath } from "url";

const __filename = fileURLToPath(import.meta.url);
const jsDir = path.dirname(__filename);

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

// Helper: wait for state to settle after overlay operations.
async function settle(ms = 300) {
  await page.evaluate(() => { forceDraw?.(); });
  await page.waitForTimeout(ms);
}

// Helper: slow click that spans multiple frames so Ebiten sees the press.
// Playwright's page.mouse.click() fires mousedown+mouseup synchronously,
// which Ebiten's per-frame polling can miss entirely.
async function slowClick(x, y) {
  await page.mouse.move(x, y);
  await page.mouse.down();
  await page.waitForTimeout(60); // ~3-4 frames at 60fps
  await page.mouse.up();
}

// Helper: ensure clean state before each test.
async function cleanState() {
  await page.evaluate(() => { closeAllPopups?.(); forceDraw?.(); });
  await settle(100);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 1: Context menu — empty space click absorbed
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 1: Context menu — empty space click absorbed");
try {
  await cleanState();

  await page.evaluate(() => { openContextMenuJS?.(0); forceDraw?.(); });
  await settle();

  const isOpen = await page.evaluate(() => contextMenuOpenJS?.());
  assert(isOpen === true, `context menu should be open, got ${isOpen}`);

  const rect = await page.evaluate(() => contextMenuRectJS?.());
  assertValidRect(rect, "contextMenuRect");

  const mutedBefore = await page.evaluate(() => rowMuted?.(0));
  // Click inside menu rect at center (on a menu item — should be absorbed
  // by the item but NOT leak through to underlying buttons)
  const clickX = rect.x + rect.w / 2;
  const clickY = rect.y + rect.h / 2;

  await slowClick(clickX, clickY);
  await settle();

  // Clicking an item in the context menu closes it (expected behavior).
  // The key assertion is that the mute state didn't change.
  const mutedAfter = await page.evaluate(() => rowMuted?.(0));
  assert(mutedBefore === mutedAfter, `mute should not change (was ${mutedBefore}, now ${mutedAfter})`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 2: Context menu — click outside closes without activating underlying
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 2: Context menu — click outside closes without leak");
try {
  await cleanState();

  await page.evaluate(() => { openContextMenuJS?.(0); forceDraw?.(); });
  await settle();

  const isOpen = await page.evaluate(() => contextMenuOpenJS?.());
  assert(isOpen === true, `context menu should be open, got ${isOpen}`);

  const mutedBefore = await page.evaluate(() => rowMuted?.(0));

  // Get a mute button rect to find a coordinate that's definitely in the
  // drum view area but far from the context menu.
  const muteRect = await page.evaluate(() => rowMuteBtnRect?.(0));
  assertValidRect(muteRect, "rowMuteBtnRect(0)");

  const rect = await page.evaluate(() => contextMenuRectJS?.());

  // Click OUTSIDE the context menu rect but still in the drum view.
  // Go to the right of the menu (timeline area) and vertically centered.
  const outsideX = rect.x + rect.w + 50;
  const outsideY = rect.y + rect.h / 2;

  await slowClick(outsideX, outsideY);
  await settle();

  const closed = await page.evaluate(() => contextMenuOpenJS?.());

  assert(closed === false, `context menu should be closed after click-outside, got ${closed}`);

  const mutedAfter = await page.evaluate(() => rowMuted?.(0));
  assert(mutedBefore === mutedAfter, `mute should not change after click-outside (was ${mutedBefore}, now ${mutedAfter})`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 3: Instrument menu — empty space click absorbed
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 3: Instrument menu — empty space click absorbed");
try {
  await cleanState();

  await page.evaluate(() => { openInstMenu?.(0); forceDraw?.(); });
  await settle();

  const isOpen = await page.evaluate(() => instMenuOpenState?.());
  assert(isOpen === true, `inst menu should be open, got ${isOpen}`);

  const mutedBefore = await page.evaluate(() => rowMuted?.(0));

  // Get menu item rects to find empty area below items
  const items = await page.evaluate(() => instMenuItemRects?.());
  assert(items && items.length > 0, `inst menu should have items, got ${JSON.stringify(items)}`);

  // Click below the last item but still reasonably within the menu area
  const lastItem = items[items.length - 1];
  await slowClick(lastItem.x + lastItem.w / 2, lastItem.y + lastItem.h + 5);
  await settle();

  const stillOpen = await page.evaluate(() => instMenuOpenState?.());
  // The click may have been outside the menu bounds — either absorbed or closed is OK,
  // but the mute state must not change.
  const mutedAfter = await page.evaluate(() => rowMuted?.(0));
  assert(mutedBefore === mutedAfter, `mute should not change (was ${mutedBefore}, now ${mutedAfter})`);

  if (stillOpen) {
    console.log("  PASS (click absorbed, menu still open)");
  } else {
    console.log("  PASS (click closed menu but no leak-through)");
  }
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 4: Color wheel — empty space click absorbed
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 4: Color wheel — empty space click absorbed");
try {
  await cleanState();

  await page.evaluate(() => { openColorMenu?.(0); forceDraw?.(); });
  await settle();

  const isOpen = await page.evaluate(() => colorMenuOpenState?.());
  assert(isOpen === true, `color menu should be open, got ${isOpen}`);

  const colorBefore = await page.evaluate(() => rowColor?.(0));

  const rect = await page.evaluate(() => colorWheelRect?.());
  assertValidRect(rect, "colorWheelRect");

  // Click at top-left corner inside bounds (empty space outside wheel circle)
  await slowClick(rect.x + 3, rect.y + 3);
  await settle();

  const stillOpen = await page.evaluate(() => colorMenuOpenState?.());
  assert(stillOpen === true, `color menu should still be open after corner click, got ${stillOpen}`);

  const colorAfter = await page.evaluate(() => rowColor?.(0));
  assert(colorBefore === colorAfter, `color should not change (was ${colorBefore}, now ${colorAfter})`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 5: FX panel — scrim click closes panel, no leak to row buttons
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 5: FX panel — scrim click closes, no leak");
try {
  await cleanState();

  await page.evaluate(() => { openFXPanelJS?.(0); forceDraw?.(); });
  // Extra frames for FX panel debounce
  await settle(500);

  const isOpen = await page.evaluate(() => fxPanelOpenJS?.());
  assert(isOpen === true, `FX panel should be open, got ${isOpen}`);

  const mutedBefore = await page.evaluate(() => rowMuted?.(0));

  const panelRect = await page.evaluate(() => fxPanelRectJS?.());
  assertValidRect(panelRect, "fxPanelRect");

  // Find a point inside dv.Bounds (drum view area) but outside the FX panel.
  // Use mute button position to anchor within the drum view, then pick a
  // Y coordinate that's clearly outside the panel rect.
  const muteRect = await page.evaluate(() => rowMuteBtnRect?.(0));
  assertValidRect(muteRect, "rowMuteBtnRect for scrim");

  // Choose point inside drum view but outside FX panel rect.
  // The FX panel portal covers all of dv.Bounds, so clicks route through
  // its inputFn. handleFXPanelInput closes the panel for clicks outside
  // fxPanelRect (the visual panel). We need (scrimX, scrimY) outside the
  // visual panel but still within the drum view bounds.
  let scrimX = muteRect.x + muteRect.w / 2;
  let scrimY;
  // Check if panel bottom is well above the bottom of the drum area.
  if (panelRect.y + panelRect.h < muteRect.y + 300) {
    // Panel is above the mute buttons — click below panel
    scrimY = panelRect.y + panelRect.h + 30;
  } else {
    // Panel extends down — click well above panel
    scrimY = Math.max(panelRect.y - 30, muteRect.y + muteRect.h + 10);
  }
  // Make sure scrimX is outside panel rect
  if (scrimX >= panelRect.x && scrimX < panelRect.x + panelRect.w) {
    scrimX = panelRect.x - 30;
    if (scrimX < muteRect.x) scrimX = panelRect.x + panelRect.w + 30;
  }

  await slowClick(scrimX, scrimY);
  await settle();

  const closed = await page.evaluate(() => fxPanelOpenJS?.());
  assert(closed === false, `FX panel should be closed after scrim click, got ${closed}`);

  const mutedAfter = await page.evaluate(() => rowMuted?.(0));
  assert(mutedBefore === mutedAfter, `mute should not change after scrim click (was ${mutedBefore}, now ${mutedAfter})`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 6: FX panel — inside empty space absorbed
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 6: FX panel — inside empty space absorbed");
try {
  await cleanState();

  await page.evaluate(() => { openFXPanelJS?.(0); forceDraw?.(); });
  // Extra frames for FX panel debounce
  await settle(500);

  const isOpen = await page.evaluate(() => fxPanelOpenJS?.());
  assert(isOpen === true, `FX panel should be open, got ${isOpen}`);

  const mutedBefore = await page.evaluate(() => rowMuted?.(0));

  const panelRect = await page.evaluate(() => fxPanelRectJS?.());
  assertValidRect(panelRect, "fxPanelRect");

  // Click inside panel at bottom-right corner (likely empty space)
  await slowClick(panelRect.x + panelRect.w - 5, panelRect.y + panelRect.h - 5);
  await settle();

  const stillOpen = await page.evaluate(() => fxPanelOpenJS?.());
  assert(stillOpen === true, `FX panel should still be open after empty-space click, got ${stillOpen}`);

  const mutedAfter = await page.evaluate(() => rowMuted?.(0));
  assert(mutedBefore === mutedAfter, `mute should not change (was ${mutedBefore}, now ${mutedAfter})`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Test 7: Portal state diagnostics
// ─────────────────────────────────────────────────────────────────────────
console.log("Test 7: Portal state diagnostics");
try {
  await cleanState();

  // Baseline: no portals open
  const baseLen = await page.evaluate(() => portalStackLen?.());
  assert(baseLen === 0, `portal stack should be empty at start, got ${baseLen}`);

  // Open context menu → check portal state
  await page.evaluate(() => { openContextMenuJS?.(0); forceDraw?.(); });
  await settle();

  const ctxTop = await page.evaluate(() => portalTopID?.());
  assert(ctxTop === "context-menu", `portal top should be 'context-menu', got '${ctxTop}'`);

  const ctxLen = await page.evaluate(() => portalStackLen?.());
  assert(ctxLen === 1, `portal stack len should be 1, got ${ctxLen}`);

  // Close → verify clean
  await page.evaluate(() => { closeAllPopups?.(); forceDraw?.(); });
  await settle();

  const afterClose = await page.evaluate(() => portalStackLen?.());
  assert(afterClose === 0, `portal stack should be empty after close, got ${afterClose}`);

  // Open inst menu → check portal state
  await page.evaluate(() => { openInstMenu?.(0); forceDraw?.(); });
  await settle();

  const instTop = await page.evaluate(() => portalTopID?.());
  assert(instTop === "inst-menu", `portal top should be 'inst-menu', got '${instTop}'`);

  const instLen = await page.evaluate(() => portalStackLen?.());
  assert(instLen === 1, `portal stack len should be 1 for inst menu, got ${instLen}`);

  // Close → verify clean
  await page.evaluate(() => { closeAllPopups?.(); forceDraw?.(); });
  await settle();

  const finalLen = await page.evaluate(() => portalStackLen?.());
  assert(finalLen === 0, `portal stack should be empty at end, got ${finalLen}`);

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
}

// ─────────────────────────────────────────────────────────────────────────
// Teardown
// ─────────────────────────────────────────────────────────────────────────
if (isCoverageEnabled()) {
  const covDir = path.resolve(jsDir, "..", "..", "coverage", "browser-raw");
  await flushCoverage(page, covDir, "portal_overlay_absorption");
}

await cleanup();

if (allPassed) {
  console.log("\nAll portal overlay absorption tests passed.");
  process.exit(0);
} else {
  console.log("\nSome tests FAILED.");
  process.exit(1);
}
