package ui

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (dv *DrumView) setTimelineSegments(row, offset int, past []bool, pastTypes []model.NodeType, present, future []bool) {
	if row < 0 || row >= len(dv.Rows) {
		return
	}
	dv.ensureRowCache()
	dv.timelineOffset[row] = offset
	copyBoolSliceInto(&dv.timelinePast[row], past)
	copyNodeTypeSliceInto(&dv.timelinePastTypes[row], pastTypes)
	copyBoolSliceInto(&dv.timelinePresent[row], present)
	copyBoolSliceInto(&dv.timelineFuture[row], future)
}

func (dv *DrumView) markAllRowsDirty() {
	dv.ensureRowCache()
	for i := range dv.rowDirty {
		dv.rowDirty[i] = true
	}
	for i := range dv.rowFullDirty {
		dv.rowFullDirty[i] = true
	}
	dv.rowsLayerDirty = true
	dv.rowsWinContentDirty = true // content changed → windowed cache must re-bake
}

// markRowsShiftDirty invalidates cached sprites for offset shifts while
// allowing incremental reuse (no forced full rebuild).
func (dv *DrumView) markRowsShiftDirty() {
	dv.ensureRowCache()
	for i := range dv.rowDirty {
		dv.rowDirty[i] = true
	}
	dv.rowsLayerDirty = true
}

// markRowCellsDirty marks a row as needing rebuild but allows the cheap
// cell-patch path (does NOT set rowFullDirty). Use for small content changes
// like playback beat advances where only a few cells changed.
func (dv *DrumView) markRowCellsDirty(i int) {
	dv.ensureRowCache()
	if i >= 0 && i < len(dv.rowDirty) {
		dv.rowDirty[i] = true
	}
	dv.rowsLayerDirty = true
	dv.rowsWinContentDirty = true // cell content changed → windowed cache re-bake
}

// markRowDirty invalidates the cached sprite for a single row. Safe for
// concurrent callers on the UI thread (Game.Update/Draw) which is the only
// place DrumView is mutated.
func (dv *DrumView) markRowDirty(i int) {
	dv.ensureRowCache()
	if i >= 0 && i < len(dv.rowDirty) {
		dv.rowDirty[i] = true
		if i < len(dv.rowFullDirty) {
			dv.rowFullDirty[i] = true
		}
	}
	dv.rowsLayerDirty = true
	dv.rowsWinContentDirty = true // row content changed → windowed cache re-bake
}

// markRowShiftDirty invalidates the cached sprite for a single row but allows
// incremental reuse (no forced full rebuild).
func (dv *DrumView) markRowShiftDirty(i int) {
	dv.ensureRowCache()
	if i >= 0 && i < len(dv.rowDirty) {
		dv.rowDirty[i] = true
		if i < len(dv.rowFullDirty) {
			dv.rowFullDirty[i] = false
		}
	}
	dv.rowsLayerDirty = true
}

func (dv *DrumView) invalidateRowCaches() {
	dv.rowCacheW = 0
	dv.rowCacheH = 0
	dv.markAllRowsDirty()
	dv.rowsLayerDirty = true
}

// OnLocaleChanged re-invalidates every text-baking cache and text-heavy zone so
// a language switch re-measures and re-bakes labels at the new locale's string
// widths. Wired to i18n.OnChange in game_new.go. Cheap: just marks caches dirty;
// the next layout/draw rebuilds them.
func (dv *DrumView) OnLocaleChanged() {
	dv.invalidateRowCaches()
	dv.invalidateLabelCaches()
	// Invalidate text-heavy panel zones so they re-layout at new string widths.
	// Nil-guard each: a partially-built DrumView (or a Game without all zones)
	// must not panic on a locale flip.
	if dv.eqPanelZone != nil {
		dv.eqPanelZone.Invalidate()
		// The Chain (Scope) tab is owned by the EQ panel zone, not DrumView.
		if cz := dv.eqPanelZone.ChainZone(); cz != nil {
			cz.Invalidate()
		}
	}
	if dv.rowRackZone != nil {
		dv.rowRackZone.Invalidate()
	}
	// The mobile bottom-nav SegmentedControl copies its labels at construction,
	// so it must be explicitly relabeled in the new locale (zone Invalidate does
	// not reach it).
	if dv.viewSwitchSegmented != nil {
		dv.viewSwitchSegmented.SetLabels(dv.bottomNavLabels())
	}
}

func (dv *DrumView) needsRowRebuild(i int) bool {
	if i < 0 || i >= len(dv.Rows) {
		return false
	}
	if len(dv.rowCache) != len(dv.Rows) || len(dv.rowDirty) != len(dv.Rows) {
		return true
	}
	if len(dv.rowFullDirty) != len(dv.Rows) {
		return true
	}
	if dv.rowDirty[i] {
		return true
	}
	if i < len(dv.rowFullDirty) && dv.rowFullDirty[i] {
		return true
	}
	if dv.rowCacheW != dv.timelineRect.Dx() || dv.rowCacheH != dv.rowHeight() {
		return true
	}
	if dv.rowCacheLen != dv.Length {
		return true
	}
	if dv.rowCache[i] == nil {
		return true
	}
	return false
}
