package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestInstrumentParityAfterReset verifies that ResetInstruments() populates the
// instrument list with all built-in synth instruments in the canonical order
// matching roster.All. This is a contract test that prevents drift
// between stub/WASM/desktop instrument lists.
func TestInstrumentParityAfterReset(t *testing.T) {
	ResetInstruments()

	got := Instruments()
	if len(got) != len(roster.All) {
		t.Fatalf("instrument count mismatch: got %d, want %d\ngot:  %v\nwant: %v", len(got), len(roster.All), got, roster.All)
	}

	for i, id := range roster.All {
		if got[i] != id {
			t.Errorf("instrument[%d] = %q, want %q", i, got[i], id)
		}
	}
}
