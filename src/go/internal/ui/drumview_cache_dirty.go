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
