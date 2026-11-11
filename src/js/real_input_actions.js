/**
 * Real Input Actions
 *
 * High-level action library for real-input browser tests. Wraps Playwright
 * mouse events with Beatmo's JS exports to provide semantic test actions
 * like "click grid cell", "shift-drag to create edge", etc.
 *
 * Builds on real_input_test_helpers.js for low-level mouse primitives.
 */

import {
  clickAndHold,
  rectCenter,
  assertValidRect,
} from "./real_input_test_helpers.js";

// ─────────────────────────────────────────────────────────────────────
// Grid Interaction
// ─────────────────────────────────────────────────────────────────────

/**
 * Click at a grid position to add or select a node.
 * Uses gridToScreen() for positions without nodes, nodeRect() for existing nodes.
 */
export async function clickGridCell(page, i, j) {
  // Try existing node first for precise centering
  const nr = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
  let x, y;
  if (nr && nr.w > 0) {
    const c = rectCenter(nr);
    x = c.x;
    y = c.y;
  } else {
    const pt = await page.evaluate(([gi, gj]) => gridToScreen?.(gi, gj), [i, j]);
    if (!pt) throw new Error(`gridToScreen(${i}, ${j}) returned null`);
    x = pt.x;
    y = pt.y;
  }
  await clickAndHold(page, x, y, 80);
  await page.waitForTimeout(100);
}

/**
 * Shift+drag between two grid positions to create an edge.
 */
export async function shiftDragEdge(page, i1, j1, i2, j2) {
  // Close any open menu that might consume input
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(50);

  const from = await page.evaluate(([gi, gj]) => {
    const nr = nodeRect?.(gi, gj);
    if (nr) return { x: nr.x + nr.w / 2, y: nr.y + nr.h / 2 };
    return gridToScreen?.(gi, gj);
  }, [i1, j1]);
  const to = await page.evaluate(([gi, gj]) => {
    const nr = nodeRect?.(gi, gj);
    if (nr) return { x: nr.x + nr.w / 2, y: nr.y + nr.h / 2 };
    return gridToScreen?.(gi, gj);
  }, [i2, j2]);
  if (!from || !to) throw new Error(`shiftDragEdge: could not resolve coordinates`);

  // Move to start position first (without any buttons pressed)
  await page.mouse.move(Math.floor(from.x), Math.floor(from.y));
  await page.waitForTimeout(50);

  // Press shift, then mousedown to start the link drag
  await page.keyboard.down("Shift");
  await page.waitForTimeout(50);
  await page.mouse.down();
  await page.waitForTimeout(80);

  // Move through intermediate points to the target
  const steps = 10;
  for (let i = 1; i <= steps; i++) {
    const t = i / steps;
    const x = Math.round(from.x + (to.x - from.x) * t);
    const y = Math.round(from.y + (to.y - from.y) * t);
    await page.mouse.move(x, y);
    await page.waitForTimeout(20);
  }

  // Release mouse while still holding shift (creates the edge)
  await page.mouse.up();
  await page.waitForTimeout(80);
  await page.keyboard.up("Shift");
  await page.waitForTimeout(100);

  // Reset UI state after the drag completes to prevent leftPrev interference.
  // The shift-drag return path in handleEditor doesn't update leftPrev,
  // so the click-release handler may fire spuriously on subsequent drags.
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(100);
}

/**
 * Right-click a node to delete it.
 */
export async function rightClickNode(page, i, j) {
  // Close any menu first
  await page.evaluate(() => closeNodeMenu?.());
  await page.waitForTimeout(50);

  const nr = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
  if (!nr) throw new Error(`No node at (${i}, ${j}) to right-click`);
  const c = rectCenter(nr);

  // Prevent browser context menu from intercepting the right-click
  await page.evaluate(() => {
    document.querySelector("canvas")?.addEventListener("contextmenu", e => e.preventDefault(), { once: true });
  });
  await page.mouse.click(c.x, c.y, { button: "right" });
  await page.waitForTimeout(200);
}

