package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Whether the number is RIGHT across the shapes that break detectors (strong upper harmonics, missing fundamentals,
// odd-only series, the range edges), in CENTS, since 2 Hz means different things at 50 and 2000 Hz. The octave
// shapes overlap pitch_test.go on purpose: that error has actually shipped.

// centsApart returns the absolute interval between two frequencies in cents.
func centsApart(a, b float64) float64 {
	if a <= 0 || b <= 0 {
		return math.Inf(1)
	}
	return math.Abs(1200 * math.Log2(a/b))
}

// 50 cents is the bar: past it the note NAME changes.
func TestDetectF0_AccuracyAcrossTheSearchRange(t *testing.T) {
	const tolCents = 50
	// From just above the detector's floor to just under its ceiling.
	for _, f0 := range []float64{55, 82.41, 110, 220, 440, 880, 1320, 1760} {
		w := harmonics(supportTestSR, 1.5, f0, []float64{1, 2, 3, 4}, []float64{1, .5, .3, .2})
		est := DetectF0Detailed(w)
		if got := centsApart(est.Hz, f0); got > tolCents {
			t.Errorf("f0=%.2f Hz: detected %.2f Hz, off by %.1f cents (tolerance %d)",
				f0, est.Hz, got, tolCents)
		}
		if !est.Reliable() {
			t.Errorf("f0=%.2f Hz: a clean harmonic tone was reported unreliable (conf %.3f)",
				f0, est.Confidence)
		}
	}
}

// Each shape tempts the detector into the wrong octave, the failure that has actually shipped.
func TestDetectF0_ResistsOctaveErrors(t *testing.T) {
	const f0 = 220.0
	const tolCents = 50

	cases := []struct {
		name    string
		numbers []float64
		amps    []float64
	}{
		// A louder 2nd harmonic baits a lock onto 440.
		{"2nd harmonic dominant", []float64{1, 2, 3, 4}, []float64{0.3, 1.0, 0.5, 0.4}},
		{"fundamental barely present", []float64{1, 2, 3, 4}, []float64{0.1, 1.0, 0.8, 0.6}},
		// An odd-only series has a gap where H2 would be, inviting halving.
		{"odd harmonics only", []float64{1, 3, 5, 7}, []float64{1.0, 0.4, 0.25, 0.15}},
		// Missing fundamental: the ear hears 220, and so should the detector.
		{"missing fundamental", []float64{2, 3, 4, 5}, []float64{1.0, 0.7, 0.5, 0.3}},
		// Every harmonic equal — no amplitude cue to anchor the fundamental.
		{"flat harmonic series", []float64{1, 2, 3, 4, 5, 6}, []float64{1, 1, 1, 1, 1, 1}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := harmonics(supportTestSR, 1.5, f0, c.numbers, c.amps)
			est := DetectF0Detailed(w)
			off := centsApart(est.Hz, f0)
			if off <= tolCents {
				return
			}
			// An octave out is a different bug from imprecision.
			switch {
			case centsApart(est.Hz, f0*2) <= tolCents:
				t.Errorf("detected %.1f Hz — an OCTAVE HIGH (want %.0f)", est.Hz, f0)
			case centsApart(est.Hz, f0/2) <= tolCents:
				t.Errorf("detected %.1f Hz — an OCTAVE LOW (want %.0f)", est.Hz, f0)
			case centsApart(est.Hz, f0*3) <= tolCents:
				t.Errorf("detected %.1f Hz — a TWELFTH HIGH (want %.0f)", est.Hz, f0)
			default:
				t.Errorf("detected %.1f Hz — off by %.0f cents (want %.0f)", est.Hz, off, f0)
			}
		})
	}
}

// Otherwise the fundamentals table would depend on the generator's render rate.
func TestDetectF0_IsSampleRateIndependent(t *testing.T) {
	const f0 = 220.0
	const tolCents = 20
	var first float64
	for i, sr := range []int{44100, 48000, 96000} {
		w := harmonics(sr, 1.5, f0, []float64{1, 2, 3}, []float64{1, .5, .3})
		est := DetectF0Detailed(w)
		if i == 0 {
			first = est.Hz
		}
		if got := centsApart(est.Hz, f0); got > tolCents {
			t.Errorf("sr=%d: detected %.2f Hz, off by %.1f cents", sr, est.Hz, got)
		}
		if got := centsApart(est.Hz, first); got > tolCents {
			t.Errorf("sr=%d: detected %.2f Hz, %.1f cents from the 44.1 kHz answer %.2f",
				sr, est.Hz, got, first)
		}
	}
}

// The fundamentals table is committed and diffed, so a wobbling detector would fail the drift guard at random.
func TestDetectF0_IsDeterministic(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	w := synth(supportTestSR, 1.5, func(t float64) float64 {
		return math.Sin(2*math.Pi*220*t) + 0.3*(rng.Float64()*2-1)
	})
	first := DetectF0Detailed(w)
	for i := 0; i < 5; i++ {
		got := DetectF0Detailed(w)
		if got != first {
			t.Fatalf("run %d returned %+v, first run returned %+v", i+1, got, first)
		}
		sup1, sup2 := AnalyzeF0Support(w, first.Hz), AnalyzeF0Support(w, first.Hz)
		if sup1 != sup2 {
			t.Fatalf("AnalyzeF0Support is not deterministic: %+v vs %+v", sup1, sup2)
		}
	}
}

