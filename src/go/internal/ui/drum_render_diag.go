package ui

import (
	"fmt"
	"image/color"
)

// colorToHexRGB formats a colour as "#RRGGBB" (alpha dropped) for the JS
// canvas watcher, which matches on hue/saturation only.
func colorToHexRGB(c color.Color) string {
	if c == nil {
		return "#000000"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// drum_render_diag.go — diagnostic instrumentation for the intermittent,
// browser/GPU-only "slim bars" artifact in the drum-row cell grid (a few cells
// render ~1px wide while neighbours are full width, persisting while the
// timeline is free/unlocked). The artifact does not reproduce in software
// renderers (desktop Ebiten, headless SwiftShader), so this captures the exact
// state on a real-GPU build when it occurs. See slimBarDiag() (js export) +
// src/js/drum_slim_bar_watch.js for the canvas-pixel watcher that calls it.

// slimBarSlimMax is the run width (px) at or below which a saturated-colour run
// counts as a "slim" bar. slimBarWideMin is the width at or above which a run
// counts as a full-width cell. A slim run flanked on BOTH sides by wide runs is
// the anomaly: the cell pitch is clearly wide (the neighbours prove it) yet this
// cell collapsed to a sliver. Uniformly narrow grids (high subdivision) have no
// wide flankers, so they are never flagged.
const (
	slimBarSlimMax = 2
	slimBarWideMin = 5
)

// detectSlimBars scans a sequence of saturated-colour run widths (measured along
// one scanline of a drum row) and returns the index of the first slim run that
// is flanked on both sides by full-width runs, or (-1,false) when the row looks
// uniform. Pure and deterministic so it can be unit-tested and mirrored 1:1 in
// the JS canvas watcher.
func detectSlimBars(widths []int) (int, bool) {
	for i := 1; i < len(widths)-1; i++ {
		if widths[i] <= slimBarSlimMax &&
			widths[i-1] >= slimBarWideMin &&
			widths[i+1] >= slimBarWideMin {
			return i, true
		}
	}
	return -1, false
}

// RenderDiag returns the current drum-row render state for the slim-bar
// diagnostic. Surfaced to JS via slimBarDiag(); the canvas watcher uses the
// geometry (cell x-range + per-row yMid/colour) to know where and what to
// sample, and logs the rest (renderPath, offset, gen) when it catches an
// anomaly so we learn which compositing path produced it.
func (dv *DrumView) RenderDiag() map[string]interface{} {
	tl := dv.timelineRect
	rh := dv.rowHeight()
	rows := make([]map[string]interface{}, 0, dv.visibleRows())
	for i := dv.rowOffset; i < dv.rowOffset+dv.visibleRows() && i < len(dv.Rows); i++ {
		yMid := dv.Bounds.Min.Y + dv.headerH + (i-dv.rowOffset)*rh + rh/2
		rows = append(rows, map[string]interface{}{
			"row":   i,
			"yMid":  yMid,
			"color": colorToHexRGB(dv.Rows[i].Color),
		})
	}
	return map[string]interface{}{
		"follow":          dv.FollowPlayback(),
		"offset":          dv.Offset,
		"length":          dv.Length,
		"rowsLayerOffset": dv.rowsLayerOffset,
		"rowsLayerGen":    dv.rowsLayerGen,
		"renderPath":      dv.lastRowsRenderPath,
		"legacyKind":      dv.lastLegacyRebuildKind,
		"cellX0":          tl.Min.X,
		"cellX1":          tl.Max.X,
		"rowHeight":       rh,
		"rows":            rows,
	}
}
