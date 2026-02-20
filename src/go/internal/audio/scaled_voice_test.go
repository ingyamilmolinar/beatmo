//go:build !test

package audio

import (
	"math"
	"testing"
)

// testBlockVoice is a BlockVoice that produces a constant value.
type testBlockVoice struct {
	val       float64
	remaining int
}

func (v *testBlockVoice) Sample() (float64, bool) {
	if v.remaining <= 0 {
		return 0, true
	}
	v.remaining--
	return v.val, false
}

func (v *testBlockVoice) SampleBlock(dst []float64) (int, bool) {
	n := len(dst)
	if n > v.remaining {
		n = v.remaining
	}
	for i := 0; i < n; i++ {
		dst[i] = v.val
	}
	v.remaining -= n
	return n, v.remaining <= 0
}

// testSampleOnlyVoice is a Voice that does NOT implement BlockVoice.
type testSampleOnlyVoice struct {
	val       float64
	remaining int
}

func (v *testSampleOnlyVoice) Sample() (float64, bool) {
	if v.remaining <= 0 {
		return 0, true
	}
	v.remaining--
	return v.val, false
}

func TestScaledVoiceSampleBlockDelegatesToBlockVoice(t *testing.T) {
	inner := &testBlockVoice{val: 0.8, remaining: 64}
	sv := &scaledVoice{v: inner, gain: 0.5}

	dst := make([]float64, 64)
	n, done := sv.SampleBlock(dst)

	if n != 64 {
		t.Fatalf("SampleBlock returned n=%d, want 64", n)
	}
	if !done {
		t.Error("SampleBlock should report done after all samples consumed")
	}
	expected := 0.8 * 0.5
	for i := 0; i < n; i++ {
		if math.Abs(dst[i]-expected) > 1e-9 {
			t.Errorf("dst[%d] = %f, want %f", i, dst[i], expected)
			break
		}
	}
}

func TestScaledVoiceSampleBlockFallsBackForNonBlockVoice(t *testing.T) {
	inner := &testSampleOnlyVoice{val: 0.6, remaining: 32}
	sv := &scaledVoice{v: inner, gain: 0.25}

	dst := make([]float64, 32)
	n, done := sv.SampleBlock(dst)

	if n != 32 {
		t.Fatalf("SampleBlock returned n=%d, want 32", n)
	}
	if !done {
		t.Error("SampleBlock should report done after all samples consumed")
	}
	expected := 0.6 * 0.25
	for i := 0; i < n; i++ {
		if math.Abs(dst[i]-expected) > 1e-9 {
			t.Errorf("dst[%d] = %f, want %f", i, dst[i], expected)
			break
		}
	}
}

func TestScaledVoiceSampleBlockPartialDone(t *testing.T) {
	// Inner has fewer samples than dst — should return early with done=true.
	inner := &testBlockVoice{val: 1.0, remaining: 10}
	sv := &scaledVoice{v: inner, gain: 2.0}

	dst := make([]float64, 64)
	n, done := sv.SampleBlock(dst)

	if n != 10 {
		t.Fatalf("SampleBlock returned n=%d, want 10", n)
	}
	if !done {
		t.Error("SampleBlock should report done when inner voice exhausted")
	}
	for i := 0; i < n; i++ {
		if math.Abs(dst[i]-2.0) > 1e-9 {
			t.Errorf("dst[%d] = %f, want 2.0", i, dst[i])
			break
		}
	}
}

func TestScaledVoiceSampleBlockFallbackPartialDone(t *testing.T) {
	// Non-BlockVoice inner with fewer samples than dst.
	inner := &testSampleOnlyVoice{val: 1.0, remaining: 5}
	sv := &scaledVoice{v: inner, gain: 3.0}

	dst := make([]float64, 64)
	n, done := sv.SampleBlock(dst)

	if n != 5 {
		t.Fatalf("SampleBlock returned n=%d, want 5", n)
	}
	if !done {
		t.Error("SampleBlock should report done when inner voice exhausted")
	}
	for i := 0; i < n; i++ {
		if math.Abs(dst[i]-3.0) > 1e-9 {
			t.Errorf("dst[%d] = %f, want 3.0", i, dst[i])
			break
		}
	}
}
