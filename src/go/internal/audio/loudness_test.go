package audio

import (
	"math"
	"testing"
)

func TestLUFSIntegrator_SilenceReturnsFloor(t *testing.T) {
	l := NewLUFSIntegrator(48000)
	if got := l.LUFSShortTerm(); got != -120 {
		t.Errorf("silent LUFS = %g, want -120", got)
	}
}

// 1 kHz sits in the K-weighting passband: MS 0.5 gives ≈ -3.7 LUFS.
func TestLUFSIntegrator_FullScaleSineIsLoud(t *testing.T) {
	const fs = 48000.0
	l := NewLUFSIntegrator(fs)
	// Fills the 3 s window.
	const seconds = 4
	samples := make([]float64, int(fs*seconds))
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 1000 * float64(i) / fs)
	}
	l.Process(samples)
	got := l.LUFSShortTerm()
	// Wide: the goal is "non-silent and not wildly off".
	if got < -10 || got > 0 {
		t.Errorf("1 kHz full-scale sine LUFS = %g, want roughly -3.7 (range -10..0)", got)
	}
}

func TestLUFSIntegrator_QuieterIsLower(t *testing.T) {
	const fs = 48000.0
	loud := NewLUFSIntegrator(fs)
	quiet := NewLUFSIntegrator(fs)
	const seconds = 4
	N := int(fs * seconds)
	for i := 0; i < N; i++ {
		x := math.Sin(2 * math.Pi * 1000 * float64(i) / fs)
		loud.Process([]float64{x})
		quiet.Process([]float64{x * 0.5})
	}
	a := loud.LUFSShortTerm()
	b := quiet.LUFSShortTerm()
	diff := a - b
	if diff < 5 || diff > 7 {
		t.Errorf("LUFS diff: loud=%g quiet=%g diff=%g want ~6", a, b, diff)
	}
}

// TestLUFSIntegrator_ResetClearsState pins Reset behaviour.
func TestLUFSIntegrator_ResetClearsState(t *testing.T) {
	l := NewLUFSIntegrator(48000)
	// Push some energy in.
	samples := make([]float64, 48000)
	for i := range samples {
		samples[i] = 1.0
	}
	l.Process(samples)
	before := l.LUFSShortTerm()
	if before <= -120 {
		t.Fatalf("LUFS before reset should be elevated, got %g", before)
	}
	l.Reset()
	after := l.LUFSShortTerm()
	if after != -120 {
		t.Errorf("LUFS after reset = %g, want -120", after)
	}
}

func TestKWeightingFilter_PassesUnitSample(t *testing.T) {
	f := NewKWeightingFilter(48000)
	y := f.Process(1.0)
	if math.IsNaN(y) || math.IsInf(y, 0) {
		t.Errorf("K-weighting filter produced non-finite output: %g", y)
	}
	f.Reset()
	if y2 := f.Process(0); math.IsNaN(y2) || math.IsInf(y2, 0) {
		t.Errorf("K-weighting filter after reset produced non-finite output: %g", y2)
	}
}

// Bilinear-derived coefficients at 44.1 kHz.
func TestKWeightingFilter_NonDefaultSampleRate(t *testing.T) {
	f := NewKWeightingFilter(44100)
	// Push a short tone burst; output must stay finite and bounded.
	for i := 0; i < 1024; i++ {
		y := f.Process(math.Sin(2 * math.Pi * 1000 * float64(i) / 44100))
		if math.IsNaN(y) || math.IsInf(y, 0) {
			t.Fatalf("44.1 kHz K-weighting produced non-finite output at i=%d: %g", i, y)
		}
		if math.Abs(y) > 10 {
			t.Fatalf("44.1 kHz K-weighting unbounded at i=%d: %g", i, y)
		}
	}
}
