/** Pure functions over fullLayoutSnapshot(), shared by the visual parity and regression suites. */

// ─── Rect Utilities ─────────────────────────────────────────────────

export function rectInBounds(rect, canvasW, canvasH) {
  if (!rect || rect.w <= 0 || rect.h <= 0) return false;
  return rect.x >= 0 && rect.y >= 0 &&
    rect.x + rect.w <= canvasW + 2 &&
    rect.y + rect.h <= canvasH + 2;
}

/** Overlap means shared area. */
export function rectsOverlap(a, b) {
  if (!a || !b || a.w <= 0 || a.h <= 0 || b.w <= 0 || b.h <= 0) return false;
  const overlapX = Math.max(0, Math.min(a.x + a.w, b.x + b.w) - Math.max(a.x, b.x));
  const overlapY = Math.max(0, Math.min(a.y + a.h, b.y + b.h) - Math.max(a.y, b.y));
  return overlapX > 1 && overlapY > 1; // 1px overlap tolerance for rounding
}

export function rectContains(outer, inner) {
  if (!outer || !inner) return false;
  return inner.x >= outer.x - 1 &&
    inner.y >= outer.y - 1 &&
    inner.x + inner.w <= outer.x + outer.w + 1 &&
    inner.y + inner.h <= outer.y + outer.h + 1;
}

export function rectMinSize(rect, minSize) {
  if (!rect) return false;
  return rect.w >= minSize && rect.h >= minSize;
}

export function isAbove(a, b) {
  if (!a || !b) return false;
  return a.y + a.h <= b.y + 2; // 2px tolerance
}

export function isLeftOf(a, b) {
  if (!a || !b) return false;
  return a.x + a.w <= b.x + 2; // 2px tolerance
}

// Each returns {ok, errors}.

/** No gap over 2 px. */
export function assertFullCanvasCoverage(snap) {
  const errors = [];
  const { canvasWidth: cw, canvasHeight: ch, gridPane, drumPane, layoutHorizontal } = snap;

  if (!gridPane || !drumPane) {
    errors.push("Missing gridPane or drumPane in snapshot");
    return { ok: false, errors };
  }

  if (layoutHorizontal) {
    // Stacked: grid above drum
    const gridBottom = gridPane.y + gridPane.h;
    const gapY = drumPane.y - gridBottom;
    if (Math.abs(gapY) > 4) {
      errors.push(`Vertical gap between grid and drum pane: ${gapY}px (max 4px)`);
    }
    // Both should span full width
    if (gridPane.w < cw - 4) {
      errors.push(`Grid pane width ${gridPane.w} < canvas width ${cw} - 4`);
    }
    if (drumPane.w < cw - 4) {
      errors.push(`Drum pane width ${drumPane.w} < canvas width ${cw} - 4`);
    }
    // Together should cover full height
    const totalH = gridPane.h + drumPane.h;
    if (totalH < ch - 8) {
      errors.push(`Grid (${gridPane.h}) + Drum (${drumPane.h}) = ${totalH} < canvas height ${ch} - 8`);
    }
  } else {
    // Side-by-side: grid left, drum right
    const gridRight = gridPane.x + gridPane.w;
    const gapX = drumPane.x - gridRight;
    if (Math.abs(gapX) > 4) {
      errors.push(`Horizontal gap between grid and drum pane: ${gapX}px (max 4px)`);
    }
    // Both should span full height
    if (gridPane.h < ch - 4) {
      errors.push(`Grid pane height ${gridPane.h} < canvas height ${ch} - 4`);
    }
    if (drumPane.h < ch - 4) {
      errors.push(`Drum pane height ${drumPane.h} < canvas height ${ch} - 4`);
    }
    // Together should cover full width
    const totalW = gridPane.w + drumPane.w;
    if (totalW < cw - 8) {
      errors.push(`Grid (${gridPane.w}) + Drum (${drumPane.w}) = ${totalW} < canvas width ${cw} - 8`);
    }
  }

  return { ok: errors.length === 0, errors };
}

