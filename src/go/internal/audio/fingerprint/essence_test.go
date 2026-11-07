package fingerprint

import (
	"math"
	"math/rand"
	"sort"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const essSR = 44100

// amps[k] is partial k+1's amplitude.
func essTone(f0 float64, amps []float64, durSec float64, env func(t float64) float64) wave.Wave {
	n := int(durSec * essSR)
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / essSR
		var v float64
		for k, a := range amps {
			v += a * math.Sin(2*math.Pi*f0*float64(k+1)*t)
		}
		s[i] = v * env(t)
	}
	return wave.Wave{Samples: s, SampleRate: essSR}
}

func essHeld(t float64) float64 { return math.Min(1, t/0.02) }
func essPluck(t float64) float64 {
	return math.Min(1, t/0.002) * math.Exp(-t/0.12)
}

// essAddNoise adds white noise at levelDB relative to w's peak.
func essAddNoise(w wave.Wave, levelDB float64, seed int64) wave.Wave {
	out := w.Clone()
	amp := w.PeakSample() * math.Pow(10, levelDB/20)
	r := rand.New(rand.NewSource(seed))
	for i := range out.Samples {
		out.Samples[i] += amp * (2*r.Float64() - 1)
	}
	return out
}

func essScale(w wave.Wave, g float64) wave.Wave {
	out := w.Clone()
	for i := range out.Samples {
		out.Samples[i] *= g
	}
	return out
}

var (
	essSaw    = []float64{1, 1. / 2, 1. / 3, 1. / 4, 1. / 5, 1. / 6, 1. / 7, 1. / 8, 1. / 9, 1. / 10}
	essHollow = []float64{1, 0, 1. / 3, 0, 1. / 5, 0, 1. / 7, 0, 1. / 9, 0} // clarinet-like: odd only
)

func essTermsOf(t *testing.T, ref, cand wave.Wave) EssenceScore {
	t.Helper()
	r := AnalyzeEssence(ref, EssenceOpts{})
	c := AnalyzeEssence(cand, EssenceOpts{WindowSec: r.WindowSec})
	return EssenceTerms(r, c)
}

func essSum(s EssenceScore) float64 {
	return s.Color + s.Evolution + s.Texture + s.Decay + s.Attack
}

func TestEssenceIdentityIsZero(t *testing.T) {
	w := essTone(220, essSaw, 1.5, essHeld)
	if s := essTermsOf(t, w, w); essSum(s) != 0 {
		t.Fatalf("identical clips must be at distance 0, got %+v", s)
	}
}

// Level is a nuisance: the same sound 20 dB quieter is the same instrument.
func TestEssenceIgnoresLevel(t *testing.T) {
	w := essTone(220, essSaw, 1.5, essPluck)
	if s := essTermsOf(t, w, essScale(w, 0.1)); essSum(s) > 1e-9 {
		t.Fatalf("a level change must not move any term, got %+v", s)
	}
}

// An absolute floor let inaudible room tone dominate: a -60 dB floor must move colour far less than changing the
// harmonic recipe.
func TestEssenceNoiseFloorDoesNotDominate(t *testing.T) {
	saw := essTone(220, essSaw, 1.5, essHeld)
	hollow := essTone(220, essHollow, 1.5, essHeld)
	noisy := essTermsOf(t, essAddNoise(saw, -60, 1), saw)
	other := essTermsOf(t, saw, hollow)
	if noisy.Color > 0.2*other.Color {
		t.Fatalf("a -60 dB noise floor moved Color %.4f, vs %.4f for a different harmonic recipe", noisy.Color, other.Color)
	}
}

// It grows steadily with the noise component, and a -60 dB floor stays negligible beside a -12 dB breath.
func TestEssenceTextureSeesNoise(t *testing.T) {
	clean := essTone(330, essSaw, 1.5, essHeld)
	prev := -1.0
	var floor, breath float64
	for _, db := range []float64{-60, -30, -20, -12, -6} {
		tex := essTermsOf(t, clean, essAddNoise(clean, db, 2)).Texture
		if tex <= prev {
			t.Fatalf("texture must grow with the noise level: %.4f at %v dB after %.4f", tex, db, prev)
		}
		prev = tex
		switch db {
		case -60:
			floor = tex
		case -12:
			breath = tex
		}
	}
	if breath < 50*floor {
		t.Fatalf("a -12 dB breath (%.4f) must dwarf a -60 dB floor (%.4f)", breath, floor)
	}
}

