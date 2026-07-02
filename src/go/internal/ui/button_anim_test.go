package ui

import (
	"image"
	"math"
	"testing"
)

// TestButtonPressSpring asserts the springy press/release depth lifecycle:
// rest = 0, press targets full depth (1) and eases toward it, release targets
// rest (0), kicks the cap UP (negative overshoot) then springs monotonically
// back and finally settles exactly at 0.
func TestButtonPressSpring(t *testing.T) {
	b := NewButton("Hit", InstButtonStyle, func() {})
	b.SetRect(image.Rect(100, 100, 160, 130))

	// Rest: a freshly constructed button reads as depth 0.
	if b.pressDepth != 0 || b.pressTarget != 0 {
		t.Fatalf("rest depth/target = %v/%v, want 0/0", b.pressDepth, b.pressTarget)
	}

	// Press inside → target full depth, depth eases toward 1.
	mx, my := 110, 110
	b.HandleInputResult(mx, my, true)
	if b.pressTarget != 1 {
		t.Fatalf("pressed target = %v, want 1", b.pressTarget)
	}
	for i := 0; i < 12; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth < 0.9 {
		t.Fatalf("held depth eased to %v, want ~1", b.pressDepth)
	}

	// Release → target rest, kick the cap UP (negative overshoot).
	b.HandleInputResult(mx, my, false)
	if b.pressTarget != 0 {
		t.Fatalf("released target = %v, want 0", b.pressTarget)
	}
	if b.pressDepth >= 0 {
		t.Fatalf("release must kick depth negative (overshoot up), got %v", b.pressDepth)
	}

	// Spring relaxes monotonically toward 0 and eventually settles at 0.
	prevDist := math.Abs(b.pressDepth)
	settled := false
	for i := 0; i < 500; i++ {
		b.AdvancePressAnim()
		dist := math.Abs(b.pressDepth)
		if dist > prevDist+1e-9 {
			t.Fatalf("spring diverged at step %d: dist %v > prev %v", i, dist, prevDist)
		}
		prevDist = dist
		if b.pressDepth == 0 {
			settled = true
			break
		}
	}
	if !settled {
		t.Fatalf("spring never settled to 0; final depth = %v", b.pressDepth)
	}

	// Settled spring is a no-op.
	b.AdvancePressAnim()
	if b.pressDepth != 0 {
		t.Fatalf("settled AdvancePressAnim moved depth off 0: %v", b.pressDepth)
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

// TestButtonGlowRingColor pins the neutral-lift hover/active ring color after
// the chrome de-accent pass (2026-06-15). Both hover and toggle states use
// TokenTextPrimary (near-white) so interaction feedback is brightness-based,
// not cyan. The previous two-shade cyan split (primary-bright/primary) was
// removed to de-accent the chrome.
func TestButtonGlowRingColor(t *testing.T) {
	hover := buttonGlowRingColor(false)
	tog := buttonGlowRingColor(true)
	if hover == TokenAccentBright() || tog == TokenAccent() {
		t.Fatalf("glow ring still cyan: hover=%v tog=%v", hover, tog)
	}
	if hover != TokenTextPrimary() {
		t.Fatalf("hover ring want neutral TokenTextPrimary, got %v", hover)
	}
	if tog != TokenTextPrimary() {
		t.Fatalf("toggled ring want neutral TokenTextPrimary, got %v", tog)
	}
}

// TestButtonGlowRingColorIsNeutral is the explicit post-de-accent guard: the
// hover/toggle glow ring must be neutral (no cyan accent) — chrome
// de-accent pass 2026-06-15.
func TestButtonGlowRingColorIsNeutral(t *testing.T) {
	hover := buttonGlowRingColor(false)
	tog := buttonGlowRingColor(true)
	if hover == TokenAccentBright() || tog == TokenAccent() {
		t.Fatalf("glow ring still cyan: hover=%v tog=%v", hover, tog)
	}
	if hover != TokenTextPrimary() {
		t.Fatalf("hover ring want neutral TokenTextPrimary, got %v", hover)
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
