package ui

import "github.com/hajimehoshi/ebiten/v2"

// Debug only (BEATMO_DEBUG_LAYOUT=1).
type LayoutPillsLayer struct {
	dv *DrumView
}

func newLayoutPillsLayer(dv *DrumView) *LayoutPillsLayer { return &LayoutPillsLayer{dv: dv} }

func (l *LayoutPillsLayer) ID() string  { return "layout-pills" }
func (l *LayoutPillsLayer) ZIndex() int { return ZLayoutPills }
func (l *LayoutPillsLayer) Visible() bool {
	return !l.dv.perfDrawLite && Profile().ShowLayoutGuides && l.dv.widgets != nil
}

func (l *LayoutPillsLayer) Draw(dst *ebiten.Image) {
	dv := l.dv
	for i := 1; i < len(dv.widgets.colPos)-1; i++ {
		idx := i - 1
		hover := (dv.layoutHoverAxis == "col" && dv.layoutHoverIdx == idx) ||
			(dv.layoutDragAxis == "col" && dv.layoutDragIdx == idx)
		if dv.layoutHandler != nil {
			hr := dv.layoutHandler.columnHandleRect(idx)
			if hr.Empty() {
				continue
			}
			hcx := (hr.Min.X + hr.Max.X) / 2
			hcy := (hr.Min.Y + hr.Max.Y) / 2
			DrawSplitterHandle(dst, hcx, hcy, false, hover)
		}
	}
	for i := 1; i < len(dv.widgets.rowPos)-1; i++ {
		idx := i - 1
		// rowEQDividerLayer draws that pill in production; skip it so the debug overlay never draws it twice.
		if dv.showRowEQDivider() && idx == dv.eqDividerRowIdx() {
			continue
		}
		hover := (dv.layoutHoverAxis == "row" && dv.layoutHoverIdx == idx) ||
			(dv.layoutDragAxis == "row" && dv.layoutDragIdx == idx)
		if dv.layoutHandler != nil {
			hr := dv.layoutHandler.rowHandleRect(idx)
			if hr.Empty() {
				continue
			}
			hcx := (hr.Min.X + hr.Max.X) / 2
			hcy := (hr.Min.Y + hr.Max.Y) / 2
			DrawSplitterHandle(dst, hcx, hcy, true, hover)
		}
	}
}
