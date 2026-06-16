package ui

import (
	"image"
	"image/color"
	"math"
	"testing"
)

// TestButtonPressSpring asserts the springy press/release scale lifecycle:
// rest = 1.0, press snaps to genGeomButtonPressScale (0.92), release snaps to
// genGeomButtonReleaseOvershoot (1.03), then AdvancePressAnim relaxes back
// toward 1.0 and finally settles exactly at 1.0.
func TestButtonPressSpring(t *testing.T) {
	b := NewButton("Hit", InstButtonStyle, func() {})
	b.SetRect(image.Rect(100, 100, 160, 130))

	// Rest: a freshly constructed button reads as scale 1.0 (zero → 1.0).
	if got := b.PressScale(); got != 1 {
		t.Fatalf("rest PressScale = %v, want 1", got)
	}

	// Press inside → press-floor scale.
	mx, my := 110, 110
	b.HandleInputResult(mx, my, true)
	if got := b.PressScale(); got != genGeomButtonPressScale {
		t.Fatalf("pressed PressScale = %v, want %v (button-press-scale)", got, genGeomButtonPressScale)
	}
	if genGeomButtonPressScale >= 1 {
		t.Fatalf("button-press-scale must shrink the button (<1), got %v", genGeomButtonPressScale)
	}

	// Release → overshoot past rest.
	b.HandleInputResult(mx, my, false)
	if got := b.PressScale(); got != genGeomButtonReleaseOvershoot {
		t.Fatalf("released PressScale = %v, want %v (button-release-overshoot)", got, genGeomButtonReleaseOvershoot)
	}
	if genGeomButtonReleaseOvershoot <= 1 {
		t.Fatalf("button-release-overshoot must overshoot rest (>1), got %v", genGeomButtonReleaseOvershoot)
	}

	// Spring relaxes monotonically toward 1.0 and eventually settles at 1.0.
	prevDist := math.Abs(b.PressScale() - 1)
	settled := false
	for i := 0; i < 500; i++ {
		b.AdvancePressAnim()
		dist := math.Abs(b.PressScale() - 1)
		if dist > prevDist+1e-9 {
			t.Fatalf("spring diverged at step %d: dist %v > prev %v", i, dist, prevDist)
		}
		prevDist = dist
		if b.PressScale() == 1 {
			settled = true
			break
		}
	}
	if !settled {
		t.Fatalf("spring never settled to 1.0; final scale = %v", b.PressScale())
	}

	// Settled spring is a no-op.
	b.AdvancePressAnim()
	if got := b.PressScale(); got != 1 {
		t.Fatalf("settled AdvancePressAnim moved scale off 1.0: %v", got)
	}
}

// TestButtonToggleState covers the latched/active flag that drives the
// pulsing accent glow.
func TestButtonToggleState(t *testing.T) {
	b := NewButton("Tab", InstButtonStyle, nil)
	if b.Toggled() {
		t.Fatal("new button must not be toggled")
	}
	b.SetToggled(true)
	if !b.Toggled() {
		t.Fatal("SetToggled(true) did not latch")
	}
	b.SetToggled(false)
	if b.Toggled() {
		t.Fatal("SetToggled(false) did not clear")
	}
}

// TestScaleRectAboutCenter asserts the press-scale geometry: shrinking keeps
// the rect centered and smaller; scale 1 / zero / empty returns the input.
func TestScaleRectAboutCenter(t *testing.T) {
	r := image.Rect(100, 100, 200, 140) // center (150,120), 100x40
	if got := scaleRectAboutCenter(r, 1); got != r {
		t.Fatalf("scale 1 must be identity, got %v", got)
	}
	if got := scaleRectAboutCenter(r, 0); got != r {
		t.Fatalf("scale 0 must be identity (guard), got %v", got)
	}
	if got := scaleRectAboutCenter(image.Rectangle{}, 0.9); !got.Empty() {
		t.Fatalf("empty input must stay empty, got %v", got)
	}
	got := scaleRectAboutCenter(r, 0.5)
	if got.Dx() >= r.Dx() || got.Dy() >= r.Dy() {
		t.Fatalf("scale 0.5 must shrink, got %v from %v", got, r)
	}
	// Center preserved (within rounding).
	wantCx, wantCy := 150, 120
	gotCx := (got.Min.X + got.Max.X) / 2
	gotCy := (got.Min.Y + got.Max.Y) / 2
	if abs(gotCx-wantCx) > 1 || abs(gotCy-wantCy) > 1 {
		t.Fatalf("center drifted: got (%d,%d), want (%d,%d)", gotCx, gotCy, wantCx, wantCy)
	}
}

// (abs is provided by game_math.go)

