//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A leading IconTarget, so it reads as the button that opens navigation.
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

// A matte keycap, never a flat rect.
func TestBeatCounterRendersMatteKeycap(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterKeycapCap(t, z, dst, beatRect); !ok {
		t.Fatalf("beat counter did not render a matte keycap cap face inside %v", beatRect)
	}
}
