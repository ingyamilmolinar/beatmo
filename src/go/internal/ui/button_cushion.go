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

// keycapCapRect returns the rect the cap face occupies inside the button
// rect r. The cap height is constant (r.Dy - wall) so its sprite cache is
// invariant across press travel — only its Y position moves. travelPx ∈
// [0, wall] descends the cap toward bottom-out; raised lifts a latched/
// active cap by keycap-active-raise px above rest.
func keycapCapRect(r image.Rectangle, travelPx int, raised bool) image.Rectangle {
	wall := genGeomKeycapWallDepth
	capH := r.Dy() - wall
	if capH < 1 {
		capH = r.Dy() // too short for a wall; degrade to a flat cap
		wall = 0
	}
	top := r.Min.Y + travelPx
	if raised {
		top -= genGeomKeycapActiveRaise
	}
	return image.Rect(r.Min.X, top, r.Max.X, top+capH)
}

// drawKeycapShell paints the dark socket/side-wall that the cap sits in. The
// cap is blitted on top; whatever shell shows below (rest) or above+below
// (pressed) reads as the extruded wall / sunken socket. shellCol is derived
// from the cap fill (no literal) so it stays in-family on any button color.
func drawKeycapShell(dst *ebiten.Image, r image.Rectangle, rad int, fill color.Color) {
	if r.Empty() || genGeomKeycapWallDepth == 0 {
		return
	}
	// Fill-only rounded body via the cached fill+border composite (border ==
	// fill so there is no visible stroke). The shell rect is static, so this
	// reuses the (w,h,fill,border,radius) sprite cache and blits once instead
	// of running the uncached drawRoundedRect primitive every frame.
	shell := adjustColor(fill, -45)
	drawRoundedButton(dst, r, shell, shell, rad, false)
}

// drawKeycapContactShadow paints a soft 1-px contact shadow just under the
// resting key so it reads as raised above the panel. Skipped when bottomed-out.
func drawKeycapContactShadow(dst *ebiten.Image, capRect image.Rectangle, rad int) {
	if capRect.Empty() {
		return
	}
	sh := image.Rect(capRect.Min.X, capRect.Max.Y, capRect.Max.X, capRect.Max.Y+1)
	drawRoundedRect(dst, sh, WithAlphaFromColor(color.Black, 64), rad, true)
}

// keycap bevel colors, pre-boxed once as typed color.Color interfaces so the
// per-frame bevel draw copies an interface header instead of boxing a fresh
// color value every button, every frame (same allocation discipline as the
// pill palette).
//
// Matte hardware bevel (retro-analogue restyle 2026-06-17): a FAINT light top
// edge + a soft dark bottom lip frame the flat cap fill so a key reads as
// chamfered — NOT glossy. The old broad ~45% sheen band and the crisp alpha-120
// specular line (which read as shine) are gone; the top edge is now a quiet
// matte highlight.
var (
	keycapBevelTopCol color.Color = WithAlpha(colTextPrimary, 40) // faint light top edge
	keycapBevelBotCol color.Color = WithAlphaFromColor(color.Black, 120)
	// Latched/active "lamp" amber — an on-palette sunset-gold member (#FFB30A)
	// used as a SOLID fill, never a glow. A latched key looks pressed-IN
	// (inverted bevel, drawKeycapActiveInset) and lit amber, like a switch that
	// stays down. Dark text sits on it (genColorBackground).
	keycapActiveFill        color.Color = genColorSunsetGold300         // #FFB30A warm lamp amber
	keycapActiveBotLightCol color.Color = WithAlpha(colTextPrimary, 50) // faint bottom-light of the inverted bevel
)

// drawKeycapBevel overlays the resting matte bevel on a cap face so it reads as
// a slim, chamfered key even when idle: a faint 1-px light top edge and a soft
// dark bottom-inner-shadow lip — no sheen band, no specular shine. Inset
// horizontally so the bands stay inside the rounded corners. Two pre-boxed
// drawRects — allocation-free.
func drawKeycapBevel(dst *ebiten.Image, capRect image.Rectangle, rad int) {
	if capRect.Dx() < 2*rad+4 || capRect.Dy() < 8 {
		return
	}
	// Faint light top edge.
	top := image.Rect(capRect.Min.X+rad, capRect.Min.Y+1, capRect.Max.X-rad, capRect.Min.Y+2)
	drawRect(dst, top, keycapBevelTopCol, true)
	// Soft dark bottom inner-shadow lip.
	band := genGeomButtonInnerShadowPx
	bot := image.Rect(capRect.Min.X+rad, capRect.Max.Y-1-band, capRect.Max.X-rad, capRect.Max.Y-1)
	drawRect(dst, bot, keycapBevelBotCol, true)
}

// drawKeycapActiveInset overlays the latched/active cushion on a cap face so it
// reads as pressed-IN (recessed): a top inner-shadow + a faint bottom light
// edge — the inverse of the resting raised bevel. No glow, no flash; the amber
// fill (painted by the caller) plus this inverted bevel ARE the active signal.
func drawKeycapActiveInset(dst *ebiten.Image, capRect image.Rectangle, rad int) {
	if capRect.Dx() < 2*rad+4 || capRect.Dy() < 8 {
		return
	}
	band := genGeomButtonInnerShadowPx
	// Top inner-shadow (pressed-in).
	top := image.Rect(capRect.Min.X+rad, capRect.Min.Y+1, capRect.Max.X-rad, capRect.Min.Y+1+band)
	drawRect(dst, top, keycapBevelBotCol, true)
	// Faint bottom light edge.
	bot := image.Rect(capRect.Min.X+rad, capRect.Max.Y-2, capRect.Max.X-rad, capRect.Max.Y-1)
	drawRect(dst, bot, keycapActiveBotLightCol, true)
}

// drawKeycapPanelButton renders a transient popup/panel action as a RAISED
// keycap (dark socket/side-wall + contact shadow + lit cap face) and returns
// the cap rect so the caller can center the label on the cap. Hover brightens
// the cap. No press-travel spring — these popups are short-lived. The cap fill
// carries the button's own (possibly semantic) color; the socket derives from
// it, so a rust Delete reads as a red key in a red-tinted socket.
func drawKeycapPanelButton(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, hovered bool) image.Rectangle {
	rad := popupButtonRadius()
	if hovered {
		fill = adjustColor(fill, 12)
		border = adjustColor(border, 20)
	}
	cap := keycapCapRect(r, 0, true)
	drawKeycapShell(dst, r, rad, fill)
	drawKeycapContactShadow(dst, cap, rad)
	drawRoundedButton(dst, cap, fill, border, rad, false)
	drawKeycapBevel(dst, cap, rad)
	return cap
}

// drawKeycapEngageFlash is a no-op after the matte retro-analogue restyle
// (2026-06-17): a latched key no longer flashes cyan. The engageAnim machinery
// (set on off→on in drawPillTabAt, decayed by AdvancePressAnim) is retained so
// the press-spring contract and its tests are unchanged, but nothing is
// painted — the static inverted-bevel amber cap (drawKeycapActiveInset) is the
// only active signal.
func drawKeycapEngageFlash(dst *ebiten.Image, capRect image.Rectangle, rad int, progress float64) {
	_, _, _, _ = dst, capRect, rad, progress
}
