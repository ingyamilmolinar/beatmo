//go:build test

package ui

import (
	"testing"
)

// TestInstMenuActiveHighlightFollowsSelection reproduces the "buttons stay
// toggled" bug. The instrument menu stays open after a selection (audition
// mode) but drawInstRow marks a row "active" by comparing its id to
// props.CurrentInstrument — a snapshot captured when the menu was opened and
// never refreshed while open. So the instrument that was current at open time
// stays highlighted forever, no matter what the user picks next, and the
// highlight never follows the live selection. Only ONE row should ever read as
// selected: the one the user most recently chose.
func TestInstMenuActiveHighlightFollowsSelection(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)
	comp := openInstMenuInstrumentsMode(t, dv, cx, cy, W, H)

	// At open the row's instrument is "kick"; the active highlight starts there.
	if got := comp.ActiveInstrumentID(); got != dv.Rows[0].Instrument {
		t.Fatalf("at open: active highlight = %q, want %q (the row's instrument)",
			got, dv.Rows[0].Instrument)
	}

	for _, want := range []string{"snare", "tom", "kick"} {
		if !pickVisibleInstrument(t, dv, comp, want, W, H) {
			t.Fatalf("instrument %q not visible; ids=%v", want, comp.VisibleInstIDs())
		}
		if got := dv.Rows[0].Instrument; got != want {
			t.Fatalf("selecting %q did not change the row (got %q)", want, got)
		}
		// The active highlight must track the live selection, not the open-time
		// snapshot — otherwise the original instrument stays "toggled".
		if got := comp.ActiveInstrumentID(); got != want {
			t.Fatalf("after selecting %q the active highlight is still %q "+
				"(stale props.CurrentInstrument snapshot — old row stays toggled)",
				want, got)
		}
	}
}
