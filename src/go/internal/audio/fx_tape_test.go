package audio

import (
	"math"
	"math/rand"
	"testing"
)

func TestTapeSaturationAddsHarmonics(t *testing.T) {
	tp := newTape(sr, map[string]float64{
		"drive": 5, "warmth": 0, "wow": 0, "flutter": 0, "mix": 1,
	})
	// Feed a 440 Hz sine for 1 second.
	n := sr
	out := processFX(tp, 440, sr, n)

	// Saturation should generate energy at the 3rd harmonic (1320 Hz).
	mag3rd := goertzelMagnitude(out, 1320, sr)
	if mag3rd < 0.001 {
		t.Fatalf("expected 3rd harmonic energy at 1320 Hz, got magnitude %v", mag3rd)
	}

	// Verify fundamental is still present.
	magFund := goertzelMagnitude(out, 440, sr)
	if magFund < 0.1 {
		t.Fatalf("expected fundamental at 440 Hz, got magnitude %v", magFund)
	}

	// 3rd harmonic should be weaker than fundamental.
	if mag3rd > magFund {
		t.Fatalf("3rd harmonic (%v) should be weaker than fundamental (%v)", mag3rd, magFund)
	}
}

func TestTapeWarmthAttenuatesHighFreq(t *testing.T) {
	tp := newTape(sr, map[string]float64{
		"drive": 1, "warmth": 1, "wow": 0, "flutter": 0, "mix": 1,
	})

	// Feed white noise.
	n := sr
	rng := rand.New(rand.NewSource(42))
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		x := rng.Float64()*2 - 1
		out[i] = tp.ProcessSample(x)
	}

	// With warmth=1 (LP cutoff around 2000 Hz), energy at 10 kHz should be
	// lower than energy at 500 Hz.
	mag500 := goertzelMagnitude(out, 500, sr)
	mag10k := goertzelMagnitude(out, 10000, sr)

	if mag10k >= mag500 {
		t.Fatalf("warmth=1 should attenuate highs: mag@500Hz=%v mag@10kHz=%v", mag500, mag10k)
	}
}

func TestTapeWowModulatesSignal(t *testing.T) {
	n := sr

	// Process with wow=0 (no modulation).
	tp0 := newTape(sr, map[string]float64{
		"drive": 2, "warmth": 0, "wow": 0, "flutter": 0, "mix": 1,
	})
	out0 := processFX(tp0, 440, sr, n)

	// Process with wow=1 (full modulation).
	tp1 := newTape(sr, map[string]float64{
		"drive": 2, "warmth": 0, "wow": 1, "flutter": 0, "mix": 1,
	})
	out1 := processFX(tp1, 440, sr, n)

	// Outputs should differ.
	diff := make([]float64, n)
	for i := range diff {
		diff[i] = out1[i] - out0[i]
	}
	diffRMS := rmsEnergy(diff)
	if diffRMS < 1e-6 {
		t.Fatalf("wow=1 should produce different output than wow=0, diffRMS=%v", diffRMS)
	}
}

func TestTapeFlutterModulatesSignal(t *testing.T) {
	n := sr

	// Process with flutter=0 (no modulation).
	tp0 := newTape(sr, map[string]float64{
		"drive": 2, "warmth": 0, "wow": 0, "flutter": 0, "mix": 1,
	})
	out0 := processFX(tp0, 440, sr, n)

	// Process with flutter=1 (full modulation).
	tp1 := newTape(sr, map[string]float64{
		"drive": 2, "warmth": 0, "wow": 0, "flutter": 1, "mix": 1,
	})
	out1 := processFX(tp1, 440, sr, n)

	// Outputs should differ.
	diff := make([]float64, n)
	for i := range diff {
		diff[i] = out1[i] - out0[i]
	}
	diffRMS := rmsEnergy(diff)
	if diffRMS < 1e-6 {
		t.Fatalf("flutter=1 should produce different output than flutter=0, diffRMS=%v", diffRMS)
	}
}

func TestTapeDryWhenMixZero(t *testing.T) {
	// Construct with mix=0 so the smoothParam starts at 0;
	// assertDryWhenMixZero also sets mix=0, so no ramp occurs.
	tp := newTape(sr, map[string]float64{
		"drive": 5, "warmth": 0.5, "wow": 0.5, "flutter": 0.5, "mix": 0,
	})
	assertDryWhenMixZero(t, tp, 440, sr, sr)
}

func TestTapeReset(t *testing.T) {
	tp := newTape(sr, DefaultParams(EffectTape))
	assertResetClearsState(t, tp)
}

func TestTapeNoNaNInf(t *testing.T) {
	tp := newTape(sr, map[string]float64{
		"drive": 10, "warmth": 1, "wow": 1, "flutter": 1, "mix": 1,
	})
	// Process a mix of extreme values.
	n := 4410
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		x := math.Sin(float64(i) * 0.3)
		if i%100 == 0 {
			x = 10 // occasional high amplitude
		}
		out[i] = tp.ProcessSample(x)
	}
	assertNoNaNOrInf(t, out)
}

func TestTapeModifiesSignal(t *testing.T) {
	tp := newTape(sr, map[string]float64{
		"drive": 5, "warmth": 0.5, "wow": 0, "flutter": 0, "mix": 1,
	})
	assertEffectModifiesSignal(t, tp, 440, sr, sr)
}
