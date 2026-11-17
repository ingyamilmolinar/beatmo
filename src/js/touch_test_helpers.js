/**
 * Touch simulation utilities for Playwright tests.
 * These helpers simulate touch events that map to Ebiten's touch APIs.
 *
 * NOTE: Synthetic TouchEvents dispatched via JavaScript are "untrusted" and may
 * not behave identically to real touch events on mobile devices. For more
 * accurate testing, consider using CDP helpers (touch_cdp_helpers.js).
 */

import { cdpTap, cdpLongPress, cdpPinch, cdpDrag, cdpTwoFingerPan } from './touch_cdp_helpers.js';

/**
 * Gets canvas position and DPR for coordinate adjustments.
 * @param {import('playwright').Page} page - Playwright page
 * @returns {Promise<{left: number, top: number, width: number, height: number, dpr: number}>}
 */
export async function getCanvasRect(page) {
  return await page.evaluate(() => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');
    const rect = canvas.getBoundingClientRect();
    return {
      left: rect.left,
      top: rect.top,
      width: rect.width,
      height: rect.height,
      dpr: window.devicePixelRatio || 1,
    };
  });
}

/**
 * Converts canvas-relative coordinates to client coordinates for TouchEvent.
 * Accounts for canvas position on the page.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} canvasX - X coordinate relative to canvas
 * @param {number} canvasY - Y coordinate relative to canvas
 * @returns {Promise<{clientX: number, clientY: number, pageX: number, pageY: number}>}
 */
export async function canvasToClientCoords(page, canvasX, canvasY) {
  const rect = await getCanvasRect(page);
  return {
    clientX: rect.left + canvasX,
    clientY: rect.top + canvasY,
    pageX: rect.left + canvasX,
    pageY: rect.top + canvasY,
  };
}

/**
 * Simulates a tap gesture (touchstart + touchend in quick succession).
 * Coordinates are canvas-relative and will be adjusted for canvas position.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} x - X coordinate (canvas-relative)
 * @param {number} y - Y coordinate (canvas-relative)
 * @param {object} [options] - Options
 * @param {boolean} [options.useCDP] - Use CDP for trusted events (default false)
 */
export async function simulateTap(page, x, y, options = {}) {
  if (options.useCDP) {
    return cdpTap(page, x, y);
  }

  const coords = await canvasToClientCoords(page, x, y);

  await page.evaluate(({ clientX, clientY, pageX, pageY }) => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');

    const touch = new Touch({
      identifier: Date.now(),
      target: canvas,
      clientX,
      clientY,
      pageX,
      pageY,
    });

    canvas.dispatchEvent(new TouchEvent('touchstart', {
      bubbles: true,
      cancelable: true,
      touches: [touch],
      targetTouches: [touch],
      changedTouches: [touch],
    }));

    canvas.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      targetTouches: [],
      changedTouches: [touch],
    }));
  }, coords);
}

/**
 * Simulates a long press gesture (touchstart, wait, touchend).
 * Coordinates are canvas-relative and will be adjusted for canvas position.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} x - X coordinate (canvas-relative)
 * @param {number} y - Y coordinate (canvas-relative)
 * @param {number} duration - Hold duration in ms (default 600ms)
 * @param {object} [options] - Options
 * @param {boolean} [options.useCDP] - Use CDP for trusted events (default false)
 */
export async function simulateLongPress(page, x, y, duration = 600, options = {}) {
  if (options.useCDP) {
    return cdpLongPress(page, x, y, duration);
  }

  const coords = await canvasToClientCoords(page, x, y);

  const touchId = await page.evaluate(({ clientX, clientY, pageX, pageY }) => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');

    const id = Date.now();
    const touch = new Touch({
      identifier: id,
      target: canvas,
      clientX,
      clientY,
      pageX,
      pageY,
    });

    canvas.dispatchEvent(new TouchEvent('touchstart', {
      bubbles: true,
      cancelable: true,
      touches: [touch],
      targetTouches: [touch],
      changedTouches: [touch],
    }));

    return id;
  }, coords);

  // Wait for long press duration
  await page.waitForTimeout(duration);

  // End the touch
  await page.evaluate(({ clientX, clientY, pageX, pageY, touchId }) => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');

    const touch = new Touch({
      identifier: touchId,
      target: canvas,
      clientX,
      clientY,
      pageX,
      pageY,
    });

    canvas.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      targetTouches: [],
      changedTouches: [touch],
    }));
  }, { ...coords, touchId });
}

