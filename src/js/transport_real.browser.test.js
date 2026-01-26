/**
 * Transport Controls Real Input Test
 *
 * Tests that Play/Stop buttons and BPM controls work via real canvas clicks.
 * Uses the FULL WASM build (main.wasm) with Playwright's native mouse methods.
 */

import {
  setupFullWasm,
  clickAndHold,
  rectCenter,
  assertValidRect,
} from "./real_input_test_helpers.js";

let cleanup;

try {
  console.log("transport_real: Setting up full WASM environment...");
  const { page, cleanup: cleanupFn } = await setupFullWasm();
  cleanup = cleanupFn;

  // Ensure we have a default path for playback
  await page.evaluate(() => {
    ensureDefaultPath?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Test 1: Play button
  // ─────────────────────────────────────────────────────────────────────
  console.log("transport_real: Testing Play button...");

  const playRect = await page.evaluate(() => playBtnRect?.());
  assertValidRect(playRect, "playBtn");
  console.log("transport_real: Play button rect:", playRect);

  const playingBefore = await page.evaluate(() => isPlaying?.());
  console.log("transport_real: Playing before:", playingBefore);

  // Click play
  const playCenter = rectCenter(playRect);
  await clickAndHold(page, playCenter.x, playCenter.y, 50);
  await page.waitForTimeout(150);

  const playingAfterPlay = await page.evaluate(() => isPlaying?.());
  console.log("transport_real: Playing after play click:", playingAfterPlay);

  if (!playingAfterPlay) {
    throw new Error("Play button did NOT start playback.");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Test 2: Stop button
  // ─────────────────────────────────────────────────────────────────────
  console.log("transport_real: Testing Stop button...");

  const stopRect = await page.evaluate(() => stopBtnRect?.());
  assertValidRect(stopRect, "stopBtn");
  console.log("transport_real: Stop button rect:", stopRect);

  const stopCenter = rectCenter(stopRect);
  await clickAndHold(page, stopCenter.x, stopCenter.y, 50);
  await page.waitForTimeout(150);

  const playingAfterStop = await page.evaluate(() => isPlaying?.());
  console.log("transport_real: Playing after stop click:", playingAfterStop);

  if (playingAfterStop) {
    throw new Error("Stop button did NOT stop playback.");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Test 3: BPM increment button
  // ─────────────────────────────────────────────────────────────────────
  console.log("transport_real: Testing BPM increment button...");

  const bpmIncRect = await page.evaluate(() => bpmIncBtnRect?.());
  assertValidRect(bpmIncRect, "bpmIncBtn");
  console.log("transport_real: BPM+ button rect:", bpmIncRect);

  const bpmBefore = await page.evaluate(() => getBPM?.());
  console.log("transport_real: BPM before:", bpmBefore);

  // Click BPM+ multiple times
  const bpmIncCenter = rectCenter(bpmIncRect);
  for (let i = 0; i < 3; i++) {
    await clickAndHold(page, bpmIncCenter.x, bpmIncCenter.y, 50);
    await page.waitForTimeout(80);
  }

  const bpmAfter = await page.evaluate(() => getBPM?.());
  console.log("transport_real: BPM after:", bpmAfter);

  if (bpmAfter <= bpmBefore) {
    throw new Error(
      `BPM did NOT increase. Before: ${bpmBefore}, After: ${bpmAfter}`
    );
  }

  // ─────────────────────────────────────────────────────────────────────
  // Test 4: BPM decrement button
  // ─────────────────────────────────────────────────────────────────────
  console.log("transport_real: Testing BPM decrement button...");

  const bpmDecRect = await page.evaluate(() => bpmDecBtnRect?.());
  assertValidRect(bpmDecRect, "bpmDecBtn");
  console.log("transport_real: BPM- button rect:", bpmDecRect);

  const bpmBeforeDec = await page.evaluate(() => getBPM?.());

  const bpmDecCenter = rectCenter(bpmDecRect);
  for (let i = 0; i < 3; i++) {
    await clickAndHold(page, bpmDecCenter.x, bpmDecCenter.y, 50);
    await page.waitForTimeout(80);
  }

  const bpmAfterDec = await page.evaluate(() => getBPM?.());
  console.log("transport_real: BPM after dec:", bpmAfterDec);

  if (bpmAfterDec >= bpmBeforeDec) {
    throw new Error(
      `BPM did NOT decrease. Before: ${bpmBeforeDec}, After: ${bpmAfterDec}`
    );
  }

  console.log("transport_real: PASS - All transport controls work via real canvas clicks");
} catch (error) {
  console.error("transport_real: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (cleanup) {
    await cleanup();
  }
}
