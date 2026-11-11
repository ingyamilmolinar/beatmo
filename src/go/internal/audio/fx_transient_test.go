package audio

import (
	"math"
	"testing"
)

const sr = 44100

// drumSignal generates a drum-like signal: hard transient followed by
// exponential decay, simulating a typical percussive hit.
func drumSignal(sr, samples int) []float64 {
	out := make([]float64, samples)
	for i := range out {
		t := float64(i) / float64(sr)
		out[i] = 0.9 * math.Exp(-t*20) // sharp exponential decay
		if i < 10 {
			out[i] = 0.9 // hard transient
		}
	}
	return out
}

// processSignal runs a signal through the effect and returns the output.
func processSignal(fx InsertEffect, input []float64) []float64 {
	out := make([]float64, len(input))
	for i, x := range input {
		out[i] = fx.ProcessSample(x)
	}
	return out
}

// TestTransientBoostsAttack verifies that boosting the attack parameter
// increases the energy of the initial transient portion of a drum signal.
func TestTransientBoostsAttack(t *testing.T) {
	drum := drumSignal(sr, 4096)

	// Default: attack=100, sustain=100
	fxDefault := newTransient(sr, map[string]float64{"attack": 100, "sustain": 100, "speed": 10})
	outDefault := processSignal(fxDefault, drum)

	// Boosted attack: attack=200, sustain=100
	fxBoosted := newTransient(sr, map[string]float64{"attack": 200, "sustain": 100, "speed": 10})
	outBoosted := processSignal(fxBoosted, drum)

	// Compare energy of the first 100 samples (transient region).
	defaultAttackEnergy := rmsEnergy(outDefault[:100])
	boostedAttackEnergy := rmsEnergy(outBoosted[:100])

	if boostedAttackEnergy <= defaultAttackEnergy {
		t.Fatalf("expected boosted attack energy (%.6f) > default (%.6f)",
			boostedAttackEnergy, defaultAttackEnergy)
	}
}

// TestTransientReducesSustain verifies that reducing the sustain parameter
// lowers the energy of the tail portion of a drum signal.
func TestTransientReducesSustain(t *testing.T) {
	drum := drumSignal(sr, 4096)

	// Default: attack=100, sustain=100
	fxDefault := newTransient(sr, map[string]float64{"attack": 100, "sustain": 100, "speed": 10})
	outDefault := processSignal(fxDefault, drum)

	// Reduced sustain: attack=100, sustain=50
	fxReduced := newTransient(sr, map[string]float64{"attack": 100, "sustain": 50, "speed": 10})
	outReduced := processSignal(fxReduced, drum)

	// Compare tail energy (samples 500-4000).
	defaultTailEnergy := rmsEnergy(outDefault[500:4000])
	reducedTailEnergy := rmsEnergy(outReduced[500:4000])

	if reducedTailEnergy >= defaultTailEnergy {
		t.Fatalf("expected reduced sustain tail energy (%.6f) < default (%.6f)",
			reducedTailEnergy, defaultTailEnergy)
	}
}

// TestTransientDefaultPassthrough verifies that with attack=100 and sustain=100
// (both unity), the output is very close to the input signal.
func TestTransientDefaultPassthrough(t *testing.T) {
	drum := drumSignal(sr, 4096)

	fx := newTransient(sr, map[string]float64{"attack": 100, "sustain": 100, "speed": 10})
	out := processSignal(fx, drum)

	// With both gains at unity, output should closely match input.
	var maxDiff float64
	for i := range drum {
		diff := math.Abs(out[i] - drum[i])
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	// Allow small deviation due to envelope dynamics during transient onset.
	if maxDiff > 0.5 {
		t.Fatalf("default params should approximate passthrough, max diff = %.6f", maxDiff)
	}

	// Also check RMS is close.
	inRMS := rmsEnergy(drum)
	outRMS := rmsEnergy(out)
	ratio := outRMS / inRMS
	if ratio < 0.8 || ratio > 1.2 {
		t.Fatalf("RMS ratio %.3f too far from 1.0 (inRMS=%.6f outRMS=%.6f)", ratio, inRMS, outRMS)
	}
}

// TestTransientReset verifies that Reset() clears the envelope state so that
// processing silence after reset produces silence.
func TestTransientReset(t *testing.T) {
	fx := newTransient(sr, map[string]float64{"attack": 200, "sustain": 50, "speed": 5})
	assertResetClearsState(t, fx)
}

// TestTransientNoNaNInf verifies that processing various signals never
// produces NaN or Inf output values.
func TestTransientNoNaNInf(t *testing.T) {
	fx := newTransient(sr, map[string]float64{"attack": 200, "sustain": 10, "speed": 1})

	// Test with drum signal.
	drum := drumSignal(sr, 2048)
	out := processSignal(fx, drum)
	assertNoNaNOrInf(t, out)

	// Test with silence.
	fx.Reset()
	silence := make([]float64, 1000)
	out = processSignal(fx, silence)
	assertNoNaNOrInf(t, out)

	// Test with extreme values.
	fx.Reset()
	extreme := []float64{0, 1, -1, 100, -100, 1e-15, -1e-15, 0.001, -0.001}
	out = processSignal(fx, extreme)
	assertNoNaNOrInf(t, out)

	// Test with sine wave.
	fx.Reset()
	sine := sineSamples(440, sr, 2048)
	out = processSignal(fx, sine)
	assertNoNaNOrInf(t, out)
}

// TestTransientModifiesSignal verifies that with non-default parameters the
// effect actually changes the signal.
func TestTransientModifiesSignal(t *testing.T) {
	fx := newTransient(sr, map[string]float64{"attack": 200, "sustain": 30, "speed": 5})

	drum := drumSignal(sr, 4096)
	out := processSignal(fx, drum)

	// Compute difference.
	diff := make([]float64, len(drum))
	for i := range diff {
		diff[i] = out[i] - drum[i]
	}
	diffRMS := rmsEnergy(diff)
	inRMS := rmsEnergy(drum)

	if diffRMS < inRMS*0.01 {
		t.Fatalf("effect with attack=200/sustain=30 should modify signal, diffRMS=%.6f inRMS=%.6f",
			diffRMS, inRMS)
	}
}
