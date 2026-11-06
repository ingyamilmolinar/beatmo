package soundview

import (
	"math"
	"math/rand"
	"testing"
)

// What the simplified Synth-tab views read (spec 2026-10-25 §3.3): a spectrum
// that keeps its absolute level (curves share ONE scale), the first moment at
// 1 ms with how far a click stands over the body, whether a note is pitched,
// and analyses that stop where the note does.

func TestSpectrumCarriesItsAbsoluteLevel(t *testing.T) {
	loud := New(gen(1, func(t float64) float64 { return 0.5 * math.Sin(2*math.Pi*440*t) }), sr, 1)
	quiet := New(gen(1, func(t float64) float64 { return 0.5 * math.Pow(10, -10.0/20) * math.Sin(2*math.Pi*440*t) }), sr, 1)
	d := loud.Spectrum().RefDB - quiet.Spectrum().RefDB
	if math.Abs(d-10) > 0.5 {
		t.Fatalf("a note 10 dB quieter has RefDB %.2f dB lower, want 10", d)
	}
}

func TestOnsetCurveShowsAFadeIn(t *testing.T) {
	a := New(gen(0.5, func(t float64) float64 { return 0.6 * math.Min(1, t/0.1) * math.Sin(2*math.Pi*220*t) }), sr, 0.5)
	c := a.OnsetCurve()
	if c.StepSec != 0.001 || len(c.DB) < 150 {
		t.Fatalf("onset curve shape: step %v, %d points", c.StepSec, len(c.DB))
	}
	at := func(sec float64) float64 { return c.DB[int(sec/c.StepSec)] }
	if at(0.01) > at(0.1)-12 {
		t.Fatalf("a 100 ms fade-in reads %.1f dB at 10 ms and %.1f dB at 100 ms: no rise", at(0.01), at(0.1))
	}
	if a.ClickDB() > 3 {
		t.Fatalf("a fade-in has no click, got ClickDB %.1f", a.ClickDB())
	}
}

func TestClickDBIsHowFarTheFirstMomentStandsOverTheBody(t *testing.T) {
	body := 0.6 * math.Pow(10, -12.0/20)
	a := New(gen(0.5, func(t float64) float64 {
		click := 0.0
		if t < 0.002 {
			click = 0.6 * math.Sin(2*math.Pi*3000*t)
		}
		return click + body*math.Sin(2*math.Pi*110*t)
	}), sr, 0.5)
	if c := a.ClickDB(); math.Abs(c-12) > 4 {
		t.Fatalf("a full-scale 2 ms click over a −12 dB body reads ClickDB %.1f, want ≈ 12", c)
	}
	plain := New(gen(0.5, func(t float64) float64 { return 0.6 * math.Exp(-t/0.3) * math.Sin(2*math.Pi*110*t) }), sr, 0.5)
	if c := plain.ClickDB(); c > 3 {
		t.Fatalf("a plain decaying tone reads ClickDB %.1f, want < 3", c)
	}
}

func TestPitchedTellsAToneFromNoise(t *testing.T) {
	tone := New(gen(0.6, func(t float64) float64 { return 0.6 * math.Sin(2*math.Pi*220*t) }), sr, 0.6)
	if !tone.Pitched() {
		t.Fatal("a 220 Hz tone is not pitched")
	}
	rng := rand.New(rand.NewSource(7))
	noise := New(gen(0.6, func(float64) float64 { return 0.4 * rng.NormFloat64() }), sr, 0.6)
	if noise.Pitched() {
		t.Fatal("white noise reads as pitched")
	}
}

// A 0.15 s hit in a 2 s buffer: the transform stops shortly after the note
// does instead of analysing 1.8 s of silence.
func TestAnalysesStopWhereTheNoteDoes(t *testing.T) {
	a := New(gen(2, func(t float64) float64 {
		if t > 0.15 {
			return 0
		}
		return 0.6 * math.Sin(2*math.Pi*220*t)
	}), sr, 2)
	sg := a.Spectrogram()
	if end := sg.T0 + float64(sg.Cols)*sg.HopSec; end > 0.4 {
		t.Fatalf("the spectrogram runs to %.2f s for a note that ends at 0.15 s", end)
	}
	tr := a.Pitch()
	if end := tr.T0 + float64(len(tr.Hz))*tr.HopSec; end > 0.4 {
		t.Fatalf("the pitch track runs to %.2f s for a note that ends at 0.15 s", end)
	}
}
