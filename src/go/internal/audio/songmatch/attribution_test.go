package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/songrender"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestBandAttribution_NamesDominantInstrument(t *testing.T) {
	cfg := fingerprint.DefaultAnalysisConfig()
	// ref is bright (high tone); rendered master is dull (low tone) → mid/low over-represented.
	ref := fingerprint.SongFingerprintOf(wave.Sine(4000, 0.9, 1.0, 44100), cfg)
	sr := 44100
	low := wave.Sine(120, 0.9, 1.0, sr).Samples
	rendered := songrender.Rendered{
		Arrangement: songrender.Arrangement{SampleRate: sr},
		Master:      low,
		Stems:       map[instrument.ID][]float64{roster.BassGuitar: low},
	}
	hints := BandAttribution(ref, rendered, cfg)
	if len(hints) == 0 {
		t.Fatal("expected at least one band-attribution hint")
	}
	named := false
	for _, h := range hints {
		if h.Instrument == roster.BassGuitar {
			named = true
		}
	}
	if !named {
		t.Errorf("expected the dominant 'bass' stem to be named; got %+v", hints)
	}
}
