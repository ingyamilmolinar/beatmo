/** CDP touch events are "trusted" and behave like real touches, unlike synthetic TouchEvents. */

/** {left, top, width, height, dpr}. */
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

export async function canvasToPageCoords(page, canvasX, canvasY) {
  const info = await getCanvasInfo(page);
  return {
    x: info.left + canvasX,
    y: info.top + canvasY,
  };
}

// Auto-incrementing touch ID to avoid conflicts
let nextTouchId = 1;

/** Canvas-relative coordinates. */
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

    // Hold long enough for WASM game loop to see the touch across multiple frames
    await page.waitForTimeout(150); // ≥1 rAF frame even at ~7fps; keeps total hold under 500ms tap threshold with CDP overhead

    // Touch end
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchEnd',
      touchPoints: [],
    });
  } finally {
    await cdp.detach();
  }
}

/** Canvas-relative; duration default 600 ms. */
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

/** Canvas-relative centre. initDelay (default 80 ms) gives the Go loop 2-3 frames under load to start two-finger tracking
 * before distances change. */
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

    // Before any distance change.
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

/** Canvas-relative. initDelay (default 80 ms) lets the Go loop see the touchStart at the right spot before a move shifts it. */
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

/** Canvas-relative centre; fingerDist default 100. */
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

/** The long-press popup flow: hold until the popup opens, slide to a button (e.g. Delete), release. holdDuration default
 * 700 ms, slideDelay 100 ms. */
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

/** options: expectedGesture, expectedX/Y, tolerance (10 px), timeout (1000 ms). Returns {success, state, error?}. */
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
