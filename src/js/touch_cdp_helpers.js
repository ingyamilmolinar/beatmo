/**
 * Chrome DevTools Protocol touch simulation utilities for Playwright tests.
 * CDP events are "trusted" and behave like real touches, unlike synthetic TouchEvents.
 */

/**
 * Gets canvas coordinates accounting for position and device pixel ratio.
 * @param {import('playwright').Page} page - Playwright page
 * @returns {Promise<{left: number, top: number, width: number, height: number, dpr: number}>}
 */
export async function getCanvasInfo(page) {
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
 * Converts canvas-relative coordinates to page coordinates.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} canvasX - X coordinate relative to canvas
 * @param {number} canvasY - Y coordinate relative to canvas
 * @returns {Promise<{x: number, y: number}>}
 */
export async function canvasToPageCoords(page, canvasX, canvasY) {
  const info = await getCanvasInfo(page);
  return {
    x: info.left + canvasX,
    y: info.top + canvasY,
  };
}

// Auto-incrementing touch ID to avoid conflicts
let nextTouchId = 1;

/**
 * Simulates a tap using CDP Input.dispatchTouchEvent.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} x - X coordinate (canvas-relative)
 * @param {number} y - Y coordinate (canvas-relative)
 */
export async function cdpTap(page, x, y) {
  const cdp = await page.context().newCDPSession(page);
  const coords = await canvasToPageCoords(page, x, y);
  const touchId = nextTouchId++;

  try {
    // Touch start
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{
        x: coords.x,
        y: coords.y,
        id: touchId,
      }],
    });

    // Very short delay for tap - just enough for event to register
    await page.waitForTimeout(16); // ~1 frame at 60fps

    // Touch end
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/**
 * Simulates a long press using CDP Input.dispatchTouchEvent.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} x - X coordinate (canvas-relative)
 * @param {number} y - Y coordinate (canvas-relative)
 * @param {number} duration - Hold duration in ms (default 600ms)
 */
export async function cdpLongPress(page, x, y, duration = 600) {
  const cdp = await page.context().newCDPSession(page);
  const coords = await canvasToPageCoords(page, x, y);

  try {
    // Touch start
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{
        x: coords.x,
        y: coords.y,
        id: 1,
      }],
    });

    // Wait for long press duration
    await page.waitForTimeout(duration);

    // Touch end
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/**
 * Simulates a pinch gesture using CDP Input.dispatchTouchEvent.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} cx - Center X coordinate (canvas-relative)
 * @param {number} cy - Center Y coordinate (canvas-relative)
 * @param {number} startDist - Initial distance between fingers
 * @param {number} endDist - Final distance between fingers
 * @param {number} steps - Number of intermediate steps (default 10)
 * @param {number} stepDelay - Delay between steps in ms (default 20)
 * @param {number} initDelay - Delay after touchStart before first touchMove (default 80ms).
 *   The Go game loop needs at least one frame to initialize two-finger tracking
 *   before seeing distance changes. Under CPU load (~30fps), 80ms guarantees
 *   2-3 frames of initialization time.
 */
export async function cdpPinch(page, cx, cy, startDist, endDist, steps = 10, stepDelay = 20, initDelay = 80) {
  const cdp = await page.context().newCDPSession(page);
  const info = await getCanvasInfo(page);
  const centerX = info.left + cx;
  const centerY = info.top + cy;

  function getTouchPoints(dist) {
    const halfDist = dist / 2;
    return [
      { x: centerX - halfDist, y: centerY, id: 1 },
      { x: centerX + halfDist, y: centerY, id: 2 },
    ];
  }

  try {
    // Touch start with both fingers
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: getTouchPoints(startDist),
    });

    // Wait for Go loop to process the two-finger initialization frame
    // before sending distance changes.
    if (initDelay > 0) {
      await page.waitForTimeout(initDelay);
    }

    // Animate the pinch
    for (let i = 1; i <= steps; i++) {
      const progress = i / steps;
      const dist = startDist + (endDist - startDist) * progress;

      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchMove',
        touchPoints: getTouchPoints(dist),
      });

      if (stepDelay > 0 && i < steps) {
        await page.waitForTimeout(stepDelay);
      }
    }

    // Touch end
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/**
 * Simulates a drag gesture using CDP Input.dispatchTouchEvent.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} startX - Starting X coordinate (canvas-relative)
 * @param {number} startY - Starting Y coordinate (canvas-relative)
 * @param {number} endX - Ending X coordinate (canvas-relative)
 * @param {number} endY - Ending Y coordinate (canvas-relative)
 * @param {number} steps - Number of intermediate steps (default 10)
 * @param {number} stepDelay - Delay between steps in ms (default 20)
 * @param {number} initDelay - Delay after touchStart before first touchMove (default 80ms).
 *   The Go game loop needs at least one frame to process the touchStart at the
 *   correct position before touchMove events shift it. Under CPU load (~30fps),
 *   80ms guarantees 2-3 frames of initialization time.
 */
