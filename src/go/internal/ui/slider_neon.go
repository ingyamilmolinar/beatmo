package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Horizontal fills left→right, vertical bottom→top; value <= 0 draws the base rail only.
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
	// A rail is a CAPSULE by design, so drawCapsule is exempt from the anti-pill ceiling.
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
}

// Matte: active strengthens the rim while dragging; calm halves it so many on-screen sliders do not shimmer.
func drawSliderThumb(dst *ebiten.Image, center image.Point, diameter int, active, calm bool) {
	if diameter < 2 {
		return
	}
	rad := diameter / 2
	r := image.Rect(center.X-rad, center.Y-rad, center.X+rad, center.Y+rad)
	// A thumb is a circle, a capsule with equal axes, so drawCapsule. The 1 px contact shadow makes it read raised.
	drawCapsule(dst, image.Rect(r.Min.X, r.Min.Y+1, r.Max.X, r.Max.Y+1),
		WithAlphaFromColor(color.Black, 64), true)
	// Matte body — no specular highlight.
	drawCapsule(dst, r, genColorOnSurface, true)
	ringAlpha := genAlphaMedium
	if active {
		ringAlpha = genAlphaStrong
	}
	if calm {
		ringAlpha /= 2
	}
	drawCapsule(dst, r, WithAlpha(colTextPrimary, ringAlpha), false)
}
