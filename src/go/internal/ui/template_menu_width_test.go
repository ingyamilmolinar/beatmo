package ui

import (
	"image"
	"testing"
)

// The popup takes the widest label's width, never a fixed one that truncates long titles.
func TestOverflowMenu_DesktopWidthFitsTemplateLabels(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Wide desktop viewport so the popup has room to grow.
	g.Layout(1280, 720)
	dv := g.drum
	if Profile().IsMobile() {
		t.Skip("desktop-only: mobile uses a full-width bottom sheet")
	}
	dv.overflowPage = 1 // template page

	popup := dv.overflowPopupRect()

	// The label start drawOverflowMenu uses: the row inset by SpaceXS, plus menuRowLabelX's icon gutter.
	rowR := image.Rect(popup.Min.X, popup.Min.Y, popup.Max.X, popup.Min.Y+touchMinTargetPx)
	rowR = insetRect(rowR, SpaceXS)
	labelX := menuRowLabelX(rowR)

	for _, it := range dv.overflowItems() {
		if it.header {
			continue
		}
		tw := StyledTextWidth(it.label, RoleBody)
		needRight := labelX + tw
		if needRight > popup.Max.X {
			t.Errorf("label %q (w=%d) overflows popup: needs x=%d but popup.Max.X=%d (popupW=%d)",
				it.label, tw, needRight, popup.Max.X, popup.Dx())
		}
	}
}
