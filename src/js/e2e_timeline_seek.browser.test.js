/** The ribbon is a PLAYHEAD seek surface and DRAG-ONLY: a tap commits nothing, a drag ending on another whole beat
 * seeks. So it asserts playheadStep(), not the view scroll drumOffset() (the touch suite covers that). */

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

  console.log("timeline_seek_real: Getting timeline rect...");

  const timelineRect = await page.evaluate(() => timelineRect?.());
  assertValidRect(timelineRect, "timeline");
  console.log("timeline_seek_real: Timeline rect:", timelineRect);

  // A plain tap would commit nothing: the gesture must end on another beat.
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

  // One seek on release, snapped to the whole beat under ~40 %.
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
