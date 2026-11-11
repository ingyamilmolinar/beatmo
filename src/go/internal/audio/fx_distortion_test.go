package audio

import (
	"math"
	"testing"
)

func TestDistortionPassThrough(t *testing.T) {
	// Drive=1 with mix=1 should produce mild soft-clipping (tanh(x) ≈ x for small x).
	d := newDistortion(44100, map[string]float64{"drive": 1, "tone": 8000, "mix": 1})
	// Small signal should pass nearly unchanged.
	in := 0.1
	var out float64
	for i := 0; i < 200; i++ {
		out = d.ProcessSample(in)
	}
	if math.Abs(out-in) > 0.02 {
		t.Errorf("low-drive passthrough: expected ~%.4f, got %.4f", in, out)
	}
}

func TestDistortionHighDrive(t *testing.T) {
	d := newDistortion(44100, map[string]float64{"drive": 20, "tone": 8000, "mix": 1})
	// High drive should saturate signal toward ±1.
	in := 0.5
	var out float64
	for i := 0; i < 200; i++ {
		out = d.ProcessSample(in)
	}
	// tanh(0.5*20) ≈ 1.0 (fully saturated)
	if out < 0.7 {
		t.Errorf("high-drive saturation: expected high output, got %.4f", out)
	}
}

func TestDistortionMix(t *testing.T) {
	d := newDistortion(44100, map[string]float64{"drive": 10, "tone": 8000, "mix": 0})
	// Mix=0 means fully dry — output should equal input.
	in := 0.3
	var out float64
	for i := 0; i < 200; i++ {
		out = d.ProcessSample(in)
	}
	if math.Abs(out-in) > 0.001 {
		t.Errorf("mix=0: expected dry signal %.4f, got %.4f", in, out)
	}
}

func TestDistortionReset(t *testing.T) {
	d := newDistortion(44100, DefaultParams(EffectDistortion))
	// Process some signal.
	for i := 0; i < 500; i++ {
		d.ProcessSample(0.5)
	}
	d.Reset()
	// After reset, internal filter state should be cleared.
	// Process a zero signal — tone filter should not produce residual.
	out := d.ProcessSample(0)
	if math.Abs(out) > 0.001 {
		t.Errorf("after reset: expected ~0, got %.4f", out)
	}
}

func TestDistortionSetParam(t *testing.T) {
	d := newDistortion(44100, map[string]float64{"drive": 1, "tone": 4000, "mix": 1})
	for i := 0; i < 200; i++ {
		d.ProcessSample(0.3)
	}
	// Increase drive
	d.SetParam("drive", 15)
	var out float64
	for i := 0; i < 200; i++ {
		out = d.ProcessSample(0.3)
	}
	// Higher drive should produce more saturation (higher output for moderate input)
	if out < 0.5 {
		t.Errorf("after drive increase: expected higher saturation, got %.4f", out)
	}
}

func TestDistortionSetParamTone(t *testing.T) {
	d := newDistortion(44100, map[string]float64{"drive": 5, "tone": 8000, "mix": 1})

	// Feed a varying signal (alternating +/- to create frequency content)
	// and measure output energy with the initial tone.
	var energyBefore float64
	for i := 0; i < 500; i++ {
		// Alternating signal has high-frequency content that the tone LP filter affects
		in := 0.5
		if i%2 == 1 {
			in = -0.5
		}
		out := d.ProcessSample(in)
		energyBefore += out * out
	}

	// Change tone to very low value and measure again.
	d.SetParam("tone", 200)
	d.Reset()
	var energyAfter float64
	for i := 0; i < 500; i++ {
		in := 0.5
		if i%2 == 1 {
			in = -0.5
		}
		out := d.ProcessSample(in)
		energyAfter += out * out
	}

	// Lower tone should attenuate the high-frequency alternating signal more.
	if energyAfter >= energyBefore {
		t.Errorf("expected lower tone to reduce energy: before=%.4f after=%.4f", energyBefore, energyAfter)
	}
}

func TestDistortionClampBoundaries(t *testing.T) {
	d := newDistortion(44100, map[string]float64{"drive": 1, "tone": 4000, "mix": 0.5})

	// Extreme drive values should still produce valid output.
	d.SetParam("drive", -5)
	out := d.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with clamped low drive, got %f", out)
	}

	d.SetParam("drive", 100)
	out = d.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with clamped high drive, got %f", out)
	}

	// Extreme tone values.
	d.SetParam("tone", 0)
	out = d.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with clamped low tone, got %f", out)
	}

	// Extreme mix values.
	d.SetParam("mix", 2.0)
	d.SetParam("mix", -1.0)
	out = d.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with clamped mix, got %f", out)
	}
}

func TestDistortionZeroSR(t *testing.T) {
	// sr=0 should fallback to 44100 internally.
	d := newDistortion(0, map[string]float64{"drive": 5, "tone": 4000, "mix": 1})
	// Should not panic and should produce valid output.
	out := d.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with sr=0, got %f", out)
	}
}
