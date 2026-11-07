package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Draw-only: hit-testing and the resize belong to layoutResizeZone. It mirrors the main splitter and renders ONLY
// the EQ boundary; the debug pills layer defers that divider to it so the two never double-draw.
type rowEQDividerLayer struct {
	dv     *DrumView
	handle SplitterHandle // shared animated line+pill (glow + grow on hover)
}

func newRowEQDividerLayer(dv *DrumView) *rowEQDividerLayer { return &rowEQDividerLayer{dv: dv} }

func (dv *DrumView) rowEQDividerLayer() *rowEQDividerLayer { return dv.rowEQDivider }

func (l *rowEQDividerLayer) ID() string  { return "row-eq-divider" }
func (l *rowEQDividerLayer) ZIndex() int { return ZRowEQDivider }

func (l *rowEQDividerLayer) Visible() bool { return l.dv.showRowEQDivider() }

func (l *rowEQDividerLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	idx := dv.eqDividerRowIdx()
	if idx < 0 {
		return
	}
	hr := dv.layoutHandler.rowHandleRect(idx)
	if hr.Empty() {
		return
	}
	y := (hr.Min.Y + hr.Max.Y) / 2

	// Also computed straight from the cursor, as the main splitter does: the resize handler's probe is off on WASM
	// (SkipCursorHover), which left this pill dark in the browser.
	hover := (dv.layoutHoverAxis == "row" && dv.layoutHoverIdx == idx) ||
		(dv.layoutDragAxis == "row" && dv.layoutDragIdx == idx)
	if !hover && dv.layoutHandler != nil && !dv.layoutHandler.dragging {
		mx, my := cursorPosition()
		if image.Pt(mx, my).In(dv.layoutHandler.rowDividerGrabRect(idx)) &&
			!dv.layoutHandler.pointInsideRowControl(mx, my) {
			hover = true
		}
	}

	l.handle.Advance(hover)
	l.handle.DrawHorizontalDivider(dst, dv.Bounds.Min.X, dv.Bounds.Max.X, y)
}

// -1 when a multi-row widget spans the boundary.
func (dv *DrumView) eqDividerRowIdx() int {
	if dv.widgets == nil || dv.layoutHandler == nil {
		return -1
	}
	// The EQ boundary is the divider with a full-width widget below it whose pill is non-empty.
	for i := 0; i < len(dv.widgets.rowPos)-2; i++ {
		if dv.layoutHandler.fullWidthWidgetBelow(i) && !dv.layoutHandler.rowHandleRect(i).Empty() {
			return i
		}
	}
	return -1
}

// Desktop only: mobile has no layout resize, so no draggable divider.
func (dv *DrumView) showRowEQDivider() bool {
	if dv.perfDrawLite || dv.widgets == nil || dv.layoutHandler == nil {
		return false
	}
	if Profile().IsMobile() || !Profile().EnableLayoutResize {
		return false
	}
	return dv.eqDividerRowIdx() >= 0
}