/**
 * Double-click a node to open its popup.
 */
export async function doubleClickNode(page, i, j) {
  const nr = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
  if (!nr) throw new Error(`No node at (${i}, ${j}) to double-click`);
  const c = rectCenter(nr);
  await page.mouse.dblclick(c.x, c.y);
  await page.waitForTimeout(100);
}

// ─────────────────────────────────────────────────────────────────────
// Transport Controls
// ─────────────────────────────────────────────────────────────────────

async function clickBtnRect(page, fnName, label) {
  const rect = await page.evaluate((fn) => globalThis[fn]?.(), fnName);
  assertValidRect(rect, label);
  const c = rectCenter(rect);
  await clickAndHold(page, c.x, c.y, 50);
  await page.waitForTimeout(100);
}

/** Click the Play button. */
export async function clickPlayBtn(page) {
  await clickBtnRect(page, "playBtnRect", "playBtn");
}

/** Click the Stop button. */
export async function clickStopBtn(page) {
  await clickBtnRect(page, "stopBtnRect", "stopBtn");
}

/** Click BPM increment button. */
export async function clickBpmInc(page) {
  await clickBtnRect(page, "bpmIncBtnRect", "bpmIncBtn");
}

/** Click BPM decrement button. */
export async function clickBpmDec(page) {
  await clickBtnRect(page, "bpmDecBtnRect", "bpmDecBtn");
}

/** Click Length increment button. */
export async function clickLenInc(page) {
  await clickBtnRect(page, "lenIncBtnRect", "lenIncBtn");
}

/** Click Length decrement button. */
export async function clickLenDec(page) {
  await clickBtnRect(page, "lenDecBtnRect", "lenDecBtn");
}

/** Click Add Row button. */
export async function clickAddRow(page) {
  await clickBtnRect(page, "addRowBtnRect", "addRowBtn");
}

// ─────────────────────────────────────────────────────────────────────
// Row Controls
// ─────────────────────────────────────────────────────────────────────

/** Click the Mute button for a drum row. */
export async function clickRowMute(page, row) {
  const rect = await page.evaluate((r) => rowMuteBtnRect?.(r), row);
  assertValidRect(rect, `rowMuteBtn(${row})`);
  const c = rectCenter(rect);
  await clickAndHold(page, c.x, c.y, 50);
  await page.waitForTimeout(100);
}

/** Click the Solo button for a drum row. */
export async function clickRowSolo(page, row) {
  const rect = await page.evaluate((r) => rowSoloBtnRect?.(r), row);
  assertValidRect(rect, `rowSoloBtn(${row})`);
  const c = rectCenter(rect);
  await clickAndHold(page, c.x, c.y, 50);
  await page.waitForTimeout(100);
}

// ─────────────────────────────────────────────────────────────────────
// Assertions
// ─────────────────────────────────────────────────────────────────────

/** Assert that a node exists at grid position. */
export async function assertNodeExists(page, i, j, msg) {
  const id = await page.evaluate(([gi, gj]) => nodeIdAt?.(gi, gj), [i, j]);
  if (id === undefined || id === -1) {
    throw new Error(msg || `Expected node at (${i}, ${j}) but none found`);
  }
  return id;
}

/** Assert that no node exists at grid position. */
export async function assertNoNode(page, i, j, msg) {
  const id = await page.evaluate(([gi, gj]) => nodeIdAt?.(gi, gj), [i, j]);
  if (id !== undefined && id !== -1) {
    throw new Error(msg || `Expected no node at (${i}, ${j}) but found id=${id}`);
  }
}

/** Assert playback state. */
export async function assertPlaying(page, expected, msg) {
  const playing = await page.evaluate(() => isPlaying?.());
  if (playing !== expected) {
    throw new Error(msg || `Expected isPlaying=${expected} but got ${playing}`);
  }
}

