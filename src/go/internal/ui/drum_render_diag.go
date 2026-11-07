package ui

import (
	"fmt"
	"image/color"
)

// The JS canvas watcher matches on hue/saturation only.
func colorToHexRGB(c color.Color) string {
	if c == nil {
		return "#000000"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}

// Instrumentation for a GPU-only artifact (a few drum cells render ~1 px wide) that software renderers never
// show, so the state is captured on a real-GPU build; the canvas watcher in src/js calls slimBarDiag().

// A slim run flanked on BOTH sides by wide runs is the anomaly: the neighbours prove the pitch is wide. A
// uniformly narrow grid (high subdivision) has no wide flankers, so it is never flagged.
const (
	slimBarSlimMax = 2
	slimBarWideMin = 5
)

// Pure, so it is unit-tested and mirrored 1:1 in the JS watcher.
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

// The watcher samples from the geometry and logs the rest when it catches an anomaly, to learn which
// compositing path produced it.
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
