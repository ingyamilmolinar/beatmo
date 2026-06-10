package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// button_cushion.go — render + math helpers backing the springy button
// "cushion" animation (press-scale + drop shadow + azure glow ring + inner
// pressed shadow). The Button state machine (pressAnim, toggled, PressScale,
// AdvancePressAnim, SetToggled) lives in uigrid.go; these are the pure helpers
// it calls from Draw. Contract is pinned by button_anim_test.go.

// scaleRectAboutCenter returns r scaled by `scale` about its center. A scale of
// 1 (rest) or 0 (guard) returns r unchanged; an empty rect stays empty.
func scaleRectAboutCenter(r image.Rectangle, scale float64) image.Rectangle {
	if r.Empty() || scale == 1 || scale == 0 {
		return r
	}
	cx := float64(r.Min.X+r.Max.X) / 2
	cy := float64(r.Min.Y+r.Max.Y) / 2
	hw := float64(r.Dx()) * scale / 2
	hh := float64(r.Dy()) * scale / 2
	return image.Rect(
		int(cx-hw+0.5), int(cy-hh+0.5),
		int(cx+hw+0.5), int(cy+hh+0.5),
	)
}

// buttonGlowAlpha returns the alpha of the azure accent glow ring. The
// single-chrome-accent discipline reserves the accent for interaction: 0 at
// rest, a steady value on hover, and a |sin| pulse when the button is latched
// (toggled). Toggled takes precedence over hover.
func buttonGlowAlpha(hovered, toggled bool, frame int64) uint8 {
	switch {
	case toggled:
		return SinPulseAlpha(frame, genAnimButtonTogglePulse)
	case hovered:
		// Steady hover glow at the pulse's resting level.
		return SinPulseAlpha(0, genAnimButtonTogglePulse)
	default:
		return 0
	}
}

// drawButtonDropShadow paints a soft 1-px drop shadow beneath a resting button
// so it reads as raised. Skipped while pressed (the caller draws the inner
// shadow instead).
func drawButtonDropShadow(dst *ebiten.Image, r image.Rectangle, rad int) {
	if r.Empty() {
		return
	}
	shadow := image.Rect(r.Min.X, r.Min.Y+1, r.Max.X, r.Max.Y+1)
	drawRoundedRect(dst, shadow, WithAlphaFromColor(color.Black, 64), rad, true)
}

// drawButtonGlowRing strokes an accent-coloured ring around the button at the
// given alpha (from buttonGlowAlpha). A zero alpha is a no-op so resting
// buttons carry no accent.
func drawButtonGlowRing(dst *ebiten.Image, r image.Rectangle, rad int, alpha uint8) {
	if alpha == 0 || r.Empty() {
		return
	}
	drawRoundedRect(dst, r, WithAlpha(TokenAccent(), alpha), rad, false)
}

// drawButtonInnerShadow darkens the top inner edge of a pressed button so it
// reads as pushed in. Intentionally subtle.
func drawButtonInnerShadow(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	top := image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+2)
	drawRect(dst, top, WithAlphaFromColor(color.Black, 48), true)
}
