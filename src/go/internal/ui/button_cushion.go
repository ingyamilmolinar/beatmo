package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

// button_cushion.go — render + math helpers backing the button "cushion" /
// keycap visuals (keycap shell + cap travel + contact shadow + engage flash,
// plus the glow alpha / ring color helpers used by animation and tests). The
// Button state machine (pressDepth, pressTarget, toggled, AdvancePressAnim,
// SetToggled) lives in uigrid.go; these are the pure helpers it calls from
// Draw. Contract is pinned by button_anim_test.go.

// buttonGlowAlpha returns the alpha of the neutral glow ring for a binary
// hover state. It is a thin wrapper over buttonGlowAlphaAnimated (hover
// progress 1 when hovered, 0 otherwise) kept for call sites and tests that
// reason about the settled hover level rather than the in-flight fade.
func buttonGlowAlpha(hovered, toggled bool, frame int64) uint8 {
	p := 0.0
	if hovered {
		p = 1
	}
	return buttonGlowAlphaAnimated(p, toggled, frame)
}

// buttonGlowAlphaAnimated returns the alpha of the neutral glow ring for an
// in-flight hover fade. A |sin| pulse fires when the button is latched
// (toggled, which takes precedence and ignores hover); otherwise the
// hover-rest ceiling is scaled by hoverProgress (0..1) so the cursor-enter
// ramp glides smoothly from 0 up to the token-driven level.
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
	// Hover-rest ceiling = a fraction (button-glow-rest) of the toggle pulse's
	// own alpha scale, so the rest glow is token-governed rather than a
	// hand-derived sample. Scaled by the fade progress.
	return uint8(float64(genAnimButtonTogglePulse.AlphaScale) * float64(genAnimButtonGlowRest) * hoverProgress)
}

// buttonGlowRingColor returns the neutral hover/toggle ring color. The chrome
// de-accent pass (2026-06-15) removed cyan from interaction feedback: hover
// and latched states both use a neutral white lift (TokenTextPrimary) so
// affordance comes from brightness rather than hue. The toggled parameter is
// retained for callers that may need to differentiate in the future.
func buttonGlowRingColor(toggled bool) color.RGBA {
	return TokenTextPrimary()
}

// keycapHoverLiftAlpha is the matte desktop hover affordance: a flat ~8% white
// lift of the whole cap (no rim, no specular catch, no neon bloom) at full fade
// progress. Mobile never hovers (the caller skips it). Retro-analogue restyle
// 2026-06-17 — hover used to stroke a hairline rim + warm top highlight, which
// read as shine; now it is a single uniform brighten.
const keycapHoverLiftAlpha = 22

// drawButtonHoverGlow paints the matte desktop hover affordance over a keycap:
// a flat, uniform lift of the whole cap fill scaled by the fade progress p
// (0..1). Deliberately NOT a rim/specular/bloom — hover is a quiet "this is
// reachable" brighten. Desktop-only (the caller enforces this and skips it on
// mobile).
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

// drawButtonInnerShadow darkens the top inner edge of a pressed button so it
// reads as pushed in. Intentionally subtle.
func drawButtonInnerShadow(dst *ebiten.Image, r image.Rectangle) {
	if r.Empty() {
		return
	}
	// Band thickness is the button-inner-shadow-px geometry token (the inner
	// pressed-in shadow inset thickness), measured down from the top edge.
	top := image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+genGeomButtonInnerShadowPx)
	drawRect(dst, top, WithAlphaFromColor(color.Black, 48), true)
}
