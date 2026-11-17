package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// chromaOf renders a set of pitches (Hz) summed and returns its chroma.
func chromaOf(freqs []float64) [12]float64 {
	sr := 44100
	var s []float64
	for _, f := range freqs {
		w := wave.Sine(f, 0.5, 0.7, sr)
		if s == nil {
			s = make([]float64, len(w.Samples))
		}
		for i := range s {
			s[i] += w.Samples[i]
		}
	}
	mag, hz := wave.MagnitudeSpectrum(wave.Wave{Samples: s, SampleRate: sr}, 16384, wave.WindowHann)
	return Chroma(mag, hz)
}

func TestDetectKey_CMajorTriad(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	// C4,E4,G4 ≈ 261.63, 329.63, 392.00
	ch := chromaOf([]float64{261.63, 329.63, 392.00})
	key, mode, _ := DetectKey(ch, cfg)
	if key != 0 || mode != 0 {
		t.Errorf("key=%d mode=%d want 0(C) major", key, mode)
	}
}

func TestBestChromaShift_TranspositionInvariant(t *testing.T) {
	ch := chromaOf([]float64{261.63, 329.63, 392.00}) // C major
	// Shift the chroma up 2 semitones manually (D major shape).
	var shifted [12]float64
	for i := 0; i < 12; i++ {
		shifted[(i+2)%12] = ch[i]
	}
	shift, corr := BestChromaShift(ch, shifted)
	if shift != 2 && shift != -10 { // +2 mod 12
		t.Errorf("shift=%d want +2 semitones", shift)
	}
	if corr < 0.99 {
		t.Errorf("corr=%.3f want ~1 for a pure transposition", corr)
	}
}
