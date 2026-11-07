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

func (dv *DrumView) markRowsShiftDirty() {
	dv.ensureRowCache()
	for i := range dv.rowDirty {
		dv.rowDirty[i] = true
	}
	dv.rowsLayerDirty = true
}

// Leaves rowFullDirty alone, so the cheap cell-patch path still applies.
func (dv *DrumView) markRowCellsDirty(i int) {
	dv.ensureRowCache()
	if i >= 0 && i < len(dv.rowDirty) {
		dv.rowDirty[i] = true
	}
	dv.rowsLayerDirty = true
	dv.rowsWinContentDirty = true // cell content changed → windowed cache re-bake
}

// UI thread only, like every DrumView mutation.
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

// Only marks caches dirty; the next layout/draw re-measures at the new locale's widths.
func (dv *DrumView) OnLocaleChanged() {
	dv.invalidateRowCaches()
	dv.invalidateLabelCaches()
	// The plot's key does not include the locale, but it carries words.
	if p := dv.synp(); p != nil {
		p.graphPlot = nil
	}
	// Nil-guarded: a partially built DrumView must survive a locale flip.
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
	// TraceSprites bake localized text but key on geometry/params, so they must be released here.
	// The field, not synp(): a locale flip must not create a panel.
	if dv.synthPanel != nil {
		dv.synthPanel.sprites.releaseAll()
	}
	// Copies its labels at construction; zone Invalidate does not reach it.
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
	if dv.rowCacheW != dv.cellGridRect.Dx() || dv.rowCacheH != dv.rowHeight() {
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
