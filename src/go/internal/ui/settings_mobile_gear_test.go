//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

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

	// The gear sits in the grid pane's z-ordered inputDispatcher, the same path on every platform.
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	if !g.inputDispatcher.Dispatch(cx, cy, true) {
		t.Fatal("inputDispatcher did not consume the press at the mobile gear rect")
	}
	if p := g.drum.portal(); p == nil || !p.Has(settingsOverlayID) {
		t.Fatal("mobile gear press did not open the settings overlay")
	}
}

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

// A fixed-width panel centred on a narrower pane pushes the title and the English pill off the left edge.
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

// Through the real Game.Update loop with a synthesised TOUCH tap.
func TestMobileSettingsGearTapOpensOverlayViaUpdate(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false; activeProfile = nil })

	globalTouchState.Reset()
	resetTouchOverride()
	// A previous test whose input stub never restored would suppress the touch override, and the tap would never
	// reach the touch path.
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

	p := g.drum.portal()
	if p == nil || !p.Has(settingsOverlayID) {
		t.Fatalf("tapping the mobile settings gear at (%d,%d) did not open the settings overlay", cx, cy)
	}
}

// A mobile-viewport desktop browser, or a phone delivering a tap as a click, produces no GestureTap: the gear
// needs a mouse path under the mobile profile too.
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

	if !g.drum.portal().IsOpen() {
		t.Fatalf("mouse-clicking the mobile settings gear at (%d,%d) did not open the settings overlay", cx, cy)
	}
}

// Through the dispatcher like every grid-pane control, never a bespoke path in Game.Update, on both profiles.
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
			if !g.drum.portal().Has(settingsOverlayID) {
				t.Fatal("dispatching a press at the gear did not open the settings overlay")
			}
		})
	}
}
