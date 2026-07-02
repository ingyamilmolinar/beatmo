//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// hoverGlowTestTreeH builds a DrumViewTree with one zone exposing a single hit
// area (rect + handler) at hitRect, lays it out, and returns the tree.
func hoverGlowTestTreeH(t *testing.T, hitRect image.Rectangle, handler HitHandler) *DrumViewTree {
	t.Helper()
	tree := NewDrumViewTree()
	z := newTestZone("tz")
	z.hitAreas = []HitArea{{Rect: hitRect, ZIndex: 110, Handler: handler, Tag: "ctrl"}}
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("tz", image.Rect(0, 0, 800, 100))
	tree.Update() // layout + publish hit areas into the HitIndex
	return tree
}

// hoverGlowTestTree is the button convenience wrapper used by the core tests.
func hoverGlowTestTree(t *testing.T, btnRect image.Rectangle) (*DrumViewTree, *Button) {
	t.Helper()
	b := NewButton("", InstButtonStyle, func() {})
	b.SetRect(btnRect)
	return hoverGlowTestTreeH(t, btnRect, &buttonHitAdapter{btn: b}), b
}

// fillDraw records one FILLED drawRoundedRect call.
type fillDraw struct {
	rect image.Rectangle
	col  color.NRGBA
}

// captureFills intercepts drawRoundedRect and records every FILLED rect
// (filled=true). Returns a snapshot getter + restore. The matte hover
// affordance (retro-analogue restyle 2026-06-17) is a flat FILLED lift of the
// control rect — no stroked rim — so the tests record fills, not rings.
func captureFills() (fills func() []fillDraw, restore func()) {
	var got []fillDraw
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, cc color.Color, radius int, filled bool) {
		if filled {
			if nc, ok := cc.(color.NRGBA); ok {
				got = append(got, fillDraw{r, nc})
			}
		}
		orig(dst, r, cc, radius, filled)
	}
	return func() []fillDraw { return got }, func() { drawRoundedRect = orig }
}

// liftAlphaOver reports the max-alpha neutral-white FILLED lift whose rect
// matches `around` within `slack` px on all four edges. The matte hover lift is
// a flat fill of the on-surface neutral (TokenTextPrimary) over the control
// rect — no rim, no specular. Returns alpha 0 when none.
func liftAlphaOver(fills []fillDraw, around image.Rectangle, slack int) uint8 {
	want := TokenTextPrimary()
	var best uint8
	for _, fd := range fills {
		if fd.col.R != want.R || fd.col.G != want.G || fd.col.B != want.B {
			continue
		}
		if abs(fd.rect.Min.X-around.Min.X) <= slack && abs(fd.rect.Min.Y-around.Min.Y) <= slack &&
			abs(fd.rect.Max.X-around.Max.X) <= slack && abs(fd.rect.Max.Y-around.Max.Y) <= slack {
			if fd.col.A > best {
				best = fd.col.A
			}
		}
	}
	return best
}

