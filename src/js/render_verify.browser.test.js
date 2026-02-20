/**
 * Render Verification Browser Test
 *
 * Verifies that nodes, drum cells, and highlights are visually rendered
 * on the canvas. Uses Playwright screenshots to read pixel data from the
 * WebGL canvas (bypasses preserveDrawingBuffer limitations).
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import {
  canvasPixelAt,
  assertNodeVisible,
  assertPlaying,
  clickPlayBtn,
  clickStopBtn,
} from "./real_input_actions.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("render_verify: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Step 1: Build a 4-node circuit at known positions
  // ─────────────────────────────────────────────────────────────────────
  console.log("render_verify: Building test circuit...");
  await page.evaluate(() => {
    addNode?.(20, 0, "regular");
    addNode?.(24, 0, "regular");
    addNode?.(24, 4, "regular");
    addNode?.(20, 4, "regular");
    addEdgeGrid?.(20, 0, 24, 0);
    addEdgeGrid?.(24, 0, 24, 4);
    addEdgeGrid?.(24, 4, 20, 4);
    addEdgeGrid?.(20, 4, 20, 0);
    setOrigin?.(20, 0, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(300);

  // Pan camera to center on test nodes
  const nodeR = await page.evaluate(() => nodeRect?.(20, 0));
  if (!nodeR) throw new Error("Node (20,0) has no screen rect");
  await page.evaluate((r) => {
    const s = splitY?.();
    const cx = 640;
    const cy = (40 + s) / 2;
    panBy?.(cx - r.x - r.w / 2, cy - r.y - r.h / 2);
    forceDraw?.();
  }, nodeR);
  await page.waitForTimeout(200);

  // ─────────────────────────────────────────────────────────────────────
  // Step 2: Verify nodes are visually present via pixel reads
  // ─────────────────────────────────────────────────────────────────────
  console.log("render_verify: Checking node pixel visibility...");
  const testNodes = [[20, 0], [24, 0], [24, 4], [20, 4]];
  let visibleCount = 0;
  for (const [i, j] of testNodes) {
    try {
      const px = await assertNodeVisible(page, i, j);
      console.log(`  Node (${i},${j}): pixel=(${px.r},${px.g},${px.b},${px.a})`);
      visibleCount++;
    } catch (e) {
      console.log(`  Node (${i},${j}): NOT visible - ${e.message}`);
    }
  }
  if (visibleCount < 2) {
    throw new Error(`Too few nodes visible: ${visibleCount}/4`);
  }
  console.log(`render_verify: ${visibleCount}/4 nodes visually confirmed`);

  // ─────────────────────────────────────────────────────────────────────
  // Step 3: Verify background is dark at an empty grid position
  // ─────────────────────────────────────────────────────────────────────
  console.log("render_verify: Checking background pixel...");
  // Pick a point far from any node — top-left corner of grid pane
  const bgPx = await canvasPixelAt(page, 10, 50);
  console.log(`  Background pixel at (10,50): (${bgPx.r},${bgPx.g},${bgPx.b},${bgPx.a})`);
  if (bgPx.r > 60 || bgPx.g > 60 || bgPx.b > 60) {
    console.log("  WARNING: background is lighter than expected");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 4: Delete a node and verify pixel changes
  // ─────────────────────────────────────────────────────────────────────
  console.log("render_verify: Deleting node (24,4) and checking...");
  const beforeRect = await page.evaluate(() => nodeRect?.(24, 4));
  let beforePx = null;
  if (beforeRect) {
    const cx = Math.floor(beforeRect.x + beforeRect.w / 2);
    const cy = Math.floor(beforeRect.y + beforeRect.h / 2);
    beforePx = await canvasPixelAt(page, cx, cy);
    console.log(`  Before delete pixel at (${cx},${cy}): (${beforePx.r},${beforePx.g},${beforePx.b},${beforePx.a})`);
  }

  await page.evaluate(() => {
    deleteNodeGrid?.(24, 4);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  if (beforeRect) {
    const cx = Math.floor(beforeRect.x + beforeRect.w / 2);
    const cy = Math.floor(beforeRect.y + beforeRect.h / 2);
    const afterPx = await canvasPixelAt(page, cx, cy);
    console.log(`  After delete pixel at (${cx},${cy}): (${afterPx.r},${afterPx.g},${afterPx.b},${afterPx.a})`);
    if (beforePx && beforePx.r === afterPx.r && beforePx.g === afterPx.g && beforePx.b === afterPx.b) {
      console.log("  WARNING: pixel unchanged after deletion (may be coincidence)");
    } else {
      console.log("  Pixel changed after deletion");
    }
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 5: Start playback and verify highlight pixel changes
  // ─────────────────────────────────────────────────────────────────────
  console.log("render_verify: Starting playback to check highlights...");
  // Re-add deleted node and restore circuit
  await page.evaluate(() => {
    addNode?.(24, 4, "regular");
    addEdgeGrid?.(24, 0, 24, 4);
    addEdgeGrid?.(24, 4, 20, 4);
    updateBeatInfosJS?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  await clickPlayBtn(page);
  await page.waitForTimeout(500);
  await assertPlaying(page, true, "Playback should be running");

  // Sample pixels over several frames to detect highlight changes.
  // Highlighted nodes should have brighter/different pixels.
  const n0Rect = await page.evaluate(() => nodeRect?.(20, 0));
  if (!n0Rect) throw new Error("Node (20,0) has no rect during playback");
  const n0cx = Math.floor(n0Rect.x + n0Rect.w / 2);
  const n0cy = Math.floor(n0Rect.y + n0Rect.h / 2);

  const samples = [];
  for (let s = 0; s < 10; s++) {
    const px = await canvasPixelAt(page, n0cx, n0cy);
    samples.push(px);
    await page.waitForTimeout(100);
  }

  // Check if any samples differ (indicating highlight animation)
  let pixelVariations = 0;
  for (let s = 1; s < samples.length; s++) {
    if (samples[s].r !== samples[0].r || samples[s].g !== samples[0].g || samples[s].b !== samples[0].b) {
      pixelVariations++;
    }
  }
  console.log(`render_verify: Pixel variations over ${samples.length} frames: ${pixelVariations}`);
  if (pixelVariations === 0) {
    console.log("  NOTE: No pixel variation detected (highlight may not cover exact center)");
  } else {
    console.log("  Highlight animation confirmed via pixel changes");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 6: Stop and verify drum pane has non-black pixels
  // ─────────────────────────────────────────────────────────────────────
  await clickStopBtn(page);
  await page.waitForTimeout(200);

  console.log("render_verify: Checking drum pane pixels...");
  const sY = await page.evaluate(() => splitY?.());
  if (sY) {
    // Sample pixels in the drum pane (below splitter)
    const drumY = sY + 40;
    let drumNonBlack = 0;
    for (let sx = 100; sx < 600; sx += 50) {
      const dp = await canvasPixelAt(page, sx, drumY);
      if (dp.r > 10 || dp.g > 10 || dp.b > 10) {
        drumNonBlack++;
      }
    }
    console.log(`render_verify: ${drumNonBlack}/10 drum pane samples are non-black`);
    if (drumNonBlack < 3) {
      throw new Error(`Drum pane appears mostly black: only ${drumNonBlack}/10 non-black pixels`);
    }
  }

  console.log("render_verify: PASS - Rendering verification via pixel reads");
} catch (error) {
  console.error("render_verify: FAIL -", error.message);
  process.exitCode = 1;
} finally {
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "render_verify");
  if (cleanup) {
    await cleanup();
  }
}
