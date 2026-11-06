//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestButtonInnerShadowStrips asserts drawButtonInnerShadow emits inner strips
// that stay inside the button rect (an inset, never an escaping shadow). Uses
// the shared captureDrawRects helper (slider_level_indicator_test.go).
func TestButtonInnerShadowStrips(t *testing.T) {
	r := image.Rect(40, 40, 120, 72)
	dst := ebiten.NewImage(200, 200)

	rects, _ := captureDrawRects(t, func() {
		drawButtonInnerShadow(dst, r)
	})

	if len(rects) == 0 {
		t.Fatal("inner shadow emitted no drawRect strips")
	}
	for i, rc := range rects {
		if !rc.In(r) {
			t.Fatalf("inner-shadow strip %d %v escapes button rect %v", i, rc, r)
		}
	}
}

// TestButtonDrawFlatHasNoDepthChrome drives the full Button.Draw path under
// the flat-tonal model: a button paints no contact shadow, no socket wall and
// no bevel in EITHER state, so pressing it changes the fill brightness but not
// the number of emitted rects. This is the inverse of the retired keycap
// contract (where a resting cap drew a shadow that a bottomed-out cap dropped)
// and is what pins "nothing is 3D" — see DESIGN.md "Elevation & Depth".
func TestButtonDrawFlatHasNoDepthChrome(t *testing.T) {
	dst := ebiten.NewImage(300, 200)

	rest := NewButton("X", InstButtonStyle, func() {})
	rest.SetRect(image.Rect(20, 20, 100, 52))
	restRects, _ := captureDrawRects(t, func() { rest.Draw(dst) })

	pressed := NewButton("X", InstButtonStyle, func() {})
	pressed.SetRect(image.Rect(20, 20, 100, 52))
	pressed.HandleInputResult(40, 36, true) // press inside → target full depth
	if !pressed.pressed {
		t.Fatal("setup: button should be pressed")
	}
	// Settle the press spring; it still runs (settle-to-silence contract) but
	// no longer drives any geometry.
	for i := 0; i < 6; i++ {
		pressed.AdvancePressAnim()
	}
	if pressed.pressDepth <= 0 {
		t.Fatalf("setup: held button should have a positive press depth, got %v", pressed.pressDepth)
	}
	pressRects, _ := captureDrawRects(t, func() { pressed.Draw(dst) })

	if len(pressRects) != len(restRects) {
		t.Fatalf("flat button must emit the same rect count pressed and at rest "+
			"(no contact shadow to drop): pressed=%d rest=%d", len(pressRects), len(restRects))
	}
	// And neither state may paint outside the button rect — a contact shadow
	// was the only thing that ever did.
	r := image.Rect(20, 20, 100, 52)
	for _, rc := range append(append([]image.Rectangle{}, restRects...), pressRects...) {
		if !rc.In(r) {
			t.Fatalf("rect %v escapes button rect %v — depth chrome is back", rc, r)
		}
	}
}
