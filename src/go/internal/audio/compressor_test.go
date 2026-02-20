package audio

import (
	"math"
	"testing"
)

func TestCompressorBelowThreshold(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -6
	c.Ratio = 4
	c.MakeupDB = 0
	c.KneeDB = 0 // hard knee for deterministic test
	c.recalc()

	// Signal well below threshold (-20 dB ≈ 0.1 amplitude) should pass through
	// with unity gain (no compression).
	input := 0.1
	// Let the envelope settle.
	var out float64
	for i := 0; i < 1000; i++ {
		out = c.ProcessSample(input)
	}
	// Should be very close to input (within 1% tolerance for envelope settling).
	if math.Abs(out-input) > 0.01 {
		t.Errorf("below threshold: expected ~%.4f, got %.4f", input, out)
	}
}

func TestCompressorAboveThreshold(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -6
	c.Ratio = 4
	c.MakeupDB = 0
	c.KneeDB = 0 // hard knee
	c.recalc()

	// Signal at 0 dB (amplitude 1.0) is 6 dB above -6 threshold.
	// Gain reduction should be 6 * (1 - 1/4) = 4.5 dB.
	// Expected output ≈ 1.0 * 10^(-4.5/20) ≈ 0.596
	input := 1.0
	var out float64
	// Let envelope fully settle with many samples.
	for i := 0; i < 10000; i++ {
		out = c.ProcessSample(input)
	}
	// Expected: ~0.596, allow ±0.05 tolerance for envelope dynamics.
	if out < 0.5 || out > 0.7 {
		t.Errorf("above threshold: expected ~0.596, got %.4f", out)
	}
	// Must be less than input (compression occurred).
	if out >= input {
		t.Errorf("compressor did not reduce gain: input=%.4f out=%.4f", input, out)
	}
}

func TestCompressorAttackRelease(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -12
	c.Ratio = 8
	c.AttackMs = 1
	c.ReleaseMs = 50
	c.MakeupDB = 0
	c.KneeDB = 0
	c.recalc()

	// Feed silence to reset envelope.
	for i := 0; i < 5000; i++ {
		c.ProcessSample(0)
	}

	// Sudden loud signal: first sample should pass nearly uncompressed
	// (envelope hasn't caught up yet).
	first := c.ProcessSample(1.0)
	if first < 0.8 {
		t.Errorf("attack: first loud sample should be nearly uncompressed, got %.4f", first)
	}

	// After attack settles (~5ms = 220 samples), output should be compressed.
	var afterAttack float64
	for i := 0; i < 500; i++ {
		afterAttack = c.ProcessSample(1.0)
	}
	if afterAttack > 0.6 {
		t.Errorf("attack: after settling, expected compression, got %.4f", afterAttack)
	}

	// Signal drops to silence: envelope should release.
	var afterRelease float64
	for i := 0; i < 5000; i++ {
		afterRelease = c.ProcessSample(0.01)
	}
	// After release, gain should return close to unity for the quiet signal.
	if afterRelease < 0.008 || afterRelease > 0.012 {
		t.Errorf("release: expected ~0.01, got %.4f", afterRelease)
	}
}

func TestCompressorSoftKnee(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -6
	c.Ratio = 4
	c.MakeupDB = 0
	c.KneeDB = 6 // wide soft knee
	c.recalc()

	// Signal at threshold should have partial compression (soft knee).
	// With hard knee it would be exactly 0 dB reduction at threshold.
	// With soft knee, compression starts below threshold.
	input := math.Pow(10, -6.0/20) // exactly at threshold
	for i := 0; i < 10000; i++ {
		c.ProcessSample(input)
	}
	out := c.ProcessSample(input)
	// Should have some compression (output < input) due to soft knee.
	if out >= input {
		t.Errorf("soft knee: expected some compression at threshold, got %.4f >= %.4f", out, input)
	}
}

func TestCompressorMakeupGain(t *testing.T) {
	c := NewCompressor(44100)
	c.ThresholdDB = -6
	c.Ratio = 4
	c.MakeupDB = 6 // +6dB makeup
	c.KneeDB = 0
	c.recalc()

	// Signal below threshold should be boosted by makeup gain.
	input := 0.1 // well below threshold
	for i := 0; i < 1000; i++ {
		c.ProcessSample(input)
	}
	out := c.ProcessSample(input)
	expected := input * math.Pow(10, 6.0/20) // ~0.1 * 2.0 = 0.2
	if math.Abs(out-expected) > 0.02 {
		t.Errorf("makeup gain: expected ~%.4f, got %.4f", expected, out)
	}
}
