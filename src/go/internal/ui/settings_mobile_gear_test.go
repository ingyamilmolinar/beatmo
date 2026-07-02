//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// TestMobileSettingsGearReplacesOverflowEntry asserts that on mobile the
// settings gear lives in the grid pane's top-right corner (like desktop) and is
// NOT duplicated in the overflow/ellipsis menu. Tapping the gear opens the
// settings overlay through the touch path.
func TestMobileSettingsGearReplacesOverflowEntry(t *testing.T) {
	setupMobileTest(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 700)
	advanceFrames(g, 2)
	if !Profile().IsMobile() {
		t.Fatalf("390x700 with forceSmallScreen should be mobile-class, got desktop")
	}

	// The gear button must have a real top-right rect on mobile.
	r := g.gridHelpButtonRect()
	if r.Empty() {
		t.Fatal("grid help (settings gear) rect is empty on mobile; want a top-right rect")
	}

	// The overflow menu must NOT carry a Settings entry on mobile anymore.
	for _, it := range g.drum.overflowItems() {
		if it.iconID == IconSettings {
			t.Fatal("mobile overflow menu still has a Settings entry; it should be moved to the gear")
		}
	}

	// Pressing the gear opens the settings overlay. The gear is wired into the
	// grid pane's z-ordered inputDispatcher (gridHelpInputHandler), so a press at
	// its rect dispatches there — the same path on every platform.
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if !g.inputDispatcher.Dispatch(cx, cy, true) {
		t.Fatal("inputDispatcher did not consume the press at the mobile gear rect")
	}
	if p := g.drum.tree.Portal(); p == nil || !p.Has(settingsOverlayID) {
		t.Fatal("mobile gear press did not open the settings overlay")
	}
}

// TestSettingsOverlayHidesShortcutsOnMobile asserts the keyboard-shortcuts
// section is gone entirely on mobile (no header, no rows, no desktop-only note)
// while it remains on desktop. The panel is correspondingly shorter on mobile.
func TestSettingsOverlayHidesShortcutsOnMobile(t *testing.T) {
	o := NewSettingsOverlay(func(i18n.Locale) {})
	screen := image.Rect(0, 0, 1200, 800)

	// Desktop: shortcuts shown.
	o.Layout(screen, screen)
	if !o.shortcutsVisible() {
		t.Fatal("desktop settings overlay should show the keyboard-shortcuts section")
	}
	desktopH := o.rect.Dy()

	// Mobile: shortcuts gone entirely.
	forceSmallScreenForTest = true
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		SetTouchScreenSize(0, 0)
		UpdateProfile()
	})
	UpdateProfile()
	o.Layout(screen, screen)
	if o.shortcutsVisible() {
		t.Fatal("mobile settings overlay must NOT show the keyboard-shortcuts section")
	}
	mobileH := o.rect.Dy()
	if mobileH >= desktopH {
		t.Fatalf("mobile overlay height %d should be shorter than desktop %d once shortcuts are removed", mobileH, desktopH)
	}
}

// TestSettingsOverlayPanelFitsMobileWidth asserts the settings panel (and its
// language pills) stay fully on-screen on a narrow mobile viewport. Before the
// fix the fixed 480px panel was centered on a 390px pane, pushing the title and
// the English pill off the left edge.
func TestSettingsOverlayPanelFitsMobileWidth(t *testing.T) {
	forceSmallScreenForTest = true
	t.Cleanup(func() {
		forceSmallScreenForTest = false
		SetTouchScreenSize(0, 0)
		UpdateProfile()
	})
	UpdateProfile()

	o := NewSettingsOverlay(func(i18n.Locale) {})
	bounds := image.Rect(0, 348, 390, 800) // mobile drum-pane bounds (portal screenBounds)
	o.Layout(bounds, bounds)

	if o.rect.Min.X < bounds.Min.X || o.rect.Max.X > bounds.Max.X {
		t.Fatalf("panel rect %v escapes mobile bounds %v horizontally", o.rect, bounds)
	}
	en, es := o.LanguagePillRects()
	if en.Min.X < bounds.Min.X || es.Max.X > bounds.Max.X {
		t.Fatalf("language pills escape mobile bounds: en=%v es=%v bounds=%v", en, es, bounds)
	}
}

