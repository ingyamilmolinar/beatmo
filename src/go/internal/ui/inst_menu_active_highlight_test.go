//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The menu stays open after a pick (audition mode), so the active row must follow the live selection, not the
// instrument current when it opened: only the latest pick reads as selected.
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

	for _, want := range []instrument.ID{roster.Snare, roster.Tom, roster.Kick} {
		if !pickVisibleInstrument(t, dv, comp, want, W, H) {
			t.Fatalf("instrument %q not visible; ids=%v", want, comp.VisibleInstIDs())
		}
		if got := dv.Rows[0].Instrument; got != want {
			t.Fatalf("selecting %q did not change the row (got %q)", want, got)
		}
		if got := comp.ActiveInstrumentID(); got != want {
			t.Fatalf("after selecting %q the active highlight is still %q "+
				"(stale props.CurrentInstrument snapshot — old row stays toggled)",
				want, got)
		}
	}
}