// TestHoverGlowOverlayDrawsRingOnButtonHover is the functional regression test:
// with the cursor parked over a button registered in the tree's HitIndex, the
// tree's own Draw path must paint a restrained neutral FILL lift over that
// button — proving the hover affordance works through the REAL input/draw loop
// (not just helper unit tests). The matte lift is a flat ~8% white brighten
// (keycapHoverLiftAlpha), NOT a rim or bloom. This is the bug the user
// reported: no hover on any button.
func TestHoverGlowOverlayDrawsRingOnButtonHover(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	btnRect := image.Rect(50, 20, 110, 50)
	restore := SetInputForTest(
		func() (int, int) { return 80, 35 },            // center of btnRect
		func(ebiten.MouseButton) bool { return false }, // not pressed (plain hover)
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree, _ := hoverGlowTestTree(t, btnRect)

	fills, restoreFill := captureFills()
	defer restoreFill()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ { // advance the fade well past zero
		tree.Draw(screen)
	}

	// A neutral matte lift must be painted over the button. It is a flat,
	// subtle brighten (keycapHoverLiftAlpha ≈ 22) — it must be present but must
	// NOT be a bold bloom (the old implementation reached >=120 via a strong
	// inner ring).
	a := liftAlphaOver(fills(), btnRect, genGeomButtonHoverGlowSpread+1)
	if a == 0 {
		t.Fatal("no matte hover lift painted over the hovered button (affordance dead)")
	}
	if a > 40 {
		t.Fatalf("hover lift too bold/bloom-like: alpha=%d, want subtle flat lift (<=40)", a)
	}
}

// hoverGlowExpectRing drives the tree with the cursor at (cx,cy) for 30 frames
// and returns the max-alpha neutral matte lift near `area`.
func hoverGlowExpectRing(t *testing.T, tree *DrumViewTree, cx, cy int, area image.Rectangle) uint8 {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	fills, restoreFill := captureFills()
	defer restoreFill()
	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}
	return liftAlphaOver(fills(), area, genGeomButtonHoverGlowSpread+1)
}

// TestHoverGlowOverlayCoversTextInput proves a clickable text input (the BPM
// box and similar) gets the same restrained matte hover lift as a button.
func TestHoverGlowOverlayCoversTextInput(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	r := image.Rect(50, 20, 130, 50)
	ti := &TextInput{Rect: r}
	tree := hoverGlowTestTreeH(t, r, &textInputHitAdapter{ti: ti})

	a := hoverGlowExpectRing(t, tree, 90, 35, r)
	if a == 0 {
		t.Fatal("text input hover lift absent (affordance dead)")
	}
	if a > 40 {
		t.Fatalf("text input hover lift too bold/bloom-like: alpha=%d, want subtle flat lift (<=40)", a)
	}
}

// TestHoverGlowOverlaySuppressedWhenInputFocused proves a focused (being-edited)
// text input shows no hover cushion — its own focus ring is the signal there.
func TestHoverGlowOverlaySuppressedWhenInputFocused(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	r := image.Rect(50, 20, 130, 50)
	ti := &TextInput{Rect: r}
	ti.focused = true
	tree := hoverGlowTestTreeH(t, r, &textInputHitAdapter{ti: ti})

	if a := hoverGlowExpectRing(t, tree, 90, 35, r); a != 0 {
		t.Fatalf("focused input should suppress the hover cushion, got alpha=%d", a)
	}
}

// TestHoverGlowOverlayCoversSlider proves the master volume slider (and similar
// slider inputs) get the hover cushion, resolved to the individual slider rect.
func TestHoverGlowOverlayCoversSlider(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	r := image.Rect(50, 24, 180, 44)
	s := NewSlider(0.5)
	s.SetRect(r)
	grp := NewSliderGroup([]*Slider{s}, nil)
	tree := hoverGlowTestTreeH(t, r, &sliderGroupHitAdapter{group: grp})

	a := hoverGlowExpectRing(t, tree, 110, 34, r)
	if a == 0 {
		t.Fatal("slider hover lift absent (affordance dead)")
	}
	if a > 40 {
		t.Fatalf("slider hover lift too bold/bloom-like: alpha=%d, want subtle flat lift (<=40)", a)
	}
}

