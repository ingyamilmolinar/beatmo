package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every shipped instrument's playback amplitude comes from its document's
// render.amplitude (loudness already folded in) — there is no table.
func TestConfigForInstrumentUsesTheDocumentAmplitude(t *testing.T) {
	ids := roster.All
	if len(ids) == 0 {
		t.Fatal("no shipped instruments")
	}
	for _, id := range ids {
		d, _ := ShippedDoc(id)
		if d.Render.Amplitude <= 0 {
			t.Errorf("%s: document amplitude %v — it would render silent", id, d.Render.Amplitude)
			continue
		}
		if got, want := ConfigForInstrument(id).Amplitude, float32(d.Render.Amplitude); got != want {
			t.Errorf("ConfigForInstrument(%q).Amplitude = %v, want the document's %v", id, got, want)
		}
	}
}

func TestLoudnessAmpFallsBackWhenAbsent(t *testing.T) {
	if _, ok := loudnessAmp("definitely-not-an-instrument"); ok {
		t.Error("loudnessAmp returned ok for an unknown id")
	}
}
