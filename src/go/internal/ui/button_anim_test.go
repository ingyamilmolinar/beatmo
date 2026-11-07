package ui

import (
	"image"
	"math"
	"testing"
)

// 0 at rest; a press eases toward 1; a release overshoots below 0, springs back monotonically and settles at exactly 0.
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

// The latched flag that drives the pulsing glow.
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

// Invisible at rest (the accent is reserved for interaction), present on hover and pulsing when toggled.
func TestButtonGlowAlpha(t *testing.T) {
	if a := buttonGlowAlpha(false, false, 0); a != 0 {
		t.Fatalf("resting glow alpha must be 0 (no accent at rest), got %d", a)
	}
	if a := buttonGlowAlpha(true, false, 0); a == 0 {
		t.Fatal("hovered glow alpha must be > 0")
	}
	// |sin| passes through 0 once per cycle, so sample several frames.
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

// Linear in the 0..1 hover progress up to the static hover ceiling; the toggled pulse ignores progress.
func TestButtonGlowAlphaAnimated(t *testing.T) {
	// Progress 0 → no glow (rest / fully faded out).
	if a := buttonGlowAlphaAnimated(0, false, 0); a != 0 {
		t.Fatalf("hover progress 0 glow = %d, want 0", a)
	}
	// The exact token-driven ceiling.
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
	// Toggled wins over hover.
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

// Hover and toggle both use TokenTextPrimary: feedback is brightness, not a coloured accent.
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

// Neutral, no accent hue.
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

// The button-glow-rest token, so DESIGN.md governs the resting glow, not a magic constant.
func TestButtonGlowAlphaHoverIsTokenDriven(t *testing.T) {
	want := uint8(float64(genAnimButtonTogglePulse.AlphaScale) * float64(genAnimButtonGlowRest))
	if got := buttonGlowAlpha(true, false, 0); got != want {
		t.Fatalf("hover glow alpha = %d, want token-driven %d", got, want)
	}
}
