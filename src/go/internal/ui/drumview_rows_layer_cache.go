package ui

import "github.com/hajimehoshi/ebiten/v2"

// rowsLayerCache caches the composition of all visible row sprites (rowCache)
// for the current offset and scroll. Highlights are drawn on top separately.
//
// It is embedded anonymously in DrumView so existing dv.rowsLayer /
// dv.rowsLayerGen / … keep resolving via field promotion; grouping just names
// this cohesive cache and lifts its fields out of the ~590-field DrumView struct
// (no behaviour change). See the drum-rows compositing paths in
// drumview_cache_rows*.go.
type rowsLayerCache struct {
	rowsLayer       *ebiten.Image
	rowsLayerW      int
	rowsLayerH      int
	rowsLayerOffset int
	rowsLayerRowOff int
	rowsLayerBaseX  int
	// rowsLayerRowWidth and rowsLayerLength snapshot the timeline-rect width
	// and step count the composite was built with. The shift-and-fill reuse
	// path copies stale pixels left and only refills a small right strip, so
	// if either dimension changes without the caller setting rowsLayerDirty,
	// the leftmost pixels keep an old cell pitch while the right strip is
	// painted at the new pitch — producing the mixed-pitch artifact users
	// see after long sessions.
	rowsLayerRowWidth int
	rowsLayerLength   int
	rowsLayerGen      int
	// lastRowsRenderPath / lastLegacyRebuildKind record which compositing path
	// produced the most recent drum-row frame, for the slim-bar diagnostic
	// (drum_render_diag.go). lastRowsRenderPath is "windowed" | "legacy" |
	// "direct" | ""; lastLegacyRebuildKind details the legacy branch taken
	// ("full" | "shift" | "overdraw" | "patch-skip" | "stale-accept" | "skip").
	lastRowsRenderPath    string
	lastLegacyRebuildKind string
	rowsLayerDirty        bool
	rowsLayerPadPx        int
	rowsLayerScratch      *ebiten.Image // double-buffer scratch for layer shifts
	rowsLayerBytes        int64
	rowsLayerFrame        int64
}
