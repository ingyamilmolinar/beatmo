//go:build test

package ui

import (
	"image"
	"testing"
)

// buildPagerButtons returns two ComponentButtonSecondary chevron buttons.
func buildPagerButtons() (*Button, *Button) {
	p := NewSpecButton("", ComponentButtonSecondary, func() {})
	p.Icon = string(IconChevronLeft)
	n := NewSpecButton("", ComponentButtonSecondary, func() {})
	n.Icon = string(IconChevronRight)
	return p, n
}

func TestMenuPagerFooter_ButtonsStayInsideFooterAtEveryWidth(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 1, 1)) // not drawn onto; geometry only
	_ = dst
	for _, w := range []int{160, 220, 340, 640} {
		footer := image.Rect(0, 100, w, 100+menuPagerFooterH())
		prev, next := buildPagerButtons()
		pr, nr := drawMenuPagerFooter(nil, footer, "Page 2 / 5", prev, next)
		if !pr.In(footer) {
			t.Fatalf("w=%d prev rect %v not inside footer %v", w, pr, footer)
		}
		if !nr.In(footer) {
			t.Fatalf("w=%d next rect %v not inside footer %v", w, nr, footer)
		}
		if pr.Max.X > nr.Min.X { // prev must sit left of next
			t.Fatalf("w=%d prev %v overlaps/crosses next %v", w, pr, nr)
		}
	}
}

// The band fits its own contents at any font metric. The pager is font-fixed by design: its sizes derive from
// TextHeight and fixed spacing, not density.
func TestMenuPagerFooter_BandFitsItsContents(t *testing.T) {
	if h, b := menuPagerFooterH(), menuPagerBtnSize(); h < b {
		t.Fatalf("footer band %d must be tall enough to contain the button %d", h, b)
	}
	if h := menuPagerFooterH(); h < TextHeight() {
		t.Fatalf("footer band %d must be tall enough to contain the page label %d", h, TextHeight())
	}
}
