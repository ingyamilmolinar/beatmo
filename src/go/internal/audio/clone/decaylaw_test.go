package clone

import (
	"math"
	"testing"
)

func TestFitDecayLawRecoversKnownLaw(t *testing.T) {
	law := DecayLaw{A: 2, P: 1.3, B: 30}
	var modes []Mode
	for i := 0; i < 20; i++ {
		f := 200 * math.Pow(60, float64(i)/19) // 200 Hz .. 12 kHz
		modes = append(modes, Mode{Freq: f, Alpha: law.Eval(f), Amp: 0.1})
	}
	got := FitDecayLaw(modes)
	if math.Abs(got.A/law.A-1) > 0.02 || math.Abs(got.P-law.P) > 0.02 || math.Abs(got.B/law.B-1) > 0.05 {
		t.Fatalf("fit %+v, want %+v", got, law)
	}
	if got.RelRMS > 0.01 {
		t.Fatalf("relative RMS error %.4f on an exact law", got.RelRMS)
	}
}

func TestFitDecayLawFewModes(t *testing.T) {
	got := FitDecayLaw([]Mode{{Freq: 440, Alpha: 2, Amp: 1}, {Freq: 880, Alpha: 4, Amp: 1}})
	// energies amp²/(4α) = 1/8 and 1/16 → mean α = (2/8 + 4/16) / (3/16) = 8/3
	if got.P != 0 || got.B != 0 || math.Abs(got.A-8.0/3) > 1e-9 {
		t.Fatalf("two modes: %+v, want a flat law at the energy-weighted mean 8/3", got)
	}
	if (FitDecayLaw(nil) != DecayLaw{}) {
		t.Fatal("no modes must give the zero law")
	}
}
