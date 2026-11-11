//go:build test

package ui

import (
	"image"
	"testing"
)

// TestRowRackScrollbarThumbVisibleDesktop guards the "instruments unreachable"
// bug: when the kit has more rows than fit the (EQ-panel-squeezed) rack, the
// scrollbar is the only cue the hidden rows exist. The shared desktop thumb at
// alpha 40 read as invisible against the dark rack, so users could not tell the
// rack scrolled. The rack thumb must be clearly visible while keeping the
// pinned narrow desktop width (TestScrollbarWidthDesktop).
func TestRowRackScrollbarThumbVisibleDesktop(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	rows := makeTestRows(20)
	z, _ := newTestRowRackZone(rows)
	tree := registerRowRackZone(z, image.Rect(0, 0, 300, 100))
	tree.Update()

	_, _, _, a := z.RowScroll().Style.ThumbColor.RGBA()
	alpha := a >> 8
	if alpha < 120 {
		t.Fatalf("desktop row-rack scrollbar thumb too faint to be discoverable: alpha=%d (want >=120)", alpha)
	}
	if w := z.RowScroll().Style.Width; w != 6 {
		t.Fatalf("desktop rack scrollbar width changed from pinned 6: got %d", w)
	}
}
