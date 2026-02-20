package audio

import (
	"math"
	"math/rand"
	"testing"
)

func TestCompressorSilenceInput(t *testing.T) {
	c := NewCompressor(44100)
	c.KneeDB = 0
	c.recalc()

	for i := 0; i < 1000; i++ {
		out := c.ProcessSample(0)
		if out != 0 {
			t.Fatalf("silence input produced non-zero output at sample %d: %f", i, out)
		}
	}
}

func TestCompressorExtremeRatio(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -6
	c.Ratio = 100 // near-limiter
	c.MakeupDB = 0
	c.KneeDB = 0
	c.recalc()

	// Settle with loud signal.
	var out float64
	for i := 0; i < 10000; i++ {
		out = c.ProcessSample(1.0)
	}
	// With ratio=100, signal at 0dB (6dB above -6 threshold):
	// reduction = 6 * (1 - 1/100) ≈ 5.94 dB
	// output ≈ 10^(-5.94/20) ≈ 0.505
	threshold := math.Pow(10, -6.0/20) // ≈ 0.501
	if out > threshold*1.3 {
		t.Errorf("extreme ratio: output %f should be near threshold %f", out, threshold)
	}
}

func TestCompressorRatioOne(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -20
	c.Ratio = 1.0 // no compression
	c.MakeupDB = 0
	c.KneeDB = 0
	c.recalc()

	input := 0.8
	var out float64
	for i := 0; i < 5000; i++ {
		out = c.ProcessSample(input)
	}
	if math.Abs(out-input) > 0.02 {
		t.Errorf("ratio=1: expected ≈ %f, got %f", input, out)
	}
}

func TestCompressorVeryLoudInput(t *testing.T) {
	c := NewCompressor(44100)
	c.KneeDB = 0
	c.recalc()

	for i := 0; i < 1000; i++ {
		out := c.ProcessSample(10.0)
		if math.IsNaN(out) || math.IsInf(out, 0) {
			t.Fatalf("loud input produced NaN/Inf at sample %d", i)
		}
	}
}

func TestCompressorQuietLoudQuietTransitions(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -12
	c.Ratio = 4
	c.MakeupDB = 0
	c.KneeDB = 0
	c.recalc()

	// Feed quiet signal.
	var quietOut float64
	for i := 0; i < 2000; i++ {
		quietOut = c.ProcessSample(0.05)
	}

	// Feed loud burst.
	var loudOut float64
	for i := 0; i < 2000; i++ {
		loudOut = c.ProcessSample(1.0)
	}

	// Feed quiet again.
	var recoveredOut float64
	for i := 0; i < 5000; i++ {
		recoveredOut = c.ProcessSample(0.05)
	}

	// Quiet should be similar before and after.
	if math.Abs(quietOut-recoveredOut) > 0.02 {
		t.Errorf("transition: quiet before=%f, after=%f", quietOut, recoveredOut)
	}
	// Loud should be compressed.
	if loudOut >= 1.0 {
		t.Errorf("loud signal not compressed: %f", loudOut)
	}
}

func TestCompressorNoNaNOrInf(t *testing.T) {
	c := NewCompressor(44100)
	c.recalc()

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		input := (rng.Float64()*2 - 1) * 5 // -5 to +5
		out := c.ProcessSample(input)
		if math.IsNaN(out) || math.IsInf(out, 0) {
			t.Fatalf("NaN/Inf at sample %d (input=%f)", i, input)
		}
	}
}

func TestCompressorHardKneeVsSoftKnee(t *testing.T) {
	// At exactly threshold: hard knee → no compression, soft knee → partial.
	sr := 44100
	thresh := -6.0
	input := math.Pow(10, thresh/20) // exactly at threshold

	// Hard knee.
	hard := NewCompressor(sr)
	hard.ThresholdDB = thresh
	hard.Ratio = 4
	hard.MakeupDB = 0
	hard.KneeDB = 0
	hard.recalc()
	var hardOut float64
	for i := 0; i < 5000; i++ {
		hardOut = hard.ProcessSample(input)
	}

	// Soft knee.
	soft := NewCompressor(sr)
	soft.ThresholdDB = thresh
	soft.Ratio = 4
	soft.MakeupDB = 0
	soft.KneeDB = 6
	soft.recalc()
	var softOut float64
	for i := 0; i < 5000; i++ {
		softOut = soft.ProcessSample(input)
	}

	// Hard knee at threshold: no reduction.
	if math.Abs(hardOut-input) > 0.02 {
		t.Errorf("hard knee at threshold: output %f, expected ≈ %f", hardOut, input)
	}
	// Soft knee at threshold: some reduction.
	if softOut >= input {
		t.Errorf("soft knee at threshold: output %f should be < input %f", softOut, input)
	}
}

func TestCompressorZeroMakeupGain(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -6
	c.Ratio = 4
	c.MakeupDB = 0
	c.KneeDB = 0
	c.recalc()

	// Below threshold: unchanged.
	input := 0.1
	var out float64
	for i := 0; i < 1000; i++ {
		out = c.ProcessSample(input)
	}
	if math.Abs(out-input) > 0.01 {
		t.Errorf("below threshold: expected ≈ %f, got %f", input, out)
	}

	// Above threshold: reduced.
	c2 := NewCompressor(44100)
	c2.ThresholdDB = -6
	c2.Ratio = 4
	c2.MakeupDB = 0
	c2.KneeDB = 0
	c2.recalc()
	var out2 float64
	for i := 0; i < 10000; i++ {
		out2 = c2.ProcessSample(1.0)
	}
	if out2 >= 1.0 {
		t.Errorf("above threshold: expected output < 1.0, got %f", out2)
	}
}
