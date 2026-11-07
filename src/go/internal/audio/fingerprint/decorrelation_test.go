package fingerprint

import (
	"math"
	"math/rand"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// lockedAdditive: 6 phase-locked harmonics under ONE slow amplitude envelope.
func lockedAdditive(sr int, dur float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		env := 1 + 0.3*math.Sin(2*math.Pi*0.7*t)
		for k := 1; k <= 6; k++ {
			s[i] += env * (0.3 / float64(k)) * math.Sin(2*math.Pi*220*float64(k)*t)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// Each harmonic with its OWN slow envelope and frequency wobble.
func independentAdditive(sr int, dur float64) wave.Wave {
	rng := rand.New(rand.NewSource(11))
	n := int(dur * float64(sr))
	s := make([]float64, n)
	type h struct{ rate, phase, wob float64 }
	hs := make([]h, 6)
	for k := range hs {
		hs[k] = h{0.4 + 1.5*rng.Float64(), 2 * math.Pi * rng.Float64(), 0.002 * rng.Float64()}
	}
	for i := range s {
		t := float64(i) / float64(sr)
		for k := 1; k <= 6; k++ {
			p := hs[k-1]
			env := 1 + 0.4*math.Sin(2*math.Pi*p.rate*t+p.phase)
			f := 220 * float64(k) * (1 + p.wob*math.Sin(2*math.Pi*1.3*t+p.phase))
			s[i] += env * (0.3 / float64(k)) * math.Sin(2*math.Pi*f*t)
		}
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestPartialIndependence_LockedVsIndependent(t *testing.T) {
	locked := NewBundle(lockedAdditive(44100, 3.0), "locked")
	free := NewBundle(independentAdditive(44100, 3.0), "free")
	li, ok := PartialIndependence(locked.Modes().Strong(20))
	if !ok || li > 0.15 {
		t.Fatalf("locked additive independence want < 0.15, got %.2f ok=%v", li, ok)
	}
	fi, ok := PartialIndependence(free.Modes().Strong(20))
	if !ok || fi < 0.3 {
		t.Fatalf("independent partials want > 0.3, got %.2f", fi)
	}
}

// Two locked partials: one sine yields one mode, and the bandwidth measure needs two.
func twoToneSteady(f1, f2, amp, dur float64, sr int) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tSec := float64(i) / float64(sr)
		s[i] = amp*math.Sin(2*math.Pi*f1*tSec) + amp*math.Sin(2*math.Pi*f2*tSec)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestPartialBandwidth_LockedTonesAreNarrow(t *testing.T) {
	b := NewBundle(twoToneSteady(440, 1320, 0.3, 3.0, 44100), "two-tone")
	bw, ok := PartialBandwidthCents(b.Wave(), b.Modes().Strong(20))
	if !ok {
		t.Fatalf("two steady locked tones should have a resolvable width")
	}
	if bw > 6 {
		t.Fatalf("locked sines should have near-zero excess bandwidth, got %.1f cents", bw)
	}
}

// Hand-built modes 12 Hz apart, both resolvable at the 32768-point analysis: each width must come from its OWN peak.
func TestPartialBandwidth_HillClimbStaysOnOwnPeak(t *testing.T) {
	f1, f2 := 600.0, 612.0
	w := twoToneSteady(f1, f2, 0.3, 3.0, 44100)
	modes := []Mode{{Hz: f1}, {Hz: f2}}
	bw, ok := PartialBandwidthCents(w, modes)
	if !ok {
		t.Fatalf("two 12Hz-separated tones should have a resolvable width")
	}
	if bw > 6 {
		t.Fatalf("width measured on its own peak should stay narrow, got %.1f cents", bw)
	}
}

// A wide chirp spreads energy near-continuously (noise has troughs that stop a descent early): a deterministic stand-in
// for a noisy shelf. A separate clean tone keeps two modes.
func chirpShelf(f0, f1, amp, dur float64, sr int) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	k := (f1 - f0) / dur // Hz/sec sweep rate
	for i := range s {
		tSec := float64(i) / float64(sr)
		phase := 2 * math.Pi * (f0*tSec + 0.5*k*tSec*tSec)
		s[i] = amp*math.Sin(phase) + 0.25*math.Sin(2*math.Pi*2000*tSec)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// A 5 Hz mode over a 1–200 Hz chirp has no -3 dB point within the walk; it must be skipped as unresolvable, never
// return NaN or Inf.
func TestPartialBandwidth_ChirpShelfNeverNaN(t *testing.T) {
	w := chirpShelf(1, 200, 0.3, 3.0, 44100)
	modes := []Mode{{Hz: 5}, {Hz: 2000}}
	bw, ok := PartialBandwidthCents(w, modes)
	if ok && (math.IsNaN(bw) || math.IsInf(bw, 0)) {
		t.Fatalf("bandwidth must be finite when ok=true, got %v", bw)
	}
	if !ok && bw != 0 {
		t.Fatalf("an unavailable result must report the zero value, got %v", bw)
	}
	t.Logf("chirp-shelf result: bw=%.2f ok=%v", bw, ok)
}

func TestPartialBandwidth_WobblyIsWider(t *testing.T) {
	b := NewBundle(independentAdditive(44100, 3.0), "free")
	bw, ok := PartialBandwidthCents(b.Wave(), b.Modes().Strong(20))
	if !ok || bw < 5 {
		t.Fatalf("frequency-wobbling partials should widen, got %.1f ok=%v", bw, ok)
	}
}

func TestPartialIndependence_ClampsToUnitInterval(t *testing.T) {
	// Perfectly anti-correlated tracks would drive 1 - r above 1.
	track := make([]float64, 40)
	anti := make([]float64, 40)
	for i := range track {
		track[i] = float64(i % 5)
		anti[i] = -float64(i % 5)
	}
	modes := []Mode{{Track: track}, {Track: anti}}
	v, ok := PartialIndependence(modes)
	if !ok {
		t.Fatalf("expected a measurable independence value")
	}
	if v < 0 || v > 1 {
		t.Fatalf("PartialIndependence must be clamped to [0,1], got %.3f", v)
	}
}

// With one flat track nothing moves together WITH it, so a zero denominator must not count as perfect lock.
func TestPearsonShared_OnlyBothFlatCountsAsLocked(t *testing.T) {
	n := decorrMinSharedFrames + 4
	flat := make([]float64, n)
	ramp := make([]float64, n) // pure trend -> flat AFTER detrending
	wiggle := make([]float64, n)
	for i := 0; i < n; i++ {
		flat[i] = -20
		ramp[i] = -20 - 0.5*float64(i)
		wiggle[i] = -20 + 3*math.Sin(float64(i))
	}

	if r, ok := pearsonShared(flat, flat); !ok || r != 1 {
		t.Errorf("both flat: got (%v, %v), want (1, true) — neither deviates from its own trend", r, ok)
	}
	// "Flat" means flat after its own trend is removed, so ramp vs flat is still the both-flat case.
	if r, ok := pearsonShared(ramp, flat); !ok || r != 1 {
		t.Errorf("ramp vs flat detrends to both-flat: got (%v, %v), want (1, true)", r, ok)
	}
	// One side genuinely moves, the other does not: undefined, so skipped.
	if r, ok := pearsonShared(flat, wiggle); ok {
		t.Errorf("one flat: got (%v, true); an undefined correlation must be skipped, not called perfect lock", r)
	}
	if r, ok := pearsonShared(wiggle, ramp); ok {
		t.Errorf("one flat (other order): got (%v, true); must be skipped", r)
	}
	if r, ok := pearsonShared(wiggle, wiggle); !ok || math.Abs(r-1) > 1e-9 {
		t.Errorf("identical moving tracks: got (%v, %v), want (1, true)", r, ok)
	}
}

// With every pair skipped the measurement is unavailable, the honest answer.
func TestPartialIndependence_FlatPartialDoesNotFakeLock(t *testing.T) {
	n := decorrMinSharedFrames + 6
	mk := func(fn func(i int) float64) Mode {
		tr := make([]float64, n)
		for i := range tr {
			tr[i] = fn(i)
		}
		return Mode{Hz: 440, Track: tr}
	}
	flat := mk(func(i int) float64 { return -20 })
	moving := mk(func(i int) float64 { return -20 + 4*math.Sin(float64(i)) })

	// Two modes, one of them flat: the only pair is undefined.
	if v, ok := PartialIndependence([]Mode{flat, moving}); ok {
		t.Errorf("independence = %v with the only pair undefined; want unavailable", v)
	}
	// Add a second moving mode and the measurable pair alone decides it.
	moving2 := mk(func(i int) float64 { return -20 + 4*math.Cos(float64(i)) })
	v, ok := PartialIndependence([]Mode{flat, moving, moving2})
	if !ok {
		t.Fatal("with one measurable pair, independence must be available")
	}
	if v < 0.5 {
		t.Errorf("independence = %.3f; sin vs cos are near-uncorrelated, so the flat "+
			"partial's fabricated r=1 must no longer be in the mean", v)
	}
}
