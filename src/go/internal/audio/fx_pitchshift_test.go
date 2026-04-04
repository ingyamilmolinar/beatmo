//go:build test || js

package audio

import (
	"math"
	"testing"
)

const pitchSR = 44100

// pitchTestSamples returns 2 seconds of test signal at the given sample rate,
// enough for the pitch shifter to stabilize.
func pitchTestSamples() int { return pitchSR * 2 }

func newTestPitchShift(pitch, mix float64) *pitchShift {
	return newPitchShift(pitchSR, map[string]float64{
		"pitch": pitch,
		"mix":   mix,
	})
}

func TestPitchShiftOctaveUp(t *testing.T) {
	ps := newTestPitchShift(12, 1)
	n := pitchTestSamples()
	input := sineSamples(440, pitchSR, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = ps.ProcessSample(x)
	}

	// Skip initial transient (first 10% of samples).
	skip := n / 10
	stable := out[skip:]

	mag880 := goertzelMagnitude(stable, 880, pitchSR)
	mag440 := goertzelMagnitude(stable, 440, pitchSR)

	// Target frequency (880 Hz) should have more energy than original (440 Hz).
	if mag880 <= mag440 {
		t.Fatalf("octave up: expected 880Hz energy (%.4f) > 440Hz energy (%.4f)", mag880, mag440)
	}
}

func TestPitchShiftOctaveDown(t *testing.T) {
	ps := newTestPitchShift(-12, 1)
	n := pitchTestSamples()
	input := sineSamples(440, pitchSR, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = ps.ProcessSample(x)
	}

	skip := n / 10
	stable := out[skip:]

	mag220 := goertzelMagnitude(stable, 220, pitchSR)
	mag440 := goertzelMagnitude(stable, 440, pitchSR)

	if mag220 <= mag440 {
		t.Fatalf("octave down: expected 220Hz energy (%.4f) > 440Hz energy (%.4f)", mag220, mag440)
	}
}

func TestPitchShiftFifthUp(t *testing.T) {
	ps := newTestPitchShift(7, 1)
	n := pitchTestSamples()
	input := sineSamples(440, pitchSR, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = ps.ProcessSample(x)
	}

	skip := n / 10
	stable := out[skip:]

	// Perfect fifth: 440 * 2^(7/12) ~ 659.26 Hz
	targetFreq := 440.0 * math.Pow(2.0, 7.0/12.0)
	magTarget := goertzelMagnitude(stable, targetFreq, pitchSR)
	mag440 := goertzelMagnitude(stable, 440, pitchSR)

	if magTarget <= mag440 {
		t.Fatalf("fifth up: expected %.0fHz energy (%.4f) > 440Hz energy (%.4f)",
			targetFreq, magTarget, mag440)
	}
}

func TestPitchShiftZeroPassthrough(t *testing.T) {
	ps := newTestPitchShift(0, 1)
	n := pitchTestSamples()
	input := sineSamples(440, pitchSR, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = ps.ProcessSample(x)
	}

	// Skip transient.
	skip := n / 10
	inRMS := rmsEnergy(input[skip:])
	outRMS := rmsEnergy(out[skip:])

	// RMS should be within 20%.
	ratio := outRMS / inRMS
	if ratio < 0.8 || ratio > 1.2 {
		t.Fatalf("pitch=0 passthrough: RMS ratio %.3f outside [0.8, 1.2] (in=%.4f out=%.4f)",
			ratio, inRMS, outRMS)
	}
}

func TestPitchShiftMixZero(t *testing.T) {
	// Create with mix=0 so the smoothParam starts at zero; assertDryWhenMixZero
	// calls SetParam("mix", 0) which is then a no-op ramp.
	ps := newTestPitchShift(12, 0)
	assertDryWhenMixZero(t, ps, 440, pitchSR, pitchTestSamples())
}

func TestPitchShiftReset(t *testing.T) {
	ps := newTestPitchShift(12, 1)
	assertResetClearsState(t, ps)
}

func TestPitchShiftNoNaN(t *testing.T) {
	pitches := []float64{-24, -12, 0, 12, 24}
	for _, p := range pitches {
		ps := newTestPitchShift(p, 1)
		out := processFX(ps, 440, pitchSR, pitchTestSamples())
		assertNoNaNOrInf(t, out)
	}
}

func TestPitchShiftWindowVariations(t *testing.T) {
	windows := []float64{20, 50, 100}
	for _, w := range windows {
		ps := newPitchShift(pitchSR, map[string]float64{
			"pitch":  7,
			"mix":    1,
			"window": w,
		})
		out := processFX(ps, 440, pitchSR, pitchTestSamples())
		assertNoNaNOrInf(t, out)

		// Verify output has meaningful energy (not silence).
		outRMS := rmsEnergy(out[pitchSR/10:]) // skip first 10%
		if outRMS < 0.01 {
			t.Fatalf("window=%.0fms: output RMS too low (%.6f), expected audible signal", w, outRMS)
		}
	}
}
