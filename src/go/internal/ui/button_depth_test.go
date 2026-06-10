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

// TestButtonDrawPressEmitsInnerShadow drives the full Button.Draw path and
// asserts a pressed button emits more drawRect activity than a resting one —
// i.e. the inner pressed-in shadow only appears on press.
func TestButtonDrawPressEmitsInnerShadow(t *testing.T) {
	dst := ebiten.NewImage(300, 200)

	rest := NewButton("X", InstButtonStyle, func() {})
	rest.SetRect(image.Rect(20, 20, 100, 52))
	restRects, _ := captureDrawRects(t, func() { rest.Draw(dst) })

	pressed := NewButton("X", InstButtonStyle, func() {})
	pressed.SetRect(image.Rect(20, 20, 100, 52))
	pressed.HandleInputResult(40, 36, true) // press inside
	if !pressed.pressed {
		t.Fatal("setup: button should be pressed")
	}
	pressRects, _ := captureDrawRects(t, func() { pressed.Draw(dst) })

	if len(pressRects) <= len(restRects) {
		t.Fatalf("pressed button must emit more drawRect calls than resting (inner shadow): pressed=%d rest=%d", len(pressRects), len(restRects))
	}
}
