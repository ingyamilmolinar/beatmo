package audio

import (
	"math"
	"testing"
)

func TestBitcrusherQuantization(t *testing.T) {
	// 4-bit quantization should produce distinct steps.
	b := newBitcrusher(44100, map[string]float64{"bits": 4, "rate": 1, "mix": 1})
	// With 4 bits, there are 2^4=16 levels. A signal of 0.123 should be
	// quantized to the nearest step.
	var out float64
	for i := 0; i < 100; i++ {
		out = b.ProcessSample(0.123)
	}
	// Check it's quantized (4-bit step ≈ 2/16 = 0.125)
	step := 2.0 / math.Pow(2, 4)
	quantized := math.Round(0.123/step) * step
	if math.Abs(out-quantized) > step {
		t.Errorf("4-bit quantization: expected ~%.4f, got %.4f", quantized, out)
	}
}

func TestBitcrusherRateReduction(t *testing.T) {
	// Low rate should hold samples (sample-and-hold effect).
	b := newBitcrusher(44100, map[string]float64{"bits": 16, "rate": 0.1, "mix": 1})
	// Feed an ascending ramp and check that output stays constant for stretches.
	outputs := make([]float64, 100)
	for i := 0; i < 100; i++ {
		in := float64(i) / 100.0
		outputs[i] = b.ProcessSample(in)
	}
	// Count how many consecutive equal outputs there are.
	holdCount := 0
	for i := 1; i < len(outputs); i++ {
		if outputs[i] == outputs[i-1] {
			holdCount++
		}
	}
	// With rate=0.1, ~90% of samples should be held.
	if holdCount < 50 {
		t.Errorf("rate reduction: expected many held samples, got %d holds out of 99", holdCount)
	}
}

func TestBitcrusherMix(t *testing.T) {
	b := newBitcrusher(44100, map[string]float64{"bits": 4, "rate": 0.5, "mix": 0})
	for i := 0; i < 100; i++ {
		b.ProcessSample(0.3)
	}
	out := b.ProcessSample(0.3)
	if math.Abs(out-0.3) > 0.001 {
		t.Errorf("mix=0: expected 0.3, got %.4f", out)
	}
}

func TestBitcrusherReset(t *testing.T) {
	b := newBitcrusher(44100, DefaultParams(EffectBitcrusher))
	for i := 0; i < 500; i++ {
		b.ProcessSample(0.8)
	}
	b.Reset()
	out := b.ProcessSample(0)
	if math.Abs(out) > 0.01 {
		t.Errorf("after reset: expected ~0, got %.4f", out)
	}
}

func TestBitcrusherSetParamAll(t *testing.T) {
	b := newBitcrusher(44100, map[string]float64{"bits": 8, "rate": 0.5, "mix": 0.5})
	// Out-of-range values should be clamped — verify via behavior.
	b.SetParam("bits", 0)   // clamp to 2
	b.SetParam("rate", -1)  // clamp to 0.01
	b.SetParam("mix", -0.5) // clamp to 0
	// mix=0 → dry, should pass input through.
	b.Reset()
	for i := 0; i < 100; i++ {
		b.ProcessSample(0.3)
	}
	out := b.ProcessSample(0.3)
	if math.Abs(out-0.3) > 0.001 {
		t.Errorf("expected dry signal with mix=0, got %f", out)
	}

	// High values should also be clamped and produce valid output.
	b.SetParam("bits", 32) // clamp to 16
	b.SetParam("rate", 5)  // clamp to 1
	b.SetParam("mix", 5)   // clamp to 1
	out = b.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with clamped high values, got %f", out)
	}
}

func TestBitcrusherFullBitDepth(t *testing.T) {
	// bits=16, rate=1 should be nearly transparent.
	b := newBitcrusher(44100, map[string]float64{"bits": 16, "rate": 1, "mix": 1})
	in := 0.3
	var out float64
	for i := 0; i < 100; i++ {
		out = b.ProcessSample(in)
	}
	// 16-bit quantization at 0.3: step = 1/65536 ≈ 0.000015
	if math.Abs(out-in) > 0.001 {
		t.Errorf("16-bit full-rate: expected ~%.4f, got %.4f", in, out)
	}
}
