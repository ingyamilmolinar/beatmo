package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSongFingerprintOf_PopulatesAllSections(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	cfg.HopSec = 0.02
	w := clickTrain(0.5, 4.0, 44100) // reuse helper from song_tempo_test.go
	// Add a tonal component so chroma/harmonics are non-empty.
	tone := wave.Sine(261.63, 0.5, 4.0, 44100).Samples
	for i := range w.Samples {
		w.Samples[i] += tone[i]
	}
	fp := SongFingerprintOf(w, cfg)
	if fp.TempoBPM <= 0 {
		t.Errorf("TempoBPM not detected")
	}
	if len(fp.Bands) != len(cfg.Bands) {
		t.Errorf("Bands=%d want %d", len(fp.Bands), len(cfg.Bands))
	}
	if len(fp.Harmonics) == 0 {
		t.Errorf("Harmonics empty")
	}
}

func TestSongFingerprintOf_Deterministic(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	w := wave.Sine(220, 0.9, 2.0, 44100)
	a := SongFingerprintOf(w, cfg)
	b := SongFingerprintOf(w, cfg)
	if a.TempoBPM != b.TempoBPM || a.Key != b.Key || a.IntonationCents != b.IntonationCents {
		t.Errorf("non-deterministic fingerprint")
	}
}
