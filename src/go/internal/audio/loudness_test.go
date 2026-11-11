package audio

import (
	"math"
	"testing"
)

// TestLUFSIntegrator_SilenceReturnsFloor: with no audio fed in the
// integrator must report the -120 dB silence floor.
func TestLUFSIntegrator_SilenceReturnsFloor(t *testing.T) {
	l := NewLUFSIntegrator(48000)
	if got := l.LUFSShortTerm(); got != -120 {
		t.Errorf("silent LUFS = %g, want -120", got)
	}
}

// TestLUFSIntegrator_FullScaleSineIsLoud: a full-amplitude 1 kHz
// sine should produce a loudness measurement around -3 LUFS on the
// K-weighted scale (1 kHz sits in the K-weighting passband; full-scale
// sine MS = 0.5; LUFS ≈ -0.691 + 10*log10(0.5) ≈ -3.7 plus ~0 dB
// K-weighting tilt at 1 kHz).
func TestLUFSIntegrator_FullScaleSineIsLoud(t *testing.T) {
	const fs = 48000.0
	l := NewLUFSIntegrator(fs)
	// Generate 4 seconds of 1 kHz full-scale sine so the integrator
	// fills its 3 s window and reaches steady state.
	const seconds = 4
	samples := make([]float64, int(fs*seconds))
	for i := range samples {
		samples[i] = math.Sin(2 * math.Pi * 1000 * float64(i) / fs)
	}
	l.Process(samples)
	got := l.LUFSShortTerm()
	// Accept a wide tolerance — the goal is "non-silent and not insanely
	// off"; the exact K-weighting response at 1 kHz approximates 0 dB
	// gain so the result should be close to -3.7 LUFS.
	if got < -10 || got > 0 {
		t.Errorf("1 kHz full-scale sine LUFS = %g, want roughly -3.7 (range -10..0)", got)
	}
}

// TestLUFSIntegrator_QuieterIsLower: halving the amplitude (-6 dBFS)
// must produce a LUFS measurement ~6 dB lower than the full-scale
// reference.
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

// TestKWeightingFilter_PassesUnitSample: smoke test that the filter
// produces finite output for a single impulse and that Reset is
// idempotent.
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

// TestKWeightingFilter_NonDefaultSampleRate ensures bilinear-derived
// coefficients yield a stable filter at 44.1 kHz.
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