// How the note dies away IS the kind of instrument.
func TestEssenceDecaySeparatesPluckFromHeld(t *testing.T) {
	held := essTone(220, essSaw, 1.0, essHeld)
	pluck := essTone(220, essSaw, 1.0, func(t float64) float64 { return math.Min(1, t/0.002) * math.Exp(-t/0.15) })
	if s := essTermsOf(t, held, pluck); s.Decay < 0.3 {
		t.Fatalf("pluck vs held decay term %.3f, want a large difference", s.Decay)
	}
	if s := essTermsOf(t, pluck, held); s.Decay < 0.3 {
		t.Fatalf("held vs a plucked reference: decay term %.3f, want a large difference", s.Decay)
	}
}

// Measured mostly where the direct sound dominates.
func TestEssenceDecayIsRobustToAReverbTail(t *testing.T) {
	env := func(t float64) float64 { return math.Min(1, t/0.002) * math.Exp(-t/0.15) }
	dry := essTone(220, essSaw, 1.5, env)
	wet := dry.Clone()
	r := rand.New(rand.NewSource(5))
	amp := dry.PeakSample() * math.Pow(10, -20.0/20)
	for i := range wet.Samples {
		x := float64(i) / essSR
		wet.Samples[i] += amp * math.Min(1, x/0.03) * math.Exp(-x/0.5) * (2*r.Float64() - 1)
	}
	held := essTone(220, essSaw, 1.5, essHeld)
	room := essTermsOf(t, wet, dry).Decay
	kind := essTermsOf(t, wet, held).Decay
	if room > 0.4*kind {
		t.Fatalf("a -20 dB room tail moved Decay %.3f, a held note %.3f", room, kind)
	}
}

// Note length is the player's, not the instrument's.
func TestEssenceDecayIgnoresHowLongANoteIsHeld(t *testing.T) {
	ref := essTone(220, essSaw, 1.0, essHeld)
	long := essTone(220, essSaw, 3.0, essHeld)
	if s := essTermsOf(t, ref, long); s.Decay > 0.02 {
		t.Fatalf("holding the same note longer moved Decay %.3f", s.Decay)
	}
}

// Whatever a candidate does after the reference died away is not part of the match.
func TestEssenceCandidateIsAnalysedOverTheReferenceWindow(t *testing.T) {
	burst := essTone(220, essSaw, 0.6, essPluck)
	ref := wave.Wave{Samples: append(append([]float64(nil), burst.Samples...), make([]float64, essSR)...), SampleRate: essSR}
	cand := ref.Clone()
	late := essTone(1500, essHollow, 0.5, essHeld)
	copy(cand.Samples[int(1.0*essSR):], late.Samples)
	r := AnalyzeEssence(ref, EssenceOpts{})
	if r.WindowSec > 0.95 {
		t.Fatalf("reference window %.2f s should end where the burst dies (< 1 s)", r.WindowSec)
	}
	if s := essTermsOf(t, ref, cand); essSum(s) > 1e-9 {
		t.Fatalf("sound after the reference window must not count, got %+v", s)
	}
}

func TestEssenceWindowEndsWhereTheReferenceDies(t *testing.T) {
	// exp(-t/0.12) in amplitude: power -30 dB at t = 0.06*ln(1000) = 0.41 s.
	w := essTone(220, essSaw, 2.0, essPluck)
	e := AnalyzeEssence(w, EssenceOpts{})
	if e.WindowSec < 0.35 || e.WindowSec > 0.6 {
		t.Fatalf("window %.3f s, want ~0.41 s (30 dB below the peak)", e.WindowSec)
	}
}

