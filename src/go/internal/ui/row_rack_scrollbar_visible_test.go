//go:build test

package ui

import (
	"image"
	"testing"
)

// With more rows than fit, the scrollbar is the only sign hidden rows exist; at alpha 40 the thumb was
// invisible on the dark rack. Clearly visible, at the pinned narrow width.
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
