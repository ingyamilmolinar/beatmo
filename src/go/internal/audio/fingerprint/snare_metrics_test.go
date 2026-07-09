package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// TestSnareToneNoise: a pure tone reads as mostly tonal, band-limited noise as
// mostly noise, and the low-body / high-buzz fractions track where the energy is.
func TestSnareToneNoise(t *testing.T) {
	const sr = 44100
	n := sr / 2
	// Pure 190 Hz body tone.
	tone := make([]float64, n)
	for i := range tone {
		t := float64(i) / sr
		tone[i] = math.Sin(2*math.Pi*190*t) * math.Exp(-6*t)
	}
	tm := AnalyzeSnare(wave.Wave{Samples: tone, SampleRate: sr})
	// The short body window leaks spectral energy, so a pure tone caps ~0.6-0.7
	// (not 1.0) — the metric is used for RELATIVE ref-vs-synth comparison.
	if tm.ToneNoiseRatio < 0.55 {
		t.Fatalf("pure tone should be mostly tonal, got ToneNoiseRatio=%.2f", tm.ToneNoiseRatio)
	}
	if tm.LowBodyFrac < 0.5 {
		t.Fatalf("190 Hz tone should be low-body-heavy, got %.2f", tm.LowBodyFrac)
	}
	if !(tm.BodyFundHz > 170 && tm.BodyFundHz < 210) {
		t.Fatalf("body fundamental = %.0f, want ~190", tm.BodyFundHz)
	}

	// High band-passed noise (the snare buzz): mostly noise, high-buzz-heavy.
	nz := highNoise(sr, n)
	nm := AnalyzeSnare(wave.Wave{Samples: nz, SampleRate: sr})
	if nm.ToneNoiseRatio > 0.35 {
		t.Fatalf("noise should be mostly non-tonal, got ToneNoiseRatio=%.2f", nm.ToneNoiseRatio)
	}
	if nm.HighBuzzFrac <= tm.HighBuzzFrac {
		t.Fatalf("high noise should have more high-buzz (%.2f) than a low tone (%.2f)", nm.HighBuzzFrac, tm.HighBuzzFrac)
	}
	if nm.Flatness <= tm.Flatness {
		t.Fatalf("noise flatness (%.2f) should exceed tone flatness (%.2f)", nm.Flatness, tm.Flatness)
	}
}

// TestSnareDualDecay: a signal whose LOW band decays fast and HIGH band rings
// long should measure BodyDecay < BuzzDecay (the fat-snare "thump + ring").
func TestSnareDualDecay(t *testing.T) {
	const sr = 44100
	n := sr
	x := make([]float64, n)
	for i := range x {
		t := float64(i) / sr
		body := math.Sin(2*math.Pi*190*t) * math.Exp(-40*t)   // fast body
		buzz := 0.6 * highOsc(t) * math.Exp(-6*t)              // long bright ring
		x[i] = body + buzz
	}
	sm := AnalyzeSnare(wave.Wave{Samples: x, SampleRate: sr})
	if !(sm.BodyDecayMs < sm.BuzzDecayMs) {
		t.Fatalf("fast body (%.0f ms) should decay before the long buzz (%.0f ms)", sm.BodyDecayMs, sm.BuzzDecayMs)
	}
}

// deterministic pseudo-noise (no rand): a hash-driven ±1 sequence, high-passed.
func highNoise(sr, n int) []float64 {
	x := make([]float64, n)
	var s uint32 = 12345
	var hp, prev float64
	for i := range x {
		s = s*1664525 + 1013904223
		v := float64(int32(s))/float64(1<<31) - 0.0
		hp = 0.9*(hp+v-prev)/1.0
		prev = v
		x[i] = hp * math.Exp(-4*float64(i)/float64(sr))
	}
	return x
}

// highOsc is a bright deterministic multi-partial buzz around 3-6 kHz.
func highOsc(t float64) float64 {
	return (math.Sin(2*math.Pi*3300*t) + math.Sin(2*math.Pi*4700*t) + math.Sin(2*math.Pi*6100*t)) / 3
}
