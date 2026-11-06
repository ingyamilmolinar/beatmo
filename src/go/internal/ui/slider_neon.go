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
	// A slider rail is a CAPSULE, not an over-rounded button: its ends are
	// semicircles by design. drawCapsule derives the radius from the rect and
	// is exempt from the anti-pill ceiling — see DESIGN.md "Shapes".
	drawCapsule(dst, rail, genColorSurface3, true)
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
	drawCapsule(dst, fill, fillCol, true)
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
	// A thumb is a CIRCLE — a capsule whose axes are equal — not a rounded
	// button, so it draws through drawCapsule and is exempt from the anti-pill
	// ceiling. See DESIGN.md "Shapes".
	// Contact shadow 1px below → reads as a raised cap.
	drawCapsule(dst, image.Rect(r.Min.X, r.Min.Y+1, r.Max.X, r.Max.Y+1),
		WithAlphaFromColor(color.Black, 64), true)
	// Matte body — no specular highlight.
	drawCapsule(dst, r, genColorOnSurface, true)
	// Neutral edge rim: stronger while dragging (active), halved when calm. No
	// cyan accent.
	ringAlpha := genAlphaMedium
	if active {
		ringAlpha = genAlphaStrong
	}
	if calm {
		ringAlpha /= 2
	}
	drawCapsule(dst, r, WithAlpha(colTextPrimary, ringAlpha), false)
}
