package ui

import (
	"image"
	"testing"
)

// TestOverflowMenu_DesktopWidthFitsTemplateLabels asserts the desktop overflow
// popup is wide enough to render the template page's labels without truncating
// them — the popup width is derived from the widest label, not a fixed 160 px.
//
// Long display titles like "Marcello — Oboe Concerto in D minor, Adagio" used to
// overflow the hardcoded 160 px popup; this guards the dynamic-width fix.
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

	// Reproduce the label-start inset used by drawOverflowMenu: the row rect is
	// inset by SpaceXS, then menuRowLabelX adds the icon gutter.
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