// A reverb tail 30 dB down must not out-vote the note.
func TestEssenceTimeAveragesFollowTheLoudPart(t *testing.T) {
	dry := essTone(110, essSaw, 0.3, essPluck)
	n := int(1.2 * essSR)
	wet := wave.Wave{Samples: make([]float64, n), SampleRate: essSR}
	copy(wet.Samples, dry.Samples)
	r := rand.New(rand.NewSource(4))
	tail := dry.PeakSample() * math.Pow(10, -30.0/20)
	for i := len(dry.Samples) / 2; i < n; i++ {
		x := float64(i-len(dry.Samples)/2) / essSR
		wet.Samples[i] += tail * math.Exp(-x/0.6) * (2*r.Float64() - 1)
	}
	other := essTone(110, essHollow, 0.3, essPluck)
	room := essTermsOf(t, wet, dry)
	inst := essTermsOf(t, dry, other)
	if room.Color > 0.5*inst.Color {
		t.Fatalf("a -30 dB room tail moved Color %.3f, a different instrument %.3f", room.Color, inst.Color)
	}
}

func TestEssenceAttackTime(t *testing.T) {
	slow := essTone(220, essSaw, 1.0, func(t float64) float64 { return math.Min(1, t/0.25) })
	fast := essTone(220, essSaw, 1.0, essPluck)
	es, ef := AnalyzeEssence(slow, EssenceOpts{}), AnalyzeEssence(fast, EssenceOpts{})
	if es.AttackSec < 0.12 || es.AttackSec > 0.3 {
		t.Fatalf("slow attack measured %.3f s, want ~0.2 s (10->90%% of a 0.25 s ramp)", es.AttackSec)
	}
	if ef.AttackSec > 0.01 {
		t.Fatalf("fast attack measured %.4f s, want < 10 ms", ef.AttackSec)
	}
	// 2 ms and 6 ms are the same onset; 6 ms and 60 ms are not.
	pluck6 := essTone(220, essSaw, 1.0, func(t float64) float64 { return math.Min(1, t/0.0075) * math.Exp(-t/0.12) })
	swell := essTone(220, essSaw, 1.0, func(t float64) float64 { return math.Min(1, t/0.075) * math.Exp(-t/0.3) })
	e6, ew := AnalyzeEssence(pluck6, EssenceOpts{}), AnalyzeEssence(swell, EssenceOpts{})
	if a := EssenceTerms(ef, e6).Attack; a != 0 {
		t.Fatalf("2 ms vs 6 ms onsets differ in Attack by %.3f, want 0 (both under the ear's onset resolution)", a)
	}
	if a := EssenceTerms(e6, ew).Attack; a < 0.5 {
		t.Fatalf("6 ms pluck vs 60 ms swell Attack %.3f, want a clear difference", a)
	}
}

// Same average colour budget, different movement.
func TestEssenceEvolutionSeesAColourChange(t *testing.T) {
	steady := essTone(220, essSaw, 1.0, essHeld)
	n := len(steady.Samples)
	moving := wave.Wave{Samples: make([]float64, n), SampleRate: essSR}
	bright := essTone(220, essSaw, 1.0, essHeld)
	dark := essTone(220, []float64{1, 0.2, 0.05}, 1.0, essHeld)
	for i := 0; i < n; i++ {
		x := math.Min(1, float64(i)/float64(n/2))
		moving.Samples[i] = (1-x)*bright.Samples[i] + x*dark.Samples[i]
	}
	s := essTermsOf(t, steady, moving)
	if s.Evolution <= 0.02 {
		t.Fatalf("a bright->dark sweep must register as Evolution, got %.4f", s.Evolution)
	}
}

func TestEssenceSilentCandidateIsFarthest(t *testing.T) {
	w := essTone(220, essSaw, 1.0, essHeld)
	r := AnalyzeEssence(w, EssenceOpts{})
	silent := AnalyzeEssence(wave.Silence(1.0, essSR), EssenceOpts{WindowSec: r.WindowSec})
	other := AnalyzeEssence(essTone(220, essHollow, 1.0, essPluck), EssenceOpts{WindowSec: r.WindowSec})
	tot := EssenceTotals(r, []Essence{other, silent})
	if !math.IsInf(tot[1], 1) || math.IsInf(tot[0], 0) {
		t.Fatalf("silence must rank last (+Inf), got %v", tot)
	}
}

// The total is the weighted sum of the terms as measured.
func TestEssenceTotalsWeighTheTerms(t *testing.T) {
	s := []EssenceScore{{Color: 0.2, Evolution: 0.1, Texture: 0.05, Decay: 0.2, Attack: 1}}
	w := EssenceWeights
	want := w.Color*0.2 + w.Evolution*0.1 + w.Texture*0.05 + w.Decay*0.2 + w.Attack*1
	if got := essenceTotalsOf(s)[0]; math.Abs(got-want) > 1e-12 {
		t.Fatalf("total %.6f, want %.6f", got, want)
	}
}

