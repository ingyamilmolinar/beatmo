//go:build !test

package audio

import (
	"math"
	"testing"
)

// mockVoice produces a constant value for testing.
type mockVoice struct {
	val    float64
	remain int
}

func (m *mockVoice) Sample() (float64, bool) {
	if m.remain <= 0 {
		return 0, true
	}
	m.remain--
	return m.val, false
}

func TestAntiPopFadeIn(t *testing.T) {
	sr := 44100
	v := newAntiPopVoice(&mockVoice{val: 1.0, remain: sr}, sr)

	// First sample should be near zero (fade-in start).
	s0, done := v.Sample()
	if done {
		t.Fatal("voice done on first sample")
	}
	if s0 > 0.01 {
		t.Errorf("first sample should be near zero during fade-in, got %f", s0)
	}

	// Sample at ~50% of fade-in should be ~0.5.
	fadeInSamples := sr * antiPopFadeInMs / 1000
	half := fadeInSamples / 2
	for i := 1; i < half; i++ {
		v.Sample()
	}
	sMid, _ := v.Sample()
	if sMid < 0.3 || sMid > 0.7 {
		t.Errorf("mid fade-in sample should be ~0.5, got %f (at pos %d of %d)", sMid, half, fadeInSamples)
	}

	// After fade-in completes, samples should be at full value.
	for i := half + 1; i < fadeInSamples+10; i++ {
		v.Sample()
	}
	sFull, _ := v.Sample()
	if math.Abs(sFull-1.0) > 0.01 {
		t.Errorf("post fade-in sample should be ~1.0, got %f", sFull)
	}
}

func TestAntiPopFadeOut(t *testing.T) {
	sr := 44100
	v := newAntiPopVoice(&mockVoice{val: 1.0, remain: sr}, sr)
	fadeInSamples := sr * antiPopFadeInMs / 1000
	fadeOutSamples := sr * antiPopFadeOutMs / 1000

	// Advance past fade-in.
	for i := 0; i < fadeInSamples+100; i++ {
		v.Sample()
	}

	// Request stop — should begin fade-out.
	v.RequestStop()
	if !v.IsStopping() {
		t.Fatal("expected IsStopping() to be true after RequestStop()")
	}

	// First fade-out sample should still be near 1.0 (quadratic: (1-0/N)^2 = 1).
	s0, done := v.Sample()
	if done {
		t.Fatal("voice done immediately after RequestStop")
	}
	if s0 < 0.9 {
		t.Errorf("first fade-out sample should be near 1.0, got %f", s0)
	}

	// Midpoint of fade-out: t=0.5, envelope = (1-0.5)^2 = 0.25.
	half := fadeOutSamples / 2
	for i := 1; i < half; i++ {
		v.Sample()
	}
	sMid, _ := v.Sample()
	if sMid < 0.1 || sMid > 0.4 {
		t.Errorf("mid fade-out sample should be ~0.25 (quadratic), got %f", sMid)
	}

	// After fade-out completes, should be done.
	for i := half + 1; i < fadeOutSamples+10; i++ {
		_, done = v.Sample()
		if done {
			return // Expected: voice is done.
		}
	}
	t.Error("voice should be done after fade-out completes")
}

func TestAntiPopNaturalCompletion(t *testing.T) {
	// Voice that naturally finishes before any stop request.
	sr := 44100
	shortLen := 100
	v := newAntiPopVoice(&mockVoice{val: 0.5, remain: shortLen}, sr)

	count := 0
	for {
		_, done := v.Sample()
		if done {
			break
		}
		count++
		if count > sr {
			t.Fatal("voice never completed")
		}
	}
	if count != shortLen {
		t.Errorf("expected %d samples, got %d", shortLen, count)
	}
}

func TestAntiPopFadeOutTiming(t *testing.T) {
	sr := 44100
	v := newAntiPopVoice(&mockVoice{val: 1.0, remain: sr * 10}, sr)
	fadeInSamples := sr * antiPopFadeInMs / 1000
	fadeOutSamples := sr * antiPopFadeOutMs / 1000

	// Advance past fade-in.
	for i := 0; i < fadeInSamples+10; i++ {
		v.Sample()
	}

	v.RequestStop()

	// Count samples until done.
	count := 0
	for {
		_, done := v.Sample()
		if done {
			break
		}
		count++
		if count > fadeOutSamples+10 {
			t.Fatalf("fade-out exceeded expected length: %d > %d", count, fadeOutSamples)
		}
	}
	// Should be exactly fadeOutSamples (last one returns done).
	if count != fadeOutSamples {
		t.Errorf("fade-out produced %d samples, expected %d", count, fadeOutSamples)
	}
}
