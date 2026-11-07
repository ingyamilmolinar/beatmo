//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A full press+release through dv.Update, like a real desktop click on a row.
func pickVisibleInstrument(t *testing.T, dv *DrumView, comp *InstrumentMenuComponent, want instrument.ID, w, h int) bool {
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

// The open menu returns InputConsumed, so its row buttons never see a release; a reused Button whose held count
// outlives the click reaches held == 2 on the next press, fails the press-edge check and swallows the click.
func TestInstMenuRepeatedSelectionRegistersEveryClick(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	// Revisits rows so each button is clicked more than once in one open session; all three share the Drums category.
	seq := []instrument.ID{roster.Snare, roster.Tom, roster.Kick, roster.Snare, roster.Tom, roster.Kick, roster.Snare}
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

// The mobile deferred tap reaches the same Button press core (fireTapAt → PressFromTree) as the desktop mouse.
func TestInstMenuRepeatedSelectionMobile(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = true
	t.Cleanup(func() { forceSmallScreenForTest = false })

	const W, H = 390, 844
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	seq := []instrument.ID{roster.Snare, roster.Tom, roster.Kick, roster.Snare, roster.Tom}
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
