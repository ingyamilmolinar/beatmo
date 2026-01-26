package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/internal/utils"
)

/* ─── geometry helpers ─────────────────────────────────────── */

// rowHeight returns the fixed pixel height for each drum row and for the
// trailing "+" button row. Keeping this constant avoids oversized buttons when
// only a few rows are present, yielding a minimal and consistent layout.
func (dv *DrumView) rowHeight() int { return 24 }

func (dv *DrumView) rowsAreaHeight() int {
	h := dv.Bounds.Dy() - dv.headerH - dv.eqH
	if h < 0 {
		return 0
	}
	return h
}

// refreshWidgetLayout synchronises cached widget rectangles and derived
// measurements (header height, EQ height, control widths) from the board.
func (dv *DrumView) refreshWidgetLayout() {
	if dv.widgets == nil {
		dv.headerH = timelineHeight
		dv.eqH = eqPanelHeight
		return
	}
	if dv.widgetRects == nil {
		dv.widgetRects = map[WidgetKind]image.Rectangle{}
	}
	dv.widgetRects[WidgetTransport] = dv.widgets.Rect(WidgetTransport)
	dv.widgetRects[WidgetRack] = dv.widgets.Rect(WidgetRack)
	dv.widgetRects[WidgetTimeline] = dv.widgets.Rect(WidgetTimeline)
	dv.widgetRects[WidgetWave] = dv.widgets.Rect(WidgetWave)

	// Row/column derived sizes
	if h := dv.widgets.RowHeight(0); h > 0 {
		dv.headerH = h
	} else {
		dv.headerH = timelineHeight
	}
	if dv.headerH < dv.rowHeight()*2 {
		dv.headerH = dv.rowHeight() * 2
	}
	if dv.headerH < timelineHeight {
		dv.headerH = timelineHeight
	}
	if h := dv.widgets.RowHeight(2); h > 0 {
		dv.eqH = h
	} else {
		dv.eqH = eqPanelHeight
	}
	if runningUnderGoTest() && eqPanelHeight == 0 {
		dv.eqH = 0
	}
	leftW := dv.widgets.ColWidth(0)
	if leftW <= 0 {
		leftW = dv.labelW + dv.controlsW
	}
	dv.controlsW = leftW - dv.labelW
	if dv.controlsW < 180 {
		dv.controlsW = 180
	}
	if dv.controlsW > 520 {
		dv.controlsW = 520
	}

	// Clamp scroll so end of list (rows + add button) stays reachable after resizes.
	maxOff := len(dv.Rows) + 1 - dv.visibleRows()
	if maxOff < 0 {
		maxOff = 0
	}
	if dv.rowOffset > maxOff {
		dv.rowOffset = maxOff
	}
}

// handleLayoutEdit enables lightweight widget resizing when layout edit mode
// is active. Toggle with the "L" key. Drags near column/row boundaries adjust
// weights and trigger a layout rebuild.
func (dv *DrumView) handleLayoutResize() {
	if dv.widgets == nil {
		return
	}
	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	// Do not treat scrollbar drags as layout resizes.
	if image.Pt(mx, my).In(dv.scrollBarRect()) {
		dv.layoutHoverAxis = ""
		dv.layoutHoverIdx = -1
		if !left {
			dv.layoutDragIdx = -1
		}
		return
	}
	if dv.layoutDragIdx >= 0 {
		cur := mx
		if dv.layoutDragAxis == "row" {
			cur = my
		}
		delta := cur - dv.layoutDragPrev
		if delta != 0 {
			dv.widgets.ResizeAxis(dv.layoutDragAxis, dv.layoutDragIdx, delta)
			dv.refreshWidgetLayout()
			dv.recalcButtons()
			dv.calcLayout()
			dv.invalidateRowCaches()
			dv.rowsLayerDirty = true
		}
		dv.layoutDragPrev = cur
		if !left {
			dv.layoutDragIdx = -1
		}
		return
	}
	dv.layoutHoverAxis = ""
	dv.layoutHoverIdx = -1
	const grab = 6
	off := dv.Bounds.Min
	for i := 1; i < len(dv.widgets.colPos)-1; i++ {
		x := dv.widgets.colPos[i] + off.X
		if utils.Abs(mx-x) <= grab && my >= dv.Bounds.Min.Y && my <= dv.Bounds.Max.Y {
			dv.layoutHoverAxis = "col"
			dv.layoutHoverIdx = i - 1
			if left {
				dv.layoutDragAxis = "col"
				dv.layoutDragIdx = i - 1
				dv.layoutDragPrev = mx
			}
			return
		}
	}
	for i := 1; i < len(dv.widgets.rowPos)-1; i++ {
		y := dv.widgets.rowPos[i] + off.Y
		if utils.Abs(my-y) <= grab && mx >= dv.Bounds.Min.X && mx <= dv.Bounds.Max.X {
			dv.layoutHoverAxis = "row"
			dv.layoutHoverIdx = i - 1
			if left {
				dv.layoutDragAxis = "row"
				dv.layoutDragIdx = i - 1
				dv.layoutDragPrev = my
				suppressClicksUntilRelease = true
			}
			return
		}
	}
}

// WidgetRectsSnapshot captures the current widget layout and key control rects.
type WidgetRectsSnapshot struct {
	Layout    LayoutSnapshot
	AddButton image.Rectangle
	Timeline  image.Rectangle
	Rack      image.Rectangle
	Wave      image.Rectangle
	Transport image.Rectangle
}

func (dv *DrumView) widgetRectsSnapshot() WidgetRectsSnapshot {
	snap := WidgetRectsSnapshot{
		AddButton: dv.addRowBtn.Rect(),
		Timeline:  dv.timelineRect,
	}
	if dv.widgets != nil {
		snap.Layout = dv.widgets.Snapshot()
	}
	if r, ok := dv.widgetRects[WidgetRack]; ok {
		snap.Rack = r
	}
	if r, ok := dv.widgetRects[WidgetWave]; ok {
		snap.Wave = r
	}
	if r, ok := dv.widgetRects[WidgetTransport]; ok {
		snap.Transport = r
	}
	return snap
}

func (dv *DrumView) visibleRows() int {
	return dv.rowsAreaHeight() / dv.rowHeight()
}

func (dv *DrumView) scrollBarRect() image.Rectangle {
	w := 6
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	y1 := rowsTop + dv.rowsAreaHeight()
	x1 := dv.widgetRects[WidgetTimeline].Max.X
	if x1 == 0 {
		x1 = dv.Bounds.Max.X
	}
	return image.Rect(x1-w, rowsTop, x1, y1)
}

func (dv *DrumView) scrollThumbRect() image.Rectangle {
	total := len(dv.Rows) + 1
	vis := dv.visibleRows()
	bar := dv.scrollBarRect()
	if total <= vis {
		return image.Rect(0, 0, 0, 0)
	}
	h := bar.Dy() * vis / total
	if h < 10 {
		h = 10
	}
	track := bar.Dy() - h
	y := bar.Min.Y
	if total-vis > 0 {
		y += track * dv.rowOffset / (total - vis)
	}
	return image.Rect(bar.Min.X, y, bar.Max.X, y+h)
}
