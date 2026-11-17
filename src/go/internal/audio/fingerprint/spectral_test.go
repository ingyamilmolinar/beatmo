package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestSpectralCentroid_BrightVsDull(t *testing.T) {
	sr := 44100
	mLow, hz := wave.MagnitudeSpectrum(wave.Sine(200, 1, 0.3, sr), 16384, wave.WindowHann)
	mHi, _ := wave.MagnitudeSpectrum(wave.Sine(4000, 1, 0.3, sr), 16384, wave.WindowHann)
	cLow := SpectralCentroid(mLow, hz)
	cHi := SpectralCentroid(mHi, hz)
	if !(cHi > cLow*5) {
		t.Errorf("centroid low=%.0f hi=%.0f — expected hi >> low", cLow, cHi)
	}
}

func TestNoiseRatio_SineLowNoiseHigh(t *testing.T) {
	var sfp, nfp Fingerprint
	sfp.F0Hz = 220
	computeSpectral(wave.Sine(220, 0.9, 1.5, 44100), &sfp)
	nfp.F0Hz = 220
	computeSpectral(wave.Noise(7, 0.9, 1.5, 44100), &nfp)
	if sfp.NoiseRatio > 0.3 {
		t.Errorf("sine NoiseRatio=%.3f want <0.3", sfp.NoiseRatio)
	}
	if nfp.NoiseRatio < 0.6 {
		t.Errorf("noise NoiseRatio=%.3f want >0.6", nfp.NoiseRatio)
	}
}

func TestPartials_HighHarmonicsSurviveVibrato(t *testing.T) {
	sr := 48000
	w := synthVibratoTone(sr, 2.0, 591.0, []float64{1, .75, .22, .58, .37, .1, .08, .06}, 5.0, 20.0)
	fp := FromWave(w, "vib")
	if fp.Partials[3] < 0.2 {
		t.Fatalf("Partials[3]=%.3f, want >0.2 (window too tight under vibrato)", fp.Partials[3])
	}
	if fp.Partials[7] < 0.02 {
		t.Fatalf("Partials[7]=%.3f, want >0.02", fp.Partials[7])
	}
}

func TestFromWave_SilenceNoNaN(t *testing.T) {
	w := wave.Wave{Samples: make([]float64, 48000), SampleRate: 48000}
	fp := FromWave(w, "silent")
	if math.IsNaN(fp.SpectralCentroid) || math.IsNaN(fp.F0Hz) {
		t.Fatalf("silence produced NaN: centroid=%v f0=%v", fp.SpectralCentroid, fp.F0Hz)
	}
}
