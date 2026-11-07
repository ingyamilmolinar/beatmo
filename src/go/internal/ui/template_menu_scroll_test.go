//go:build test

package ui

import (
	"image"
	"testing"
)

// Short enough that the template list overflows the popup.
func openTemplatePageClamped(t *testing.T) *DrumView {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(420, 360) // short pane → popup is height-clamped
	dv := g.drum
	dv.overflowPage = 1 // template page (Templates header + Back + the genre templates)
	dv.OpenOverflowMenu()
	return dv
}

// From the menu's real hit geometry.
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
	// Scrolled until the last row stops moving rather than a fixed count, which under-scrolls a longer list; scrolled
	// toward the end its Y must decrease.
	prevY := 1 << 30
	for i := 0; i < scroll.ScrollBehavior().VS.Total+4; i++ {
		// The menu wheel is smooth with no cooldown clock to advance.
		scroll.HandleWheel(-1) // scroll down one item
		row, _ := lastTemplateBtn(dv)
		if row.Min.Y == prevY {
			break // reached the clamp — further wheel steps don't move the list
		}
		prevY = row.Min.Y
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
