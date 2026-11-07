package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Debug only (BEATMO_DEBUG_LAYOUT=1). Drawn late so the guides sit on top of content, as a debug overlay should.
type LayoutGuidesLayer struct {
	dv *DrumView
}

func newLayoutGuidesLayer(dv *DrumView) *LayoutGuidesLayer { return &LayoutGuidesLayer{dv: dv} }

func (l *LayoutGuidesLayer) ID() string  { return "layout-guides" }
func (l *LayoutGuidesLayer) ZIndex() int { return ZLayoutGuides }
func (l *LayoutGuidesLayer) Visible() bool {
	return !l.dv.perfDrawLite && Profile().ShowLayoutGuides && l.dv.widgets != nil
}

func (l *LayoutGuidesLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	// A buffer the size of the drum view uses board-relative coordinates; a screen sub-image is offset by Bounds.
	local := dv.Bounds.Dx() == dst.Bounds().Dx() && dv.Bounds.Dy() == dst.Bounds().Dy()
	offX, offY := 0, 0
	if !local {
		offX, offY = dv.Bounds.Min.X, dv.Bounds.Min.Y
	}
	baseCol := color.Color(colGridLine)
	hoverCol := color.Color(colButtonBorder)
	drawLine := func(x0, y0, x1, y1, _ int, c color.Color) {
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
		var col color.Color = baseCol
		inner := i > 0 && i < len(dv.widgets.colPos)-1
		if inner {
			idx := i - 1
			if (dv.layoutHoverAxis == "col" && dv.layoutHoverIdx == idx) ||
				(dv.layoutDragAxis == "col" && dv.layoutDragIdx == idx) {
				th = 3
				col = hoverCol
			}
		}
		if inner && dv.layoutHandler != nil {
			for _, seg := range dv.layoutHandler.columnDividerSegments(i - 1) {
				drawLine(x-th/2, seg.Min.Y, x+th/2+1, seg.Max.Y, th, col)
			}
		} else {
			drawLine(x-th/2, colMinY, x+th/2+1, colMaxY, th, col)
		}
	}
	for i := 0; i < len(dv.widgets.rowPos); i++ {
		y := dv.widgets.rowPos[i] + offY
		th := 1
		var col color.Color = baseCol
		inner := i > 0 && i < len(dv.widgets.rowPos)-1
		if inner {
			idx := i - 1
			if (dv.layoutHoverAxis == "row" && dv.layoutHoverIdx == idx) ||
				(dv.layoutDragAxis == "row" && dv.layoutDragIdx == idx) {
				th = 3
				col = hoverCol
			}
		}
		if inner && dv.layoutHandler != nil {
			for _, seg := range dv.layoutHandler.rowDividerSegments(i - 1) {
				drawLine(seg.Min.X, y-th/2, seg.Max.X, y+th/2+1, th, col)
			}
		} else {
			drawLine(rowMinX, y-th/2, rowMaxX, y+th/2+1, th, col)
		}
	}
}
