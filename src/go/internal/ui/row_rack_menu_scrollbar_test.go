//go:build test

package ui

import (
	"image"
	"testing"
)

// TestRowControlsClearScrollbarDesktop guards the "⋯ overflow button overlaps
// the scrollbar" bug: when the kit has more instruments than fit the
// EQ-panel-squeezed rack, the row-rack scrollbar draws at the panel's right
// edge (VS.BarRect == [View.Max.X-width, View.Max.X]). The right-anchored
// control cluster must reserve that width so the ⋯ chip (and every other
// per-row control) renders entirely to the LEFT of the scrollbar — never under
// it or flush against it.
func TestRowControlsClearScrollbarDesktop(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	// 20 rows in a short rack forces the scrollbar (Total > Visible).
	rows := makeTestRows(20)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 300, 100))
	tree.Update()
	// Populate the scrollbar viewport exactly as the per-frame Draw does so we
	// can read its on-screen BarRect.
	z.syncScroll()

	if !z.RowScroll().HasScroll() {
		t.Fatalf("test precondition failed: expected scrollbar (Total=%d Visible=%d)",
			z.RowScroll().VS.Total, z.RowScroll().VS.Visible)
	}
	bar := z.RowScroll().BarRect()
	if bar.Empty() {
		t.Fatalf("scrollbar BarRect empty; cannot verify clearance")
	}

	gap := Profile().ControlGap
	if gap < SpaceXS {
		gap = SpaceXS
	}

	menus := z.RowMenuBtns()
	for i, b := range menus {
		r := b.Rect()
		if r.Empty() {
			continue // off-screen (scrolled out) rows have no rect
		}
		if r.Max.X > bar.Min.X {
			t.Errorf("row %d ⋯ button overlaps scrollbar: menu.Max.X=%d > bar.Min.X=%d",
				i, r.Max.X, bar.Min.X)
		}
		// Comfortable breathing room: at least one control-gap of clearance.
		if clear := bar.Min.X - r.Max.X; clear < gap {
			t.Errorf("row %d ⋯ button too close to scrollbar: clearance=%d px (want >=%d)",
				i, clear, gap)
		}
	}
}
