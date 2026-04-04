package wave

import (
	"math"
	"testing"
)

const (
	sr44100 = 44100
	sr48000 = 48000
	epsilon = 1e-9
)

// --- Wave.Duration ---

func TestDuration_OneSecond(t *testing.T) {
	w := Wave{Samples: make([]float64, 44100), SampleRate: sr44100}
	got := w.Duration()
	if math.Abs(got-1.0) > epsilon {
		t.Fatalf("Duration: want 1.0s, got %f", got)
	}
}

func TestDuration_Empty(t *testing.T) {
	w := Wave{SampleRate: sr44100}
	got := w.Duration()
	if got != 0 {
		t.Fatalf("Duration: want 0, got %f", got)
	}
}

func TestDuration_48kHz(t *testing.T) {
	w := Wave{Samples: make([]float64, sr48000), SampleRate: sr48000}
	got := w.Duration()
	if math.Abs(got-1.0) > epsilon {
		t.Fatalf("Duration: want 1.0s, got %f", got)
	}
}

// --- Wave.Len ---

func TestLen(t *testing.T) {
	w := Wave{Samples: make([]float64, 100), SampleRate: sr44100}
	if w.Len() != 100 {
		t.Fatalf("Len: want 100, got %d", w.Len())
	}
}

// --- Wave.PeakSample ---

func TestPeakSample_Positive(t *testing.T) {
	w := Wave{Samples: []float64{0.1, 0.5, 0.3, 0.8, 0.2}, SampleRate: sr44100}
	if math.Abs(w.PeakSample()-0.8) > epsilon {
		t.Fatalf("PeakSample: want 0.8, got %f", w.PeakSample())
	}
}

func TestPeakSample_Negative(t *testing.T) {
	w := Wave{Samples: []float64{0.1, -0.9, 0.3, 0.5}, SampleRate: sr44100}
	if math.Abs(w.PeakSample()-0.9) > epsilon {
		t.Fatalf("PeakSample: want 0.9, got %f", w.PeakSample())
	}
}

func TestPeakSample_Empty(t *testing.T) {
	w := Wave{SampleRate: sr44100}
	if w.PeakSample() != 0 {
		t.Fatalf("PeakSample: want 0, got %f", w.PeakSample())
	}
}

// --- Wave.Slice ---

func TestSlice_MiddleRange(t *testing.T) {
	// 1 second at 1000 SR for easy math
	samples := make([]float64, 1000)
	for i := range samples {
		samples[i] = float64(i)
	}
	w := Wave{Samples: samples, SampleRate: 1000, Label: "test"}
	s := w.Slice(100, 200) // 100ms to 200ms => samples 100..199
	if s.Len() != 100 {
		t.Fatalf("Slice len: want 100, got %d", s.Len())
	}
	if s.Samples[0] != 100 {
		t.Fatalf("Slice first sample: want 100, got %f", s.Samples[0])
	}
	if s.Samples[99] != 199 {
		t.Fatalf("Slice last sample: want 199, got %f", s.Samples[99])
	}
	if s.SampleRate != 1000 {
		t.Fatalf("Slice SampleRate: want 1000, got %d", s.SampleRate)
	}
}

func TestSlice_ClampsToStart(t *testing.T) {
	w := Wave{Samples: []float64{1, 2, 3, 4, 5}, SampleRate: 1000}
	s := w.Slice(-50, 2) // negative start => clamp to 0
	if s.Samples[0] != 1 {
		t.Fatalf("Slice clamp start: want first sample 1, got %f", s.Samples[0])
	}
}

func TestSlice_ClampsToEnd(t *testing.T) {
	w := Wave{Samples: []float64{1, 2, 3, 4, 5}, SampleRate: 1000}
	s := w.Slice(0, 99999) // way past end
	if s.Len() != 5 {
		t.Fatalf("Slice clamp end: want 5, got %d", s.Len())
	}
}

// --- Wave.Clone ---

func TestClone_DeepCopy(t *testing.T) {
	orig := Wave{Samples: []float64{1, 2, 3}, SampleRate: sr44100, Label: "orig"}
	c := orig.Clone()
	if c.Len() != orig.Len() {
		t.Fatalf("Clone len mismatch")
	}
	if c.SampleRate != orig.SampleRate {
		t.Fatalf("Clone SampleRate mismatch")
	}
	if c.Label != orig.Label {
		t.Fatalf("Clone Label mismatch")
	}
	// Mutate clone, original must be unaffected.
	c.Samples[0] = 999
	if orig.Samples[0] != 1 {
		t.Fatalf("Clone is not deep: original mutated")
	}
}

