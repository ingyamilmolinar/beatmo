package clone

import (
	"math"
	"math/rand"
	"testing"
)

func lossFixture(t *testing.T) (*LossRef, []Mode) {
	t.Helper()
	modes := barModes()
	x := modalSignal(modes, 48000, 0.05, 1.5, 0, 1)
	p, err := Prepare(x, 48000, PrepareOptions{Label: "loss"})
	if err != nil {
		t.Fatal(err)
	}
	return NewLossRef(p, 1.2), modes
}

func TestLossOfTheReferenceItselfIsZero(t *testing.T) {
	r, modes := lossFixture(t)
	// Same modes, other level, other leading silence: alignment + equal peak
	// make it the same signal.
	cand := append(make([]float64, 300), Synthesize(modes, 48000, 72000)...)
	for i := range cand {
		cand[i] *= 0.37
	}
	rep := r.Score(cand, 0, DefaultLossWeights, LossReport{})
	if rep.LogMagDB > 1e-6 || rep.SC > 1e-6 || rep.Envelope > 1e-6 || rep.Descriptors > 1e-6 || rep.Total > 1e-5 {
		t.Fatalf("self loss %+v, want 0", rep)
	}
}

func TestLossRisesWithEachKindOfError(t *testing.T) {
	r, modes := lossFixture(t)
	score := func(ms []Mode, noise float64) LossReport {
		x := Synthesize(ms, 48000, 72000)
		rng := rand.New(rand.NewSource(3))
		for i := range x {
			x[i] += noise * rng.NormFloat64()
		}
		return r.Score(x, 0, DefaultLossWeights, LossReport{})
	}
	detune := func(c float64) []Mode {
		ms := append([]Mode(nil), modes...)
		ms[0].Freq *= math.Pow(2, c/1200)
		return ms
	}
	faster := func(x float64) []Mode {
		ms := append([]Mode(nil), modes...)
		for i := range ms {
			ms[i].Alpha *= x
		}
		return ms
	}
	seq := map[string][]LossReport{
		"detune": {score(detune(0), 0), score(detune(10), 0), score(detune(40), 0), score(detune(150), 0)},
		"decay":  {score(faster(1), 0), score(faster(1.3), 0), score(faster(2), 0), score(faster(4), 0)},
		"noise":  {score(modes, 0), score(modes, 1e-3), score(modes, 1e-2), score(modes, 5e-2)},
	}
	for name, s := range seq {
		for i := 1; i < len(s); i++ {
			if !(s[i].MRSTFT > s[i-1].MRSTFT) {
				t.Errorf("%s: MR-STFT not rising at step %d: %.4f → %.4f", name, i, s[i-1].MRSTFT, s[i].MRSTFT)
			}
		}
		t.Logf("%s MR-STFT: %.4f %.4f %.4f %.4f  env: %.2f %.2f %.2f %.2f", name,
			s[0].MRSTFT, s[1].MRSTFT, s[2].MRSTFT, s[3].MRSTFT, s[0].Envelope, s[1].Envelope, s[2].Envelope, s[3].Envelope)
	}
	if d := seq["decay"]; !(d[3].Envelope > d[1].Envelope) {
		t.Errorf("envelope blind to a 4× decay: %.2f vs %.2f", d[3].Envelope, d[1].Envelope)
	}
}

func TestLossScalesNormaliseTheTotal(t *testing.T) {
	r, modes := lossFixture(t)
	ms := append([]Mode(nil), modes...)
	ms[1].Alpha *= 2
	x := Synthesize(ms, 48000, 72000)
	raw := r.Score(x, 3, DefaultLossWeights, LossReport{})
	norm := r.Score(x, 3, DefaultLossWeights, raw)
	if math.Abs(norm.Total-4) > 1e-9 && !(raw.Descriptors == 0 && math.Abs(norm.Total-3) < 1e-9) {
		t.Fatalf("each term ÷ itself should sum to the number of live terms, got %.6f (%+v)", norm.Total, raw)
	}
}

func TestPriorTermMeasuresTheDocumentAgainstTheAnalysis(t *testing.T) {
	a := analysisOf(eightModes(), PitchInfo{Pitched: true, F0Hz: 261.63})
	d, rep, err := Invert(a, InvertOptions{ID: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if p := PriorTerm(d, a, rep.FitPitch, 48000); p > 1e-9 {
		t.Fatalf("prior of the inverted document = %g, want 0", p)
	}
	st := d.Stages["modal"]
	st.Values["modal_decay_x"] = 2 // +6.02 dB on every decay
	if p := PriorTerm(d, a, rep.FitPitch, 48000); math.Abs(p-20*math.Log10(2)) > 1e-6 {
		t.Fatalf("prior with Decay ×2 = %g, want 6.02", p)
	}
}

func BenchmarkLossScore(b *testing.B) {
	modes := barModes()
	x := modalSignal(modes, 48000, 0.05, 3, 1e-4, 1)
	p, err := Prepare(x, 48000, PrepareOptions{Label: "bench"})
	if err != nil {
		b.Fatal(err)
	}
	r := NewLossRef(p, 3)
	cand := Synthesize(modes, 48000, 3*48000)
	for _, w := range []struct {
		name string
		w    LossWeights
	}{{"all", DefaultLossWeights}, {"no-descriptors", LossWeights{MRSTFT: 1, Envelope: 1, Prior: 1}}} {
		b.Run(w.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				r.Score(cand, 0, w.w, LossReport{})
			}
		})
	}
}
