package audio

import (
	"math"
	"testing"
)

func TestReverbImpulseTail(t *testing.T) {
	r := newReverb(44100, map[string]float64{"room": 0.5, "damping": 0.5, "mix": 1})
	// Send impulse.
	r.ProcessSample(1.0)
	// After many samples, reverb tail should still be producing output.
	hasOutput := false
	for i := 0; i < 10000; i++ {
		out := r.ProcessSample(0)
		if i > 100 && math.Abs(out) > 0.001 {
			hasOutput = true
		}
	}
	if !hasOutput {
		t.Error("reverb produced no tail after impulse")
	}
}

func TestReverbRoomSizeAffectsDecay(t *testing.T) {
	// Larger room should produce a longer tail.
	rSmall := newReverb(44100, map[string]float64{"room": 0.1, "damping": 0.5, "mix": 1})
	rLarge := newReverb(44100, map[string]float64{"room": 0.9, "damping": 0.5, "mix": 1})

	rSmall.ProcessSample(1.0)
	rLarge.ProcessSample(1.0)

	// Measure energy at ~200ms (8820 samples at 44100).
	var energySmall, energyLarge float64
	for i := 0; i < 8820; i++ {
		s := rSmall.ProcessSample(0)
		l := rLarge.ProcessSample(0)
		energySmall += s * s
		energyLarge += l * l
	}
	// Large room should have more residual energy.
	if energyLarge <= energySmall {
		t.Errorf("larger room should have more energy: small=%.6f large=%.6f", energySmall, energyLarge)
	}
}

func TestReverbMix(t *testing.T) {
	r := newReverb(44100, map[string]float64{"room": 0.5, "damping": 0.5, "mix": 0})
	// Mix=0 → fully dry.
	for i := 0; i < 500; i++ {
		r.ProcessSample(0.4)
	}
	out := r.ProcessSample(0.4)
	if math.Abs(out-0.4) > 0.01 {
		t.Errorf("mix=0: expected dry signal 0.4, got %.4f", out)
	}
}

func TestReverbReset(t *testing.T) {
	r := newReverb(44100, DefaultParams(EffectReverb))
	// Build up reverb tail.
	for i := 0; i < 5000; i++ {
		r.ProcessSample(0.8)
	}
	r.Reset()
	// After reset, no residual should remain.
	out := r.ProcessSample(0)
	if math.Abs(out) > 0.001 {
		t.Errorf("after reset: expected silence, got %.4f", out)
	}
}

func TestReverbSetParamRoomFeedback(t *testing.T) {
	rSmall := newReverb(44100, map[string]float64{"room": 0.1, "damping": 0.5, "mix": 1})
	rSmall.ProcessSample(1.0)
	for i := 0; i < 5000; i++ {
		rSmall.ProcessSample(0)
	}
	smallOut := math.Abs(rSmall.ProcessSample(0))

	// Now change room to large and re-test.
	rSmall.SetParam("room", 0.9)
	rSmall.Reset()
	rSmall.ProcessSample(1.0)
	for i := 0; i < 5000; i++ {
		rSmall.ProcessSample(0)
	}
	largeOut := math.Abs(rSmall.ProcessSample(0))

	// Larger room → more residual energy at this point.
	if largeOut <= smallOut {
		t.Errorf("expected larger room to produce more tail: small=%.6f large=%.6f", smallOut, largeOut)
	}
}

func TestReverbSetParamDamping(t *testing.T) {
	// Measure total energy over a window to compare damping effect.
	r := newReverb(44100, map[string]float64{"room": 0.8, "damping": 0, "mix": 1})
	r.ProcessSample(1.0)
	var energyNoDamp float64
	for i := 0; i < 3000; i++ {
		out := r.ProcessSample(0)
		energyNoDamp += out * out
	}

	// Set high damping and re-test.
	r.SetParam("damping", 0.9)
	r.Reset()
	r.ProcessSample(1.0)
	var energyDamp float64
	for i := 0; i < 3000; i++ {
		out := r.ProcessSample(0)
		energyDamp += out * out
	}

	// High damping should reduce total tail energy (LP in feedback darkens repeats).
	if energyDamp >= energyNoDamp {
		t.Errorf("expected damping to reduce tail energy: noDamp=%.6f damp=%.6f", energyNoDamp, energyDamp)
	}
}