/**
 * Simulates a pinch gesture (two touches with changing distance).
 * Coordinates are canvas-relative and will be adjusted for canvas position.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} cx - Center X coordinate (canvas-relative)
 * @param {number} cy - Center Y coordinate (canvas-relative)
 * @param {number} startDist - Initial distance between fingers
 * @param {number} endDist - Final distance between fingers
 * @param {number} steps - Number of intermediate steps (default 10)
 * @param {object} [options] - Options
 * @param {boolean} [options.useCDP] - Use CDP for trusted events (default false)
 */
export async function simulatePinch(page, cx, cy, startDist, endDist, steps = 10, options = {}) {
  if (options.useCDP) {
    return cdpPinch(page, cx, cy, startDist, endDist, steps);
  }

  const rect = await getCanvasRect(page);
  const pageCx = rect.left + cx;
  const pageCy = rect.top + cy;

  await page.evaluate(({ cx, cy, startDist, endDist, steps }) => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');

    const id1 = Date.now();
    const id2 = id1 + 1;

    function createTouches(dist) {
      const halfDist = dist / 2;
      return [
        new Touch({
          identifier: id1,
          target: canvas,
          clientX: cx - halfDist,
          clientY: cy,
          pageX: cx - halfDist,
          pageY: cy,
        }),
        new Touch({
          identifier: id2,
          target: canvas,
          clientX: cx + halfDist,
          clientY: cy,
          pageX: cx + halfDist,
          pageY: cy,
        }),
      ];
    }

    // Start with initial distance
    let touches = createTouches(startDist);
    canvas.dispatchEvent(new TouchEvent('touchstart', {
      bubbles: true,
      cancelable: true,
      touches: touches,
      targetTouches: touches,
      changedTouches: touches,
    }));

    // Animate the pinch
    for (let i = 1; i <= steps; i++) {
      const progress = i / steps;
      const dist = startDist + (endDist - startDist) * progress;
      touches = createTouches(dist);

      canvas.dispatchEvent(new TouchEvent('touchmove', {
        bubbles: true,
        cancelable: true,
        touches: touches,
        targetTouches: touches,
        changedTouches: touches,
      }));
    }

    // End touches
    canvas.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      targetTouches: [],
      changedTouches: touches,
    }));
  }, { cx: pageCx, cy: pageCy, startDist, endDist, steps });
}

/**
 * Simulates a two-finger pan gesture.
 * Coordinates are canvas-relative and will be adjusted for canvas position.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} startX - Starting center X (canvas-relative)
 * @param {number} startY - Starting center Y (canvas-relative)
 * @param {number} deltaX - Horizontal movement
 * @param {number} deltaY - Vertical movement
 * @param {number} fingerDist - Distance between fingers (default 100)
 * @param {number} steps - Number of intermediate steps (default 10)
 * @param {object} [options] - Options
 * @param {boolean} [options.useCDP] - Use CDP for trusted events (default false)
 */
