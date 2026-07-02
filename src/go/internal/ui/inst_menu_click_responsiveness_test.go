//go:build test

package ui

import (
	"testing"
)

// instMenuFindAlternateInst returns the index of an instrument button whose
// instrument id differs from cur, or -1. Mapping is parallel: InstBtns()[i]
// corresponds to VisibleInstIDs()[i].
func instMenuFindAlternateInst(comp *InstrumentMenuComponent, cur string) (idx int, id string) {
	ids := comp.VisibleInstIDs()
	for i := range comp.InstBtns() {
		if i < len(ids) && ids[i] != cur {
			return i, ids[i]
		}
	}
	return -1, ""
}

// openInstMenuInstrumentsMode opens the instrument menu via a row-label click
// and navigates into instruments mode (selecting the first category if the menu
// opened in categories mode). Setup-only: category selection goes through the
// same input path on both platforms, so it is robust to the behavior under test.
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

// TestInstMenuDesktopInstrumentSelectsOnPress reproduces the "instrument menu
// doesn't respond to clicks well" bug on desktop.
//
// The instrument menu routes ALL input — including desktop mouse — through the
// mobile touch / deferred-tap state machine (drumview_overlay_inst_comp.go
// HandleInput), so a desktop click only fires on RELEASE. Every other desktop
// menu (context menu, overflow) fires immediately on the PRESS edge via
// Button.HandleInputResult. This test asserts the canonical desktop behavior:
// pressing an instrument button selects it on the press edge.
func TestInstMenuDesktopInstrumentSelectsOnPress(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, cur)
	if idx < 0 {
		t.Fatalf("no alternate instrument button found; cur=%q instBtns=%d ids=%v",
			cur, len(comp.InstBtns()), comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	// PRESS only — no release yet. On desktop the selection must register on
	// the press edge, just like the context/overflow menus.
	rel := pressDrumView(dv, tx, ty, W, H)
	dv.Update()
	rel()

	if dv.Rows[0].Instrument != wantID {
		t.Fatalf("instrument not selected on press edge: got %q want %q "+
			"(desktop click is not immediate — routed through deferred-tap path)",
			dv.Rows[0].Instrument, wantID)
	}
}

// TestInstMenuDesktopClickWithDriftSelects reproduces the other half of the
// bug: a desktop click whose pointer drifts a few pixels between press and
// release (common with a real mouse / trackpad) is swallowed entirely because
// the touch-scroll dead zone (tapMaxMovePx = 10) commits a "scroll" and cancels
// the deferred tap. On desktop there is no content-drag scrolling, so a
// press-drag-release on a list item must still select it.
func TestInstMenuDesktopClickWithDriftSelects(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, cur)
	if idx < 0 {
		t.Fatalf("no alternate instrument button found; cur=%q instBtns=%d ids=%v",
			cur, len(comp.InstBtns()), comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	// Press, drift downward ~14px over a couple frames (past the 10px dead
	// zone), then release near the drifted position — a slightly-sloppy click.
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

	if dv.Rows[0].Instrument != wantID {
		t.Fatalf("instrument not selected after a click with minor pointer drift: "+
			"got %q want %q (click swallowed by touch-scroll dead zone)",
			dv.Rows[0].Instrument, wantID)
	}
}

// TestInstMenuMobileSelectsViaMouseUpdate drives the full DrumView.Update loop
// on mobile using MOUSE-simulated input (SetInputForTest), which suppresses the
// touch override. This passes — proving the deferred-tap logic itself is sound;
// the real-touch path (TestInstMenuMobileSelectsViaRealTouch) is what breaks.
func TestInstMenuMobileSelectsViaMouseUpdate(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, cx, cy := newCategoryDrumView(t, W, H)

	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)
	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, cur)
	if idx < 0 {
		t.Fatalf("no alternate instrument; ids=%v", comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2
	clickDrumViewAt(t, dv, tx, ty, W, H)
	idleFrames(dv, 2, W, H)()

	if dv.Rows[0].Instrument != wantID {
		t.Fatalf("mobile: instrument not selected via Update loop: got %q want %q", dv.Rows[0].Instrument, wantID)
	}
}

// realTouchTapGame simulates a real single-finger tap at (x,y) through the full
// Game.Update loop: finger down for holdFrames, then lift, then settle frames.
// Uses the mock touch state (NOT SetInputForTest), so the touch→mouse override
// and gesture/tap-injection path in game_update.go are exercised for real.
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

// TestInstMenuMobileSelectsViaRealTouch reproduces the reported bug: on mobile,
// tapping an instrument button with a REAL touch (through globalTouchState +
// the touch override + gesture/tap-injection in game_update.go) does not select
// the instrument. Mouse-simulated taps work (see ...ViaMouseUpdate); real touch
// does not.
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
	idx, wantID := instMenuFindAlternateInst(comp, cur)
	if idx < 0 {
		t.Fatalf("no alternate instrument; cur=%q ids=%v", cur, comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	realTouchTapGame(t, g, tx, ty, 2)

	if dv.Rows[0].Instrument != wantID {
		t.Fatalf("mobile real-touch: instrument not selected: got %q want %q "+
			"(tap on instrument button did not register through the touch path)",
			dv.Rows[0].Instrument, wantID)
	}
}

// TestInstMenuMobileJitteryTapSelects reproduces the real-iPhone-Safari bug:
// a tap with small diagonal finger jitter (8px on each axis) is classified as a
// TAP by the gesture detector (per-axis dx<=10 && dy<=10, gesture.go) but the
// menu's deferred-tap scroll dead zone (TouchScroller.Move) uses EUCLIDEAN
// distance (sqrt(8^2+8^2)=11.3 >= 10), so it COMMITS a scroll and cancels the
// deferred tap — the instrument is never selected. Headless CDP taps have zero
// jitter, so this only manifests with a real finger.
func TestInstMenuMobileJitteryTapSelects(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)
	idleFrames(dv, 2, W, H)() // clear suppressClicksUntilRelease

	cur := dv.Rows[0].Instrument
	idx, wantID := instMenuFindAlternateInst(comp, cur)
	if idx < 0 {
		t.Fatalf("no alternate instrument; ids=%v", comp.VisibleInstIDs())
	}
	r := comp.InstBtns()[idx].Rect()
	tx := r.Min.X + r.Dx()/2
	ty := r.Min.Y + r.Dy()/2

	// Jittery tap: press, drift (8,8) — within the gesture detector's per-axis
	// 10px tap box but past the Euclidean dead zone — then release.
	comp.HandleInput(tx, ty, true)
	comp.HandleInput(tx+8, ty+8, true)
	comp.HandleInput(tx+8, ty+8, false)

	if dv.Rows[0].Instrument != wantID {
		t.Fatalf("mobile jittery tap did not select instrument: got %q want %q "+
			"(scroll dead-zone Euclidean vs gesture per-axis mismatch cancelled the tap)",
			dv.Rows[0].Instrument, wantID)
	}
}
