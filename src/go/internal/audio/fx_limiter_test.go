//go:build test || js

package audio

import (
	"math"
	"testing"
)

func TestLimiterCeiling(t *testing.T) {
	// Feed a hot signal (+6 dB, amplitude 2.0). No output sample should
	// exceed the ceiling level.
	l := newLimiter(44100, DefaultParams(EffectLimiter))
	const sr = 44100
	const n = 8000
	ceilingLin := l.ceilingLin

	for i := 0; i < n; i++ {
		x := 2.0 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
		out := l.ProcessSample(x)
		if math.Abs(out) > ceilingLin+1e-6 {
			t.Fatalf("sample %d: |output|=%.6f exceeds ceiling=%.6f", i, math.Abs(out), ceilingLin)
		}
	}
}

func TestLimiterPassesBelowThreshold(t *testing.T) {
	// A quiet signal (amplitude 0.1, ~-20 dB) should be below or near the
	// default -1 dB threshold and pass through scaled by ceiling/threshold.
	l := newLimiter(44100, DefaultParams(EffectLimiter))
	const sr = 44100
	const n = 4000
	scale := l.ceilingLin / l.thresholdLin

	input := make([]float64, n)
	output := make([]float64, n)
	for i := range input {
		input[i] = 0.1 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}
	for i, x := range input {
		output[i] = l.ProcessSample(x)
	}

	// Skip first samples for envelope settling.
	inRMS := rmsEnergy(input[500:])
	outRMS := rmsEnergy(output[500:])

	expectedRMS := inRMS * scale
	ratio := outRMS / expectedRMS
	if ratio < 0.90 || ratio > 1.10 {
		t.Errorf("quiet signal should pass scaled: outRMS=%.6f expectedRMS=%.6f ratio=%.4f",
			outRMS, expectedRMS, ratio)
	}
}

func TestLimiterReset(t *testing.T) {
	l := newLimiter(44100, DefaultParams(EffectLimiter))
	assertResetClearsState(t, l)
}

func TestLimiterNoNaN(t *testing.T) {
	l := newLimiter(44100, DefaultParams(EffectLimiter))
	output := make([]float64, 1000)
	for i := range output {
		output[i] = l.ProcessSample(0)
	}
	assertNoNaNOrInf(t, output)
}

func TestLimiterSetParam(t *testing.T) {
	// With threshold=-1 dB, a 2.0 amplitude signal is well above threshold
	// and gets limited. After raising threshold to 0 dB, less limiting occurs
	// and the peak output should be higher.
	l := newLimiter(44100, map[string]float64{
		"threshold": -6,
		"release":   50,
		"ceiling":   -6,
	})
	const sr = 44100
	const n = 4000

	input := make([]float64, n)
	for i := range input {
		input[i] = 2.0 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}

	// First pass with threshold=-6 dB.
	out1 := make([]float64, n)
	for i, x := range input {
		out1[i] = l.ProcessSample(x)
	}
	var peak1 float64
	for _, v := range out1[500:] {
		if a := math.Abs(v); a > peak1 {
			peak1 = a
		}
	}

	// Raise threshold to 0 dB — less limiting, so peaks can be higher.
	l.Reset()
	l.SetParam("threshold", 0)
	l.SetParam("ceiling", 0)

	out2 := make([]float64, n)
	for i, x := range input {
		out2[i] = l.ProcessSample(x)
	}
	var peak2 float64
	for _, v := range out2[500:] {
		if a := math.Abs(v); a > peak2 {
			peak2 = a
		}
	}

	// With higher threshold, peaks should be higher (less compression).
	if peak2 <= peak1 {
		t.Errorf("higher threshold should allow higher peaks: peak1=%.6f peak2=%.6f", peak1, peak2)
	}
}

func TestLimiterBrickWall(t *testing.T) {
	// Feed an impulse of amplitude 5.0. The limiter should never let output
	// exceed the ceiling.
	l := newLimiter(44100, DefaultParams(EffectLimiter))
	ceilingLin := l.ceilingLin

	// Single large impulse followed by silence.
	impulse := 5.0
	out := l.ProcessSample(impulse)
	if math.Abs(out) > ceilingLin+1e-6 {
		t.Fatalf("impulse: |output|=%.6f exceeds ceiling=%.6f", math.Abs(out), ceilingLin)
	}

	// Negative impulse.
	out = l.ProcessSample(-impulse)
	if math.Abs(out) > ceilingLin+1e-6 {
		t.Fatalf("neg impulse: |output|=%.6f exceeds ceiling=%.6f", math.Abs(out), ceilingLin)
	}

	// Several more impulses at varying amplitudes.
	for _, amp := range []float64{10.0, 3.0, 100.0, 0.001} {
		out = l.ProcessSample(amp)
		if math.Abs(out) > ceilingLin+1e-6 {
			t.Fatalf("amp=%.1f: |output|=%.6f exceeds ceiling=%.6f", amp, math.Abs(out), ceilingLin)
		}
	}
}
