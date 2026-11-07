package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// decayingTone: a 1 kHz sine decaying at rateDBPerSec, so its 1k band T60 is 60/rate.
func decayingTone(sr int, dur, hz, rateDBPerSec float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.5 * math.Pow(10, -rateDBPerSec*t/20) * math.Sin(2*math.Pi*hz*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestBandT60_FitsConstructedDecay(t *testing.T) {
	b := NewBundle(decayingTone(44100, 3.0, 1000, 30), "d")
	bd := BandT60(b.STFT(trackFFTSize, trackHop))
	i := 3 // 1k band
	if !bd.OK[i] {
		t.Fatal("1k band should be measurable")
	}
	if math.Abs(bd.T60Sec[i]-2.0) > 0.2 {
		t.Fatalf("T60 want 2.0 s (60/30), got %.2f", bd.T60Sec[i])
	}
}

func TestBandT60_FiniteWhenClipEndsAboveMinus20dB(t *testing.T) {
	// A 1 s clip at 15 dB/s reaches only -15 dB; the fitted rate still yields 4.0 s.
	b := NewBundle(decayingTone(44100, 1.0, 1000, 15), "short")
	bd := BandT60(b.STFT(trackFFTSize, trackHop))
	if !bd.OK[3] || math.IsInf(bd.T60Sec[3], 0) || math.Abs(bd.T60Sec[3]-4.0) > 0.6 {
		t.Fatalf("want ~4.0 s extrapolated, got %.2f ok=%v", bd.T60Sec[3], bd.OK[3])
	}
}

// A pure 1 kHz tone has no T60 on bands with no energy of their own; leakage from louder partials must not be fitted.
func TestBandT60_EnergyGateRejectsQuietBands(t *testing.T) {
	b := NewBundle(decayingTone(44100, 2.0, 1000, 20), "pure1k")
	bd := BandT60(b.STFT(trackFFTSize, trackHop))
	if !bd.OK[3] { // 1k band
		t.Fatal("1k band should be measurable for a pure 1kHz decaying tone")
	}
	for _, i := range []int{0, 1, 6} { // 125, 250, 8k -- far from 1k, no leakage excuse
		if bd.OK[i] {
			t.Errorf("band %s should be gated out as carrying no energy for a pure 1kHz tone, got OK (T60=%.3f)",
				octaveBandLabels[i], bd.T60Sec[i])
		}
	}
}

func TestDecaySlopeVsFreq_HighsDieFaster(t *testing.T) {
	b := NewBundle(twoDecayingModes(44100, 2.0), "bar") // 440@-20, 1215@-60 dB/s
	s, ok := DecaySlopeVsFreq(b.Modes())
	// needs >=3 modes; add a third by using a richer tone
	if ok && s >= 0 {
		t.Fatalf("slope must be negative (highs decay faster), got %.2f", s)
	}
	rates, labels, ok := PerModeDecay(b.Modes(), 8)
	if !ok || len(rates) != 2 || labels[0] != "m1" {
		t.Fatalf("per-mode decay want 2 rates, got %v %v ok=%v", rates, labels, ok)
	}
	// Fails only when mode 1 decays faster (more negative) than mode 2: highs die faster.
	if rates[0] < rates[1] {
		t.Fatalf("mode 1 should decay slower (less negative): %v", rates)
	}
}

func TestRingT60IsDemoted(t *testing.T) {
	d, ok := DefaultRegistry().ByID(IDEnvRingT60)
	if !ok || !d.Demoted {
		t.Fatal("env.ring_t60 must be registered with Demoted: true")
	}
}
