package audio

import (
	"math"
	"testing"
)

func TestFlangerModifiesSignal(t *testing.T) {
	f := newFlanger(44100, map[string]float64{
		"rate": 0.5, "depth": 3, "feedback": 0.5, "mix": 0.5,
	})
	assertEffectModifiesSignal(t, f, 1000, 44100, 44100)
}

func TestFlangerMixZero(t *testing.T) {
	f := newFlanger(44100, map[string]float64{
		"rate": 0.5, "depth": 3, "feedback": 0.5, "mix": 0.5,
	})
	assertDryWhenMixZero(t, f, 1000, 44100, 44100)
}

func TestFlangerReset(t *testing.T) {
	f := newFlanger(44100, DefaultParams(EffectFlanger))
	assertResetClearsState(t, f)
}

func TestFlangerNoNaNOnSilence(t *testing.T) {
	f := newFlanger(44100, map[string]float64{
		"rate": 5, "depth": 10, "feedback": 0.95, "mix": 1,
	})
	out := make([]float64, 1000)
	for i := range out {
		out[i] = f.ProcessSample(0)
	}
	assertNoNaNOrInf(t, out)
}

func TestFlangerSetParam(t *testing.T) {
	f := newFlanger(44100, map[string]float64{
		"rate": 0.5, "depth": 3, "feedback": 0.5, "mix": 1,
	})
	// Process with default params
	out1 := processFX(f, 1000, 44100, 44100)
	rms1 := rmsEnergy(out1)

	// Reset and change depth
	f.Reset()
	f.SetParam("depth", 10)
	out2 := processFX(f, 1000, 44100, 44100)
	rms2 := rmsEnergy(out2)

	// Different depth should produce different output
	if math.Abs(rms1-rms2) < 1e-6 {
		t.Fatalf("changing depth should alter output: rms1=%v rms2=%v", rms1, rms2)
	}
}

func TestFlangerFeedbackPolarity(t *testing.T) {
	// Positive vs negative feedback produce different comb filter characteristics.
	fPos := newFlanger(44100, map[string]float64{
		"rate": 0.5, "depth": 3, "feedback": 0.9, "mix": 1,
	})
	outPos := processFX(fPos, 1000, 44100, 44100)
	rmsPos := rmsEnergy(outPos)

	fNeg := newFlanger(44100, map[string]float64{
		"rate": 0.5, "depth": 3, "feedback": -0.9, "mix": 1,
	})
	outNeg := processFX(fNeg, 1000, 44100, 44100)
	rmsNeg := rmsEnergy(outNeg)

	if math.Abs(rmsPos-rmsNeg) < 1e-6 {
		t.Fatalf("positive vs negative feedback should produce different output: rmsPos=%v rmsNeg=%v", rmsPos, rmsNeg)
	}
}

func TestFlangerZeroSR(t *testing.T) {
	f := newFlanger(0, map[string]float64{
		"rate": 0.5, "depth": 3, "feedback": 0.5, "mix": 0.5,
	})
	out := f.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with sr=0, got %f", out)
	}
}