export async function cdpDrag(page, startX, startY, endX, endY, steps = 10, stepDelay = 20, initDelay = 80) {
  const cdp = await page.context().newCDPSession(page);
  const info = await getCanvasInfo(page);

  const pageStartX = info.left + startX;
  const pageStartY = info.top + startY;
  const pageEndX = info.left + endX;
  const pageEndY = info.top + endY;

  try {
    // Touch start
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{
        x: pageStartX,
        y: pageStartY,
        id: 1,
      }],
    });

    // Wait for game loop to process touchStart at correct position
    if (initDelay > 0) {
      await page.waitForTimeout(initDelay);
    }

    // Animate the drag
    for (let i = 1; i <= steps; i++) {
      const progress = i / steps;
      const x = pageStartX + (pageEndX - pageStartX) * progress;
      const y = pageStartY + (pageEndY - pageStartY) * progress;

      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchMove',
        touchPoints: [{
          x,
          y,
          id: 1,
        }],
      });

      if (stepDelay > 0 && i < steps) {
        await page.waitForTimeout(stepDelay);
      }
    }

    // Touch end
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/**
 * Simulates a two-finger pan gesture using CDP Input.dispatchTouchEvent.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} startX - Starting center X coordinate (canvas-relative)
 * @param {number} startY - Starting center Y coordinate (canvas-relative)
 * @param {number} deltaX - Horizontal movement
 * @param {number} deltaY - Vertical movement
 * @param {number} fingerDist - Distance between fingers (default 100)
 * @param {number} steps - Number of intermediate steps (default 10)
 * @param {number} stepDelay - Delay between steps in ms (default 20)
 */
export async function cdpTwoFingerPan(page, startX, startY, deltaX, deltaY, fingerDist = 100, steps = 10, stepDelay = 20) {
  const cdp = await page.context().newCDPSession(page);
  const info = await getCanvasInfo(page);

  const pageCenterX = info.left + startX;
  const pageCenterY = info.top + startY;
  const halfDist = fingerDist / 2;

  function getTouchPoints(cx, cy) {
    return [
      { x: cx - halfDist, y: cy, id: 1 },
      { x: cx + halfDist, y: cy, id: 2 },
    ];
  }

  try {
    // Touch start with both fingers
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: getTouchPoints(pageCenterX, pageCenterY),
    });

    // Animate the pan
    for (let i = 1; i <= steps; i++) {
      const progress = i / steps;
      const cx = pageCenterX + deltaX * progress;
      const cy = pageCenterY + deltaY * progress;

      await cdp.send('Input.dispatchTouchEvent', {
        type: 'touchMove',
        touchPoints: getTouchPoints(cx, cy),
      });

      if (stepDelay > 0 && i < steps) {
        await page.waitForTimeout(stepDelay);
      }
    }

    // Touch end
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/**
 * Simulates a long press followed by a slide to a target position and release.
 * Used for the long-press popup workflow where the user long-presses a node,
 * the popup appears, and then the user slides to a button (e.g., Delete) and releases.
 * @param {import('playwright').Page} page - Playwright page
 * @param {number} x - Start X coordinate (canvas-relative)
 * @param {number} y - Start Y coordinate (canvas-relative)
 * @param {number} targetX - Target X coordinate to slide to (canvas-relative)
 * @param {number} targetY - Target Y coordinate to slide to (canvas-relative)
 * @param {number} holdDuration - How long to hold before sliding in ms (default 700)
 * @param {number} slideDelay - How long to hold on target before releasing in ms (default 100)
 */
export async function cdpLongPressAndSlide(page, x, y, targetX, targetY, holdDuration = 700, slideDelay = 100) {
  const cdp = await page.context().newCDPSession(page);
  const coordsStart = await canvasToPageCoords(page, x, y);
  const coordsEnd = await canvasToPageCoords(page, targetX, targetY);
  try {
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchStart',
      touchPoints: [{ x: coordsStart.x, y: coordsStart.y, id: 1 }],
    });
    await page.waitForTimeout(holdDuration);
    // Slide to target
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: coordsEnd.x, y: coordsEnd.y, id: 1 }],
    });
    await page.waitForTimeout(slideDelay);
    // Release
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/**
 * Verifies that a touch was received by the Go layer.
 * @param {import('playwright').Page} page - Playwright page
 * @param {object} options - Options for verification
 * @param {string} [options.expectedGesture] - Expected gesture type (tap, longPress, etc.)
 * @param {number} [options.expectedX] - Expected X coordinate (approximate)
 * @param {number} [options.expectedY] - Expected Y coordinate (approximate)
 * @param {number} [options.tolerance] - Coordinate tolerance in pixels (default 10)
 * @param {number} [options.timeout] - Timeout in ms to wait for gesture (default 1000)
 * @returns {Promise<{success: boolean, state: object, error?: string}>}
 */
export async function verifyTouchReceived(page, options = {}) {
  const { expectedGesture, expectedX, expectedY, tolerance = 10, timeout = 1000 } = options;

  const startTime = Date.now();
  let lastState = null;

  while (Date.now() - startTime < timeout) {
    lastState = await page.evaluate(() => {
      if (typeof touchDebugState !== 'function') {
        return { error: 'touchDebugState not available' };
      }
      return touchDebugState();
    });

    if (lastState.error) {
      return { success: false, state: lastState, error: lastState.error };
    }

    // Check if we got the expected gesture
    if (expectedGesture && lastState.lastGesture === expectedGesture) {
      // Check coordinates if provided
      if (expectedX !== undefined && expectedY !== undefined) {
        const dx = Math.abs(lastState.lastPos.x - expectedX);
        const dy = Math.abs(lastState.lastPos.y - expectedY);
        if (dx <= tolerance && dy <= tolerance) {
          return { success: true, state: lastState };
        }
        // Coordinates don't match yet, keep waiting
      } else {
        // No coordinate check needed
        return { success: true, state: lastState };
      }
    }

    await page.waitForTimeout(50);
  }

  return {
    success: false,
    state: lastState,
    error: `Timeout waiting for gesture. Expected: ${expectedGesture}, Got: ${lastState?.lastGesture}`,
  };
}