// TestMobileSettingsGearTapOpensOverlayViaUpdate drives the REAL Game.Update()
// input loop with a synthesized TOUCH tap at the gear's center (not a direct
// handleTapInGrid call — see [[feedback_functional_tests_over_isolation]]). The
// touch path works; this is the passing companion that localizes the bug to the
// mouse/non-touch path (see TestMobileSettingsGearMouseClickOpensOverlayViaUpdate).
func TestMobileSettingsGearTapOpensOverlayViaUpdate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	globalTouchState.Reset()
	resetTouchOverride()
	// Re-install the package-default input vars before this test installs its own
	// _ebCursorPosition/touch mocks: a prior test that replaced cursorPosition (or
	// left inputForTestActive set) via a stub whose restore never ran would
	// otherwise suppress the touch override so the synthesized tap never reaches
	// the touch dispatch path and the gear overlay never opens.
	resetInputForTest()
	t.Cleanup(func() { globalTouchState.Reset(); resetTouchOverride() })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 800)

	oldEbCursor := _ebCursorPosition
	_ebCursorPosition = func() (int, int) { return 0, 0 }
	defer func() { _ebCursorPosition = oldEbCursor }()

	mock := newMockTouchState()
	restoreTouch := SetTouchForTest(mock.TouchIDs, mock.TouchPosition)
	defer restoreTouch()

	advanceFrames(g, 2) // let layout settle

	if !Profile().IsMobile() {
		t.Fatal("expected mobile profile after Layout(390,800)")
	}
	r := g.gridHelpButtonRect()
	if r.Empty() {
		t.Fatal("gear rect empty on mobile")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	// Synthesize a tap: finger down, then a quick release at the same point.
	mock.addTouch(1, cx, cy)
	advanceFrames(g, 1)
	mock.removeTouch(1)
	advanceFrames(g, 3) // gesture detection + dispatch

	p := g.drum.tree.Portal()
	if p == nil || !p.Has(settingsOverlayID) {
		t.Fatalf("tapping the mobile settings gear at (%d,%d) did not open the settings overlay", cx, cy)
	}
}

// TestMobileSettingsGearMouseClickOpensOverlayViaUpdate reproduces the reported
// bug for the MOUSE input path under the mobile profile (a mobile-viewport
// desktop browser, or mobile browsers that deliver a tap as a synthesized
// click/pointer event rather than touch). The button-widget handler that
// processes the gear (game_update.go) is gated `!Profile().IsMobile()`, and a
// mouse click produces no GestureTap — so the only mobile gear path
// (handleTapInGrid via GestureTap) never fires and the click is dropped.
// Mirrors the desktop TestGridHelpButtonClickThroughUpdate, but under mobile.
//
// EXPECTED TO FAIL until the gear gets a mobile mouse/pointer handler.
func TestMobileSettingsGearMouseClickOpensOverlayViaUpdate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 800)

	if !Profile().IsMobile() {
		t.Fatal("expected mobile profile after Layout(390,800)")
	}
	r := g.gridHelpButtonRect()
	if r.Empty() {
		t.Fatal("gear rect empty on mobile")
	}
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 390, 800 },
	)
	defer restore()

	g.Update()

	if !g.drum.tree.Portal().IsOpen() {
		t.Fatalf("mouse-clicking the mobile settings gear at (%d,%d) did not open the settings overlay", cx, cy)
	}
}

// TestSettingsGearIsPartOfInputDispatcher asserts the gear is wired into the
// z-axis input infra (the grid pane's inputDispatcher) rather than special-cased
// in Game.Update. A press at the gear's rect, routed through the dispatcher the
// same way every other grid-pane control is, must be consumed AND open the
// overlay — on BOTH desktop and mobile profiles. This is the structural
// requirement: the gear participates in z-ordered dispatch, not a bespoke path.
func TestSettingsGearIsPartOfInputDispatcher(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mobile bool
		w, h   int
	}{
		{"desktop", false, 1200, 800},
		{"mobile", true, 390, 800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertDefaultParityState(t)
			withDefaultStart(t, false)
			forceSmallScreenForTest = tc.mobile
			t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(tc.w, tc.h)
			advanceFrames(g, 2) // build the dispatcher handler list

			if Profile().IsMobile() != tc.mobile {
				t.Fatalf("profile mismatch: IsMobile=%v want %v", Profile().IsMobile(), tc.mobile)
			}
			r := g.gridHelpButtonRect()
			if r.Empty() {
				t.Fatal("gear rect empty")
			}
			cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2

			// Dispatch a press at the gear through the z-ordered input dispatcher.
			consumed := g.inputDispatcher.Dispatch(cx, cy, true)
			if !consumed {
				t.Fatal("inputDispatcher did not consume the press at the gear rect — gear is not registered as a z-ordered InputHandler")
			}
			if !g.drum.tree.Portal().Has(settingsOverlayID) {
				t.Fatal("dispatching a press at the gear did not open the settings overlay")
			}
		})
	}
}
