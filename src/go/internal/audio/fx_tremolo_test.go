package audio

import (
	"math"
	"testing"
)

func TestTremoloModifiesSignal(t *testing.T) {
	tr := newTremolo(44100, map[string]float64{
		"rate": 4, "depth": 0.5, "shape": 0, "mix": 1,
	})
	assertEffectModifiesSignal(t, tr, 1000, 44100, 44100)
}

func TestTremoloMixZero(t *testing.T) {
	tr := newTremolo(44100, map[string]float64{
		"rate": 4, "depth": 0.5, "shape": 0, "mix": 1,
	})
	assertDryWhenMixZero(t, tr, 1000, 44100, 44100)
}

func TestTremoloReset(t *testing.T) {
	tr := newTremolo(44100, DefaultParams(EffectTremolo))
	assertResetClearsState(t, tr)
}

func TestTremoloNoNaNOnSilence(t *testing.T) {
	tr := newTremolo(44100, map[string]float64{
		"rate": 20, "depth": 1, "shape": 2, "mix": 1,
	})
	out := make([]float64, 1000)
	for i := range out {
		out[i] = tr.ProcessSample(0)
	}
	assertNoNaNOrInf(t, out)
}

func TestTremoloSetParam(t *testing.T) {
	tr := newTremolo(44100, map[string]float64{
		"rate": 4, "depth": 0.2, "shape": 0, "mix": 1,
	})
	// Process with low depth
	out1 := processFX(tr, 1000, 44100, 44100)
	rms1 := rmsEnergy(out1)

	// Reset and increase depth significantly
	tr.Reset()
	tr.SetParam("depth", 1.0)
	out2 := processFX(tr, 1000, 44100, 44100)
	rms2 := rmsEnergy(out2)

	// Higher depth should reduce RMS more
	if math.Abs(rms1-rms2) < 1e-6 {
		t.Fatalf("changing depth should alter output: rms1=%v rms2=%v", rms1, rms2)
	}
}

func TestTremoloDepthZeroPassthrough(t *testing.T) {
	// depth=0 means modGain = 1.0 - 0*lfo = 1.0, so output should equal input.
	tr := newTremolo(44100, map[string]float64{
		"rate": 4, "depth": 0, "shape": 0, "mix": 1,
	})
	input := sineSamples(1000, 44100, 44100)
	for _, x := range input {
		out := tr.ProcessSample(x)
		if math.Abs(out-x) > 1e-6 {
			t.Fatalf("depth=0 should be passthrough, got %v want %v", out, x)
		}
	}
}

func TestTremoloDepthOneReducesRMS(t *testing.T) {
	// depth=1 with mix=1 should significantly reduce RMS compared to depth=0.
	tr0 := newTremolo(44100, map[string]float64{
		"rate": 4, "depth": 0, "shape": 0, "mix": 1,
	})
	out0 := processFX(tr0, 1000, 44100, 44100)
	rms0 := rmsEnergy(out0)

	tr1 := newTremolo(44100, map[string]float64{
		"rate": 4, "depth": 1, "shape": 0, "mix": 1,
	})
	out1 := processFX(tr1, 1000, 44100, 44100)
	rms1 := rmsEnergy(out1)

	// depth=1 should reduce RMS by at least 10%
	if rms1 >= rms0*0.9 {
		t.Fatalf("depth=1 should reduce RMS significantly: rms0=%v rms1=%v", rms0, rms1)
	}
}

func TestTremoloShapes(t *testing.T) {
	// All three shapes should produce different output patterns.
	shapes := []int{0, 1, 2}
	rmsValues := make([]float64, 3)

	for _, shape := range shapes {
		tr := newTremolo(44100, map[string]float64{
			"rate": 4, "depth": 1, "shape": float64(shape), "mix": 1,
		})
		out := processFX(tr, 1000, 44100, 44100)
		rmsValues[shape] = rmsEnergy(out)
	}

	// At least one pair should differ (sine vs triangle vs square have different
	// modulation envelopes).
	allSame := math.Abs(rmsValues[0]-rmsValues[1]) < 1e-6 &&
		math.Abs(rmsValues[1]-rmsValues[2]) < 1e-6
	if allSame {
		t.Fatalf("all shapes produced identical RMS: sine=%v tri=%v square=%v",
			rmsValues[0], rmsValues[1], rmsValues[2])
	}
}

func TestTremoloZeroSR(t *testing.T) {
	tr := newTremolo(0, map[string]float64{
		"rate": 4, "depth": 0.5, "shape": 0, "mix": 1,
	})
	out := tr.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with sr=0, got %f", out)
	}
}
