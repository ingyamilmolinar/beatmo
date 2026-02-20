package audio

import (
	"math"
	"testing"
)

const (
	testSR = 44100
	testN  = 44100 // 1 second
)

// ── Filter frequency tests ─────────────────────────────────────────────────

func TestFilterLPAttenuatesHigh(t *testing.T) {
	fx := newFilter(testSR, map[string]float64{"mode": 0, "cutoff": 1000, "q": 0.707, "mix": 1})
	out := processFX(fx, 5000, testSR, testN)
	assertFreqAttenuated(t, out, testSR, 5000, -10)
}

func TestFilterHPAttenuatesLow(t *testing.T) {
	fx := newFilter(testSR, map[string]float64{"mode": 1, "cutoff": 1000, "q": 0.707, "mix": 1})
	out := processFX(fx, 200, testSR, testN)
	assertFreqAttenuated(t, out, testSR, 200, -10)
}

func TestFilterBPPassesBand(t *testing.T) {
	fx := newFilter(testSR, map[string]float64{"mode": 2, "cutoff": 1000, "q": 2, "mix": 1})
	out := processFX(fx, 1000, testSR, testN)
	assertFreqPresent(t, out, testSR, 1000, -6)
}

// ── Distortion harmonic tests ───────────────────────────────────────────────

func TestDistortionCreatesHarmonics(t *testing.T) {
	fx := newDistortion(testSR, map[string]float64{"drive": 10, "tone": 8000, "mix": 1})
	// Process a long signal so smoothed params settle to target values
	input := sineSamples(440, testSR, testN*2)
	out := make([]float64, len(input))
	for i, x := range input {
		out[i] = fx.ProcessSample(x)
	}
	// Use only the second half (after params have settled)
	settled := out[testN:]
	// tanh is odd -> only odd harmonics: 3rd (1320Hz), 5th (2200Hz)
	mag3rd := goertzelMagnitude(settled, 1320, testSR)
	mag5th := goertzelMagnitude(settled, 2200, testSR)
	if mag3rd < 0.01 && mag5th < 0.01 {
		t.Fatalf("expected odd harmonics from tanh distortion: 1320Hz=%v, 2200Hz=%v", mag3rd, mag5th)
	}
}

// ── Delay echo test ─────────────────────────────────────────────────────────

func TestDelayEchoTiming(t *testing.T) {
	delayMs := 100.0
	fx := newDelay(testSR, map[string]float64{"time": delayMs, "feedback": 0, "mix": 1})
	// Feed an impulse
	input := make([]float64, testSR)
	input[0] = 1.0
	out := make([]float64, len(input))
	for i, x := range input {
		out[i] = fx.ProcessSample(x)
	}
	// Expect echo at ~4410 samples (100ms at 44100Hz)
	expectedSample := int(delayMs * 0.001 * float64(testSR))
	// Find peak in output around expected position
	peakIdx := 0
	peakVal := 0.0
	for i := expectedSample - 50; i < expectedSample+50 && i < len(out); i++ {
		if i < 0 {
			continue
		}
		if math.Abs(out[i]) > peakVal {
			peakVal = math.Abs(out[i])
			peakIdx = i
		}
	}
	if peakVal < 0.5 {
		t.Fatalf("no echo peak found near sample %d, max was %v at %d", expectedSample, peakVal, peakIdx)
	}
	if math.Abs(float64(peakIdx-expectedSample)) > 10 {
		t.Fatalf("echo peak at %d, expected near %d", peakIdx, expectedSample)
	}
}

// ── Bitcrusher harmonic products ────────────────────────────────────────────

func TestBitcrusherCreatesDistortion(t *testing.T) {
	fx := newBitcrusher(testSR, map[string]float64{"bits": 4, "rate": 1, "mix": 1})
	out := processFX(fx, 440, testSR, testN)
	// 4-bit quantization should create harmonic distortion products.
	// Check for harmonics rather than RMS change (quantization preserves amplitude).
	mag880 := goertzelMagnitude(out, 880, testSR)
	mag1320 := goertzelMagnitude(out, 1320, testSR)
	if mag880 < 0.001 && mag1320 < 0.001 {
		t.Fatalf("4-bit crush should produce harmonics: 880Hz=%v, 1320Hz=%v", mag880, mag1320)
	}
}

// ── Phaser notch test ───────────────────────────────────────────────────────

