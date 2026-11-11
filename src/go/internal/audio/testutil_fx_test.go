package audio

import (
	"math"
	"testing"
)

// processFX runs n samples of a sine wave at freq through the effect, returning the output.
func processFX(fx InsertEffect, freq float64, sr, n int) []float64 {
	input := sineSamples(freq, sr, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = fx.ProcessSample(x)
	}
	return out
}

// assertEffectModifiesSignal verifies that an effect changes the signal (not passthrough).
func assertEffectModifiesSignal(t *testing.T, fx InsertEffect, freq float64, sr, n int) {
	t.Helper()
	input := sineSamples(freq, sr, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = fx.ProcessSample(x)
	}
	dryRMS := rmsEnergy(input)
	wetRMS := rmsEnergy(out)
	// Compute difference RMS
	diff := make([]float64, n)
	for i := range diff {
		diff[i] = out[i] - input[i]
	}
	diffRMS := rmsEnergy(diff)
	if diffRMS < dryRMS*0.001 && math.Abs(wetRMS-dryRMS) < dryRMS*0.001 {
		t.Fatalf("effect did not modify signal: dryRMS=%v wetRMS=%v diffRMS=%v", dryRMS, wetRMS, diffRMS)
	}
}

// assertDryWhenMixZero verifies output equals input when mix=0.
func assertDryWhenMixZero(t *testing.T, fx InsertEffect, freq float64, sr, n int) {
	t.Helper()
	fx.SetParam("mix", 0)
	input := sineSamples(freq, sr, n)
	for i, x := range input {
		out := fx.ProcessSample(x)
		if math.Abs(out-x) > 1e-6 {
			t.Fatalf("sample %d: mix=0 should be passthrough, got %v want %v", i, out, x)
		}
	}
}

// assertResetClearsState verifies that after Reset(), processing silence produces silence.
func assertResetClearsState(t *testing.T, fx InsertEffect) {
	t.Helper()
	// Feed some signal to build up state
	for i := 0; i < 1000; i++ {
		fx.ProcessSample(math.Sin(float64(i) * 0.1))
	}
	fx.Reset()
	// After reset, processing zeros should produce near-zero
	var maxOut float64
	for i := 0; i < 100; i++ {
		out := math.Abs(fx.ProcessSample(0))
		if out > maxOut {
			maxOut = out
		}
	}
	if maxOut > 1e-6 {
		t.Fatalf("after Reset(), processing silence produced non-zero output: max=%v", maxOut)
	}
}

// assertNoNaNOrInf verifies no sample is NaN or Inf.
func assertNoNaNOrInf(t *testing.T, output []float64) {
	t.Helper()
	for i, v := range output {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatalf("sample %d is %v", i, v)
		}
	}
}

// assertFreqAttenuated verifies energy at freq is below maxDB.
func assertFreqAttenuated(t *testing.T, samples []float64, sr int, freq, maxDB float64) {
	t.Helper()
	db := goertzelMagnitudeDB(samples, freq, sr)
	if db > maxDB {
		t.Fatalf("expected energy at %.0f Hz below %.1f dB, got %.1f dB", freq, maxDB, db)
	}
}

// assertFreqPresent verifies energy at freq is above minDB.
func assertFreqPresent(t *testing.T, samples []float64, sr int, freq, minDB float64) {
	t.Helper()
	db := goertzelMagnitudeDB(samples, freq, sr)
	if db < minDB {
		t.Fatalf("expected energy at %.0f Hz above %.1f dB, got %.1f dB", freq, minDB, db)
	}
}
