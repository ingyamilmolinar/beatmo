//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Bit for bit the voice cache's buffer, with and without a Sampler edit.
func TestGraphRenderIsThePlayedVoice(t *testing.T) {
	for _, id := range []instrument.ID{roster.Kick808, roster.Violin} {
		for _, withEdit := range []bool{false, true} {
			if withEdit {
				SetSampleEdit(id, SampleEdit{StartFrac: 0.05, EndFrac: 0.7, Reverse: true, GainDB: -3})
			}
			d, ok := InstrumentDoc(id)
			if !ok {
				t.Fatalf("%s: no document", id)
			}
			got, sr, err := RenderDocForGraph(id, d, 0)
			if err != nil {
				t.Fatalf("%s edit=%v: %v", id, withEdit, err)
			}
			want, _, ok := docVoiceBuffer(id, SampleRate(), 0, docMelodic(id), false, originPrewarm)
			if !ok {
				t.Fatalf("%s: no played buffer", id)
			}
			if sr != SampleRate() || len(got) != len(want) {
				t.Fatalf("%s edit=%v: graph render %d samples @%d, played %d @%d", id, withEdit, len(got), sr, len(want), SampleRate())
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("%s edit=%v: sample %d differs: graph %v, played %v", id, withEdit, i, got[i], want[i])
				}
			}
			if withEdit {
				ClearSampleEdit(id)
			}
		}
	}
}
