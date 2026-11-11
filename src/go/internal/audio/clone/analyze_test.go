package clone

import (
	"encoding/json"
	"math"
	"math/rand"
	"reflect"
	"testing"
)

func TestAnalyzeEndToEnd(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 2.5, 0.004, 31)
	p, err := Prepare(x, 48000, PrepareOptions{Label: "bar"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("timings %+v", a.Timings)
	if a.Format != AnalysisFormat || len(a.Modes) < 4 || a.Residual.EnergyFraction > 0.1 {
		t.Fatalf("analysis %+v", a)
	}
	if math.Abs(a.GlideCents) > 2 {
		t.Fatalf("glide %.2f cents on fixed-frequency modes", a.GlideCents)
	}
	b, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Modes, b.Modes) || !reflect.DeepEqual(a.Strike, b.Strike) || !reflect.DeepEqual(a.Residual, b.Residual) {
		t.Fatal("Analyze is not deterministic")
	}
}

func TestAnalyzeNoiselessJSONIsFinite(t *testing.T) {
	x := modalSignal(barModes(), 48000, 0.05, 1.5, 0, 0)
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatalf("noiseless analysis is not valid JSON: %v", err)
	}
}

func TestAnalyzeRefusesASustainedNoise(t *testing.T) {
	r := rand.New(rand.NewSource(41))
	x := make([]float64, 48000*2)
	for i := 2400; i < len(x); i++ {
		x[i] = 0.2 * r.NormFloat64() * math.Exp(-1.5*float64(i-2400)/48000)
	}
	for i := 0; i < 2400; i++ {
		x[i] = 0.0005 * r.NormFloat64()
	}
	p, err := Prepare(x, 48000, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, err := Analyze(p)
	if !IsRefusal(err, RefuseNotModal) {
		t.Fatalf("decaying noise: err = %v, want not_modal", err)
	}
	if a.Residual.EnergyFraction <= 0.5 || a.Format != AnalysisFormat {
		t.Fatalf("refusal must still return the partial analysis: %+v", a)
	}
}

func TestAnalyzeReportsAGlide(t *testing.T) {
	rate := 48000
	n := 2 * rate
	x := make([]float64, 2400+n)
	var ph float64
	for i := 0; i < n; i++ {
		tt := float64(i) / float64(rate)
		f := 200 * (1 + 0.03*math.Exp(-2*tt)) // a membrane's tension glide: 3 % sharp at the strike
		ph += 2 * math.Pi * f / float64(rate)
		x[2400+i] = 0.4 * math.Exp(-1.5*tt) * math.Cos(ph)
	}
	r := rand.New(rand.NewSource(42))
	for i := range x {
		x[i] += 0.0005 * r.NormFloat64()
	}
	p, err := Prepare(x, rate, PrepareOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := Analyze(p)
	if a.GlideCents > -10 {
		t.Fatalf("glide %.1f cents, want a downward drift > 10 cents", a.GlideCents)
	}
	if len(a.Notes) == 0 {
		t.Fatal("a glide must be reported in Notes")
	}
}
