//go:build test

package ui

import (
	"image"
	"testing"
)

// openTemplatePageClamped opens the overflow menu on the template page with a
// drum pane short enough that the template list overflows the popup.
func openTemplatePageClamped(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(420, 360) // short pane → popup is height-clamped
	dv := g.drum
	dv.overflowPage = 1 // template page (Templates header + Back + 7 genres)
	dv.OpenOverflowMenu()
	return dv
}

// lastTemplateBtn returns the button rect for the last template row ("Techno")
// from the menu's real hit geometry.
func lastTemplateBtn(dv *DrumView) (image.Rectangle, bool) {
	popup := dv.overflowPopupRect()
	btns := dv.overflowPopupBtns(popup)
	// Last button is the close button; the one before it is the last item row.
	if len(btns) < 2 {
		return image.Rectangle{}, false
	}
	return btns[len(btns)-2].Rect(), true
}

func TestTemplateMenu_ScrollsToReachLastItem(t *testing.T) {
	dv := openTemplatePageClamped(t)

	scroll := dv.OverflowMenuScrollForTest()
	if scroll == nil {
		t.Fatal("overflow MenuScroll is nil")
	}
	if !scroll.HasScroll() {
		t.Fatalf("expected HasScroll()=true on clamped template page; Total=%d Visible=%d",
			scroll.ScrollBehavior().VS.Total, scroll.ScrollBehavior().VS.Visible)
	}

	popup := dv.overflowPopupRect()
	before, ok := lastTemplateBtn(dv)
	if !ok {
		t.Fatal("could not locate last template button")
	}
	// Bug: with no offset applied, the last row sits at the same Y regardless of
	// scroll. After wheel-scrolling toward the end, its Y must decrease (move up
	// into the viewport).
	for i := 0; i < 20; i++ {
		scroll.HandleWheel(-1) // scroll down one item
	}
	after, _ := lastTemplateBtn(dv)

	if after.Min.Y >= before.Min.Y {
		t.Fatalf("last template did not move up after scrolling: before.Y=%d after.Y=%d (offset not applied)",
			before.Min.Y, after.Min.Y)
	}
	if after.Min.Y < popup.Min.Y || after.Max.Y > popup.Max.Y {
		t.Fatalf("last template not within popup after scroll: row=%v popup=%v", after, popup)
	}
}
