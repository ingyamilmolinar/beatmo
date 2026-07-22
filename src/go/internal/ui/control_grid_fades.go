package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// controlGridFadeBands is the band count for the edge-fade gradients — enough for
// a smooth dissolve at panel scale while staying cheap (fillVerticalGradient is
// per-frame alloc-free, backed by pixelCache).
const controlGridFadeBands = 8

// drawControlGridEdgeFades paints the scroll affordance for a ControlGrid: a top
// fade when content is scrolled down from the first row, and a bottom fade over
// the reserved peek band whenever more rows exist below. Both dissolve the grid's
// content into the panel surface bg, so the fade itself is the "more below" (or
// "more above") signal. No-op when the grid fits (no peek band reserved).
func drawControlGridEdgeFades(dst *ebiten.Image, g *ControlGrid, bg color.Color) {
	h := g.PeekH()
	if h <= 0 {
		return
	}
	r := g.ContentRect()
	if r.Empty() {
		return
	}
	// Opaque + fully-transparent variants of the panel surface bg, so the
	// gradient dissolves grid content into the surface. WithAlphaFromColor
	// narrows the runtime bg (unknown concrete type) — no inline literal.
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
