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
	// Subtle glow stroke: inset -1 (expand outward by 1px), stroke only.
	drawRoundedRect(dst, fill.Inset(-1), WithAlpha(fillCol, AlphaSubtle), rad+1, false)
}

// drawSliderThumb renders a round glowing thumb centered at center. active
// brightens the glow (during drag); calm lowers the glow ceiling and skips the
// outer bloom (used by many-on-screen param sliders so they don't shimmer).
func drawSliderThumb(dst *ebiten.Image, center image.Point, diameter int, active, calm bool) {
	if diameter < 2 {
		return
	}
	rad := diameter / 2
	r := image.Rect(center.X-rad, center.Y-rad, center.X+rad, center.Y+rad)
	ring := buttonGlowRingColor(active)
	if !calm {
		spread := genGeomButtonHoverGlowSpread
		drawRoundedRect(dst, r.Inset(-spread), WithAlpha(ring, AlphaSubtle), rad+spread, false)
	}
	drawRoundedRect(dst, r, genColorOnSurface, rad, true)
	// Rest-glow ceiling: same derivation as buttonGlowAlphaAnimated at hoverProgress=1
	// (uint8(AlphaScale * GlowRest * 1.0)). calm halves it; active uses AlphaStrong.
	restGlow := uint8(float64(genAnimButtonTogglePulse.AlphaScale) * float64(genAnimButtonGlowRest))
	ringAlpha := restGlow
	if calm {
		ringAlpha = restGlow / 2
	}
	if active {
		ringAlpha = AlphaStrong
	}
	drawRoundedRect(dst, r, WithAlpha(ring, ringAlpha), rad, false)
}
