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

// ringDraw records one stroked drawRoundedRect call.
type ringDraw struct {
	rect image.Rectangle
	col  color.NRGBA
}

// captureRings intercepts drawRoundedRect and records every STROKED ring
// (filled=false). Returns a snapshot getter + restore.
func captureRings() (rings func() []ringDraw, restore func()) {
	var got []ringDraw
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, cc color.Color, radius int, filled bool) {
		if !filled {
			if nc, ok := cc.(color.NRGBA); ok {
				got = append(got, ringDraw{r, nc})
			}
		}
		orig(dst, r, cc, radius, filled)
	}
	return func() []ringDraw { return got }, func() { drawRoundedRect = orig }
}

// brightRingNear reports the max-alpha primary-bright stroked ring whose four
// edges are within `slack` px of `around` (covers the button rect, its inset
// bright rings, and the expanded bloom). Returns alpha 0 when none.
func brightRingNear(rings []ringDraw, around image.Rectangle, slack int) uint8 {
	want := TokenAccentBright()
	var best uint8
	for _, rd := range rings {
		if rd.col.R != want.R || rd.col.G != want.G || rd.col.B != want.B {
			continue
		}
		if abs(rd.rect.Min.X-around.Min.X) <= slack && abs(rd.rect.Min.Y-around.Min.Y) <= slack &&
			abs(rd.rect.Max.X-around.Max.X) <= slack && abs(rd.rect.Max.Y-around.Max.Y) <= slack {
			if rd.col.A > best {
				best = rd.col.A
			}
		}
	}
	return best
}

// TestHoverGlowOverlayDrawsRingOnButtonHover is the functional regression test:
// with the cursor parked over a button registered in the tree's HitIndex, the
// tree's own Draw path must stroke a primary-bright glow ring around that
// button — proving the hover affordance works through the REAL input/draw
// loop (not just helper unit tests). This is the bug the user reported: no
// hover on any button.
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

	rings, restoreRing := captureRings()
	defer restoreRing()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ { // advance the fade well past zero
		tree.Draw(screen)
	}

	// A primary-bright ring must be drawn around the button, and it must be
	// HIGHLY VISIBLE — alpha well above the old hairline (~27). Slack covers
	// the bloom expansion (button-hover-glow-spread).
	a := brightRingNear(rings(), btnRect, genGeomButtonHoverGlowSpread+1)
	if a == 0 {
		t.Fatal("no primary-bright hover ring drawn around the hovered button (affordance dead)")
	}
	if a < 120 {
		t.Fatalf("hover ring barely visible: alpha=%d, want a bold ring (>=120)", a)
	}
}

// hoverGlowExpectRing drives the tree with the cursor at (cx,cy) for 30 frames
// and returns the max-alpha primary-bright ring near `area`.
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
	rings, restoreRing := captureRings()
	defer restoreRing()
	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}
	return brightRingNear(rings(), area, genGeomButtonHoverGlowSpread+1)
}

// TestHoverGlowOverlayCoversTextInput proves a clickable text input (the BPM
// box and similar) gets the same bold hover cushion as a button.
func TestHoverGlowOverlayCoversTextInput(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	r := image.Rect(50, 20, 130, 50)
	ti := &TextInput{Rect: r}
	tree := hoverGlowTestTreeH(t, r, &textInputHitAdapter{ti: ti})

	if a := hoverGlowExpectRing(t, tree, 90, 35, r); a < 120 {
		t.Fatalf("text input hover ring barely visible/absent: alpha=%d, want >=120", a)
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

	if a := hoverGlowExpectRing(t, tree, 110, 34, r); a < 120 {
		t.Fatalf("slider hover ring barely visible/absent: alpha=%d, want >=120", a)
	}
}

// TestHoverGlowOverlayRowVolumesAreIndividual is the regression for the
// reported bug: the per-row volume column is registered as ONE slider-group hit
// area spanning every row, but hovering a single row's volume must glow only
// THAT row's slider — not the whole grouped column.
func TestHoverGlowOverlayRowVolumesAreIndividual(t *testing.T) {
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	// Two stacked row-volume sliders sharing one group hit area (its rect is
	// the union — what the old code glowed).
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
	rings, restoreRing := captureRings()
	defer restoreRing()
	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}

	slack := genGeomButtonHoverGlowSpread + 1
	if a := brightRingNear(rings(), row1, slack); a < 120 {
		t.Fatalf("row-1 volume hover ring absent/weak: alpha=%d, want >=120", a)
	}
	// The glow must NOT be painted around the whole grouped column.
	if a := brightRingNear(rings(), union, slack); a != 0 {
		t.Fatalf("hover glow drawn around the whole row-volume group (alpha=%d) — should be the individual slider only", a)
	}
}

// TestHoverGlowOverlayNoRingWhenCursorAway proves the glow fades out / is
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

	rings, restoreRing := captureRings()
	defer restoreRing()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}
	if a := brightRingNear(rings(), btnRect, genGeomButtonHoverGlowSpread+1); a != 0 {
		t.Fatalf("hover glow ring drawn (alpha=%d) while cursor was away from every button", a)
	}
}

// TestHoverGlowOverlayMobileNoRing pins the desktop-only rule: on a mobile
// profile, no hover glow even with the cursor over a button.
func TestHoverGlowOverlayMobileNoRing(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()
	if !Profile().IsMobile() {
		t.Skip("could not force mobile profile")
	}

	btnRect := image.Rect(50, 20, 110, 50)
	restore := SetInputForTest(
		func() (int, int) { return 80, 35 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	tree, _ := hoverGlowTestTree(t, btnRect)

	rings, restoreRing := captureRings()
	defer restoreRing()

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		tree.Draw(screen)
	}
	if a := brightRingNear(rings(), btnRect, genGeomButtonHoverGlowSpread+1); a != 0 {
		t.Fatalf("hover glow ring drawn (alpha=%d) on a mobile profile (must stay flat)", a)
	}
}
