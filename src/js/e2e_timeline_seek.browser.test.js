/**
 * Timeline Seek Real Input Test
 *
 * The timeline ribbon is a PLAYHEAD SEEK surface, but it is DRAG-ONLY: a
 * pure tap (press+release on the same beat) commits nothing — only a drag
 * that ends on a DIFFERENT whole beat than where it started emits a seek
 * (see `ribbonStepAt` / `timelineScrubHitAdapter` in `timeline_zone.go`,
 * shipped by the menu-hosted timeline-navigation UX). It is NOT a
 * view-offset scrubber, so this test asserts `playheadStep()` (the absolute
 * predictor/playhead subdivision) moves to a whole-beat boundary after a
 * drag, rather than `drumOffset()` (the old view-scroll behavior, exercised
 * separately by grid-drag / two-finger-pan touch tests).
 *
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 */

import {
  setupFullWasm,
  dragMouse,
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
  // Test 2: Expand timeline and DRAG from the start of the ribbon to ~40%
  // across; assert the playhead SEEKS to a whole-beat boundary (not a
  // drumOffset view-scroll). The ribbon is drag-only, so a plain tap would
  // not commit anything — the gesture must end on a different beat than it
  // started to emit a seek.
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
  const startX = timelineRect.x + 10;
  const fortyPercentX = timelineRect.x + timelineRect.w * 0.4;

  // Drag from the start of the ribbon to ~40% across it. The gesture ends on
  // a different beat than it started, so the ribbon commits one seek on
  // release, snapping the playhead to the nearest whole beat under ~40%.
  console.log("timeline_seek_real: Dragging from start to ~40% across timeline...");
  await dragMouse(page, startX, centerY, fortyPercentX, centerY);
  await page.waitForTimeout(150);

  const playheadAfterForty = await page.evaluate(() => playheadStep?.());
  console.log("timeline_seek_real: Playhead step after 40% drag:", playheadAfterForty);

  if (playheadAfterForty === playheadBefore) {
    throw new Error(
      `Expected playheadStep() to change after dragging the ribbon: before=${playheadBefore} after=${playheadAfterForty}`
    );
  }
  if (!playheadAfterForty) {
    throw new Error(`Expected playheadStep() to be non-zero after a ~40% drag, got ${playheadAfterForty}`);
  }
  if (playheadAfterForty % unitsPerBeat !== 0) {
    throw new Error(
      `Expected playheadStep() to land on a whole-beat boundary: step=${playheadAfterForty} unitsPerBeat=${unitsPerBeat}`
    );
  }

  // Drag back from ~40% to near the start of the ribbon: still a whole-beat
  // seek, just to a different (lower) beat.
  console.log("timeline_seek_real: Dragging back to start of timeline...");
  await dragMouse(page, fortyPercentX, centerY, startX, centerY);
  await page.waitForTimeout(150);

  const playheadAfterStart = await page.evaluate(() => playheadStep?.());
  console.log("timeline_seek_real: Playhead step after start drag:", playheadAfterStart);

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
  console.log("timeline_seek_real: PASS - Timeline ribbon drag seeks the playhead to a whole-beat boundary");
} catch (error) {
  console.error("timeline_seek_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "timeline_seek_real");
  if (cleanup) {
    await cleanup();
  }
}
