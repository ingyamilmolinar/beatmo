package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// toneWithBite: a steady 330 Hz tone preceded by 25 ms of broadband noise
// at the same level.
func toneWithBite(sr int, dur float64, bite bool) wave.Wave {
	rng := rand.New(rand.NewSource(5))
	n := int(dur * float64(sr))
	s := make([]float64, n)
	biteN := int(0.025 * float64(sr))
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.4 * math.Sin(2*math.Pi*330*t)
		if bite && i < biteN {
			s[i] += 0.4 * (2*rng.Float64() - 1)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestOnsetBiteRatio_BiteVsClean(t *testing.T) {
	withB, okB := OnsetBiteRatio(NewBundle(toneWithBite(44100, 1.5, true), "b").Tracks())
	clean, okC := OnsetBiteRatio(NewBundle(toneWithBite(44100, 1.5, false), "c").Tracks())
	if !okB || !okC {
		t.Fatalf("want both bite ratios measurable (both clips are 1.5s, well over the 600ms window), got okB=%v okC=%v", okB, okC)
	}
	if withB < 3*clean {
		t.Fatalf("bite ratio with noise burst (%.2f) should dwarf the clean tone's (%.2f)", withB, clean)
	}
}

func TestOnsetPitchGlide_DownwardGlide(t *testing.T) {
	sr := 44100
	n := int(1.0 * float64(sr))
	s := make([]float64, n)
	phase := 0.0
	for i := range s {
		t := float64(i) / float64(sr)
		// 40 cents sharp at t=0 decaying to 0 by 150 ms
		cents := 40 * math.Exp(-t/0.05)
		f := 440 * math.Pow(2, cents/1200)
		phase += 2 * math.Pi * f / float64(sr)
		s[i] = 0.4 * math.Sin(phase)
	}
	g, ok := OnsetPitchGlideCents(wave.Wave{Samples: s, SampleRate: sr}, 440)
	if !ok || g < 10 || g > 45 {
		t.Fatalf("want a positive glide of ~20-40 cents, got %.1f ok=%v", g, ok)
	}
}

func TestResidualCentroid_HissAboveWire(t *testing.T) {
	// The centroid of a harmonic-notched spectrum must rise when the noise is
	// high-passed.
	rng := rand.New(rand.NewSource(9))
	mk := func(hp bool) wave.Wave {
		sr, n := 44100, 44100
		s := make([]float64, n)
		prev := 0.0
		for i := range s {
			t := float64(i) / float64(sr)
			v := 2*rng.Float64() - 1
			if hp {
				v, prev = v-prev, v // crude first-difference high-pass
			}
			s[i] = 0.3*math.Sin(2*math.Pi*200*t) + 0.2*v
		}
		return wave.Wave{Samples: s, SampleRate: sr}
	}
	c := func(w wave.Wave) float64 {
		mag, binHz := wave.MagnitudeSpectrum(w, 16384, wave.WindowHann)
		return ResidualCentroidHz(mag, binHz, 200)
	}
	if c(mk(true)) <= c(mk(false)) {
		t.Fatal("high-passed residual must have the higher residual centroid")
	}
}

// TestResidualCentroidAtModes_UnpitchedBarPlusHighpassNoise exercises the
// f0-free sibling of ResidualCentroidHz: an unpitched two-mode bar tone
// (anchor 220 Hz, second mode at 1:2.76 -- a plausible free-free bar ratio,
// nowhere near a harmonic k*f0 series) plus high-passed noise. Notching the
// two REAL modes out must raise the centroid both above the un-notched
// centroid of the same signal and well above the anchor frequency -- proof
// the notch is removing the modes' energy rather than being a no-op.
func TestResidualCentroidAtModes_UnpitchedBarPlusHighpassNoise(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	sr, n := 44100, 44100
	anchor := 220.0
	second := anchor * 2.76
	s := make([]float64, n)
	prev := 0.0
	for i := range s {
		t := float64(i) / float64(sr)
		v := 2*rng.Float64() - 1
		v, prev = v-prev, v // crude first-difference high-pass
		s[i] = 0.25*math.Sin(2*math.Pi*anchor*t) + 0.2*math.Sin(2*math.Pi*second*t) + 0.3*v
	}
	w := wave.Wave{Samples: s, SampleRate: sr}
	mag, binHz := wave.MagnitudeSpectrum(w, 16384, wave.WindowHann)

	plain := spectralCentroidHz(mag, binHz)
	notched := ResidualCentroidAtModes(mag, binHz, []float64{anchor, second})

	if notched <= plain {
		t.Fatalf("notching the two modes should raise the residual centroid above the un-notched one: notched=%.1f plain=%.1f", notched, plain)
	}
	if notched <= anchor {
		t.Fatalf("residual centroid %.1f should sit well above the anchor %.1f once the modes are notched out", notched, anchor)
	}
}