// TestButtonGlowAlpha asserts the single-chrome-accent discipline: the cyan
// glow ring is invisible at rest (accent reserved for interaction), present on
// hover, and present (pulsing) when toggled.
func TestButtonGlowAlpha(t *testing.T) {
	if a := buttonGlowAlpha(false, false, 0); a != 0 {
		t.Fatalf("resting glow alpha must be 0 (no accent at rest), got %d", a)
	}
	if a := buttonGlowAlpha(true, false, 0); a == 0 {
		t.Fatal("hovered glow alpha must be > 0")
	}
	// Toggled glow is frame-driven; sample a few frames and require at least
	// one visibly non-zero (the |sin| pulse passes through 0 once per cycle).
	anyVisible := false
	for f := int64(0); f < 80; f++ {
		if buttonGlowAlpha(false, true, f) > 0 {
			anyVisible = true
			break
		}
	}
	if !anyVisible {
		t.Fatal("toggled glow never became visible across a pulse cycle")
	}
}

// TestButtonGlowAlphaAnimated covers the smooth fade-in/out hover glow: the
// alpha scales linearly with a 0..1 hover progress so the cursor-enter ramp
// glides from 0 to the same token-driven ceiling the static hover used, while
// the toggled (latched) pulse ignores progress entirely.
func TestButtonGlowAlphaAnimated(t *testing.T) {
	// Progress 0 → no glow (rest / fully faded out).
	if a := buttonGlowAlphaAnimated(0, false, 0); a != 0 {
		t.Fatalf("hover progress 0 glow = %d, want 0", a)
	}
	// Progress 1, untoggled → the exact token-driven ceiling (== old static
	// hover level) so the fade settles where the static glow used to sit.
	want := uint8(float64(genAnimButtonTogglePulse.AlphaScale) * float64(genAnimButtonGlowRest))
	if a := buttonGlowAlphaAnimated(1, false, 0); a != want {
		t.Fatalf("hover progress 1 glow = %d, want token-driven %d", a, want)
	}
	// Monotonic non-decreasing in progress — a smooth ramp, no dips.
	prev := uint8(0)
	for i := 0; i <= 10; i++ {
		p := float64(i) / 10
		got := buttonGlowAlphaAnimated(p, false, 0)
		if got < prev {
			t.Fatalf("glow not monotonic at p=%.1f: %d < %d", p, got, prev)
		}
		prev = got
	}
	// Toggled ignores hover progress: even at progress 0 the latched pulse is
	// visible across a cycle (precedence: toggled over hover, unchanged).
	anyVisible := false
	for f := int64(0); f < 80; f++ {
		if buttonGlowAlphaAnimated(0, true, f) > 0 {
			anyVisible = true
			break
		}
	}
	if !anyVisible {
		t.Fatal("toggled glow never visible across a pulse cycle (progress must be ignored when toggled)")
	}
}

// Hover-fade ramp behavior + the desktop-only / mobile-flat rules now live in
// the per-frame overlay (hover_glow_overlay.go) rather than on Button — see
// TestHoverGlowOverlay* in hover_glow_overlay_test.go (functional, through the
// real tree Draw path).

// TestButtonGlowRingColor pins the Vice City two-shade hover/active split: the
// hover ring uses primary-bright (#3FE0E8, DESIGN.md "Hover ring"), while the
// latched/active pulse uses the base primary accent (#00C8E0).
func TestButtonGlowRingColor(t *testing.T) {
	if got := buttonGlowRingColor(false); got != TokenAccentBright() {
		t.Fatalf("hover ring color = %v, want primary-bright %v", got, TokenAccentBright())
	}
	if got := buttonGlowRingColor(true); got != TokenAccent() {
		t.Fatalf("toggled ring color = %v, want primary %v", got, TokenAccent())
	}
	// Guard the two shades are actually distinct (a regression that aliased
	// them would silently erase the hover-vs-active signal).
	var _ color.RGBA = buttonGlowRingColor(false)
	if buttonGlowRingColor(false) == buttonGlowRingColor(true) {
		t.Fatal("hover and active ring colors must differ (primary-bright vs primary)")
	}
}

// TestButtonGlowAlphaHoverIsTokenDriven pins the hover-rest glow level to the
// button-glow-rest design token (a fraction of the toggle pulse's own alpha
// scale) instead of a hand-derived SinPulse sample, so the resting glow is
// governed by DESIGN.md, not a magic constant.
func TestButtonGlowAlphaHoverIsTokenDriven(t *testing.T) {
	want := uint8(float64(genAnimButtonTogglePulse.AlphaScale) * float64(genAnimButtonGlowRest))
	if got := buttonGlowAlpha(true, false, 0); got != want {
		t.Fatalf("hover glow alpha = %d, want token-driven %d", got, want)
	}
}
