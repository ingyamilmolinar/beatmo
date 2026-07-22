//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func TestAnchorPopupRectStaysInBounds(t *testing.T) {
	bounds := image.Rect(0, 0, 1280, 720)
	anchors := []image.Rectangle{
		image.Rect(10, 10, 110, 50),      // top-left
		image.Rect(1180, 10, 1270, 50),   // top-right
		image.Rect(10, 660, 110, 710),    // bottom-left (the bug class: menus ran off-screen)
		image.Rect(1180, 660, 1270, 710), // bottom-right
		image.Rect(600, 350, 700, 390),   // center
		image.Rect(-20, 700, 60, 760),    // partially outside
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

func TestAnchoredPopupRectDropsBelowAnchor(t *testing.T) {
	win := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(1180, 10, 1210, 40) // top-right button
	got := anchoredPopupRect(anchor, 200, 300, win, GridAnchorOpts{})
	if got.Min.Y < anchor.Max.Y {
		t.Fatalf("popup should drop below anchor; got %v (anchor %v)", got, anchor)
	}
	if !got.In(win) {
		t.Fatalf("popup escaped window: %v", got)
	}
}

func TestAnchoredPopupRectClearsAvoidZone(t *testing.T) {
	win := image.Rect(0, 0, 1280, 720)
	anchor := image.Rect(1180, 10, 1210, 40)
	zone := image.Rect(1170, 5, 1215, 45) // gear cluster around the button
	got := anchoredPopupRect(anchor, 200, 300, win, GridAnchorOpts{
		Avoid: []image.Rectangle{zone},
	})
	if got.Overlaps(zone) {
		t.Fatalf("popup %v overlaps avoid zone %v", got, zone)
	}
	if !got.In(win) {
		t.Fatalf("popup escaped window: %v", got)
	}
}

func TestGridAnchoredPopupRectDropsBelowGridButton(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 2)

	gear := g.gridHelpButtonRect()
	if gear.Empty() {
		t.Fatal("gear button rect empty after layout")
	}
	got := g.gridAnchoredPopupRect(gear, 200, 260, GridAnchorOpts{})

	// Drops at/below the gear top and clears the reserved gear zone.
	if got.Min.Y < gear.Min.Y {
		t.Fatalf("popup %v should be at/below the gear top %v", got, gear)
	}
	if z := g.gridHelpReservedRect(); !z.Empty() && got.Overlaps(z) {
		t.Fatalf("popup %v overlaps gear reserved zone %v", got, z)
	}
	// Stays within the window (NOT pinned into the bottom drum pane).
	if win := image.Rect(0, 0, 1280, 720); !got.In(win) {
		t.Fatalf("popup %v escaped window %v", got, win)
	}
}
