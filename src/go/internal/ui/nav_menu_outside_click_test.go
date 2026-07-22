//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestNavMenuClosesOnOutsideClick drives the REAL tree: with the nav menu open,
// a press on a point OUTSIDE its card must close it via the shared portal
// click-outside path — the same behavior every other popup has.
func TestNavMenuClosesOnOutsideClick(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.Update()

	g.drum.OpenNavigation()
	g.Update()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu did not open")
	}
	card := g.drum.navMenuComp.Bounds()
	if card.Empty() {
		t.Skip("nav menu card empty in this harness")
	}

	// A point well outside the card (and its beat-counter anchor): bottom-left of
	// the drum pane, clear of the card which sits mid-top.
	ox := g.drum.Bounds.Min.X + 8
	oy := g.drum.Bounds.Max.Y - 8
	if image.Pt(ox, oy).In(card.Union(g.drum.beatCounterRect)) {
		t.Fatalf("chosen outside point (%d,%d) is inside the menu bounds %v", ox, oy, card)
	}

	press := func(down bool) {
		reset := SetInputForTest(
			func() (int, int) { return ox, oy },
			func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 1280, 720 },
		)
		g.Update()
		reset()
	}
	press(true)  // press outside
	press(false) // release

	if g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must close when clicking outside it (shared portal click-outside)")
	}
}

// TestNavMenuNotStuckCapturingAfterOpen guards against a stuck capture: if
// Capturing() were true after the menu opens, RootTree would route every press
// to the component and click-outside would never fire.
func TestNavMenuNotStuckCapturingAfterOpen(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.Update()
	g.drum.OpenNavigation()
	g.Update()
	g.Update()
	if g.drum.navMenuComp.Capturing() {
		t.Fatal("nav menu Capturing() stuck true after open — click-outside would be blocked")
	}
}

// TestNavMenuClosesOnGridPaneClick: clicking in the GRID pane (a different
// subtree) while the nav menu is open must also close it — the blocking portal
// owns input, and the drum RootTree's click-outside fires for the foreign point.
func TestNavMenuClosesOnGridPaneClick(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.Update()
	g.drum.OpenNavigation()
	g.Update()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu did not open")
	}
	// A point in the grid pane (above the drum pane).
	gx, gy := 40, gridTopOffset()+8
	press := func(down bool) {
		reset := SetInputForTest(
			func() (int, int) { return gx, gy },
			func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 1280, 720 },
		)
		g.Update()
		reset()
	}
	press(true)
	press(false)
	if g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must close when clicking in the grid pane")
	}
}

// TestNavMenuClosesOnOutsideTapMobile drives the REAL mobile touch pipeline
// (globalTouchState → Game.Update → injectTouchTap): with the nav menu bottom
// sheet open, a tap OUTSIDE it must close it, like every other mobile popup.
func TestNavMenuClosesOnOutsideTapMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)
	globalTouchState.Reset()
	t.Cleanup(globalTouchState.Reset)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	g.Update()

	g.drum.OpenNavigation()
	g.Update()
	if !g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu did not open on mobile")
	}
	card := g.drum.navMenuComp.Bounds()
	if card.Empty() {
		t.Skip("nav menu card empty in this harness")
	}

	mock := newMockTouchState()
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()

	// A tap well ABOVE the bottom-sheet card (top of the drum pane), clear of it.
	ox := g.drum.Bounds.Min.X + g.drum.Bounds.Dx()/2
	oy := card.Min.Y - 40
	if oy < g.drum.Bounds.Min.Y+4 {
		oy = g.drum.Bounds.Min.Y + 4
	}
	if image.Pt(ox, oy).In(card) {
		t.Skipf("no room above the sheet for an outside tap (card=%v)", card)
	}

	mock.addTouch(1, ox, oy)
	g.Update()
	g.Update()
	mock.removeTouch(1)
	g.Update()
	g.Update()
	g.Update()

	if g.drum.IsNavMenuOpen() {
		t.Fatal("nav menu must close on an outside tap on mobile (shared portal click-outside)")
	}
}
