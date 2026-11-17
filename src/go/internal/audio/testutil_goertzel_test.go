package audio

import (
	"math"
	"testing"
)

// goertzelMagnitude measures the energy at a specific frequency in a signal
// using the Goertzel algorithm. Returns the magnitude (linear scale).
// This is equivalent to a single-bin DFT and is more efficient than FFT
// when only a few frequencies need to be measured.
func goertzelMagnitude(samples []float64, targetFreq float64, sr int) float64 {
	n := len(samples)
	if n == 0 {
		return 0
	}
	k := int(math.Round(targetFreq * float64(n) / float64(sr)))
	w := 2.0 * math.Pi * float64(k) / float64(n)
	coeff := 2.0 * math.Cos(w)

	s0, s1, s2 := 0.0, 0.0, 0.0
	for _, x := range samples {
		s0 = x + coeff*s1 - s2
		s2 = s1
		s1 = s0
	}
	// Magnitude squared
	power := s1*s1 + s2*s2 - coeff*s1*s2
	if power < 0 {
		power = 0
	}
	return math.Sqrt(power) / float64(n) * 2.0
}

// goertzelMagnitudeDB measures energy at a frequency and returns dB (relative to 1.0).
func goertzelMagnitudeDB(samples []float64, targetFreq float64, sr int) float64 {
	mag := goertzelMagnitude(samples, targetFreq, sr)
	if mag < 1e-20 {
		return -400
	}
	return 20.0 * math.Log10(mag)
}

// TestGoertzelPureSine verifies the Goertzel algorithm detects a known sine wave.
func TestGoertzelPureSine(t *testing.T) {
	const sr = 44100
	const n = 44100 // 1 second
	const freq = 1000.0

	samples := sineSamples(freq, sr, n)

	// Should detect strong energy at 1000 Hz
	mag1k := goertzelMagnitude(samples, freq, sr)
	if mag1k < 0.9 {
		t.Fatalf("expected strong energy at %v Hz, got magnitude %v", freq, mag1k)
	}

	// Should detect very little energy at 2000 Hz (no harmonic)
	mag2k := goertzelMagnitude(samples, 2000, sr)
	if mag2k > 0.01 {
		t.Fatalf("expected near-zero energy at 2000 Hz, got magnitude %v", mag2k)
	}
}

// TestGoertzelDB verifies dB conversion.
func TestGoertzelDB(t *testing.T) {
	const sr = 44100
	const n = 44100

	samples := sineSamples(1000, sr, n)
	db1k := goertzelMagnitudeDB(samples, 1000, sr)
	db5k := goertzelMagnitudeDB(samples, 5000, sr)

	// 1kHz should be near 0 dB (full scale sine)
	if db1k < -3 {
		t.Fatalf("expected ~0 dB at 1kHz, got %v dB", db1k)
	}

	// 5kHz should be far below (< -60 dB)
	if db5k > -60 {
		t.Fatalf("expected < -60 dB at 5kHz, got %v dB", db5k)
	}
}

// TestGoertzelSilence verifies zero signal returns very low energy.
func TestGoertzelSilence(t *testing.T) {
	samples := make([]float64, 44100)
	db := goertzelMagnitudeDB(samples, 1000, 44100)
	if db > -300 {
		t.Fatalf("expected very low dB for silence, got %v", db)
	}
}
