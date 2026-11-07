//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawRect and pixel() run thousands of times per frame; a per-call DrawImageOptions or a boxing colour conversion
// roughly doubles every tab's allocations. The colours are pre-boxed: call-site boxing is the caller's cost.
func TestDrawRectZeroAlloc(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	var c color.Color = color.RGBA{10, 20, 30, 255}
	pixel(c) // warm the 1x1 pixel cache
	r := image.Rect(0, 0, 50, 50)

	if got := testing.AllocsPerRun(200, func() { drawRect(dst, r, c, true) }); got > 0 {
		t.Errorf("drawRect(filled) allocates %.1f per call; want 0", got)
	}
	if got := testing.AllocsPerRun(200, func() { drawRect(dst, r, c, false) }); got > 0 {
		t.Errorf("drawRect(stroke) allocates %.1f per call; want 0", got)
	}
	if got := testing.AllocsPerRun(200, func() { pixel(c) }); got > 0 {
		t.Errorf("pixel() allocates %.1f per call on cache hit; want 0", got)
	}

	px := pixel(c)
	if got := testing.AllocsPerRun(200, func() { drawRectBlit(dst, px, 50, 50, 0, 0) }); got > 0 {
		t.Errorf("drawRectBlit allocates %.1f per call; want 0", got)
	}
}
