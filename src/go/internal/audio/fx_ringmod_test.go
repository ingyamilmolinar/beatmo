package audio

import (
	"math"
	"testing"
)

func TestRingModModifiesSignal(t *testing.T) {
	rm := newRingMod(44100, map[string]float64{"frequency": 440, "shape": 0, "mix": 0.5})
	assertEffectModifiesSignal(t, rm, 440, 44100, 44100)
}

func TestRingModMixZero(t *testing.T) {
	rm := newRingMod(44100, map[string]float64{"frequency": 440, "shape": 0, "mix": 1})
	assertDryWhenMixZero(t, rm, 440, 44100, 44100)
}

func TestRingModReset(t *testing.T) {
	rm := newRingMod(44100, map[string]float64{"frequency": 440, "shape": 0, "mix": 1})
	assertResetClearsState(t, rm)
}

func TestRingModNoNaN(t *testing.T) {
	rm := newRingMod(44100, map[string]float64{"frequency": 440, "shape": 0, "mix": 1})
	out := make([]float64, 1000)
	for i := range out {
		out[i] = rm.ProcessSample(0)
	}
	assertNoNaNOrInf(t, out)
}

func TestRingModSumDifference(t *testing.T) {
	// Ring modulation of two sine waves at f1 and f2 produces
	// energy at f1-f2 (difference) and f1+f2 (sum).
	const sr = 44100
	const n = 44100 // 1 second for good frequency resolution
	const inputFreq = 440.0
	const carrierFreq = 100.0

	rm := newRingMod(sr, map[string]float64{"frequency": carrierFreq, "shape": 0, "mix": 1})
	input := sineSamples(inputFreq, sr, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = rm.ProcessSample(x)
	}

	// Expect energy at 340 Hz (440-100) and 540 Hz (440+100)
	magDiff := goertzelMagnitude(out, inputFreq-carrierFreq, sr)
	magSum := goertzelMagnitude(out, inputFreq+carrierFreq, sr)

	if magDiff < 0.1 {
		t.Errorf("expected difference frequency (%.0f Hz) magnitude > 0.1, got %v",
			inputFreq-carrierFreq, magDiff)
	}
	if magSum < 0.1 {
		t.Errorf("expected sum frequency (%.0f Hz) magnitude > 0.1, got %v",
			inputFreq+carrierFreq, magSum)
	}
}

func TestRingModSquareWave(t *testing.T) {
	const sr = 44100
	const n = 4410

	// Process the same input with sine carrier and square carrier
	sineMod := newRingMod(sr, map[string]float64{"frequency": 200, "shape": 0, "mix": 1})
	sqMod := newRingMod(sr, map[string]float64{"frequency": 200, "shape": 1, "mix": 1})

	input := sineSamples(440, sr, n)
	sineOut := make([]float64, n)
	sqOut := make([]float64, n)
	for i, x := range input {
		sineOut[i] = sineMod.ProcessSample(x)
		sqOut[i] = sqMod.ProcessSample(x)
	}

	// Outputs should differ
	var diffEnergy float64
	for i := range sineOut {
		d := sineOut[i] - sqOut[i]
		diffEnergy += d * d
	}
	diffRMS := math.Sqrt(diffEnergy / float64(n))
	if diffRMS < 0.01 {
		t.Errorf("square wave carrier should differ from sine carrier, diffRMS=%v", diffRMS)
	}
}
