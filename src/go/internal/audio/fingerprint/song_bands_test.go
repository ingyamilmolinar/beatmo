package fingerprint

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestBandEnergies_PutsSineInRightBand(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	mag, hz := wave.MagnitudeSpectrum(wave.Sine(100, 0.9, 0.5, 44100), cfg.FFTSize, wave.WindowHann)
	e := BandEnergies(mag, hz, cfg.Bands) // bands: sub,low,mid,high,air
	// 100 Hz lands in "low" (60–250), index 1; that band must dominate.
	for i := range e {
		if i != 1 && e[i] > e[1] {
			t.Errorf("band %d energy %.3g exceeds low-band %.3g for a 100 Hz tone", i, e[i], e[1])
		}
	}
}

func TestTonalPercussiveRatio_SineHighNoiseLow(t *testing.T) {
	cfg := DefaultAnalysisConfig()
	tone := TonalPercussiveRatio(wave.Sine(440, 0.9, 0.8, 44100), cfg)
	noise := TonalPercussiveRatio(wave.Noise(7, 0.9, 0.8, 44100), cfg)
	if !(tone > noise) {
		t.Errorf("tonal(sine)=%.3f should exceed tonal(noise)=%.3f", tone, noise)
	}
}
