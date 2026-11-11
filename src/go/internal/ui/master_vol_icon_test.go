//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

type filledDraw struct {
	Rect  image.Rectangle
	Color color.Color // original (NRGBA from WithAlpha) — keep premultiplication out of the chain
}

// captureFilledDrawRect intercepts filled drawRect calls during fn.
func captureFilledDrawRect(t *testing.T, fn func()) []filledDraw {
	t.Helper()
	var calls []filledDraw
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			calls = append(calls, filledDraw{Rect: r, Color: c})
		}
		orig(dst, r, c, filled)
	}
	t.Cleanup(func() { drawRect = orig })
	fn()
	return calls
}

// isPrimaryColor returns true iff `c` is the genColorPrimary token at any
// alpha. Compares NRGBA channels (the form WithAlpha returns) so we
// avoid lossy premultiplied-RGBA round-trips.
func isPrimaryColor(c color.Color) bool {
	if n, ok := c.(color.NRGBA); ok {
		return n.R == genColorPrimary.R && n.G == genColorPrimary.G && n.B == genColorPrimary.B
	}
	if rgba, ok := c.(color.RGBA); ok {
		return rgba.R == genColorPrimary.R && rgba.G == genColorPrimary.G && rgba.B == genColorPrimary.B
	}
	return false
}
