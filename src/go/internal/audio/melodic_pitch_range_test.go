//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestSynthBassIsMelodic asserts that bass-guitar and the synth bass family
// (acid/reese/fm/808) render at the node pitch (render.melodic in their
// documents).
//
// The broad pitch-range render coverage and the cello formant-vs-resample
// check live in all_instruments_pitch_table_test.go.
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