export async function simulateTwoFingerPan(page, startX, startY, deltaX, deltaY, fingerDist = 100, steps = 10, options = {}) {
  if (options.useCDP) {
    return cdpTwoFingerPan(page, startX, startY, deltaX, deltaY, fingerDist, steps);
  }

  const rect = await getCanvasRect(page);
  const pageStartX = rect.left + startX;
  const pageStartY = rect.top + startY;

  await page.evaluate(({ startX, startY, deltaX, deltaY, fingerDist, steps }) => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');

    const id1 = Date.now();
    const id2 = id1 + 1;
    const halfDist = fingerDist / 2;

    function createTouches(cx, cy) {
      return [
        new Touch({
          identifier: id1,
          target: canvas,
          clientX: cx - halfDist,
          clientY: cy,
          pageX: cx - halfDist,
          pageY: cy,
        }),
        new Touch({
          identifier: id2,
          target: canvas,
          clientX: cx + halfDist,
          clientY: cy,
          pageX: cx + halfDist,
          pageY: cy,
        }),
      ];
    }

    // Start touches
    let touches = createTouches(startX, startY);
    canvas.dispatchEvent(new TouchEvent('touchstart', {
      bubbles: true,
      cancelable: true,
      touches: touches,
      targetTouches: touches,
      changedTouches: touches,
    }));

    // Animate the pan
    for (let i = 1; i <= steps; i++) {
      const progress = i / steps;
      const cx = startX + deltaX * progress;
      const cy = startY + deltaY * progress;
      touches = createTouches(cx, cy);

      canvas.dispatchEvent(new TouchEvent('touchmove', {
        bubbles: true,
        cancelable: true,
        touches: touches,
        targetTouches: touches,
        changedTouches: touches,
      }));
    }

    // End touches
    canvas.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      targetTouches: [],
      changedTouches: touches,
    }));
  }, { startX: pageStartX, startY: pageStartY, deltaX, deltaY, fingerDist, steps });
}

/**
 * Simulates a single-finger drag gesture.
 * Coordinates are canvas-relative and will be adjusted for canvas position.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} startX - Starting X (canvas-relative)
 * @param {number} startY - Starting Y (canvas-relative)
 * @param {number} endX - Ending X (canvas-relative)
 * @param {number} endY - Ending Y (canvas-relative)
 * @param {number} steps - Number of intermediate steps (default 10)
 * @param {object} [options] - Options
 * @param {boolean} [options.useCDP] - Use CDP for trusted events (default false)
 */
export async function simulateDrag(page, startX, startY, endX, endY, steps = 10, options = {}) {
  if (options.useCDP) {
    return cdpDrag(page, startX, startY, endX, endY, steps);
  }

  const rect = await getCanvasRect(page);
  const pageStartX = rect.left + startX;
  const pageStartY = rect.top + startY;
  const pageEndX = rect.left + endX;
  const pageEndY = rect.top + endY;

  await page.evaluate(({ startX, startY, endX, endY, steps }) => {
    const canvas = document.querySelector('canvas');
    if (!canvas) throw new Error('Canvas not found');

    const id = Date.now();

    function createTouch(x, y) {
      return new Touch({
        identifier: id,
        target: canvas,
        clientX: x,
        clientY: y,
        pageX: x,
        pageY: y,
      });
    }

    // Start touch
    let touch = createTouch(startX, startY);
    canvas.dispatchEvent(new TouchEvent('touchstart', {
      bubbles: true,
      cancelable: true,
      touches: [touch],
      targetTouches: [touch],
      changedTouches: [touch],
    }));

    // Animate the drag
    for (let i = 1; i <= steps; i++) {
      const progress = i / steps;
      const x = startX + (endX - startX) * progress;
      const y = startY + (endY - startY) * progress;
      touch = createTouch(x, y);

      canvas.dispatchEvent(new TouchEvent('touchmove', {
        bubbles: true,
        cancelable: true,
        touches: [touch],
        targetTouches: [touch],
        changedTouches: [touch],
      }));
    }

    // End touch
    touch = createTouch(endX, endY);
    canvas.dispatchEvent(new TouchEvent('touchend', {
      bubbles: true,
      cancelable: true,
      touches: [],
      targetTouches: [],
      changedTouches: [touch],
    }));
  }, { startX: pageStartX, startY: pageStartY, endX: pageEndX, endY: pageEndY, steps });
}

// Re-export CDP helpers for tests that want to use them directly
export { cdpTap, cdpLongPress, cdpPinch, cdpDrag, cdpTwoFingerPan, getCanvasInfo, verifyTouchReceived } from './touch_cdp_helpers.js';
