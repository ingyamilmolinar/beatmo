//go:build test

package ui

import (
	"testing"
)

// pickVisibleInstrument selects the visible instrument row whose id == want by
// driving a full press+release through dv.Update() (the production input loop),
// mirroring a real desktop mouse click on a menu row. Returns false if the id
// is not currently visible in the menu.
func pickVisibleInstrument(t *testing.T, dv *DrumView, comp *InstrumentMenuComponent, want string, w, h int) bool {
	t.Helper()
	ids := comp.VisibleInstIDs()
	btns := comp.InstBtns()
	row := -1
	for i := range btns {
		if i < len(ids) && ids[i] == want {
			row = i
			break
		}
	}
	if row < 0 {
		return false
	}
	r := btns[row].Rect()
	clickDrumViewAt(t, dv, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2, w, h)
	return true
}

// TestInstMenuRepeatedSelectionRegistersEveryClick reproduces the "very flaky"
// click bug on the main instrument menu. The menu STAYS OPEN after a selection
// (audition mode) and returns InputConsumed (one-shot, not captured), so the
// input tree never delivers a release to the menu's row buttons. The same
// *Button objects persist across presses, so a row that was clicked once keeps
// Button.held == 1 forever. Because the desktop dispatch loop returns on the
// FIRST consuming button, a previously-clicked row that sits later in iteration
// order never gets its state reset — and when the user clicks it again,
// Button.held increments 1 -> 2, so the `held == 1` press-edge check fails and
// the click is silently swallowed.
//
// This drives the real DrumView.Update() loop and asserts that re-selecting a
// previously-selected instrument always registers.
func TestInstMenuRepeatedSelectionRegistersEveryClick(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	// Sequence revisits instruments so each row button is clicked more than
	// once within a single open session. "kick", "snare", "tom" all share the
	// first (Drums) category that openInstMenuInstrumentsMode lands on.
	seq := []string{"snare", "tom", "kick", "snare", "tom", "kick", "snare"}
	for step, want := range seq {
		if comp.Mode() != InstMenuModeInstruments {
			t.Fatalf("step %d (%q): menu left instruments mode (%v)", step, want, comp.Mode())
		}
		if !pickVisibleInstrument(t, dv, comp, want, W, H) {
			t.Fatalf("step %d: instrument %q not visible; ids=%v", step, want, comp.VisibleInstIDs())
		}
		if got := dv.Rows[0].Instrument; got != want {
			t.Fatalf("step %d: click on %q was swallowed — Rows[0].Instrument=%q "+
				"(stale Button.held debounce ate the press)", step, want, got)
		}
	}
}

// TestInstMenuRepeatedSelectionMobile is the mobile counterpart: the bottom-sheet
// instrument menu must also register every audition pick across one open session.
// The mobile path fires via the deferred tap (fireTapAt → PressFromTree, the
// same Button press core the desktop mouse path uses), so this guards that
// repeated selection stays reliable on touch as well.
func TestInstMenuRepeatedSelectionMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	seq := []string{"snare", "tom", "kick", "snare", "tom"}
	for step, want := range seq {
		if comp.Mode() != InstMenuModeInstruments {
			t.Fatalf("step %d (%q): menu left instruments mode (%v)", step, want, comp.Mode())
		}
		if !pickVisibleInstrument(t, dv, comp, want, W, H) {
			t.Fatalf("step %d: instrument %q not visible; ids=%v", step, want, comp.VisibleInstIDs())
		}
		idleFrames(dv, 1, W, H)()
		if got := dv.Rows[0].Instrument; got != want {
			t.Fatalf("mobile step %d: click on %q was swallowed — Rows[0].Instrument=%q", step, want, got)
		}
		// The active highlight must follow the live selection on mobile too.
		if got := comp.ActiveInstrumentID(); got != want {
			t.Fatalf("mobile step %d: active highlight = %q, want %q", step, got, want)
		}
	}
}
