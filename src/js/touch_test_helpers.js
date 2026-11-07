/** Synthetic TouchEvents are "untrusted" and may differ from real touches; touch_cdp_helpers.js is more faithful. */

import { cdpTap, cdpLongPress, cdpPinch, cdpDrag, cdpTwoFingerPan } from './touch_cdp_helpers.js';

/** {left, top, width, height, dpr}. */
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

/** Accounts for the canvas position on the page. */
export async function canvasToClientCoords(page, canvasX, canvasY) {
  const rect = await getCanvasRect(page);
  return {
    clientX: rect.left + canvasX,
    clientY: rect.top + canvasY,
    pageX: rect.left + canvasX,
    pageY: rect.top + canvasY,
  };
}

/** Canvas-relative; options.useCDP for trusted events. */
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

/** Canvas-relative; duration default 600 ms; options.useCDP for trusted events. */
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

/** Canvas-relative centre; options.useCDP for trusted events. */
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

/** Canvas-relative centre; fingerDist default 100; options.useCDP for trusted events. */
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

/** Canvas-relative; options.useCDP for trusted events. */
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
