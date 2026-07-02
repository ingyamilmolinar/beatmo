package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// drawSliderRail renders a rounded neon rail: a dim full-length base, then a
// neon fill covering the value (0..1) fraction, plus one subtle glow stroke.
// horizontal: fill grows left->right; vertical: fill grows bottom->top. fillCol
// is the accent (cyan for popup/param slider, the row instrument color for the
// in-row indicator). value<=0 draws the base rail only.
func drawSliderRail(dst *ebiten.Image, rail image.Rectangle, value float64, horizontal bool, fillCol color.RGBA) {
	if rail.Empty() {
		return
	}
	if value < 0 {
		value = 0
	}
	if value > 1 {
		value = 1
	}
	// Radius = half of the narrow axis so the rail ends are fully pill-shaped.
	narrow := rail.Dy()
	if rail.Dx() < narrow {
		narrow = rail.Dx()
	}
	rad := narrow / 2
	drawRoundedRect(dst, rail, genColorSurface3, rad, true)
	if value <= 0 {
		return
	}
	var fill image.Rectangle
	if horizontal {
		fw := int(float64(rail.Dx()) * value)
		if fw <= 0 {
			return
		}
		fill = image.Rect(rail.Min.X, rail.Min.Y, rail.Min.X+fw, rail.Max.Y)
	} else {
		fh := int(float64(rail.Dy()) * value)
		if fh <= 0 {
			return
		}
		fill = image.Rect(rail.Min.X, rail.Max.Y-fh, rail.Max.X, rail.Max.Y)
	}
	drawRoundedRect(dst, fill, fillCol, rad, true)
	// Matte rail — no outer glow stroke (retro-analogue restyle 2026-06-17).
}

// drawSliderThumb renders a matte cap thumb centered at center. A neutral edge
// rim conveys depth; active strengthens that rim (during drag) while calm
// halves it (used by many-on-screen param sliders so they don't shimmer). No
// specular highlight, no cyan rim, no bloom — matte retro-analogue restyle
// (2026-06-17).
func drawSliderThumb(dst *ebiten.Image, center image.Point, diameter int, active, calm bool) {
	if diameter < 2 {
		return
	}
	rad := diameter / 2
	r := image.Rect(center.X-rad, center.Y-rad, center.X+rad, center.Y+rad)
	// Contact shadow 1px below → reads as a raised cap.
	drawRoundedRect(dst, image.Rect(r.Min.X, r.Min.Y+1, r.Max.X, r.Max.Y+1),
		WithAlphaFromColor(color.Black, 64), rad, true)
	// Matte body — no specular highlight.
	drawRoundedRect(dst, r, genColorOnSurface, rad, true)
	// Neutral edge rim: stronger while dragging (active), halved when calm. No
	// cyan accent.
	ringAlpha := genAlphaMedium
	if active {
		ringAlpha = genAlphaStrong
	}
	if calm {
		ringAlpha /= 2
	}
	drawRoundedRect(dst, r, WithAlpha(colTextPrimary, ringAlpha), rad, false)
}
