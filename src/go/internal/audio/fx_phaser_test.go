package audio

import (
	"math"
	"testing"
)

func TestPhaserModifiesSignal(t *testing.T) {
	p := newPhaser(44100, map[string]float64{
		"stages": 4, "rate": 0.5, "depth": 0.7, "feedback": 0.5, "mix": 0.5,
	})
	assertEffectModifiesSignal(t, p, 1000, 44100, 44100)
}

func TestPhaserMixZero(t *testing.T) {
	p := newPhaser(44100, map[string]float64{
		"stages": 4, "rate": 0.5, "depth": 0.7, "feedback": 0.5, "mix": 0.5,
	})
	assertDryWhenMixZero(t, p, 1000, 44100, 44100)
}

func TestPhaserReset(t *testing.T) {
	p := newPhaser(44100, DefaultParams(EffectPhaser))
	assertResetClearsState(t, p)
}

func TestPhaserNoNaNOnSilence(t *testing.T) {
	p := newPhaser(44100, map[string]float64{
		"stages": 12, "rate": 5, "depth": 1, "feedback": 0.95, "mix": 1,
	})
	out := make([]float64, 1000)
	for i := range out {
		out[i] = p.ProcessSample(0)
	}
	assertNoNaNOrInf(t, out)
}

func TestPhaserSetParam(t *testing.T) {
	p := newPhaser(44100, map[string]float64{
		"stages": 4, "rate": 0.5, "depth": 0.7, "feedback": 0.5, "mix": 1,
	})
	// Process some signal with default params
	out1 := processFX(p, 1000, 44100, 44100)
	rms1 := rmsEnergy(out1)

	// Reset and change feedback to 0
	p.Reset()
	p.SetParam("feedback", 0)
	out2 := processFX(p, 1000, 44100, 44100)
	rms2 := rmsEnergy(out2)

	// Different feedback should produce different output
	if math.Abs(rms1-rms2) < 1e-6 {
		t.Fatalf("changing feedback should alter output: rms1=%v rms2=%v", rms1, rms2)
	}
}

func TestPhaserStagesAffectOutput(t *testing.T) {
	// stages=2 vs stages=12 should produce different RMS due to different
	// numbers of notches in the frequency response.
	p2 := newPhaser(44100, map[string]float64{
		"stages": 2, "rate": 0.5, "depth": 0.7, "feedback": 0.5, "mix": 1,
	})
	out2 := processFX(p2, 1000, 44100, 44100)
	rms2 := rmsEnergy(out2)

	p12 := newPhaser(44100, map[string]float64{
		"stages": 12, "rate": 0.5, "depth": 0.7, "feedback": 0.5, "mix": 1,
	})
	out12 := processFX(p12, 1000, 44100, 44100)
	rms12 := rmsEnergy(out12)

	if math.Abs(rms2-rms12) < 1e-6 {
		t.Fatalf("stages=2 vs stages=12 should produce different output: rms2=%v rms12=%v", rms2, rms12)
	}
}

func TestPhaserZeroSR(t *testing.T) {
	p := newPhaser(0, map[string]float64{
		"stages": 4, "rate": 0.5, "depth": 0.7, "feedback": 0.5, "mix": 0.5,
	})
	out := p.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with sr=0, got %f", out)
	}
}