// --- Sine ---

func TestSine_Length(t *testing.T) {
	w := Sine(440, 1.0, 1.0, sr44100)
	if w.Len() != sr44100 {
		t.Fatalf("Sine length: want %d, got %d", sr44100, w.Len())
	}
}

func TestSine_FirstSampleNearZero(t *testing.T) {
	w := Sine(440, 1.0, 1.0, sr44100)
	if math.Abs(w.Samples[0]) > 1e-6 {
		t.Fatalf("Sine first sample: want ~0, got %f", w.Samples[0])
	}
}

func TestSine_PeakNearAmplitude(t *testing.T) {
	amp := 0.75
	w := Sine(440, amp, 1.0, sr44100)
	peak := w.PeakSample()
	// Allow 1% tolerance due to discrete sampling.
	if math.Abs(peak-amp) > amp*0.01 {
		t.Fatalf("Sine peak: want ~%f, got %f", amp, peak)
	}
}

func TestSine_SampleRate(t *testing.T) {
	w := Sine(440, 1.0, 0.5, sr48000)
	if w.SampleRate != sr48000 {
		t.Fatalf("Sine SR: want %d, got %d", sr48000, w.SampleRate)
	}
	if w.Len() != 24000 {
		t.Fatalf("Sine 0.5s at 48k: want 24000, got %d", w.Len())
	}
}

// --- Silence ---

func TestSilence_Length(t *testing.T) {
	w := Silence(1.0, sr44100)
	if w.Len() != sr44100 {
		t.Fatalf("Silence length: want %d, got %d", sr44100, w.Len())
	}
}

func TestSilence_AllZeros(t *testing.T) {
	w := Silence(0.1, sr44100)
	for i, s := range w.Samples {
		if s != 0 {
			t.Fatalf("Silence sample[%d] = %f, want 0", i, s)
		}
	}
}

// --- Square ---

func TestSquare_PositiveRatio(t *testing.T) {
	w := Square(440, 1.0, 1.0, sr44100)
	pos := 0
	for _, s := range w.Samples {
		if s > 0 {
			pos++
		}
	}
	ratio := float64(pos) / float64(w.Len())
	// Should be ~50% (allow 5% tolerance).
	if math.Abs(ratio-0.5) > 0.05 {
		t.Fatalf("Square positive ratio: want ~0.5, got %f", ratio)
	}
}

func TestSquare_PeakEqualsAmplitude(t *testing.T) {
	amp := 0.6
	w := Square(440, amp, 1.0, sr44100)
	if math.Abs(w.PeakSample()-amp) > epsilon {
		t.Fatalf("Square peak: want %f, got %f", amp, w.PeakSample())
	}
}

func TestSquare_Length(t *testing.T) {
	w := Square(440, 1.0, 0.5, sr44100)
	want := 22050
	if w.Len() != want {
		t.Fatalf("Square length: want %d, got %d", want, w.Len())
	}
}

// --- Impulse ---

func TestImpulse_OnlyOneNonZero(t *testing.T) {
	idx := 50
	amp := 0.9
	w := Impulse(amp, idx, 0.01, sr44100)
	nonZero := 0
	for _, s := range w.Samples {
		if s != 0 {
			nonZero++
		}
	}
	if nonZero != 1 {
		t.Fatalf("Impulse non-zero count: want 1, got %d", nonZero)
	}
	if math.Abs(w.Samples[idx]-amp) > epsilon {
		t.Fatalf("Impulse sample[%d]: want %f, got %f", idx, amp, w.Samples[idx])
	}
}

func TestImpulse_IndexOutOfRange(t *testing.T) {
	// If sampleIndex >= len, no non-zero samples.
	w := Impulse(1.0, 9999, 0.001, sr44100)
	for i, s := range w.Samples {
		if s != 0 {
			t.Fatalf("Impulse OOB: sample[%d] = %f, want 0", i, s)
		}
	}
}

// --- DC ---

func TestDC_AllSamplesEqual(t *testing.T) {
	amp := 0.42
	w := DC(amp, 0.01, sr44100)
	for i, s := range w.Samples {
		if math.Abs(s-amp) > epsilon {
			t.Fatalf("DC sample[%d]: want %f, got %f", i, amp, s)
		}
	}
}