/** Wait for a condition to become true, polling periodically. */
export async function waitForCondition(page, evalFn, opts = {}) {
  const timeout = opts.timeout ?? 3000;
  const interval = opts.interval ?? 50;
  const start = Date.now();
  while (Date.now() - start < timeout) {
    const result = await page.evaluate(evalFn);
    if (result) return result;
    await page.waitForTimeout(interval);
  }
  throw new Error(opts.message || "waitForCondition timed out");
}

// ─────────────────────────────────────────────────────────────────────
// Pixel Verification (via Playwright screenshot)
// ─────────────────────────────────────────────────────────────────────

/**
 * Capture a screenshot of the canvas and return raw RGBA pixel data.
 * Uses Playwright's screenshot API which works regardless of WebGL
 * preserveDrawingBuffer settings.
 *
 * @returns {{ width: number, height: number, data: Buffer }}
 */
export async function captureCanvasPixels(page) {
  const canvas = page.locator("canvas");
  const buf = await canvas.screenshot({ type: "png" });
  // Decode PNG to raw RGBA using Node.js sharp or manual parsing.
  // Playwright returns a PNG buffer. We use page.evaluate to read
  // pixel data from an off-screen canvas as a simpler approach.
  const result = await page.evaluate(async (pngBase64) => {
    const img = new Image();
    const loaded = new Promise((resolve, reject) => {
      img.onload = resolve;
      img.onerror = reject;
    });
    img.src = "data:image/png;base64," + pngBase64;
    await loaded;
    const c = document.createElement("canvas");
    c.width = img.width;
    c.height = img.height;
    const ctx = c.getContext("2d");
    ctx.drawImage(img, 0, 0);
    const id = ctx.getImageData(0, 0, c.width, c.height);
    // Return dimensions and a subset of pixel data for efficiency.
    // Full data would be too large; callers should use pixelAt instead.
    return { width: id.width, height: id.height };
  }, buf.toString("base64"));
  return { ...result, _buf: buf };
}

/**
 * Read a single pixel from the canvas at (x, y) coordinates.
 * Uses Playwright screenshot + offscreen canvas decode.
 *
 * @returns {{ r: number, g: number, b: number, a: number }}
 */
export async function canvasPixelAt(page, x, y) {
  const canvas = page.locator("canvas");
  const buf = await canvas.screenshot({ type: "png" });
  const pixel = await page.evaluate(async ({ pngBase64, px, py }) => {
    const img = new Image();
    const loaded = new Promise((resolve, reject) => {
      img.onload = resolve;
      img.onerror = reject;
    });
    img.src = "data:image/png;base64," + pngBase64;
    await loaded;
    const c = document.createElement("canvas");
    c.width = img.width;
    c.height = img.height;
    const ctx = c.getContext("2d");
    ctx.drawImage(img, 0, 0);
    const id = ctx.getImageData(px, py, 1, 1);
    return { r: id.data[0], g: id.data[1], b: id.data[2], a: id.data[3] };
  }, { pngBase64: buf.toString("base64"), px: Math.floor(x), py: Math.floor(y) });
  return pixel;
}

/**
 * Assert that a node at grid position (i, j) is visually present on screen.
 * Checks that the pixel at the node's screen center is not the background color.
 */
export async function assertNodeVisible(page, i, j, msg) {
  const nr = await page.evaluate(([gi, gj]) => nodeRect?.(gi, gj), [i, j]);
  if (!nr || nr.w <= 0) {
    throw new Error(msg || `No screen rect for node at (${i}, ${j})`);
  }
  const cx = Math.floor(nr.x + nr.w / 2);
  const cy = Math.floor(nr.y + nr.h / 2);
  const px = await canvasPixelAt(page, cx, cy);
  // Background is approximately (30, 30, 30) — colBGTop.
  const isBg = px.r < 40 && px.g < 40 && px.b < 40;
  if (isBg || px.a < 128) {
    throw new Error(msg || `Node at (${i},${j}) not visible: pixel=(${px.r},${px.g},${px.b},${px.a}) at (${cx},${cy})`);
  }
  return px;
}
