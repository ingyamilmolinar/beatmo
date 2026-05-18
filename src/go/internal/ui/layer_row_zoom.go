package ui

import "github.com/hajimehoshi/ebiten/v2"

// RowZoomChipsLayer paints the mobile row-zoom +/− chips. Theme 3 of the
// mobile UI consistency pass. Rect is non-empty only on mobile in Pads
// view (zeroed elsewhere by layout).
//
// Z = ZRowZoomChips (160).
type RowZoomChipsLayer struct {
	dv *DrumView
}

func newRowZoomChipsLayer(dv *DrumView) *RowZoomChipsLayer { return &RowZoomChipsLayer{dv: dv} }

func (l *RowZoomChipsLayer) ID() string    { return "row-zoom-chips" }
func (l *RowZoomChipsLayer) ZIndex() int   { return ZRowZoomChips }
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