func TestDC_Length(t *testing.T) {
	w := DC(1.0, 1.0, sr44100)
	if w.Len() != sr44100 {
		t.Fatalf("DC length: want %d, got %d", sr44100, w.Len())
	}
}

// --- Noise ---

func TestNoise_Deterministic(t *testing.T) {
	a := Noise(42, 1.0, 0.01, sr44100)
	b := Noise(42, 1.0, 0.01, sr44100)
	if a.Len() != b.Len() {
		t.Fatalf("Noise deterministic: length mismatch")
	}
	for i := range a.Samples {
		if a.Samples[i] != b.Samples[i] {
			t.Fatalf("Noise deterministic: sample[%d] differs", i)
		}
	}
}

func TestNoise_DifferentSeed(t *testing.T) {
	a := Noise(42, 1.0, 0.01, sr44100)
	b := Noise(99, 1.0, 0.01, sr44100)
	same := true
	for i := range a.Samples {
		if a.Samples[i] != b.Samples[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatalf("Noise different seeds produced identical output")
	}
}

func TestNoise_Length(t *testing.T) {
	w := Noise(1, 1.0, 0.5, sr44100)
	want := 22050
	if w.Len() != want {
		t.Fatalf("Noise length: want %d, got %d", want, w.Len())
	}
}

func TestNoise_BoundedByAmplitude(t *testing.T) {
	amp := 0.5
	w := Noise(7, amp, 0.1, sr44100)
	for i, s := range w.Samples {
		if math.Abs(s) > amp+epsilon {
			t.Fatalf("Noise sample[%d] = %f exceeds amplitude %f", i, s, amp)
		}
	}
}

// --- Compositors ---

func TestChain(t *testing.T) {
	src := &WaveSource{W: DC(1.0, 0.01, 1000)}
	gain := TransformFunc(func(w Wave) Wave {
		out := w.Clone()
		for i := range out.Samples {
			out.Samples[i] *= 0.5
		}
		return out
	})
	result := Chain(src, gain)
	for i, s := range result.Samples {
		if math.Abs(s-0.5) > epsilon {
			t.Fatalf("Chain sample[%d]: want 0.5, got %f", i, s)
		}
	}
}

func TestObserve(t *testing.T) {
	w := Sine(440, 1.0, 0.1, sr44100)
	obs := ObserverFunc(func(w Wave) Observation {
		return Observation{Kind: ObsPeakRMS, Peak: w.PeakSample()}
	})
	results := Observe(w, obs)
	if len(results) != 1 {
		t.Fatalf("Observe: want 1 result, got %d", len(results))
	}
	if results[0].Kind != ObsPeakRMS {
		t.Fatalf("Observe kind: want ObsPeakRMS, got %v", results[0].Kind)
	}
	if results[0].Peak < 0.99 {
		t.Fatalf("Observe peak: want ~1.0, got %f", results[0].Peak)
	}
}

func TestPipeline(t *testing.T) {
	src := &WaveSource{W: DC(2.0, 0.01, 1000)}
	halve := TransformFunc(func(w Wave) Wave {
		out := w.Clone()
		for i := range out.Samples {
			out.Samples[i] *= 0.5
		}
		return out
	})
	obs := ObserverFunc(func(w Wave) Observation {
		return Observation{Kind: ObsPeakRMS, Peak: w.PeakSample()}
	})
	result, observations := Pipeline(src, []Transform{halve}, []Observer{obs})
	// DC(2.0) halved => 1.0
	for i, s := range result.Samples {
		if math.Abs(s-1.0) > epsilon {
			t.Fatalf("Pipeline sample[%d]: want 1.0, got %f", i, s)
		}
	}
	if len(observations) != 1 {
		t.Fatalf("Pipeline observations: want 1, got %d", len(observations))
	}
	if math.Abs(observations[0].Peak-1.0) > epsilon {
		t.Fatalf("Pipeline obs peak: want 1.0, got %f", observations[0].Peak)
	}
}

// --- WaveSource ---

func TestWaveSource_Render(t *testing.T) {
	w := Sine(440, 1.0, 0.1, sr44100)
	src := &WaveSource{W: w}
	rendered := src.Render()
	if rendered.Len() != w.Len() {
		t.Fatalf("WaveSource.Render len mismatch")
	}
}
