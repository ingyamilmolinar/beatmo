package fingerprint

import (
	"math"
	"testing"
)

func TestComputeVibrato_RecoversInjectedRate(t *testing.T) {
	sr := 48000
	w := synthVibratoTone(sr, 2.0, 440, []float64{1, .5, .25}, 5.5, 20)
	seg := sustainWindow(w)
	vf := computeVibrato(seg, 440, DefaultAnalysisConfig())
	if !vf.Detected {
		t.Fatal("vibrato not detected")
	}
	if math.Abs(vf.RateHz-5.5) > 0.7 {
		t.Fatalf("RateHz=%.2f, want ~5.5", vf.RateHz)
	}
	if math.Abs(vf.ExtentCents-20) > 6 {
		t.Fatalf("ExtentCents=%.1f, want ~20", vf.ExtentCents)
	}
	// Pin Jitter and OnsetSec so constant-returning implementations are caught.
	if math.IsNaN(vf.Jitter) || math.IsInf(vf.Jitter, 0) {
		t.Fatalf("Jitter is not finite: %v", vf.Jitter)
	}
	if vf.Jitter >= 0.5 {
		t.Fatalf("Jitter=%.4f, want < 0.5 for regular vibrato", vf.Jitter)
	}
	if vf.OnsetSec < 0 {
		t.Fatalf("OnsetSec=%.4f, must be >= 0", vf.OnsetSec)
	}
	if vf.OnsetSec >= 1.5 {
		t.Fatalf("OnsetSec=%.4f, must be < 1.5 s for a 2 s vibrato tone", vf.OnsetSec)
	}
}

func TestComputeVibrato_FlatToneNotDetected(t *testing.T) {
	w := synthTone(48000, 2.0, 440, []float64{1, .5, .25})
	vf := computeVibrato(sustainWindow(w), 440, DefaultAnalysisConfig())
	if vf.Detected {
		t.Fatalf("flat tone falsely detected vibrato: %+v", vf)
	}
	// I3: extent must be small for a steady tone (detrended track removes
	// FFT-bin quantization drift that would otherwise inflate this).
	if vf.ExtentCents >= 4.0 {
		t.Fatalf("flat tone ExtentCents=%.2f, want < 4.0 (detrended; no quantization inflation)", vf.ExtentCents)
	}
}
