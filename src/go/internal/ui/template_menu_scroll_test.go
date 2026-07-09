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
	dv.overflowPage = 1 // template page (Templates header + Back + the genre templates)
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
	// into the viewport). Scroll until the bottom is reached (the last row stops
	// moving) rather than a fixed step count — the template list grew well past
	// the original 7 genres, so a hard-coded count under-scrolls the longer list.
	prevY := 1 << 30
	for i := 0; i < scroll.ScrollBehavior().VS.Total+4; i++ {
		// The menu wheel is clicky (one item per notch + cooldown), so advance
		// the cooldown clock between notches the way real frames do before each
		// wheel event — otherwise the second notch is locked out.
		for j := 0; j < controlGridScrollCooldownFrames; j++ {
			scroll.TickStep()
		}
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