/** Transport, rack, timeline and wave. */
export function assertNoWidgetOverlaps(snap) {
  const errors = [];
  const w = snap.widgets;
  if (!w) return { ok: true, errors };

  const names = ["transport", "rack", "timeline", "wave"];
  const rects = names.map(n => ({ name: n, rect: w[n] })).filter(r => r.rect && r.rect.w > 0);

  for (let i = 0; i < rects.length; i++) {
    for (let j = i + 1; j < rects.length; j++) {
      if (rectsOverlap(rects[i].rect, rects[j].rect)) {
        errors.push(
          `Widget overlap: ${rects[i].name} (${JSON.stringify(rects[i].rect)}) ` +
          `overlaps ${rects[j].name} (${JSON.stringify(rects[j].rect)})`
        );
      }
    }
  }

  return { ok: errors.length === 0, errors };
}

/** Inside the viewport with positive area. */
export function assertButtonsAccessible(snap) {
  const errors = [];
  const { buttons, canvasWidth: cw, canvasHeight: ch } = snap;
  if (!buttons) return { ok: true, errors };

  for (const [name, rect] of Object.entries(buttons)) {
    if (!rect) continue; // null buttons are optional
    if (rect.w <= 0 || rect.h <= 0) {
      errors.push(`Button "${name}" has zero/negative area: ${rect.w}x${rect.h}`);
      continue;
    }
    if (!rectInBounds(rect, cw, ch)) {
      errors.push(
        `Button "${name}" out of viewport: ` +
        `{x:${rect.x},y:${rect.y},w:${rect.w},h:${rect.h}} vs canvas ${cw}x${ch}`
      );
    }
  }

  return { ok: errors.length === 0, errors };
}

/** The LARGEST dimension, since mute/solo and BPM ± are narrow by design; this catches truly broken controls
 * (zero-sized, collapsed placeholders). Both dimensions under 8 px is a placeholder off the visible scroll and skipped. */
export function assertTouchTargetSizes(snap) {
  const errors = [];
  const dl = snap.drumLayout;
  if (!dl || !dl.isSmallScreen) return { ok: true, errors }; // skip for desktop

  // Catches broken elements, accepts narrow-by-design toggles.
  const minDim = 20;

  // Check buttons
  if (snap.buttons) {
    for (const [name, rect] of Object.entries(snap.buttons)) {
      if (!rect || rect.w <= 0) continue;
      if (Math.max(rect.w, rect.h) < minDim) {
        errors.push(
          `Button "${name}" too small for touch: ${rect.w}x${rect.h} (max dim < ${minDim}px)`
        );
      }
    }
  }

  // Check per-row controls (skip collapsed/placeholder rects)
  if (snap.rows) {
    for (let i = 0; i < snap.rows.length; i++) {
      const row = snap.rows[i];
      if (!row) continue;
      for (const ctrl of ["mute", "solo"]) {
        const rect = row[ctrl];
        if (!rect || rect.w <= 0 || rect.h <= 0) continue;
        // Skip collapsed/placeholder controls (both dims < 8px)
        if (Math.max(rect.w, rect.h) < 8) continue;
        if (Math.max(rect.w, rect.h) < minDim) {
          errors.push(
            `Row ${i} ${ctrl} button too small: ${rect.w}x${rect.h} (max dim < ${minDim}px)`
          );
        }
      }
    }
  }

  return { ok: errors.length === 0, errors };
}

