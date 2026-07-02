package songmatch

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSeedVarName(t *testing.T) {
	cases := map[string]string{"sax": "saxSeed", "guitar-electric": "guitarElectricSeed", "french-horn": "frenchHornSeed"}
	for in, want := range cases {
		if got := SeedVarName(in); got != want {
			t.Errorf("SeedVarName(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTunePassages_ProducesSeedHint(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	// A 2 s reference; tune the sax against its [0,1] s passage.
	ref := wave.Sine(220, 0.9, 2.0, 44100)
	m := fingerprint.TimeManifest{
		TuningPassages: []fingerprint.TuningPassage{{StartSec: 0, EndSec: 1.0, Instrument: "sax"}},
	}
	hints, err := TunePassages(ref, m, 80, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(hints) != 1 || hints[0].Instrument != "sax" || hints[0].Kind != "seed" {
		t.Fatalf("want one sax seed hint, got %+v", hints)
	}
	if len(hints[0].SeedDeltas) == 0 {
		t.Error("seed hint has no tuned params")
	}
}
