package fingerprint

import (
	"math"
	"testing"
)

// TestWholeSignalCoupling_PositiveOnBrightLoudSwell verifies WholeSignalCentroidRMSCorr:
// a swell tone (louder=brighter over the whole note) must yield a positive macro corr;
// a flat-spectrum tone must yield ~0.
func TestWholeSignalCoupling_PositiveOnBrightLoudSwell(t *testing.T) {
	w := synthBrightLoudTone(48000, 2.0, 440)
	got := WholeSignalCentroidRMSCorr(w, DefaultAnalysisConfig())
	if got < 0.4 {
		t.Fatalf("whole-signal corr=%.2f, want >0.4 (louder=brighter swell)", got)
	}
	flat := synthTone(48000, 2.0, 440, []float64{1, .5, .25})
	if c := WholeSignalCentroidRMSCorr(flat, DefaultAnalysisConfig()); math.Abs(c) > 0.3 {
		t.Fatalf("flat whole-signal corr=%.2f, want ~0", c)
	}
}

// TestCoupling_PositiveWhenBrightnessTracksLoudness verifies that
// synthBrightLoudTone (where upper partials grow proportionally to loudness)
// yields a high CentroidRMSCorr, while a static-spectrum tone yields ~0.
func TestCoupling_PositiveWhenBrightnessTracksLoudness(t *testing.T) {
	sr := 48000
	coupled := synthBrightLoudTone(sr, 2.0, 440)
	flat := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	c1 := computeCoupling(sustainWindow(coupled), DefaultAnalysisConfig())
	c0 := computeCoupling(sustainWindow(flat), DefaultAnalysisConfig())
	if c1.CentroidRMSCorr < 0.4 {
		t.Fatalf("coupled corr=%.2f, want >0.4", c1.CentroidRMSCorr)
	}
	if math.Abs(c0.CentroidRMSCorr) > 0.3 {
		t.Fatalf("flat corr=%.2f, want ~0", c0.CentroidRMSCorr)
	}
}

// TestCoupling_BrightnessModDepth verifies that the coupled tone has higher
// modulation depth than the flat tone (its centroid varies more relative to
// its mean).
func TestCoupling_BrightnessModDepth(t *testing.T) {
	sr := 48000
	coupled := synthBrightLoudTone(sr, 2.0, 440)
	flat := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	c1 := computeCoupling(sustainWindow(coupled), DefaultAnalysisConfig())
	c0 := computeCoupling(sustainWindow(flat), DefaultAnalysisConfig())
	if c1.BrightnessModDepth <= c0.BrightnessModDepth {
		t.Fatalf("coupled depth=%.4f should exceed flat depth=%.4f",
			c1.BrightnessModDepth, c0.BrightnessModDepth)
	}
}

// TestCoupling_SilentInput verifies that a zero (silent) wave returns the zero
// DynamicCoupling struct without panicking.
func TestCoupling_SilentInput(t *testing.T) {
	sr := 48000
	flat := synthTone(sr, 0.5, 440, []float64{0})
	dc := computeCoupling(flat, DefaultAnalysisConfig())
	if dc.CentroidRMSCorr != 0 || dc.BrightnessModDepth != 0 || dc.BrightnessModRateHz != 0 {
		t.Fatalf("expected zero struct for silent input, got %+v", dc)
	}
}

// TestCoupling_ModRateHz verifies that BrightnessModRateHz is within [0, 15] Hz
// for typical signals and is near-zero for a flat (non-modulated) tone.
func TestCoupling_ModRateHz(t *testing.T) {
	sr := 48000
	flat := synthTone(sr, 2.0, 440, []float64{1, .5, .25})
	dc := computeCoupling(sustainWindow(flat), DefaultAnalysisConfig())
	if dc.BrightnessModRateHz < 0 || dc.BrightnessModRateHz > 15 {
		t.Fatalf("mod rate %.2f Hz out of [0,15] range", dc.BrightnessModRateHz)
	}
	// A steady-spectrum flat tone must report ~0 modulation rate (not merely in
	// range) — the significance guard in dominantFreq should suppress noise.
	if dc.BrightnessModRateHz >= 1.0 {
		t.Fatalf("flat tone BrightnessModRateHz=%.2f, want < 1.0 (significance guard should yield ~0)", dc.BrightnessModRateHz)
	}
}
