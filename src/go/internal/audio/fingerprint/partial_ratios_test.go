package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// --- direct-formula sanity (mirrors kick_inharmonicity_test.go's style) -----

func TestPartialRatiosFromPeaks_HarmonicSeries(t *testing.T) {
	// A clean harmonic stack: 1,2,3,4 x f0=200. Every rank's ratio equals
	// its expected consecutive harmonic number exactly.
	pr := partialRatiosFromPeaks([]spectralPartial{
		{Freq: 200, Amp: 1}, {Freq: 400, Amp: 1}, {Freq: 600, Amp: 1}, {Freq: 800, Amp: 1},
	}, 200)
	want := []float64{1, 2, 3, 4}
	if len(pr.Ratios) != len(want) {
		t.Fatalf("Ratios = %v, want len %d", pr.Ratios, len(want))
	}
	for i, w := range want {
		if math.Abs(pr.Ratios[i]-w) > 1e-9 {
			t.Errorf("Ratios[%d] = %.4f, want %.4f", i, pr.Ratios[i], w)
		}
	}
	if pr.SeriesDeviation > 1e-9 {
		t.Errorf("SeriesDeviation = %.5f, want ~0 for a perfect harmonic series", pr.SeriesDeviation)
	}
}

func TestPartialRatiosFromPeaks_BarLikeNonConsecutive(t *testing.T) {
	// A tuned-bar-like stack: partials near 1x, 4x, 9x f0 (skips 2,3,5-8) --
	// each individual partial is close to SOME integer, so a
	// nearest-integer mistuning check would call this nearly perfectly in
	// tune. SeriesDeviation must instead catch that rank 2 (by frequency)
	// sits at 4x, not the 2x a consecutive series would place there.
	pr := partialRatiosFromPeaks([]spectralPartial{
		{Freq: 200, Amp: 1}, {Freq: 804, Amp: 1}, {Freq: 1830, Amp: 1},
	}, 200)
	want := []float64{1, 4.02, 9.15}
	if len(pr.Ratios) != len(want) {
		t.Fatalf("Ratios = %v, want len %d", pr.Ratios, len(want))
	}
	for i, w := range want {
		if math.Abs(pr.Ratios[i]-w) > 1e-6 {
			t.Errorf("Ratios[%d] = %.4f, want %.4f", i, pr.Ratios[i], w)
		}
	}
	if pr.SeriesDeviation < 0.5 {
		t.Errorf("SeriesDeviation = %.4f, want clearly large (bar overtones skip ranks)", pr.SeriesDeviation)
	}
}

func TestPartialRatiosFromPeaks_PureTone(t *testing.T) {
	pr := partialRatiosFromPeaks([]spectralPartial{{Freq: 200, Amp: 1}}, 200)
	if len(pr.Ratios) != 1 || math.Abs(pr.Ratios[0]-1) > 1e-9 {
		t.Errorf("Ratios = %v, want [1]", pr.Ratios)
	}
	if pr.SeriesDeviation != 0 {
		t.Errorf("SeriesDeviation = %g, want exactly 0 for a single partial at f0", pr.SeriesDeviation)
	}
}

func TestPartialRatiosFromPeaks_Degenerate(t *testing.T) {
	if got := partialRatiosFromPeaks(nil, 200); got.Ratios != nil || got.SeriesDeviation != 0 {
		t.Errorf("no peaks: got %+v, want zero value", got)
	}
	if got := partialRatiosFromPeaks([]spectralPartial{{Freq: 200, Amp: 1}}, 0); got.Ratios != nil {
		t.Errorf("f0<=0: got %+v, want zero value", got)
	}
	if got := partialRatiosFromPeaks([]spectralPartial{{Freq: 200, Amp: 1}}, -10); got.Ratios != nil {
		t.Errorf("f0<0: got %+v, want zero value", got)
	}
}

// --- full-pipeline sanity (FFT-resolvable content, mirrors
// TestKickAnalyze_InharmonicityPipeline) --------------------------------------

func TestAnalyzePartialRatios_Pipeline(t *testing.T) {
	const sr = 44100
	const fftSize = 16384
	binHz := float64(sr) / float64(fftSize)

	// Fundamental-dominant amplitudes so peak-picking finds all 3 partials
	// with the fundamental included; spacings are all >> binHz so each
	// partial resolves to its own bin cleanly.
	harmonicWave := weightedPartials(sr, 1.0, []float64{220, 440, 660}, []float64{1.0, 0.6, 0.4})
	barWave := weightedPartials(sr, 1.0, []float64{220, 884, 2013}, []float64{1.0, 0.6, 0.4})

	harmMag, gotBinHz := wave.MagnitudeSpectrum(wave.Wave{Samples: harmonicWave.Samples, SampleRate: sr}, fftSize, wave.WindowHann)
	barMag, _ := wave.MagnitudeSpectrum(wave.Wave{Samples: barWave.Samples, SampleRate: sr}, fftSize, wave.WindowHann)
	if math.Abs(gotBinHz-binHz) > 1e-6 {
		t.Fatalf("binHz = %.4f, want %.4f", gotBinHz, binHz)
	}

	harmPR := AnalyzePartialRatios(harmMag, gotBinHz, 220)
	barPR := AnalyzePartialRatios(barMag, gotBinHz, 220)

	if len(harmPR.Ratios) < 3 || len(barPR.Ratios) < 3 {
		t.Fatalf("harmPR.Ratios=%v barPR.Ratios=%v, want >=3 partials each", harmPR.Ratios, barPR.Ratios)
	}
	if harmPR.SeriesDeviation > 0.05 {
		t.Errorf("harmonic tone SeriesDeviation = %.4f, want near 0", harmPR.SeriesDeviation)
	}
	if barPR.SeriesDeviation <= harmPR.SeriesDeviation {
		t.Errorf("bar-like SeriesDeviation (%.4f) must exceed harmonic (%.4f)", barPR.SeriesDeviation, harmPR.SeriesDeviation)
	}
	if barPR.SeriesDeviation < 0.5 {
		t.Errorf("bar-like SeriesDeviation = %.4f, want clearly large (>0.5)", barPR.SeriesDeviation)
	}
}

func TestAnalyzePartialRatios_Degenerate(t *testing.T) {
	mag := make([]float64, 100)
	if got := AnalyzePartialRatios(mag, 10, 0); got.Ratios != nil {
		t.Errorf("f0<=0: got %+v, want zero value", got)
	}
	if got := AnalyzePartialRatios(mag, 0, 200); got.Ratios != nil {
		t.Errorf("binHz<=0: got %+v, want zero value", got)
	}
	if got := AnalyzePartialRatios(nil, 10, 200); got.Ratios != nil {
		t.Errorf("empty mag: got %+v, want zero value", got)
	}
}