// TestHoverGlowOverlayRowVolumesAreIndividual is the regression for the
// reported bug: the per-row volume column is registered as ONE slider-group hit
// area spanning every row, but hovering a single row's volume must lift only
// THAT row's slider — not the whole grouped column.
func TestHoverGlowOverlayRowVolumesAreIndividual(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	// Two stacked row-volume sliders sharing one group hit area (its rect is
	// the union — what the old code lifted).
	row0 := image.Rect(60, 20, 120, 44)
	row1 := image.Rect(60, 50, 120, 74)
	s0, s1 := NewSlider(0.5), NewSlider(0.5)
	s0.SetRect(row0)
	s1.SetRect(row1)
	grp := NewSliderGroup([]*Slider{s0, s1}, nil)
	union := row0.Union(row1)
	tree := hoverGlowTestTreeH(t, union, &sliderGroupHitAdapter{group: grp})

	// Hover the SECOND row's volume.
	restore := SetInputForTest(
		func() (int, int) { return 90, 62 }, // inside row1
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()
	fills, restoreFill := captureFills()
	defer restoreFill()
	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}

	slack := genGeomButtonHoverGlowSpread + 1
	if a := liftAlphaOver(fills(), row1, slack); a == 0 {
		t.Fatal("row-1 volume hover lift absent (affordance dead)")
	}
	// The lift must NOT be painted over the whole grouped column.
	if a := liftAlphaOver(fills(), union, slack); a != 0 {
		t.Fatalf("hover lift painted over the whole row-volume group (alpha=%d) — should be the individual slider only", a)
	}
}

// TestHoverGlowOverlayNoRingWhenCursorAway proves the lift fades out / is
// absent when the cursor is not over any button.
func TestHoverGlowOverlayNoRingWhenCursorAway(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	btnRect := image.Rect(50, 20, 110, 50)
	restore := SetInputForTest(
		func() (int, int) { return 400, 300 }, // far from btnRect
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree, _ := hoverGlowTestTree(t, btnRect)

	fills, restoreFill := captureFills()
	defer restoreFill()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}
	if a := liftAlphaOver(fills(), btnRect, genGeomButtonHoverGlowSpread+1); a != 0 {
		t.Fatalf("hover lift painted (alpha=%d) while cursor was away from every button", a)
	}
}

// TestHoverGlowOverlayMobileNoGlowWithoutPress pins that on mobile (no hover
// concept) a button NOT being pressed shows no lift even with the cursor
// parked over it — the glow is press-driven there, not hover-driven.
func TestHoverGlowOverlayMobileNoGlowWithoutPress(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	if !Profile().IsMobile() {
		t.Skip("could not force mobile profile")
	}

	btnRect := image.Rect(50, 20, 110, 50)
	restore := SetInputForTest(
		func() (int, int) { return 80, 35 },
		func(ebiten.MouseButton) bool { return false }, // not pressed
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree, _ := hoverGlowTestTree(t, btnRect)

	fills, restoreFill := captureFills()
	defer restoreFill()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}
	if a := liftAlphaOver(fills(), btnRect, genGeomButtonHoverGlowSpread+1); a != 0 {
		t.Fatalf("lift painted (alpha=%d) on mobile with no press (must stay flat until pressed)", a)
	}
}

// TestHoverGlowOverlayMobileTapLift pins cross-platform animation parity: since
// touch has no hover, PRESSING a button on mobile lifts it with the same matte
// affordance desktop shows on hover. Without this the mobile chrome was flat
// (the retired "mobile stays flat" rule), diverging from desktop.
func TestHoverGlowOverlayMobileTapLift(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	if !Profile().IsMobile() {
		t.Skip("could not force mobile profile")
	}

	btnRect := image.Rect(50, 20, 110, 50)
	restore := SetInputForTest(
		func() (int, int) { return 80, 35 },           // finger on the button
		func(ebiten.MouseButton) bool { return true }, // pressed (held)
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree, _ := hoverGlowTestTree(t, btnRect)
	tree.Update() // dispatch the press → capture the button

	fills, restoreFill := captureFills()
	defer restoreFill()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ { // advance the fade past zero
		tree.Draw(screen)
	}
	a := liftAlphaOver(fills(), btnRect, genGeomButtonHoverGlowSpread+1)
	if a == 0 {
		t.Fatal("no matte tap lift painted over the pressed button on mobile (animation parity broken)")
	}
	if a > 40 {
		t.Fatalf("tap lift too bold/bloom-like: alpha=%d, want subtle flat lift (<=40)", a)
	}
}