// Level never moves any of them.
func TestEssenceTraitsPointTheRightWay(t *testing.T) {
	bright := AnalyzeEssence(essTone(220, essSaw, 1.0, essHeld), EssenceOpts{})
	dark := AnalyzeEssence(essTone(220, []float64{1, 0.2, 0.05}, 1.0, essHeld), EssenceOpts{})
	if b, d := bright.Traits().BrightnessHz, dark.Traits().BrightnessHz; b <= d*1.5 {
		t.Fatalf("saw brightness %.0f Hz must sit well above a near-sine's %.0f Hz", b, d)
	}
	clean := essTone(220, essSaw, 1.0, essHeld)
	noisy := AnalyzeEssence(essAddNoise(clean, -6, 3), EssenceOpts{})
	if n, c := noisy.Traits().Noise, bright.Traits().Noise; n <= c+0.05 {
		t.Fatalf("a -6 dB noise bed must raise the noise share: %.3f vs clean %.3f", n, c)
	}
	plucked := AnalyzeEssence(essTone(220, essSaw, 1.0, essPluck), EssenceOpts{})
	if p, h := plucked.Traits().DecayDB, bright.Traits().DecayDB; p >= h-6 {
		t.Fatalf("a pluck must sit well below a held note after its peak: %.1f vs %.1f dB", p, h)
	}
	quiet := AnalyzeEssence(essScale(clean, 0.05), EssenceOpts{}).Traits()
	if loud := bright.Traits(); math.Abs(quiet.BrightnessHz-loud.BrightnessHz) > 1 || math.Abs(quiet.Noise-loud.Noise) > 1e-6 || math.Abs(quiet.DecayDB-loud.DecayDB) > 1e-6 {
		t.Fatalf("level moved the traits: %+v vs %+v", quiet, loud)
	}
}

// A +0.5 dB bump late in the plateau puts the arg-max there; it must still read as held.
func TestEssenceDecayIgnoresWhereTheHeldPlateauPeaks(t *testing.T) {
	early := essTone(220, essSaw, 1.0, essHeld)
	late := essTone(220, essSaw, 1.0, func(t float64) float64 {
		g := essHeld(t)
		if t > 0.85 && t < 0.92 {
			g *= 1.06 // +0.5 dB, the kind of swell a bow or breath makes
		}
		return g
	})
	for name, w := range map[string]wave.Wave{"early": early, "late": late} {
		e := AnalyzeEssence(w, EssenceOpts{})
		if d := e.Traits().DecayDB; d < -2 {
			t.Errorf("%s-peaked held tone reads %.1f dB of decay; a held note must read ~0", name, d)
		}
	}
	a := AnalyzeEssence(early, EssenceOpts{})
	b := AnalyzeEssence(late, EssenceOpts{WindowSec: a.WindowSec})
	if s := EssenceTerms(a, b); s.Decay > 0.05 {
		t.Fatalf("the same held tone with a late swell differs in Decay by %.3f", s.Decay)
	}
}

// Even counts take the mean of the two middles; duplicates included.
func TestEssMedianIsTheSortedMiddle(t *testing.T) {
	r := rand.New(rand.NewSource(13))
	for n := 1; n <= 300; n++ {
		for rep := 0; rep < 4; rep++ {
			v := make([]float64, n)
			for i := range v {
				v[i] = r.ExpFloat64()
				if rep%2 == 1 {
					v[i] = float64(r.Intn(5)) // many ties
				}
			}
			c := append([]float64(nil), v...)
			sort.Float64s(c)
			want := c[n/2]
			if n%2 == 0 {
				want = (c[n/2-1] + c[n/2]) / 2
			}
			if got := essMedian(v); math.Float64bits(got) != math.Float64bits(want) {
				t.Fatalf("n=%d rep %d: median %v, sorted middle %v", n, rep, got, want)
			}
		}
	}
	if essMedian(nil) != 0 {
		t.Fatal("empty median must be 0")
	}
}