export function assertResponsiveBreakpoints(snap) {
  const errors = [];
  const dl = snap.drumLayout;
  const tc = snap.touchConfig;
  if (!dl || !tc) return { ok: true, errors };

  if (dl.isSmallScreen) {
    // Mobile: should use touch-sized constants
    if (tc.rowHeight < 40) {
      errors.push(`Small screen but rowHeight=${tc.rowHeight} (expected >= 40)`);
    }
    if (tc.minTarget < 40) {
      errors.push(`Small screen but minTarget=${tc.minTarget} (expected >= 40)`);
    }
    // minCellWidth is the zoom-out floor, the same on both screen classes, not a touch size; > 0 still catches a zeroed
    // token.
    if (tc.minCellWidth < 1) {
      errors.push(`Small screen but minCellWidth=${tc.minCellWidth} (expected >= 1)`);
    }
  } else {
    // Desktop: should use desktop constants
    if (tc.minTarget !== 0) {
      errors.push(`Desktop but minTarget=${tc.minTarget} (expected 0)`);
    }
  }

  return { ok: errors.length === 0, errors };
}

/** Stacked: grid above drum, transport above timeline above rows. Side by side: grid left of drum. */
export function assertComponentOrdering(snap) {
  const errors = [];
  const { gridPane, drumPane, layoutHorizontal, widgets } = snap;

  if (!gridPane || !drumPane) return { ok: true, errors };

  if (layoutHorizontal) {
    // Stacked: grid above drum
    if (!isAbove(gridPane, drumPane)) {
      errors.push(
        `Stacked layout but grid not above drum: ` +
        `grid bottom=${gridPane.y + gridPane.h}, drum top=${drumPane.y}`
      );
    }
  } else {
    // Side-by-side: grid left of drum
    if (!isLeftOf(gridPane, drumPane)) {
      errors.push(
        `Side-by-side layout but grid not left of drum: ` +
        `grid right=${gridPane.x + gridPane.w}, drum left=${drumPane.x}`
      );
    }
  }

  // Within drum: transport should be above or at top, timeline below it
  if (widgets?.transport && widgets?.timeline) {
    if (!isAbove(widgets.transport, widgets.timeline) &&
        widgets.transport.y > widgets.timeline.y + 2) {
      errors.push(
        `Transport not above timeline: transport.y=${widgets.transport.y}, ` +
        `timeline.y=${widgets.timeline.y}`
      );
    }
  }

  return { ok: errors.length === 0, errors };
}

/** 25-75 % of the canvas dimension. */
export function assertSplitterRange(snap) {
  const errors = [];
  const { canvasWidth: cw, canvasHeight: ch, layoutHorizontal, splitterY, splitterX } = snap;

  if (layoutHorizontal) {
    if (ch > 0) {
      const pct = (splitterY / ch) * 100;
      if (pct < 15 || pct > 85) {
        errors.push(`Splitter Y at ${pct.toFixed(0)}% of height (expected 15-85%)`);
      }
    }
  } else {
    if (cw > 0) {
      const pct = (splitterX / cw) * 100;
      if (pct < 15 || pct > 85) {
        errors.push(`Splitter X at ${pct.toFixed(0)}% of width (expected 15-85%)`);
      }
    }
  }

  return { ok: errors.length === 0, errors };
}

