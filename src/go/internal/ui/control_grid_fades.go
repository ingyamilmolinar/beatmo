package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// fillVerticalGradient is alloc-free, so this stays cheap.
const controlGridFadeBands = 8

// The fade itself is the "more above/below" signal. No-op when the grid fits.
func drawControlGridEdgeFades(dst *ebiten.Image, g *ControlGrid, bg color.Color) {
	h := g.PeekH()
	if h <= 0 {
		return
	}
	r := g.ContentRect()
	if r.Empty() {
		return
	}
	// WithAlphaFromColor narrows the runtime bg without an inline literal.
	opaque := WithAlphaFromColor(bg, 0xFF)
	transparent := WithAlphaFromColor(bg, 0x00)
	if g.ShowTopFade() {
		band := image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+h)
		fillVerticalGradient(dst, band, opaque, transparent, controlGridFadeBands)
	}
	if g.ShowBottomFade() {
		band := image.Rect(r.Min.X, r.Max.Y-h, r.Max.X, r.Max.Y)
		fillVerticalGradient(dst, band, transparent, opaque, controlGridFadeBands)
	}
}
