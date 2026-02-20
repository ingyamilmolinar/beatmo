package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

func (dv *DrumView) bg(w, h int) *ebiten.Image {
	if dv.bgDirty || len(dv.bgCache) == 0 || !dv.bgCache[0].Bounds().Eq(image.Rect(0, 0, w, h)) {
		dv.bgCache = make([]*ebiten.Image, 1)
		img := ebiten.NewImage(w, h)
		img.Fill(colBGBottom)
		dv.bgCache[0] = img
		dv.bgDirty = false
	}
	return dv.bgCache[0]
}

// updateRowRects updates positions of existing per-row widgets without
// reallocating them. It computes the geometry based on the current bounds,
// visible window, and scroll offset. This keeps per-frame work minimal.
func (dv *DrumView) updateRowRects() {
	if len(dv.Rows) == 0 {
		return
	}
	if dv.mainVolSlider != nil {
		dv.mainVolSlider.SetRect(dv.mainVolRect)
	}
	// Ensure we have widgets for each row. This is a safety net for callers
	// that may have changed the number of rows without invoking calcLayout yet.
	if len(dv.rowLabels) != len(dv.Rows) ||
		len(dv.rowEditBtns) != len(dv.Rows) ||
		len(dv.rowSaveBtns) != len(dv.Rows) ||
		len(dv.rowColorBtns) != len(dv.Rows) ||
		len(dv.rowVolSliders) != len(dv.Rows) ||
		len(dv.rowMuteBtns) != len(dv.Rows) ||
		len(dv.rowSoloBtns) != len(dv.Rows) ||
		len(dv.rowOriginBtns) != len(dv.Rows) ||
		len(dv.rowDeleteBtns) != len(dv.Rows) ||
		len(dv.rowMenuBtns) != len(dv.Rows) {
		// Full rebuild on mismatch to guarantee integrity.
		dv.calcLayout()
		return
	}
	if len(dv.Rows[0].Steps) == 0 {
		return
	}
	w := dv.timelineRect.Dx()
	if w <= 0 {
		w = dv.Bounds.Dx() - dv.labelW - dv.controlsW
	}
	dv.cell = w / len(dv.Rows[0].Steps)
	vis := dv.visibleRows()
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	panelRect := dv.widgetRects[WidgetRack]
	if panelRect.Empty() {
		panelRect = image.Rect(dv.Bounds.Min.X, rowsTop, dv.Bounds.Min.X+dv.labelW+dv.controlsW, dv.Bounds.Max.Y-dv.eqH)
	}
	// Clamp rack top to match capped headerH (widget board may allocate more).
	if panelRect.Min.Y < rowsTop {
		panelRect.Min.Y = rowsTop
	}
	for i := range dv.Rows {
		rowRect := dv.rowRectForIndex(i, rowsTop, vis, panelRect)
		dv.positionRowWidgets(i, rowRect)
	}
	dv.positionAddRowBtn(rowsTop, panelRect, vis)
}

// --- Row cache helpers ---

