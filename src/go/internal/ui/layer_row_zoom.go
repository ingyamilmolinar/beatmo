package ui

import "github.com/hajimehoshi/ebiten/v2"

// Mobile Pads view only; the layout zeroes the rect elsewhere.
type RowZoomChipsLayer struct {
	dv *DrumView
}

func newRowZoomChipsLayer(dv *DrumView) *RowZoomChipsLayer { return &RowZoomChipsLayer{dv: dv} }

func (l *RowZoomChipsLayer) ID() string  { return "row-zoom-chips" }
func (l *RowZoomChipsLayer) ZIndex() int { return ZRowZoomChips }
func (l *RowZoomChipsLayer) Visible() bool {
	dv := l.dv
	inc := dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty()
	dec := dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty()
	return inc || dec
}

func (l *RowZoomChipsLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	if dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty() {
		dv.rowZoomIncBtn.Draw(dst)
	}
	if dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty() {
		dv.rowZoomDecBtn.Draw(dst)
	}
}
