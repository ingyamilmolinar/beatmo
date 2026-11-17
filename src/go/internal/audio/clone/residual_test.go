package clone

import (
	"math"
	"math/rand"
	"testing"
)

// TestMeasureResidualRecoversABandEnvelope: a band-limited noise envelope can
// only be measured to ~1/sqrt(B·τ). Measured spread of the fast rate over 20
// seeds: 26 % in the 2 kHz third-octave (B ≈ 470 Hz) at τ = 1/30 s, 12 % in
// the 8 kHz one (B ≈ 1.9 kHz) — so the estimator is judged statistically:
// unbiased on average over 10 seeds, every seed within a 2.5σ bound.
func TestMeasureResidualRecoversABandEnvelope(t *testing.T) {
	const fc, m, r1, r2 = 8063.0, 0.7, 30.0, 6.0
	var sumFast, sumTail, sumMix float64
	const seeds = 10
	for seed := int64(1); seed <= seeds; seed++ {
		got := measureOneBandEnvelope(t, seed, fc, m, r1, r2)
		if math.Abs(got.FastRate/r1-1) > 0.35 || math.Abs(got.TailRate/r2-1) > 0.2 || math.Abs(got.FastMix-m) > 0.15 {
			t.Errorf("seed %d: band %+v, want fast %.0f mix %.2f tail %.0f", seed, got, r1, m, r2)
		}
		if got.AttackSec > 0.01 {
			t.Errorf("seed %d: attack %.4f s for an instant onset", seed, got.AttackSec)
		}
		sumFast += got.FastRate
		sumTail += got.TailRate
		sumMix += got.FastMix
	}
	if b := sumFast/seeds/r1 - 1; math.Abs(b) > 0.10 {
		t.Errorf("fast rate biased %+.0f%% over %d seeds", 100*b, seeds)
	}
	if b := sumTail/seeds/r2 - 1; math.Abs(b) > 0.05 {
		t.Errorf("tail rate biased %+.0f%% over %d seeds", 100*b, seeds)
	}
	if b := sumMix/seeds - m; math.Abs(b) > 0.05 {
		t.Errorf("fast mix biased %+.2f over %d seeds", b, seeds)
	}
}

func measureOneBandEnvelope(t *testing.T, seed int64, fc, m, r1, r2 float64) NoiseBand {
	t.Helper()
	rate := 48000
	pre := 2400
	n := pre + int(1.5*float64(rate))
	r := rand.New(rand.NewSource(seed))
	white := make([]float64, n)
	for i := range white {
		white[i] = r.NormFloat64()
	}
	nb := bandpass(white, rate, fc, thirdOctaveQ)
	x := make([]float64, n)
	for i := pre; i < n; i++ {
		tt := float64(i-pre) / float64(rate)
		x[i] = 0.05 * nb[i] * (m*math.Exp(-r1*tt) + (1-m)*math.Exp(-r2*tt))
	}
	p := Prepared{Samples: x, Rate: rate, Onset: pre, WindowStart: pre, WindowEnd: n, FloorRMS: 1e-9}
	res := MeasureResidual(p, nil, 0.002)
	if res.EnergyFraction < 0.999 {
		t.Fatalf("no modes: residual fraction %.3f, want 1", res.EnergyFraction)
	}
	for _, b := range res.Bands {
		if math.Abs(b.CenterHz-fc) < 5 {
			if !b.Active {
				t.Fatalf("%.0f Hz band inactive: %+v", fc, b)
			}
			return b
		}
	}
	t.Fatalf("no band at %.0f Hz", fc)
	return NoiseBand{}
}

func TestMeasureResidualOfAPureModalSignalIsSmall(t *testing.T) {
	modes := barModes()
	x := modalSignal(modes, 48000, 0.05, 2, 0, 0)
	p := Prepared{Samples: x, Rate: 48000, Onset: 2400, WindowStart: 2400, WindowEnd: len(x), FloorRMS: 1e-9}
	if f := MeasureResidual(p, modes, 0.002).EnergyFraction; f > 1e-6 {
		t.Fatalf("exact modes leave residual fraction %.2e", f)
	}
}
