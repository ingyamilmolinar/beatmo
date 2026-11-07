package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// The canonical order prevents drift between the stub, WASM and desktop instrument lists.
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
