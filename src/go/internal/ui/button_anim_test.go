package ui

import (
	"image"
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

// TestButtonGlowAlpha asserts the single-chrome-accent discipline: the azure
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
