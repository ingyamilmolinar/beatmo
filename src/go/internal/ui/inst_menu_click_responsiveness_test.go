//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// InstBtns()[i] corresponds to VisibleInstIDs()[i].
func instMenuFindAlternateInst(comp *InstrumentMenuComponent, cur string) (idx int, id string) {
	ids := comp.VisibleInstIDs()
	for i := range comp.InstBtns() {
		if i < len(ids) && ids[i] != instrument.ID(cur) {
			return i, string(ids[i])
		}
	}
	return -1, ""
}

// Setup only: category selection takes the same input path on both platforms, so it does not depend on the
// behaviour under test.
func openInstMenuInstrumentsMode(t *testing.T, dv *DrumView, cx, cy, w, h int) *InstrumentMenuComponent {
	t.Helper()
	clickDrumViewAt(t, dv, cx, cy, w, h)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open after clicking row label")
	}
	comp := dv.instMenuComp
	if comp == nil {
		t.Fatal("instMenuComp is nil")
	}
	if comp.Mode() == InstMenuModeCategories {
		idleFrames(dv, 2, w, h)()
		cats := comp.CategoryBtns()
		if len(cats) == 0 {
			t.Fatal("no category buttons in categories mode")
		}
		cr := cats[0].Rect()
		clickDrumViewAt(t, dv, cr.Min.X+cr.Dx()/2, cr.Min.Y+cr.Dy()/2, w, h)
		idleFrames(dv, 2, w, h)()
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %v", comp.Mode())
	}
	if len(comp.InstBtns()) == 0 {
		t.Fatalf("no instrument buttons after navigating to instruments mode")
	}
	return comp
}

// Every desktop menu fires on the PRESS edge; routing this one through the touch deferred-tap machine made a
// desktop click fire only on release.
func TestInstMenuDesktopInstrumentSelectsOnPress(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, string(cur))
	if idx < 0 {
		t.Fatalf("no alternate instrument button found; cur=%q instBtns=%d ids=%v",
			cur, len(comp.InstBtns()), comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	rel := pressDrumView(dv, tx, ty, W, H)
	dv.Update()
	rel()

	if dv.Rows[0].Instrument != instrument.ID(wantID) {
		t.Fatalf("instrument not selected on press edge: got %q want %q "+
			"(desktop click is not immediate — routed through deferred-tap path)",
			dv.Rows[0].Instrument, wantID)
	}
}

// A real mouse drifts a few pixels between press and release; desktop has no content-drag scrolling, so the
// touch scroll dead zone must not swallow the click.
func TestInstMenuDesktopClickWithDriftSelects(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, string(cur))
	if idx < 0 {
		t.Fatalf("no alternate instrument button found; cur=%q instBtns=%d ids=%v",
			cur, len(comp.InstBtns()), comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	// Drifts past the 10 px dead zone: a slightly sloppy click.
	rel := pressDrumView(dv, tx, ty, W, H)
	dv.Update()
	rel()
	rel = pressDrumView(dv, tx, ty+7, W, H)
	dv.Update()
	rel()
	rel = pressDrumView(dv, tx, ty+14, W, H)
	dv.Update()
	rel()
	rel = releaseDrumView(dv, tx, ty+14, W, H)
	dv.Update()
	rel()

	if dv.Rows[0].Instrument != instrument.ID(wantID) {
		t.Fatalf("instrument not selected after a click with minor pointer drift: "+
			"got %q want %q (click swallowed by touch-scroll dead zone)",
			dv.Rows[0].Instrument, wantID)
	}
}

// SetInputForTest suppresses the touch override, so this checks the deferred-tap logic apart from the real
// touch path.
func TestInstMenuMobileSelectsViaMouseUpdate(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, cx, cy := newCategoryDrumView(t, W, H)

	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)
	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, string(cur))
	if idx < 0 {
		t.Fatalf("no alternate instrument; ids=%v", comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2
	clickDrumViewAt(t, dv, tx, ty, W, H)
	idleFrames(dv, 2, W, H)()

	if dv.Rows[0].Instrument != instrument.ID(wantID) {
		t.Fatalf("mobile: instrument not selected via Update loop: got %q want %q", dv.Rows[0].Instrument, wantID)
	}
}

// Mock touch state, not SetInputForTest, so the touch→mouse override and tap injection run for real.
func realTouchTapGame(t *testing.T, g *Game, x, y, holdFrames int) {
	t.Helper()
	mock := newMockTouchState()
	mock.addTouch(1, x, y)
	restore := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restore()
	if holdFrames < 1 {
		holdFrames = 1
	}
	for i := 0; i < holdFrames; i++ {
		_ = g.Update()
	}
	mock.removeTouch(1)
	// Lift frame fires GestureTap; injection + release need several frames.
	for i := 0; i < 6; i++ {
		_ = g.Update()
	}
}

// Through globalTouchState, the touch override and tap injection, where mouse-simulated taps hid the failure.
func TestInstMenuMobileSelectsViaRealTouch(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })
	resetTouchOverride()
	t.Cleanup(resetTouchOverride)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	if len(dv.Rows) == 0 {
		t.Fatal("no rows in demo game")
	}

	// Open the instrument menu for row 0 and reach instruments mode.
	dv.openInstMenuForRow(0)
	for i := 0; i < 3; i++ {
		_ = g.Update()
	}
	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instrument menu should be open")
	}
	if comp.Mode() == InstMenuModeCategories {
		// Setup: enter the first category directly (not the behavior under test).
		cats := comp.CategoryBtns()
		if len(cats) == 0 {
			t.Fatal("no category buttons")
		}
		if cats[0].OnClick != nil {
			cats[0].OnClick()
		}
		for i := 0; i < 3; i++ {
			_ = g.Update()
		}
	}
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %v", comp.Mode())
	}
	if len(comp.InstBtns()) == 0 {
		t.Fatal("no instrument buttons")
	}

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, string(cur))
	if idx < 0 {
		t.Fatalf("no alternate instrument; cur=%q ids=%v", cur, comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	realTouchTapGame(t, g, tx, ty, 2)

	if dv.Rows[0].Instrument != instrument.ID(wantID) {
		t.Fatalf("mobile real-touch: instrument not selected: got %q want %q "+
			"(tap on instrument button did not register through the touch path)",
			dv.Rows[0].Instrument, wantID)
	}
}

// The gesture detector judges a tap per axis (dx, dy <= 10) while the menu's scroll dead zone is EUCLIDEAN: an
// (8,8) jitter is a tap to one and a scroll to the other, so the pick was lost. CDP taps have no jitter.
func TestInstMenuMobileJitteryTapSelects(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)
	idleFrames(dv, 2, W, H)() // clear suppressClicksUntilRelease

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, string(cur))
	if idx < 0 {
		t.Fatalf("no alternate instrument; ids=%v", comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	// Inside the per-axis tap box, past the Euclidean dead zone.
	comp.HandleInput(tx, ty, true)
	comp.HandleInput(tx+8, ty+8, true)
	comp.HandleInput(tx+8, ty+8, false)

	if dv.Rows[0].Instrument != instrument.ID(wantID) {
		t.Fatalf("mobile jittery tap did not select instrument: got %q want %q "+
			"(scroll dead-zone Euclidean vs gesture per-axis mismatch cancelled the tap)",
			dv.Rows[0].Instrument, wantID)
	}
}
