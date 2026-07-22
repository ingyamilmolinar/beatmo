//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSynthCardTapDoesNotSelfClosePopup reproduces the mobile "opens and closes
// immediately" bug and pins the opening-gesture guard.
//
// The synth ParamCard opens its wheel popup on the touch-DOWN frame (audio
// subtree). On finger-lift, injectTouchTap replays the SAME physical gesture as
// a 2-frame tap. The overlay subtree — whose portal this gesture just opened —
// used to treat the replay as a fresh press over the open popup and dismiss it
// (anchor tap-to-close, or the generic click-outside branch, since the popup is
// placed below the card and never overlaps the tap point). The guard must keep
// the popup open through the injected replay of its own opening tap.
//
// Faithful driver: the real production tree dispatch (SetInputForTest → g.Update
// → HitIndex → synthControlHitAdapter), with the tap-injection flag toggled per
// frame to mirror the touch-down → held → lift-replay sequence.
func TestSynthCardTapDoesNotSelfClosePopup(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	_, r := firstContinuousSynthControl(t, g)
	cx, cy := r.Min.X+4, r.Max.Y-4

	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()
	defer SetTouchTapInjectedForTest(false)

	// Frame 1: touch-down CAPTURES the gesture (so a sweep could scroll) but must
	// NOT open the popup — capture-on-press defers opening to the lift.
	pressed = true
	g.Update()
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("card touch-down opened the wheel popup; it must wait for release so a swipe can scroll")
	}

	// Frame 2: finger still held, stationary — still no popup.
	g.Update()
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("popup opened while the finger was still held (before any lift)")
	}

	// Frame 3: lift — a stationary tap opens the popup on release.
	pressed = false
	g.Update()
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("a stationary tap did not open the wheel popup on release")
	}

	// Frame 4: the SAME gesture replayed via tap injection must NOT self-close the
	// popup it just opened (the open-then-close bug this test guards).
	SetTouchTapInjectedForTest(true)
	g.Update()
	if !dv.synthWheelPopup.IsOpen() {
		t.Fatal("the injected replay of the opening tap closed the popup it just opened")
	}
}

// TestSynthCardSecondTapClosesPopup guards the toggle: a genuine SECOND tap on
// the opener (a new gesture, after the first fully released) must still close
// the popup — the guard suppresses only the opening gesture's own replay, not a
// later distinct tap.
func TestSynthCardSecondTapClosesPopup(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	_, r := firstContinuousSynthControl(t, g)
	cx, cy := r.Min.X+4, r.Max.Y-4

	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()
	defer SetTouchTapInjectedForTest(false)

	// First tap: open + held + injected replay (stays open, per the guard).
	pressed = true
	g.Update()
	g.Update()
	SetTouchTapInjectedForTest(true)
	g.Update()
	SetTouchTapInjectedForTest(false)
	pressed = false
	g.Update() // full release — clears the opening-gesture guard.
	if !dv.synthWheelPopup.IsOpen() {
		t.Fatal("precondition: popup should be open after the first tap settled")
	}

	// Second, distinct tap on the opener: a fresh press edge → toggle close.
	pressed = true
	g.Update()
	if dv.synthWheelPopup.IsOpen() {
		t.Fatal("a second, separate tap on the opener must close the popup (toggle)")
	}
}
