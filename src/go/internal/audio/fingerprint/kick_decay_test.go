package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// twoBandDecay renders two exponentially-decaying sines with INDEPENDENT decay
// constants: fLow (in a low KickBand) with tauLow, fHigh (in a high KickBand)
// with tauHigh. Lets a test control per-band decay separately.
func twoBandDecay(sr int, dur, fLow, tauLow, fHigh, tauHigh float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = math.Exp(-t/tauLow)*math.Sin(2*math.Pi*fLow*t) +
			math.Exp(-t/tauHigh)*math.Sin(2*math.Pi*fHigh*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func bandIndexFor(hz float64) int {
	for i, b := range KickBands() {
		if hz >= b.LoHz && hz < b.HiHz {
			return i
		}
	}
	return -1
}

func TestBandT60_LowRingsLongerThanHigh(t *testing.T) {
	const sr = 44100
	// 80 Hz sub decays slow (tau 0.06 → T60 ≈ 0.41 s); 3 kHz click decays fast
	// (tau 0.01 → T60 ≈ 0.069 s). Signal long enough that both reach −60 dB.
	w := twoBandDecay(sr, 0.6, 80, 0.06, 3000, 0.01)
	fp := KickAnalyze(w)

	loB := bandIndexFor(80)   // band 1: 60-120
	hiB := bandIndexFor(3000) // band 6: >2000
	if loB < 0 || hiB < 0 {
		t.Fatalf("band indices: lo=%d hi=%d", loB, hiB)
	}
	if fp.BandT60[loB] <= 0 || fp.BandT60[hiB] <= 0 {
		t.Fatalf("expected both bands measurable: lo=%.4f hi=%.4f", fp.BandT60[loB], fp.BandT60[hiB])
	}
	if fp.BandT60[loB] <= fp.BandT60[hiB] {
		t.Errorf("low-band T60 (%.4f) must exceed high-band T60 (%.4f)", fp.BandT60[loB], fp.BandT60[hiB])
	}
}

func TestBandT60_Ballpark(t *testing.T) {
	const sr = 44100
	// Control a single band precisely: 100 Hz (band 1: 60-120), tau = 0.04.
	// True T60 for an amplitude envelope e^{-t/tau} is ln(1000)·tau ≈ 6.908·tau.
	const tau = 0.04
	want := math.Log(1000) * tau // ≈ 0.276 s
	w := expDecaySine(sr, 0.6, 100, tau)
	fp := KickAnalyze(w)

	b := bandIndexFor(100)
	got := fp.BandT60[b]
	if got <= 0 {
		t.Fatalf("band %d T60 = %.4f, want measurable", b, got)
	}
	if math.Abs(got-want)/want > 0.30 {
		t.Errorf("BandT60[%d] = %.4f s, want %.4f ±30%%", b, got, want)
	}
	if fp.BandT60Floored[b] {
		t.Errorf("band %d unexpectedly floored (decay should complete within signal)", b)
	}
}

func TestBandT60_SlowDecayFloored(t *testing.T) {
	const sr = 44100
	// A very slow decay (tau 0.5 → T60 ≈ 3.5 s) in a 0.3 s signal cannot reach
	// −60 dB → the band is clamped to the remaining length and flagged.
	w := expDecaySine(sr, 0.3, 100, 0.5)
	fp := KickAnalyze(w)
	b := bandIndexFor(100)
	if fp.BandT60[b] <= 0 {
		t.Fatalf("band %d T60 = %.4f, want measurable (clamped)", b, fp.BandT60[b])
	}
	if !fp.BandT60Floored[b] {
		t.Errorf("band %d should be floored for a decay too slow to reach −60 dB", b)
	}
	if fp.BandT60[b] > 0.31 {
		t.Errorf("floored T60 = %.4f, want clamped to ~signal length (0.3 s)", fp.BandT60[b])
	}
}

func TestKickDecayDistance_FastVsSlow(t *testing.T) {
	const sr = 44100
	// Same 80 Hz tone, different decay rates → positive Decay distance term.
	fast := KickAnalyze(expDecaySine(sr, 0.6, 80, 0.03))
	slow := KickAnalyze(expDecaySine(sr, 0.6, 80, 0.10))

	d := KickDistance(fast, slow)
	if d.Decay <= 0 {
		t.Errorf("Decay distance = %.4f, want clearly > 0 for fast-vs-slow decay", d.Decay)
	}
	// Identity + symmetry.
	if id := KickDistance(fast, fast); id.Decay != 0 {
		t.Errorf("KickDistance(x,x).Decay = %g, want 0", id.Decay)
	}
	if dr := KickDistance(slow, fast); math.Abs(dr.Decay-d.Decay) > 1e-12 {
		t.Errorf("Decay term not symmetric: %g vs %g", d.Decay, dr.Decay)
	}
}

func TestKickDistance_FullIdentityWithNewTerms(t *testing.T) {
	const sr = 44100
	w := KickAnalyze(twoBandDecay(sr, 0.6, 80, 0.06, 3000, 0.01))
	if d := KickDistance(w, w); d.Total > 1e-9 || d.Roughness != 0 || d.Decay != 0 {
		t.Errorf("KickDistance(x,x): Total=%g Roughness=%g Decay=%g, want all ~0", d.Total, d.Roughness, d.Decay)
	}
}
