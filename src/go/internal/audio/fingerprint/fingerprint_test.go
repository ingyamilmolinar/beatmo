package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestFromWave_PopulatesNewMetrics(t *testing.T) {
	w := synthVibratoTone(48000, 2.0, 440, []float64{1, .6, .3, .15}, 6.0, 25)
	fp := FromWave(w, "x")
	if fp.Vibrato == nil || !fp.Vibrato.Detected {
		t.Fatal("Vibrato not populated/detected")
	}
	if fp.Coupling == nil || fp.SpectralEnv == nil || fp.Harmonics == nil || fp.NoiseProf == nil {
		t.Fatal("new metric sub-structs not populated")
	}
}

func TestFromWave_SilenceLeavesNewMetricsNil(t *testing.T) {
	w := wave.Wave{Samples: make([]float64, 48000), SampleRate: 48000}
	fp := FromWave(w, "silent")
	if fp.Vibrato != nil || fp.Coupling != nil {
		t.Fatal("silence should leave new metrics nil (early-out)")
	}
}
