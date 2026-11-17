//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestBeatCounterDrawsLeadingTargetIcon verifies the beat/time counter renders a
// leading IconTarget affordance so it reads as a pressable button (it opens the
// navigation menu on tap).
func TestBeatCounterDrawsLeadingTargetIcon(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	var iconRects []image.Rectangle
	orig := drawTargetIcon
	drawTargetIcon = func(d *ebiten.Image, r image.Rectangle, col color.Color) {
		iconRects = append(iconRects, r)
		orig(d, r, col)
	}
	t.Cleanup(func() { drawTargetIcon = orig })

	z.drawBeatCounter(dst, 0)

	if len(iconRects) == 0 {
		t.Fatal("beat counter did not draw a leading IconTarget affordance")
	}
	// The icon must sit inside the counter rect and toward its left (leading).
	r := iconRects[0]
	if !r.In(beatRect) {
		t.Fatalf("leading icon %v not inside beat counter rect %v", r, beatRect)
	}
	if r.Min.X > beatRect.Min.X+beatRect.Dx()/2 {
		t.Fatalf("leading icon %v should be on the LEFT of the counter %v", r, beatRect)
	}
}

// TestBeatCounterRendersMatteKeycap verifies the counter renders as a matte
// keycap (a rounded cap face with the shared secondary fill via
// drawKeycapPanelButton), NOT a flat rect — DESIGN.md "Keycap elevation"
// requires every discrete control to read as a slim matte key.
func TestBeatCounterRendersMatteKeycap(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterKeycapCap(t, z, dst, beatRect); !ok {
		t.Fatalf("beat counter did not render a matte keycap cap face inside %v", beatRect)
	}
}
