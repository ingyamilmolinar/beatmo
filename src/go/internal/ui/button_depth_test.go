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

// TestButtonDrawDepthContactShadow drives the full Button.Draw path under the
// keycap-travel model: a resting/raised cap (pressDepth <= 0) draws a contact
// shadow under it so it reads as lifted, while a held-down cap (pressDepth > 0
// after the first AdvancePressAnim tick in Draw) bottoms out and drops it — so
// the pressed button emits FEWER drawRect calls than the resting one.
func TestButtonDrawDepthContactShadow(t *testing.T) {
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
	// Settle the cap toward bottom-out so pressDepth > 0 (contact shadow drops).
	for i := 0; i < 6; i++ {
		pressed.AdvancePressAnim()
	}
	if pressed.pressDepth <= 0 {
		t.Fatalf("setup: held cap should have descended (depth>0), got %v", pressed.pressDepth)
	}
	pressRects, _ := captureDrawRects(t, func() { pressed.Draw(dst) })

	if len(pressRects) >= len(restRects) {
		t.Fatalf("bottomed-out cap should drop the contact shadow (fewer drawRect than resting): pressed=%d rest=%d", len(pressRects), len(restRects))
	}
}
