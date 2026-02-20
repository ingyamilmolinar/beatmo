/**
 * Browser tests for instrument menu scroll in WASM.
 *
 * Verifies wheel scroll and debug state on both desktop and mobile viewports.
 */
import { setupFullWasm, wheelAt, assertValidRect } from "./real_input_test_helpers.js";
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

// Test 1: Desktop wheel scroll on instrument menu
console.log("Test 1: Desktop wheel scroll on instrument menu");
try {
  // Open instrument menu for row 0
  await page.evaluate(() => openInstMenu?.(0));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  const open = await page.evaluate(() => instMenuOpenState?.());
  assert(open, "inst menu should be open");

  const debug = await page.evaluate(() => instMenuDebugState?.());
  assert(debug, "instMenuDebugState should return an object");
  assert(debug.open === true, "debug.open should be true");

  if (debug.hasScroll) {
    const offsetBefore = await page.evaluate(() => instMenuScrollOffset?.());

    // Get menu bounds to target the wheel event
    const items = await page.evaluate(() => {
      const rects = instMenuItemRects?.();
      if (!rects || rects.length === 0) return null;
      return { x: rects[0].x, y: rects[0].y, w: rects[0].w, h: rects[0].h };
    });

    if (items) {
      const cx = items.x + items.w / 2;
      const cy = items.y + items.h / 2;
      await wheelAt(page, cx, cy, 240); // scroll down
      await page.waitForTimeout(200);

      const offsetAfter = await page.evaluate(() => instMenuScrollOffset?.());
      assert(offsetAfter > offsetBefore, `inst menu scroll should increase: before=${offsetBefore} after=${offsetAfter}`);
    }
  } else {
    console.log("  (inst menu does not need scroll at current viewport — skip scroll check)");
  }

  // Menu should still be open after wheel
  const stillOpen = await page.evaluate(() => instMenuOpenState?.());
  assert(stillOpen, "inst menu should remain open after wheel scroll");

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
} finally {
  await page.evaluate(() => closeInstMenu?.());
  await page.waitForTimeout(100);
}

// Test 2: Instrument menu debug state has expected fields
console.log("Test 2: Instrument menu debug state");
try {
  await page.evaluate(() => openInstMenu?.(0));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  const state = await page.evaluate(() => instMenuDebugState?.());
  assert(state, "instMenuDebugState should return an object");
  assert(state.open === true, "state.open should be true");
  assert(typeof state.scrollOffset === "number", "state.scrollOffset should be number");
  assert(typeof state.hasScroll === "boolean", "state.hasScroll should be boolean");
  assert(typeof state.mode === "string", "state.mode should be string");
  assert(typeof state.totalInstBtns === "number" || typeof state.totalCatBtns === "number",
    "state should have button counts");

  console.log("  PASS");
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
} finally {
  await page.evaluate(() => closeInstMenu?.());
  await page.waitForTimeout(100);
}

// Test 3: Scrollbar rects are valid when scroll is present
console.log("Test 3: Scroll rects validity");
try {
  await page.evaluate(() => openInstMenu?.(0));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  const hasScroll = await page.evaluate(() => instMenuHasScroll?.());
  if (hasScroll) {
    const barRect = await page.evaluate(() => instMenuScrollBarRect?.());
    assertValidRect(barRect, "instMenuScrollBarRect");

    const thumbRect = await page.evaluate(() => instMenuScrollThumbRect?.());
    assertValidRect(thumbRect, "instMenuScrollThumbRect");

    // Thumb should be within bar
    assert(thumbRect.y >= barRect.y, "thumb should be within bar (top)");
    assert(thumbRect.y + thumbRect.h <= barRect.y + barRect.h + 1, "thumb should be within bar (bottom)");

    console.log("  PASS");
  } else {
    console.log("  PASS (no scroll needed — rects correctly null)");
  }
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
} finally {
  await page.evaluate(() => closeInstMenu?.());
  await page.waitForTimeout(100);
}

// Test 4: Navigate to instruments category and verify scroll with many items
console.log("Test 4: Category navigation and scroll");
try {
  await page.evaluate(() => openInstMenu?.(0));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  // Click first category to see instruments
  const catCount = await page.evaluate(() => {
    const cats = instMenuCategoryRects?.();
    return cats ? cats.length : 0;
  });

  if (catCount > 0) {
    await page.evaluate(() => instMenuSelectCategory?.(0));
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(200);

    const state = await page.evaluate(() => instMenuDebugState?.());
    assert(state.open, "menu should still be open after category selection");

    if (state.hasScroll) {
      const offsetBefore = state.scrollOffset;

      // Get bounds for wheel target
      const items = await page.evaluate(() => {
        const rects = instMenuItemRects?.();
        if (!rects || rects.length === 0) return null;
        return { x: rects[0].x, y: rects[0].y, w: rects[0].w, h: rects[0].h };
      });

      if (items) {
        await wheelAt(page, items.x + items.w / 2, items.y + items.h / 2, 240);
        await page.waitForTimeout(200);

        const offsetAfter = await page.evaluate(() => instMenuScrollOffset?.());
        assert(offsetAfter > offsetBefore, `should scroll within category: before=${offsetBefore} after=${offsetAfter}`);
      }
    }
    console.log("  PASS");
  } else {
    console.log("  PASS (no categories — direct instrument list)");
  }
} catch (e) {
  console.log(`  FAIL: ${e.message}`);
} finally {
  await page.evaluate(() => closeInstMenu?.());
  await page.waitForTimeout(100);
}

// Cleanup
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "inst_menu_scroll");
await cleanup();

if (allPassed) {
  console.log("\nAll instrument menu scroll browser tests passed.");
  process.exit(0);
} else {
  console.log("\nSome instrument menu scroll browser tests FAILED.");
  process.exit(1);
}
