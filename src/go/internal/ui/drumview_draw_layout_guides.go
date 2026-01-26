package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawLayoutGuides renders lightweight outlines when hovering/dragging widget splits.
func (dv *DrumView) drawLayoutGuides(dst *ebiten.Image) {
	if dv.widgets == nil {
		return
	}
	// When drawing into a local buffer sized to the drum view (e.g., tests),
	// use coordinates relative to the widget board. When drawing to the full
	// screen, offset by the drum view's bounds.
	local := dv.Bounds.Dx() == dst.Bounds().Dx() && dv.Bounds.Dy() == dst.Bounds().Dy()
	offX, offY := 0, 0
	if !local {
		offX, offY = dv.Bounds.Min.X, dv.Bounds.Min.Y
	}
	baseCol := colGridLine
	hoverCol := colButtonBorder
	drawLine := func(x0, y0, x1, y1, thick int, c color.Color) {
		if x0 > x1 {
			x0, x1 = x1, x0
		}
		if y0 > y1 {
			y0, y1 = y1, y0
		}
		drawRect(dst, image.Rect(x0, y0, x1, y1), c, true)
	}

	colMinY, colMaxY := dv.Bounds.Min.Y, dv.Bounds.Max.Y
	rowMinX, rowMaxX := dv.Bounds.Min.X, dv.Bounds.Max.X
	if local {
		colMinY, colMaxY = 0, dst.Bounds().Max.Y
		rowMinX, rowMaxX = 0, dst.Bounds().Max.X
	}
	for i := 0; i < len(dv.widgets.colPos); i++ {
		x := dv.widgets.colPos[i] + offX
		th := 1
		col := baseCol
		if i > 0 && i < len(dv.widgets.colPos)-1 {
			idx := i - 1
			if (dv.layoutHoverAxis == "col" && dv.layoutHoverIdx == idx) || (dv.layoutDragAxis == "col" && dv.layoutDragIdx == idx) {
				th = 3
				col = hoverCol
			}
		}
		drawLine(x-th/2, colMinY, x+th/2+1, colMaxY, th, col)
	}
	for i := 0; i < len(dv.widgets.rowPos); i++ {
		y := dv.widgets.rowPos[i] + offY
		th := 1
		col := baseCol
		if i > 0 && i < len(dv.widgets.rowPos)-1 {
			idx := i - 1
			if (dv.layoutHoverAxis == "row" && dv.layoutHoverIdx == idx) || (dv.layoutDragAxis == "row" && dv.layoutDragIdx == idx) {
				th = 3
				col = hoverCol
			}
		}
		drawLine(rowMinX, y-th/2, rowMaxX, y+th/2+1, th, col)
	}
}
