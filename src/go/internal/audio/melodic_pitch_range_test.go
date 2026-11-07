//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestSynthBassIsMelodic(t *testing.T) {
	for _, id := range []instrument.ID{roster.BassGuitar, roster.BassAcid, roster.BassReese, roster.BassFm, roster.Bass808} {
		if !HasInstrumentDoc(id) {
			if _, ok := EnsureInstrumentDoc(id); !ok {
				t.Fatalf("%s has no instrument document", id)
			}
		}
		if !IsMelodicInstrument(id) {
			t.Errorf("IsMelodicInstrument(%q) = false; want true (melodic pitched instrument)", id)
		}
	}
}