/** Widget, button and row rects and pane proportions within tolerance px (default 5) of the reference. */
export function assertLayoutParity(reference, device, tolerance = 5) {
  const errors = [];

  if (!reference || !device) {
    errors.push("Missing reference or device snapshot");
    return { ok: false, errors };
  }

  // Compare pane proportions (ratio-based, not absolute px)
  const refGridRatio = reference.canvasHeight > 0
    ? reference.gridPane.h / reference.canvasHeight : 0;
  const devGridRatio = device.canvasHeight > 0
    ? device.gridPane.h / device.canvasHeight : 0;
  const ratioDiff = Math.abs(refGridRatio - devGridRatio);
  if (ratioDiff > 0.15) {
    errors.push(
      `Grid pane height ratio differs: ref=${(refGridRatio * 100).toFixed(0)}% ` +
      `vs device=${(devGridRatio * 100).toFixed(0)}% (diff=${(ratioDiff * 100).toFixed(0)}%)`
    );
  }

  // Compare layout orientation
  if (reference.layoutHorizontal !== device.layoutHorizontal) {
    errors.push(
      `Layout orientation mismatch: ref=${reference.layoutHorizontal ? "stacked" : "side-by-side"} ` +
      `vs device=${device.layoutHorizontal ? "stacked" : "side-by-side"}`
    );
  }

  // Compare isSmallScreen
  if (reference.drumLayout?.isSmallScreen !== device.drumLayout?.isSmallScreen) {
    errors.push(
      `isSmallScreen mismatch: ref=${reference.drumLayout?.isSmallScreen} ` +
      `vs device=${device.drumLayout?.isSmallScreen}`
    );
  }

  // Compare widget rect positions (relative to drum pane)
  if (reference.widgets && device.widgets) {
    for (const name of ["transport", "rack", "timeline", "wave"]) {
      const refR = reference.widgets[name];
      const devR = device.widgets[name];
      if (!refR || !devR) continue;

      // Compare relative positions within drum pane
      const refRelX = refR.x - (reference.drumPane?.x || 0);
      const devRelX = devR.x - (device.drumPane?.x || 0);
      const refRelY = refR.y - (reference.drumPane?.y || 0);
      const devRelY = devR.y - (device.drumPane?.y || 0);

      if (Math.abs(refRelX - devRelX) > tolerance) {
        errors.push(`Widget "${name}" X offset differs: ref=${refRelX} vs device=${devRelX} (tol=${tolerance})`);
      }
      if (Math.abs(refRelY - devRelY) > tolerance) {
        errors.push(`Widget "${name}" Y offset differs: ref=${refRelY} vs device=${devRelY} (tol=${tolerance})`);
      }
      if (Math.abs(refR.w - devR.w) > tolerance) {
        errors.push(`Widget "${name}" width differs: ref=${refR.w} vs device=${devR.w} (tol=${tolerance})`);
      }
      if (Math.abs(refR.h - devR.h) > tolerance) {
        errors.push(`Widget "${name}" height differs: ref=${refR.h} vs device=${devR.h} (tol=${tolerance})`);
      }
    }
  }

  // Compare button presence
  if (reference.buttons && device.buttons) {
    for (const name of ["play", "stop", "bpmInc", "bpmDec", "addRow"]) {
      const refB = reference.buttons[name];
      const devB = device.buttons[name];
      if (refB && !devB) {
        errors.push(`Button "${name}" present in reference but missing on device`);
      }
      if (refB && devB) {
        if (Math.abs(refB.w - devB.w) > tolerance) {
          errors.push(`Button "${name}" width differs: ref=${refB.w} vs device=${devB.w}`);
        }
        if (Math.abs(refB.h - devB.h) > tolerance) {
          errors.push(`Button "${name}" height differs: ref=${refB.h} vs device=${devB.h}`);
        }
      }
    }
  }

  // Compare row count
  const refRows = reference.drumLayout?.numRows || 0;
  const devRows = device.drumLayout?.numRows || 0;
  if (refRows !== devRows) {
    errors.push(`Row count differs: ref=${refRows} vs device=${devRows}`);
  }

  return { ok: errors.length === 0, errors };
}

/** Only the checks the snapshot's content applies to. */
export function runAllLayoutChecks(snap) {
  if (!snap) {
    return { ok: false, errors: ["Snapshot is null/undefined"], results: {} };
  }

  const checks = {
    canvasCoverage: assertFullCanvasCoverage(snap),
    noWidgetOverlaps: assertNoWidgetOverlaps(snap),
    buttonsAccessible: assertButtonsAccessible(snap),
    touchTargetSizes: assertTouchTargetSizes(snap),
    responsiveBreakpoints: assertResponsiveBreakpoints(snap),
    componentOrdering: assertComponentOrdering(snap),
    splitterRange: assertSplitterRange(snap),
  };

  const allErrors = [];
  for (const [name, result] of Object.entries(checks)) {
    for (const err of result.errors) {
      allErrors.push(`[${name}] ${err}`);
    }
  }

  return {
    ok: allErrors.length === 0,
    errors: allErrors,
    results: checks,
  };
}
