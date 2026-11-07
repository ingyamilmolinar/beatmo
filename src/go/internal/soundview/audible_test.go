package soundview

import (
	"math"
	"math/rand"
	"testing"
)

// Known answers for the oracle that judges dial relevance, its generator and the dial contracts.

func tone(sec, hz, gain float64) []float32 {
	return gen(sec, func(t float64) float64 {
		return gain * math.Exp(-t/0.6) * math.Sin(2*math.Pi*hz*t)
	})
}

func TestIdenticalNotesAreNotAudiblyDifferent(t *testing.T) {
	a := tone(1, 220, 0.8)
	c := CompareNotes(a, a, sr)
	if c.Audible() || c.GlobalDB != 0 || c.LocalDB != 0 || c.BandDB != 0 || c.Cents != 0 {
		t.Fatalf("identical notes: %+v, want no change", c)
	}
}

func TestALevelChangeIsAudibleOnlyAboveTheJND(t *testing.T) {
	a := tone(1, 220, 0.5)
	if c := CompareNotes(a, tone(1, 220, 0.5*math.Pow(10, 2.0/20)), sr); !c.Audible() || math.Abs(c.GlobalDB-2) > 0.2 {
		t.Fatalf("+2 dB: %+v, want audible with GlobalDB ≈ 2", c)
	}
	if c := CompareNotes(a, tone(1, 220, 0.5*math.Pow(10, 0.3/20)), sr); c.Audible() {
		t.Fatalf("+0.3 dB: %+v, want inaudible", c)
	}
}

func TestAChangeConfinedToPartOfTheNoteIsAudible(t *testing.T) {
	// A 150 ms fade-in on a 1.5 s note barely moves the whole-note mean but a
	// stretch of it differs by many dB.
	a := gen(1.5, func(t float64) float64 { return 0.5 * math.Sin(2*math.Pi*220*t) })
	b := gen(1.5, func(t float64) float64 { return 0.5 * math.Min(1, t/0.15) * math.Sin(2*math.Pi*220*t) })
	c := CompareNotes(a, b, sr)
	if !c.Audible() || c.LocalDB < AudibleLocalDB {
		t.Fatalf("fade-in: %+v, want audible through LocalDB ≥ %v", c, AudibleLocalDB)
	}
}

func TestAOneBandChangeIsAudible(t *testing.T) {
	// A partial at 2.5 kHz raised from -30 dB to -20 dB under a loud 220 Hz
	// tone: one critical band moves 10 dB.
	mk := func(g float64) []float32 {
		return gen(1, func(t float64) float64 {
			return 0.6*math.Sin(2*math.Pi*220*t) + g*math.Sin(2*math.Pi*2500*t)
		})
	}
	c := CompareNotes(mk(0.6*math.Pow(10, -30.0/20)), mk(0.6*math.Pow(10, -20.0/20)), sr)
	if !c.Audible() || c.BandDB < AudibleBandDB {
		t.Fatalf("one band: %+v, want audible through BandDB ≥ %v", c, AudibleBandDB)
	}
}

func TestAPitchShiftIsAudibleOnlyAboveFiveCents(t *testing.T) {
	// Steady, so the band envelope cannot see the shift (a decaying tone's leakage would be caught by the bands first).
	steady := func(hz float64) []float32 {
		return gen(1, func(t float64) float64 { return 0.6 * math.Sin(2*math.Pi*hz*t) })
	}
	a := steady(440)
	if c := CompareNotes(a, steady(440*math.Pow(2, 12.0/1200)), sr); !c.Audible() {
		t.Fatalf("+12 cents: %+v, want audible", c)
	} else if c.GlobalDB < AudibleMinDB && c.LocalDB < AudibleLocalDB && c.BandDB < AudibleBandDB && math.Abs(c.Cents-12) > 2 {
		t.Fatalf("+12 cents decided by the partial shift: %.1f cents, want ≈ 12", c.Cents)
	}
	a = tone(1, 440, 0.8)
	if c := CompareNotes(a, tone(1, 440*math.Pow(2, 2.0/1200), 0.8), sr); c.Audible() {
		t.Fatalf("+2 cents: %+v, want inaudible", c)
	}
}

func TestALongerNoteCountsAsTheChangeItIs(t *testing.T) {
	a := tone(0.3, 220, 0.8)
	b := tone(1.0, 220, 0.8)
	if c := CompareNotes(a, b, sr); !c.Audible() {
		t.Fatalf("0.3 s vs 1 s: %+v, want audible", c)
	}
}

func TestCompareNotesAcceptsFloat64Renders(t *testing.T) {
	a := tone(1, 220, 0.5)
	b := make([]float64, len(a))
	for i, v := range a {
		b[i] = float64(v) * math.Pow(10, 2.0/20)
	}
	if c := CompareNotes(b, b, sr); c.Audible() {
		t.Fatalf("float64 identical: %+v", c)
	}
	af := make([]float64, len(a))
	for i, v := range a {
		af[i] = float64(v)
	}
	if c := CompareNotes(af, b, sr); !c.Audible() {
		t.Fatalf("float64 +2 dB: %+v, want audible", c)
	}
}

func TestSilentIsAnAllButZeroRender(t *testing.T) {
	if !Silent(make([]float32, 4800)) {
		t.Fatal("zeros must be silent")
	}
	if !Silent([]float32{}) {
		t.Fatal("an empty render is silent")
	}
	rng := rand.New(rand.NewSource(1))
	faint := gen(0.1, func(float64) float64 { return 1e-6 * rng.NormFloat64() })
	if !Silent(faint) {
		t.Fatal("-120 dBFS noise is silent")
	}
	if Silent(tone(0.1, 220, 0.01)) {
		t.Fatal("a -40 dBFS tone is not silent")
	}
}
