package audio

import (
	"math"
	"testing"
)

func TestChorusModulation(t *testing.T) {
	c := newChorus(44100, map[string]float64{"rate": 2, "depth": 10, "mix": 1})
	// Feed a constant signal and verify output varies (LFO modulation).
	var min, max float64
	min = 2
	max = -2
	for i := 0; i < 44100; i++ { // 1 second
		out := c.ProcessSample(0.5)
		if out < min {
			min = out
		}
		if out > max {
			max = out
		}
	}
	// With modulation, the output should vary.
	variation := max - min
	if variation < 0.01 {
		t.Errorf("chorus should produce variation, got min=%.4f max=%.4f", min, max)
	}
}

func TestChorusDepthZero(t *testing.T) {
	c := newChorus(44100, map[string]float64{"rate": 2, "depth": 0, "mix": 1})
	// Zero depth → no modulation, output should be close to input after initial delay.
	for i := 0; i < 1000; i++ {
		c.ProcessSample(0.5)
	}
	out := c.ProcessSample(0.5)
	if math.Abs(out-0.5) > 0.1 {
		t.Errorf("depth=0: expected ~0.5, got %.4f", out)
	}
}

func TestChorusMix(t *testing.T) {
	c := newChorus(44100, map[string]float64{"rate": 2, "depth": 10, "mix": 0})
	// Mix=0 → dry signal only.
	for i := 0; i < 500; i++ {
		c.ProcessSample(0.4)
	}
	out := c.ProcessSample(0.4)
	if math.Abs(out-0.4) > 0.001 {
		t.Errorf("mix=0: expected 0.4, got %.4f", out)
	}
}

func TestChorusReset(t *testing.T) {
	c := newChorus(44100, DefaultParams(EffectChorus))
	for i := 0; i < 5000; i++ {
		c.ProcessSample(0.8)
	}
	c.Reset()
	out := c.ProcessSample(0)
	if math.Abs(out) > 0.01 {
		t.Errorf("after reset: expected ~0, got %.4f", out)
	}
}

func TestChorusSetParamRecalc(t *testing.T) {
	c := newChorus(44100, map[string]float64{"rate": 1, "depth": 5, "mix": 1})
	// Warm up.
	for i := 0; i < 1000; i++ {
		c.ProcessSample(0.5)
	}
	// Change rate and depth — should trigger recalc.
	c.SetParam("rate", 8)
	c.SetParam("depth", 20)
	// Process more samples — should not panic.
	for i := 0; i < 1000; i++ {
		out := c.ProcessSample(0.5)
		if math.IsNaN(out) || math.IsInf(out, 0) {
			t.Fatalf("invalid output after param recalc at sample %d: %f", i, out)
		}
	}
}

func TestChorusZeroSR(t *testing.T) {
	// sr=0 should fallback to 44100 in recalc.
	c := newChorus(0, map[string]float64{"rate": 2, "depth": 10, "mix": 1})
	out := c.ProcessSample(0.5)
	if math.IsNaN(out) || math.IsInf(out, 0) {
		t.Errorf("expected valid output with sr=0, got %f", out)
	}
}
