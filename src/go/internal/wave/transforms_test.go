package wave

import (
	"math"
	"testing"
)

// --- NormalizeTransform ---

func TestNormalizeTransform_DC025_To0dB(t *testing.T) {
	w := DC(0.25, 0.1, 44100)
	norm := NormalizeTransform(0).Apply(w)
	peak := norm.PeakSample()
	if math.Abs(peak-1.0) > 1e-9 {
		t.Fatalf("expected peak 1.0 after 0dB normalize, got %v", peak)
	}
}

func TestNormalizeTransform_DC01_ToMinus6dB(t *testing.T) {
	w := DC(0.1, 0.1, 44100)
	norm := NormalizeTransform(-6).Apply(w)
	// -6 dB => amplitude = 10^(-6/20) = 0.501187...
	expected := math.Pow(10, -6.0/20.0)
	peak := norm.PeakSample()
	if math.Abs(peak-expected) > 1e-4 {
		t.Fatalf("expected peak ~%v after -6dB normalize, got %v", expected, peak)
	}
}

func TestNormalizeTransform_Silence(t *testing.T) {
	w := Silence(0.1, 44100)
	norm := NormalizeTransform(0).Apply(w)
	peak := norm.PeakSample()
	if peak != 0 {
		t.Fatalf("expected silence to stay silence, got peak %v", peak)
	}
}

func TestNormalizeTransform_OriginalNotMutated(t *testing.T) {
	w := DC(0.25, 0.1, 44100)
	origPeak := w.PeakSample()
	_ = NormalizeTransform(0).Apply(w)
	if w.PeakSample() != origPeak {
		t.Fatalf("original wave was mutated: peak was %v, now %v", origPeak, w.PeakSample())
	}
}

// --- TrimTransform ---

func TestTrimTransform_100to200ms(t *testing.T) {
	w := DC(1.0, 1.0, 44100) // 1 second
	trimmed := TrimTransform(100, 200).Apply(w)
	expected := 4410 // 100ms at 44100Hz
	if trimmed.Len() != expected {
		t.Fatalf("expected %d samples, got %d", expected, trimmed.Len())
	}
}

func TestTrimTransform_ClampsOutOfBounds(t *testing.T) {
	w := DC(1.0, 0.5, 44100) // 0.5 seconds = 22050 samples
	trimmed := TrimTransform(0, 1000).Apply(w) // endMs=1000 exceeds 500ms duration
	if trimmed.Len() != 22050 {
		t.Fatalf("expected 22050 samples (clamped), got %d", trimmed.Len())
	}
}

// --- WindowTransform ---

func TestWindowTransform_Hann_DC(t *testing.T) {
	w := DC(1.0, 0.01, 44100) // short wave
	windowed := WindowTransform(WindowHann).Apply(w)
	n := windowed.Len()
	if n == 0 {
		t.Fatal("windowed wave is empty")
	}
	// First and last samples should be ~0
	if math.Abs(windowed.Samples[0]) > 1e-9 {
		t.Fatalf("expected first sample ~0, got %v", windowed.Samples[0])
	}
	if math.Abs(windowed.Samples[n-1]) > 1e-9 {
		t.Fatalf("expected last sample ~0, got %v", windowed.Samples[n-1])
	}
	// Middle sample should be ~1.0
	mid := windowed.Samples[n/2]
	if math.Abs(mid-1.0) > 1e-3 {
		t.Fatalf("expected middle sample ~1.0, got %v", mid)
	}
}

func TestWindowTransform_Hamming_DC(t *testing.T) {
	w := DC(1.0, 0.01, 44100)
	windowed := WindowTransform(WindowHamming).Apply(w)
	n := windowed.Len()
	// Hamming endpoints: 0.54 - 0.46*cos(0) = 0.54 - 0.46 = 0.08
	if math.Abs(windowed.Samples[0]-0.08) > 1e-3 {
		t.Fatalf("expected first sample ~0.08 for Hamming, got %v", windowed.Samples[0])
	}
	// Middle should be ~1.0
	mid := windowed.Samples[n/2]
	if math.Abs(mid-1.0) > 1e-3 {
		t.Fatalf("expected middle sample ~1.0 for Hamming, got %v", mid)
	}
}

func TestWindowTransform_Blackman_DC(t *testing.T) {
	w := DC(1.0, 0.01, 44100)
	windowed := WindowTransform(WindowBlackman).Apply(w)
	n := windowed.Len()
	// Blackman endpoints: 0.42 - 0.5*cos(0) + 0.08*cos(0) = 0.42 - 0.5 + 0.08 = 0.0
	if math.Abs(windowed.Samples[0]) > 1e-3 {
		t.Fatalf("expected first sample ~0 for Blackman, got %v", windowed.Samples[0])
	}
	// Middle should be ~1.0
	mid := windowed.Samples[n/2]
	if math.Abs(mid-1.0) > 1e-3 {
		t.Fatalf("expected middle sample ~1.0 for Blackman, got %v", mid)
	}
}

func TestWindowTransform_OriginalNotMutated(t *testing.T) {
	w := DC(1.0, 0.01, 44100)
	origFirst := w.Samples[0]
	_ = WindowTransform(WindowHann).Apply(w)
	if w.Samples[0] != origFirst {
		t.Fatalf("original wave was mutated: first sample was %v, now %v", origFirst, w.Samples[0])
	}
}

// --- ResampleTransform ---

func TestResampleTransform_DoublesLength(t *testing.T) {
	w := Sine(440, 1.0, 0.1, 22050) // 2205 samples
	resampled := ResampleTransform(44100).Apply(w)
	expected := 4410 // doubled
	if resampled.Len() != expected {
		t.Fatalf("expected %d samples after upsample, got %d", expected, resampled.Len())
	}
	if resampled.SampleRate != 44100 {
		t.Fatalf("expected sample rate 44100, got %d", resampled.SampleRate)
	}
}

func TestResampleTransform_SameRate(t *testing.T) {
	w := Sine(440, 1.0, 0.1, 44100)
	resampled := ResampleTransform(44100).Apply(w)
	if resampled.Len() != w.Len() {
		t.Fatalf("expected same length %d, got %d", w.Len(), resampled.Len())
	}
	// Should be a clone, not the same slice
	resampled.Samples[0] = 999
	if w.Samples[0] == 999 {
		t.Fatal("resample at same rate should return a clone, not alias original")
	}
}
