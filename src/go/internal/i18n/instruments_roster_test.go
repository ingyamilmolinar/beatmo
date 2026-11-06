package i18n

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestBuiltinInstrumentNamesNameShippedInstruments: every localized default
// name is keyed by an instrument on the roster, so a renamed instrument
// cannot silently fall back to its raw id in the UI.
func TestBuiltinInstrumentNamesNameShippedInstruments(t *testing.T) {
	for id := range builtinInstruments {
		if !slices.Contains(roster.All, id) {
			t.Errorf("builtinInstruments[%q] names no shipped instrument", id)
		}
	}
}
