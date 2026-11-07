package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// Pure helpers the Button state machine (uigrid.go) calls from Draw.

// The settled level, for callers that ignore the in-flight fade.
func buttonGlowAlpha(hovered, toggled bool, frame int64) uint8 {
	p := 0.0
	if hovered {
		p = 1
	}
	return buttonGlowAlphaAnimated(p, toggled, frame)
}

// Toggled pulses and ignores hover; otherwise the rest level scales with hoverProgress so the enter ramp glides.
func buttonGlowAlphaAnimated(hoverProgress float64, toggled bool, frame int64) uint8 {
	if toggled {
		return SinPulseAlpha(frame, genAnimButtonTogglePulse)
	}
	if hoverProgress <= 0 {
		return 0
	}
	if hoverProgress > 1 {
		hoverProgress = 1
	}
	// A token fraction of the toggle pulse's alpha scale, so the rest glow is token-governed.
	return uint8(float64(genAnimButtonTogglePulse.AlphaScale) * float64(genAnimButtonGlowRest) * hoverProgress)
}

// Neutral white for both states: affordance comes from brightness, not hue. toggled is unused.
func buttonGlowRingColor(toggled bool) color.RGBA {
	return TokenTextPrimary()
}

// A flat ~8% lift of the whole cap: hover is a quiet brighten, never a rim or shine.
const keycapHoverLiftAlpha = 22

// Desktop only; mobile never hovers.
func drawButtonHoverGlow(dst *ebiten.Image, r image.Rectangle, rad int, p float64) {
	if p <= 0 || r.Empty() {
		return
	}
	if p > 1 {
		p = 1
	}
	a := uint8(float64(keycapHoverLiftAlpha) * p)
	drawRoundedRect(dst, r, WithAlpha(colTextPrimary, a), rad, true)
}

func drawButtonInnerShadow(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	top := image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+genGeomButtonInnerShadowPx)
	drawRect(dst, top, WithAlphaFromColor(color.Black, 48), true)
}