func TestPhaserCreatesNotches(t *testing.T) {
	// Static phaser (rate=0 effectively, or very slow) to measure notch
	fx := newPhaser(testSR, map[string]float64{
		"stages": 6, "rate": 0.1, "depth": 0.5, "feedback": 0.7, "mix": 1,
	})
	out := processFX(fx, 1000, testSR, testN)
	// The phaser should modify the signal significantly
	dryRMS := rmsEnergy(sineSamples(1000, testSR, testN))
	wetRMS := rmsEnergy(out)
	diff := math.Abs(wetRMS - dryRMS)
	if diff < dryRMS*0.01 {
		t.Fatalf("phaser should modify signal: dryRMS=%v wetRMS=%v", dryRMS, wetRMS)
	}
}

// ── Flanger comb filter test ────────────────────────────────────────────────

func TestFlangerCombFiltering(t *testing.T) {
	fx := newFlanger(testSR, map[string]float64{
		"rate": 0.1, "depth": 5, "feedback": 0.7, "mix": 1,
	})
	out := processFX(fx, 1000, testSR, testN)
	assertEffectModifiesSignal(t, newFlanger(testSR, map[string]float64{
		"rate": 0.1, "depth": 5, "feedback": 0.7, "mix": 1,
	}), 1000, testSR, testN)
	assertNoNaNOrInf(t, out)
}

// ── Ring mod sum/difference frequencies ─────────────────────────────────────

func TestRingModFrequencies(t *testing.T) {
	fx := newRingMod(testSR, map[string]float64{"frequency": 100, "shape": 0, "mix": 1})
	input := sineSamples(440, testSR, testN)
	out := make([]float64, testN)
	for i, x := range input {
		out[i] = fx.ProcessSample(x)
	}
	// Should produce sum (540Hz) and difference (340Hz)
	mag340 := goertzelMagnitude(out, 340, testSR)
	mag540 := goertzelMagnitude(out, 540, testSR)
	if mag340 < 0.1 {
		t.Fatalf("expected difference frequency at 340Hz, got magnitude %v", mag340)
	}
	if mag540 < 0.1 {
		t.Fatalf("expected sum frequency at 540Hz, got magnitude %v", mag540)
	}
}

// ── Waveshaper fold harmonics ───────────────────────────────────────────────

func TestWaveshaperFoldHarmonics(t *testing.T) {
	fx := newWaveshaper(testSR, map[string]float64{"curve": 2, "drive": 5, "mix": 1})
	out := processFX(fx, 440, testSR, testN)
	// Wave folding should produce odd harmonics
	mag880 := goertzelMagnitude(out, 880, testSR)
	mag1320 := goertzelMagnitude(out, 1320, testSR)
	if mag880 < 0.01 && mag1320 < 0.01 {
		t.Fatalf("wave folding should produce harmonics: 880Hz=%v, 1320Hz=%v", mag880, mag1320)
	}
}

// ── All effects: no NaN/Inf on silence, impulse, DC ─────────────────────────

func TestAllEffectsStabilityOnSilence(t *testing.T) {
	types := EffectTypeOrder()
	silence := make([]float64, 1000)
	for _, et := range types {
		t.Run(string(et), func(t *testing.T) {
			fx := NewEffectProcessor(EffectSlot{Type: et, Enabled: true}, testSR)
			out := make([]float64, len(silence))
			for i := range silence {
				out[i] = fx.ProcessSample(0)
			}
			assertNoNaNOrInf(t, out)
		})
	}
}

func TestAllEffectsStabilityOnImpulse(t *testing.T) {
	types := EffectTypeOrder()
	for _, et := range types {
		t.Run(string(et), func(t *testing.T) {
			fx := NewEffectProcessor(EffectSlot{Type: et, Enabled: true}, testSR)
			out := make([]float64, 1000)
			out[0] = fx.ProcessSample(1.0)
			for i := 1; i < len(out); i++ {
				out[i] = fx.ProcessSample(0)
			}
			assertNoNaNOrInf(t, out)
		})
	}
}

func TestAllEffectsStabilityOnDC(t *testing.T) {
	types := EffectTypeOrder()
	for _, et := range types {
		t.Run(string(et), func(t *testing.T) {
			fx := NewEffectProcessor(EffectSlot{Type: et, Enabled: true}, testSR)
			out := make([]float64, 1000)
			for i := range out {
				out[i] = fx.ProcessSample(0.5)
			}
			assertNoNaNOrInf(t, out)
		})
	}
}
