//go:build !test

package audio

import (
	"math"
	"testing"
)

// mockBlockVoice implements both Voice and BlockVoice interfaces.
type mockBlockVoice struct {
	val    float64
	remain int
}

func (m *mockBlockVoice) Sample() (float64, bool) {
	if m.remain <= 0 {
		return 0, true
	}
	m.remain--
	return m.val, false
}

func (m *mockBlockVoice) SampleBlock(dst []float64) (int, bool) {
	n := 0
	for i := range dst {
		if m.remain <= 0 {
			return n, true
		}
		dst[i] = m.val
		m.remain--
		n++
	}
	return n, false
}

func TestAntiPopVeryShortVoice(t *testing.T) {
	sr := 44100
	// Voice with remain=10, much shorter than fadeIn (220 samples).
	v := newAntiPopVoice(&mockVoice{val: 1.0, remain: 10}, sr)

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
	if count != 10 {
		t.Errorf("expected 10 samples, got %d", count)
	}
}

func TestAntiPopRepeatedRequestStop(t *testing.T) {
	sr := 44100
	fadeInSamples := sr * antiPopFadeInMs / 1000
	v := newAntiPopVoice(&mockVoice{val: 1.0, remain: sr}, sr)

	// Advance past fade-in.
	for i := 0; i < fadeInSamples+100; i++ {
		v.Sample()
	}

	v.RequestStop()
	initialFadeOutPos := v.fadeOutPos

	// Call RequestStop again — should not reset fadeOutPos.
	v.Sample() // advance fadeOutPos by 1
	v.RequestStop()

	if v.fadeOutPos <= initialFadeOutPos {
		t.Error("repeated RequestStop should not reset fadeOutPos")
	}
}

func TestAntiPopBlockVoiceDelegation(t *testing.T) {
	sr := 44100
	inner := &mockBlockVoice{val: 1.0, remain: sr}
	v := newAntiPopVoice(inner, sr)

	dst := make([]float64, 64)
	n, done := v.SampleBlock(dst)
	if done {
		t.Fatal("voice done too early")
	}
	if n != 64 {
		t.Errorf("SampleBlock returned n=%d, want 64", n)
	}

	// First sample should have fade-in envelope applied.
	if dst[0] > 0.01 {
		t.Errorf("first sample should be near zero (fade-in), got %f", dst[0])
	}
}

func TestAntiPopBlockVoiceFallback(t *testing.T) {
	sr := 44100
	// mockVoice does NOT implement BlockVoice, so SampleBlock should use fallback.
	inner := &mockVoice{val: 1.0, remain: sr}
	v := newAntiPopVoice(inner, sr)

	dst := make([]float64, 32)
	n, done := v.SampleBlock(dst)
	if done {
		t.Fatal("voice done too early")
	}
	if n != 32 {
		t.Errorf("SampleBlock fallback returned n=%d, want 32", n)
	}

	// Should still have fade-in envelope.
	if dst[0] > 0.01 {
		t.Errorf("first sample should be near zero (fade-in), got %f", dst[0])
	}
}

func TestAntiPopBlockVsFadeInParity(t *testing.T) {
	sr := 44100
	fadeInSamples := sr * antiPopFadeInMs / 1000
	n := fadeInSamples + 10

	// Sample-by-sample path.
	vSample := newAntiPopVoice(&mockBlockVoice{val: 1.0, remain: n * 2}, sr)
	sampleOut := make([]float64, n)
	for i := 0; i < n; i++ {
		val, _ := vSample.Sample()
		sampleOut[i] = val
	}

	// Block path.
	vBlock := newAntiPopVoice(&mockBlockVoice{val: 1.0, remain: n * 2}, sr)
	blockOut := make([]float64, n)
	vBlock.SampleBlock(blockOut)

	for i := 0; i < n; i++ {
		if math.Abs(sampleOut[i]-blockOut[i]) > 1e-10 {
			t.Errorf("fade-in parity mismatch at sample %d: sample=%f block=%f", i, sampleOut[i], blockOut[i])
			break
		}
	}
}

func TestAntiPopBlockVsFadeOutParity(t *testing.T) {
	sr := 44100
	fadeInSamples := sr * antiPopFadeInMs / 1000
	fadeOutSamples := sr * antiPopFadeOutMs / 1000

	// Sample-by-sample path.
	vSample := newAntiPopVoice(&mockBlockVoice{val: 1.0, remain: sr * 10}, sr)
	for i := 0; i < fadeInSamples+100; i++ {
		vSample.Sample()
	}
	vSample.RequestStop()
	sampleOut := make([]float64, fadeOutSamples+10)
	for i := range sampleOut {
		val, done := vSample.Sample()
		if done {
			sampleOut = sampleOut[:i]
			break
		}
		sampleOut[i] = val
	}

	// Block path.
	vBlock := newAntiPopVoice(&mockBlockVoice{val: 1.0, remain: sr * 10}, sr)
	skipDst := make([]float64, fadeInSamples+100)
	vBlock.SampleBlock(skipDst)
	vBlock.RequestStop()
	blockOut := make([]float64, fadeOutSamples+10)
	nBlock, _ := vBlock.SampleBlock(blockOut)
	blockOut = blockOut[:nBlock]

	minLen := len(sampleOut)
	if len(blockOut) < minLen {
		minLen = len(blockOut)
	}
	for i := 0; i < minLen; i++ {
		if math.Abs(sampleOut[i]-blockOut[i]) > 1e-10 {
			t.Errorf("fade-out parity mismatch at sample %d: sample=%f block=%f", i, sampleOut[i], blockOut[i])
			break
		}
	}
}

func TestAntiPopBlockDoneInMiddle(t *testing.T) {
	sr := 44100
	// Inner voice has only 20 samples remaining.
	inner := &mockBlockVoice{val: 0.5, remain: 20}
	v := newAntiPopVoice(inner, sr)

	dst := make([]float64, 64)
	n, done := v.SampleBlock(dst)
	if !done {
		t.Error("expected done=true when inner finishes mid-block")
	}
	if n != 20 {
		t.Errorf("expected n=20, got %d", n)
	}
}

func TestAntiPopFadeOutDuringFadeIn(t *testing.T) {
	sr := 44100
	fadeInSamples := sr * antiPopFadeInMs / 1000
	v := newAntiPopVoice(&mockVoice{val: 1.0, remain: sr}, sr)

	// Advance to middle of fade-in.
	half := fadeInSamples / 2
	for i := 0; i < half; i++ {
		v.Sample()
	}

	// Request stop during fade-in.
	v.RequestStop()

	// Both envelopes should apply simultaneously.
	val, done := v.Sample()
	if done {
		t.Fatal("voice done immediately after RequestStop during fade-in")
	}
	// Should have both fade-in and fade-out attenuation → small value.
	if val > 0.8 {
		t.Errorf("expected attenuated sample during overlapping fade-in/out, got %f", val)
	}
}