func (dv *DrumView) ensureRowCache() {
	if len(dv.rowCache) != len(dv.Rows) {
		dv.rowCache = make([]*ebiten.Image, len(dv.Rows))
		dv.rowCacheScratch = make([]*ebiten.Image, len(dv.Rows))
		dv.rowDirty = make([]bool, len(dv.Rows))
		dv.rowFullDirty = make([]bool, len(dv.Rows))
		for i := range dv.rowDirty {
			dv.rowDirty[i] = true
		}
		for i := range dv.rowFullDirty {
			dv.rowFullDirty[i] = true
		}
		dv.rowCacheOff = make([]int, len(dv.Rows))
		dv.rowCacheGen = make([]int, len(dv.Rows))
		dv.rowCacheSig = make([]uint64, len(dv.Rows))
		dv.rowCacheSteps = make([][]bool, len(dv.Rows))
		dv.rowCacheTypes = make([][]model.NodeType, len(dv.Rows))
		for i := range dv.rowCacheOff {
			dv.rowCacheOff[i] = dv.Offset
		}
	}
	if len(dv.rowCacheScratch) != len(dv.Rows) {
		scratch := make([]*ebiten.Image, len(dv.Rows))
		copy(scratch, dv.rowCacheScratch)
		dv.rowCacheScratch = scratch
	}
	if len(dv.rowCacheSig) != len(dv.Rows) {
		sig := make([]uint64, len(dv.Rows))
		copy(sig, dv.rowCacheSig)
		dv.rowCacheSig = sig
	}
	if len(dv.rowCacheSteps) != len(dv.Rows) {
		steps := make([][]bool, len(dv.Rows))
		copy(steps, dv.rowCacheSteps)
		dv.rowCacheSteps = steps
	}
	if len(dv.rowCacheTypes) != len(dv.Rows) {
		types := make([][]model.NodeType, len(dv.Rows))
		copy(types, dv.rowCacheTypes)
		dv.rowCacheTypes = types
	}
	if len(dv.rowFrame) != len(dv.Rows) {
		rf := make([]int64, len(dv.Rows))
		copy(rf, dv.rowFrame)
		dv.rowFrame = rf
	}
	if len(dv.rowRepaint) != len(dv.Rows) {
		rr := make([]int, len(dv.Rows))
		copy(rr, dv.rowRepaint)
		dv.rowRepaint = rr
	}
	if len(dv.timelineOffset) != len(dv.Rows) {
		offsets := make([]int, len(dv.Rows))
		copy(offsets, dv.timelineOffset)
		dv.timelineOffset = offsets
	}
	if len(dv.timelinePast) != len(dv.Rows) {
		past := make([][]bool, len(dv.Rows))
		copy(past, dv.timelinePast)
		dv.timelinePast = past
	}
	if len(dv.timelinePastTypes) != len(dv.Rows) {
		types := make([][]model.NodeType, len(dv.Rows))
		copy(types, dv.timelinePastTypes)
		dv.timelinePastTypes = types
	}
	if len(dv.timelinePresent) != len(dv.Rows) {
		present := make([][]bool, len(dv.Rows))
		copy(present, dv.timelinePresent)
		dv.timelinePresent = present
	}
	if len(dv.timelineFuture) != len(dv.Rows) {
		future := make([][]bool, len(dv.Rows))
		copy(future, dv.timelineFuture)
		dv.timelineFuture = future
	}
}

func copyBoolSliceInto(dst *[]bool, src []bool) {
	if len(src) == 0 {
		if *dst != nil {
			*dst = (*dst)[:0]
		} else {
			*dst = nil
		}
		return
	}
	if cap(*dst) < len(src) {
		*dst = make([]bool, len(src))
	} else {
		*dst = (*dst)[:len(src)]
	}
	copy(*dst, src)
}

func copyNodeTypeSliceInto(dst *[]model.NodeType, src []model.NodeType) {
	if len(src) == 0 {
		if *dst != nil {
			*dst = (*dst)[:0]
		} else {
			*dst = nil
		}
		return
	}
	if cap(*dst) < len(src) {
		*dst = make([]model.NodeType, len(src))
	} else {
		*dst = (*dst)[:len(src)]
	}
	copy(*dst, src)
}

func (dv *DrumView) cacheRowSteps(row int) {
	if dv == nil || row < 0 || row >= len(dv.Rows) {
		return
	}
	if len(dv.rowCacheSteps) != len(dv.Rows) {
		steps := make([][]bool, len(dv.Rows))
		copy(steps, dv.rowCacheSteps)
		dv.rowCacheSteps = steps
	}
	if len(dv.rowCacheTypes) != len(dv.Rows) {
		types := make([][]model.NodeType, len(dv.Rows))
		copy(types, dv.rowCacheTypes)
		dv.rowCacheTypes = types
	}
	copyBoolSliceInto(&dv.rowCacheSteps[row], dv.Rows[row].Steps)
	copyNodeTypeSliceInto(&dv.rowCacheTypes[row], dv.Rows[row].CellTypes)
}