// A DC offset arrives from an unbalanced render or an asymmetric waveshaper.
func TestDetectF0_IsImmuneToPhaseAndDCOffset(t *testing.T) {
	const f0 = 220.0
	const tolCents = 20
	base := DetectF0Detailed(synth(supportTestSR, 1.5, func(t float64) float64 {
		return math.Sin(2 * math.Pi * f0 * t)
	}))

	for _, phase := range []float64{0.25, 0.5, 1.0, 2.0} {
		est := DetectF0Detailed(synth(supportTestSR, 1.5, func(t float64) float64 {
			return math.Sin(2*math.Pi*f0*t + phase)
		}))
		if got := centsApart(est.Hz, base.Hz); got > tolCents {
			t.Errorf("phase %.2f rad: detected %.2f Hz, %.1f cents from the zero-phase answer %.2f",
				phase, est.Hz, got, base.Hz)
		}
	}

	for _, dc := range []float64{0.1, 0.5, 2.0} {
		est := DetectF0Detailed(synth(supportTestSR, 1.5, func(t float64) float64 {
			return math.Sin(2*math.Pi*f0*t) + dc
		}))
		if got := centsApart(est.Hz, f0); got > tolCents {
			t.Errorf("DC offset %.1f: detected %.2f Hz, off by %.1f cents", dc, est.Hz, got)
		}
	}
}

// The pitched/unpitched decision leans on it.
func TestDetectF0_ConfidenceFallsAsNoiseRises(t *testing.T) {
	var confs []float64
	for _, noise := range []float64{0, 0.5, 2, 8} {
		rng := rand.New(rand.NewSource(23)) // same noise sequence, scaled
		w := synth(supportTestSR, 1.5, func(t float64) float64 {
			return math.Sin(2*math.Pi*220*t) + noise*(rng.Float64()*2-1)
		})
		confs = append(confs, DetectF0Detailed(w).Confidence)
	}
	if confs[0] <= confs[len(confs)-1] {
		t.Errorf("confidence did not fall with noise: clean %.3f vs noisiest %.3f (%v)",
			confs[0], confs[len(confs)-1], confs)
	}
	if confs[0] < 0.9 {
		t.Errorf("a clean tone scored only %.3f confidence", confs[0])
	}
}

func TestDetectF0_SearchRangeEdges(t *testing.T) {
	// Below the floor: pins at the floor and must be flagged, NOT trusted.
	for _, f0 := range []float64{20, 30, 44, 49} {
		w := harmonics(supportTestSR, 1.5, f0, []float64{1, 2, 3}, []float64{1, .5, .3})
		est := DetectF0Detailed(w)
		if !est.F0AtSearchFloor() {
			t.Errorf("f0=%.0f Hz (below the %d Hz floor): F0AtSearchFloor false, reported %.2f Hz",
				f0, F0SearchMinHz, est.Hz)
		}
	}

	// Comfortably inside the range: not flagged.
	for _, f0 := range []float64{60, 110, 440} {
		w := harmonics(supportTestSR, 1.5, f0, []float64{1, 2, 3}, []float64{1, .5, .3})
		if est := DetectF0Detailed(w); est.F0AtSearchFloor() {
			t.Errorf("f0=%.0f Hz is well inside the range but was flagged as floor-pinned (%.2f Hz)",
				f0, est.Hz)
		}
	}

	// The detector cannot resolve a period that short, so the result must not look accurate.
	w := synth(supportTestSR, 1.5, func(t float64) float64 {
		return math.Sin(2 * math.Pi * 3000 * t)
	})
	est := DetectF0Detailed(w)
	if est.Hz > float64(F0SearchMaxHz) {
		t.Errorf("reported %.2f Hz, above the %d Hz search ceiling — the bound is not being applied",
			est.Hz, F0SearchMaxHz)
	}
}

func TestDetectF0_HandlesShortAndOddLengthBuffers(t *testing.T) {
	for _, n := range []int{5, 100, 4095, 4096, 4097, 6143, 8191} {
		s := make([]float64, n)
		for i := range s {
			s[i] = math.Sin(2 * math.Pi * 220 * float64(i) / supportTestSR)
		}
		w := wave.Wave{Samples: s, SampleRate: supportTestSR}
		est := DetectF0Detailed(w) // a panic is the failure
		if est.Hz < 0 || math.IsNaN(est.Hz) || math.IsInf(est.Hz, 0) {
			t.Errorf("n=%d: returned a nonsense f0 %v", n, est.Hz)
		}
		if est.Confidence < 0 || est.Confidence > 1 || math.IsNaN(est.Confidence) {
			t.Errorf("n=%d: confidence %v is outside 0..1", n, est.Confidence)
		}
	}
}
