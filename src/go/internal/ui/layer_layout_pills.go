package ui

import "github.com/hajimehoshi/ebiten/v2"

// LayoutPillsLayer paints the splitter pill handles on inner column/row
// dividers — debug-only chrome controlled by Profile().ShowLayoutGuides
// (false in production; flip via BEATMO_DEBUG_LAYOUT=1).
//
// Z = ZLayoutPills (180) — above all primary content, below LayoutGuides
// and portal overlays.
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
		// The EQ↔rows divider is drawn in production by rowEQDividerLayer
		// (line + pill). When that layer owns it, skip it here so the debug
		// overlay never double-draws the same pill on top.
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
