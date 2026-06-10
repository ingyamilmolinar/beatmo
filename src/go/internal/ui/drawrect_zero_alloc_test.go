//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawRect and pixel() are the universal fill primitives — thousands of
// calls per frame on the audio-panel tabs. Two past regressions made them
// allocate per call: (1) a fresh ebiten.DrawImageOptions escaped to the
// heap on every blit (fixed by the shared drawRectOp scratch, mirroring
// the drawArcVS/drawArcIS pool in knob.go), and (2) packRGBA routed
// through color.RGBAModel.Convert, which boxes its result into a
// color.Color interface. Either coming back roughly doubles the per-Draw
// alloc count of every tab in eq_panel_draw_alloc_discipline_test.go.
//
// The colors below are pre-boxed into color.Color variables so the
// closures don't re-box a concrete struct per call — that call-site boxing
// is the caller's cost, not drawRect's.
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
