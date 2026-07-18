/**
 * Timeline Seek Real Input Test
 *
 * Task 10 turned the timeline ribbon into a PLAYHEAD SEEK surface: a press
 * on the ribbon snaps to the nearest whole beat under the pointer and
 * commits an immediate seek (see `ribbonStepAt` / `timelineScrubHitAdapter`
 * in `timeline_zone.go`). It is NOT a view-offset scrubber anymore, so this
 * test asserts `playheadStep()` (the absolute predictor/playhead
 * subdivision) moves to a whole-beat boundary on click, rather than
 * `drumOffset()` (which was the old view-scroll behavior and is exercised
 * separately by grid-drag / two-finger-pan touch tests).
 *
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 */

import {
  setupFullWasm,
  clickAndHold,
  assertValidRect,
} from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("timeline_seek_real: Setting up full WASM environment...");
  ({ page, cleanup } = await setupFullWasm());

  // Ensure we have a default path
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Test 1: Get timeline rect and verify it's valid
  // ─────────────────────────────────────────────────────────────────────
  console.log("timeline_seek_real: Getting timeline rect...");

  const timelineRect = await page.evaluate(() => timelineRect?.());
  assertValidRect(timelineRect, "timeline");
  console.log("timeline_seek_real: Timeline rect:", timelineRect);

  // ─────────────────────────────────────────────────────────────────────
  // Test 2: Expand timeline and click ~40% across the ribbon; assert the
  // playhead SEEKS to a whole-beat boundary (not a drumOffset view-scroll).
  // ─────────────────────────────────────────────────────────────────────
  console.log("timeline_seek_real: Expanding timeline...");

  // Expand the timeline so we have room to seek across multiple beats.
  await page.evaluate(() => {
    setTimelineBeats?.(16);
    setDrumLength?.(4);
  });
  await page.waitForTimeout(100);

  const unitsPerBeat = await page.evaluate(() => timelineUnitsPerBeat?.());
  console.log("timeline_seek_real: Units per beat:", unitsPerBeat);
  if (!unitsPerBeat || unitsPerBeat <= 0) {
    throw new Error(`timelineUnitsPerBeat() returned invalid value: ${unitsPerBeat}`);
  }

  const playheadBefore = await page.evaluate(() => playheadStep?.());
  console.log("timeline_seek_real: Playhead step before:", playheadBefore);

  const centerY = timelineRect.y + timelineRect.h / 2;

  // Click ~40% across the timeline ribbon.
  console.log("timeline_seek_real: Clicking ~40% across timeline...");
  const fortyPercentX = timelineRect.x + timelineRect.w * 0.4;
  await clickAndHold(page, fortyPercentX, centerY, 50);
  await page.waitForTimeout(150);

  const playheadAfterForty = await page.evaluate(() => playheadStep?.());
  console.log("timeline_seek_real: Playhead step after 40% click:", playheadAfterForty);

  if (playheadAfterForty === playheadBefore) {
    throw new Error(
      `Expected playheadStep() to change after clicking the ribbon: before=${playheadBefore} after=${playheadAfterForty}`
    );
  }
  if (!playheadAfterForty) {
    throw new Error(`Expected playheadStep() to be non-zero after a ~40% click, got ${playheadAfterForty}`);
  }
  if (playheadAfterForty % unitsPerBeat !== 0) {
    throw new Error(
      `Expected playheadStep() to land on a whole-beat boundary: step=${playheadAfterForty} unitsPerBeat=${unitsPerBeat}`
    );
  }

  // Click near the start of the ribbon: still a whole-beat seek, just to a
  // different (likely lower) beat.
  console.log("timeline_seek_real: Clicking start of timeline...");
  const startX = timelineRect.x + 10;
  await clickAndHold(page, startX, centerY, 50);
  await page.waitForTimeout(150);

  const playheadAfterStart = await page.evaluate(() => playheadStep?.());
  console.log("timeline_seek_real: Playhead step after start click:", playheadAfterStart);

  if (playheadAfterStart % unitsPerBeat !== 0) {
    throw new Error(
      `Expected playheadStep() to land on a whole-beat boundary: step=${playheadAfterStart} unitsPerBeat=${unitsPerBeat}`
    );
  }

  const timelineBeats = await page.evaluate(() => timelineBeats?.());
  const drumLength = await page.evaluate(() => drumLength?.());
  console.log("timeline_seek_real: Timeline beats:", timelineBeats, "Drum length:", drumLength);

  // NOTE: view-offset scrolling (drumOffset()) is no longer driven by the
  // ribbon — it's exercised by grid-drag / two-finger-pan in the touch
  // suite (see touch_gestures.browser.test.js, touch_pinch_no_node.browser.test.js).
  console.log("timeline_seek_real: PASS - Timeline ribbon click seeks the playhead to a whole-beat boundary");
} catch (error) {
  console.error("timeline_seek_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "timeline_seek_real");
  if (cleanup) {
    await cleanup();
  }
}
