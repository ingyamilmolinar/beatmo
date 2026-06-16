//go:build test

package ui

import (
	"image"
	"testing"
)

func TestAnchorPopupRectStaysInBounds(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	anchors := []image.Rectangle{
		image.Rect(10, 10, 110, 50),       // top-left
		image.Rect(1180, 10, 1270, 50),    // top-right
		image.Rect(10, 660, 110, 710),     // bottom-left (the bug class: menus ran off-screen)
		image.Rect(1180, 660, 1270, 710),  // bottom-right
		image.Rect(600, 350, 700, 390),    // center
		image.Rect(-20, 700, 60, 760),     // partially outside
	}
	sides := []PopupSide{PopupBelow, PopupAbove, PopupLeft, PopupRight}
	sizes := []image.Point{{160, 300}, {300, 500}, {2000, 2000}, {40, 40}}

	for _, a := range anchors {
		for _, s := range sides {
			for _, sz := range sizes {
				got := AnchorPopupRect(bounds, a, sz.X, sz.Y, s)
				if !got.In(bounds) {
					t.Errorf("AnchorPopupRect(%v, %v, %v, side=%d) = %v escapes bounds", a, sz.X, sz.Y, s, got)
				}
				if got.Empty() {
					t.Errorf("AnchorPopupRect(%v, %v, %v, side=%d) returned empty rect", a, sz.X, sz.Y, s)
				}
			}
		}
	}
}

func TestAnchorPopupRectFlipsAboveWhenNoRoomBelow(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(100, 650, 200, 690) // near the bottom edge
	got := AnchorPopupRect(bounds, anchor, 160, 300, PopupBelow)
	if got.Max.Y > anchor.Min.Y {
		t.Errorf("expected flip above anchor; got %v (anchor %v)", got, anchor)
	}
}

func TestAnchorPopupRectFlipsLeftWhenNoRoomRight(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(1200, 100, 1270, 140)
	got := AnchorPopupRect(bounds, anchor, 300, 200, PopupRight)
	if got.Min.X >= anchor.Min.X {
		t.Errorf("expected flip left of anchor; got %v (anchor %v)", got, anchor)
	}
}

func TestAnchorPopupRectPrefersRequestedSideWhenItFits(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(100, 100, 200, 140)
	got := AnchorPopupRect(bounds, anchor, 160, 300, PopupBelow)
	if got.Min.Y < anchor.Max.Y {
		t.Errorf("expected popup below anchor; got %v", got)
	}
	if got.Min.X != anchor.Min.X {
		t.Errorf("expected left edges aligned; got %v", got)
	}
}

func TestClampRectInto(t *testing.T) {
	bounds := image.Rect(0, 0, 100, 100)
	cases := []struct {
		in, want image.Rectangle
	}{
		{image.Rect(10, 10, 20, 20), image.Rect(10, 10, 20, 20)},     // already inside
		{image.Rect(-10, 5, 10, 25), image.Rect(0, 5, 20, 25)},       // off left
		{image.Rect(90, 90, 130, 120), image.Rect(60, 70, 100, 100)}, // off bottom-right
		{image.Rect(-50, -50, 250, 250), bounds},                     // larger than bounds
	}
	for _, c := range cases {
		if got := ClampRectInto(c.in, bounds); got != c.want {
			t.Errorf("ClampRectInto(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
