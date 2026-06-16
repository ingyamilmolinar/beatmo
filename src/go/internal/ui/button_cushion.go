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

// buttonGlowAlpha returns the alpha of the cyan accent glow ring for a binary
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

// buttonGlowAlphaAnimated returns the alpha of the cyan accent glow ring for an
// in-flight hover fade. The single-chrome-accent discipline reserves the
// accent for interaction: a |sin| pulse when the button is latched (toggled,
// which takes precedence and ignores hover), otherwise the hover-rest ceiling
// scaled by hoverProgress (0..1) so the cursor-enter ramp glides smoothly from
// 0 up to the same token-driven level the static hover used to sit at.
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

// buttonGlowRingColor picks the Vice City accent shade for the glow ring by
// state: the hover ring uses primary-bright (#3FE0E8 — DESIGN.md's designated
// "Hover ring; never decorative"), while the latched/active pulse uses the
// base primary accent (#00C8E0). Both stay inside the single cyan
// chrome-accent family (no hot-pink — that is reserved for focus rings).
func buttonGlowRingColor(toggled bool) color.RGBA {
	if toggled {
		return TokenAccent()
	}
	return TokenAccentBright()
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
// given color and alpha (from buttonGlowRingColor / buttonGlowAlphaAnimated).
// A zero alpha is a no-op so resting buttons carry no accent.
func drawButtonGlowRing(dst *ebiten.Image, r image.Rectangle, rad int, col color.RGBA, alpha uint8) {
	if alpha == 0 || r.Empty() {
		return
	}
	drawRoundedRect(dst, r, WithAlpha(col, alpha), rad, false)
}

// drawButtonHoverGlow paints the high-visibility, cushioned hover affordance
// around r, with every layer's alpha scaled by the fade progress p (0..1):
//
//  1. a soft primary-bright bloom expanded by button-hover-glow-spread, so the
//     hover reads as a clear neon lift rather than a hairline;
//  2. a dark contrast keyline hugging the button edge, then a crisp 2-px
//     primary-bright ring just inside it — the dark backing makes the bright
//     ring legible on both dark chrome and bright accent fills (contrast both
//     ways);
//  3. a light top edge + dark bottom edge bevel that lifts the button toward
//     the cursor (a subtle 3D feel; light-from-above).
//
// Drawn on top of the (possibly cached) button by the hover overlay, so it is
// independent of any zone sprite cache. Desktop-only is enforced by the caller.
func drawButtonHoverGlow(dst *ebiten.Image, r image.Rectangle, rad int, p float64) {
	if p <= 0 || r.Empty() {
		return
	}
	if p > 1 {
		p = 1
	}
	bright := TokenAccentBright()
	// sa scales a base alpha by the fade progress.
	sa := func(base uint8) uint8 { return uint8(float64(base) * p) }
	spread := genGeomButtonHoverGlowSpread

	// 1) Soft outer bloom (two falloff steps for a smooth halo).
	drawRoundedRect(dst, r.Inset(-spread), WithAlpha(bright, sa(genAlphaSubtle)), rad+spread, false)
	drawRoundedRect(dst, r.Inset(-1), WithAlpha(bright, sa(genAlphaMedium)), rad+1, false)

	// 2) Dark contrast keyline at the very edge, then the crisp bright ring
	// (2 px) just inside it — bright-on-dark reads against any background.
	drawRoundedRect(dst, r, WithAlphaFromColor(color.Black, sa(genAlphaStrong)), rad, false)
	drawRoundedRect(dst, r.Inset(1), WithAlpha(bright, sa(genAlphaStrong)), rad, false)
	drawRoundedRect(dst, r.Inset(2), WithAlpha(bright, sa(genAlphaMedium)), rad, false)

	// 3) Bevel: bright top edge + dark bottom edge → a raised 3D cushion.
	if r.Dx() > 6 && r.Dy() > 6 {
		band := genGeomButtonInnerShadowPx
		top := image.Rect(r.Min.X+2, r.Min.Y+1, r.Max.X-2, r.Min.Y+1+band)
		drawRect(dst, top, WithAlpha(colTextPrimary, sa(genAlphaStrong)), true)
		bot := image.Rect(r.Min.X+2, r.Max.Y-1-band, r.Max.X-2, r.Max.Y-1)
		drawRect(dst, bot, WithAlphaFromColor(color.Black, sa(genAlphaMedium)), true)
	}
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
