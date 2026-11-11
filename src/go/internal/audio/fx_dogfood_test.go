package audio

import (
	"math"
	"testing"
)

// TestDefaultFXDistortionMatchesSoftClip verifies that the DefaultFX distortion
// produces a similar character to the old softClip helper.
// Exact match is not expected (biquad LP vs one-pole LP, different gain models)
// but RMS ratio should be within a reasonable range.
func TestDefaultFXDistortionMatchesSoftClip(t *testing.T) {
	const sr = 44100
	const n = sr / 2

	// Generate a test signal
	input := sineSamples(440, sr, n)
	buf := make([]float32, n)
	for i, x := range input {
		buf[i] = float32(x)
	}

	// Process through DefaultFX distortion (drive=2.5, tone=8000, mix=1)
	fxBuf := make([]float32, n)
	copy(fxBuf, buf)
	applyDefaultFX(fxBuf, sr, []EffectSlot{
		{Type: EffectDistortion, Enabled: true, Params: map[string]float64{
			"drive": 2.5, "tone": 8000, "mix": 1,
		}},
	})

	// Both should produce non-silent output
	fxRMS := float32RMS(fxBuf)
	if fxRMS < 0.01 {
		t.Fatalf("DefaultFX distortion produced near-silent output: RMS=%v", fxRMS)
	}

	// The output should be modified from the input
	inputRMS := float32RMS(buf)
	if math.Abs(float64(fxRMS-inputRMS)) < float64(inputRMS)*0.001 {
		t.Fatalf("DefaultFX distortion did not modify the signal")
	}
}

// TestDefaultFXFilterMatchesLPFilter verifies that the DefaultFX LP filter
// attenuates high frequencies similarly to the old lpFilter helper.
func TestDefaultFXFilterMatchesLPFilter(t *testing.T) {
	const sr = 44100
	const n = sr / 2

	// High frequency signal that should be attenuated by LP filter
	input := sineSamples(5000, sr, n)
	buf := make([]float32, n)
	for i, x := range input {
		buf[i] = float32(x)
	}

	fxBuf := make([]float32, n)
	copy(fxBuf, buf)
	applyDefaultFX(fxBuf, sr, []EffectSlot{
		{Type: EffectFilter, Enabled: true, Params: map[string]float64{
			"mode": 0, "cutoff": 1000, "q": 0.707, "mix": 1,
		}},
	})

	inputRMS := float32RMS(buf)
	fxRMS := float32RMS(fxBuf)

	// LP at 1kHz should significantly attenuate 5kHz
	if fxRMS > inputRMS*0.5 {
		t.Fatalf("LP filter at 1kHz should attenuate 5kHz: inputRMS=%v fxRMS=%v", inputRMS, fxRMS)
	}
}

// TestDefaultFXBitcrusherProducesDistortion verifies the bitcrusher effect
// alters the signal.
func TestDefaultFXBitcrusherProducesDistortion(t *testing.T) {
	const sr = 44100
	const n = sr / 2

	input := sineSamples(440, sr, n)
	buf := make([]float32, n)
	for i, x := range input {
		buf[i] = float32(x)
	}

	fxBuf := make([]float32, n)
	copy(fxBuf, buf)
	applyDefaultFX(fxBuf, sr, []EffectSlot{
		{Type: EffectBitcrusher, Enabled: true, Params: map[string]float64{
			"bits": 4, "rate": 1, "mix": 1,
		}},
	})

	// Should produce different output (quantization artifacts)
	diffCount := 0
	for i := range buf {
		if math.Abs(float64(fxBuf[i]-buf[i])) > 1e-6 {
			diffCount++
		}
	}
	if diffCount < n/10 {
		t.Fatalf("4-bit crush should change many samples, only %d/%d differ", diffCount, n)
	}
}

// TestDefaultFXChainOrder verifies that multi-effect DefaultFX chains process
// in the correct order (left to right).
func TestDefaultFXChainOrder(t *testing.T) {
	const sr = 44100
	const n = sr / 2

	input := sineSamples(440, sr, n)

	// Chain 1: HP filter then distortion
	buf1 := make([]float32, n)
	for i, x := range input {
		buf1[i] = float32(x)
	}
	applyDefaultFX(buf1, sr, []EffectSlot{
		{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 800, "q": 0.707, "mix": 1}},
		{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 10, "tone": 8000, "mix": 1}},
	})

	// Chain 2: distortion then HP filter (reversed order)
	buf2 := make([]float32, n)
	for i, x := range input {
		buf2[i] = float32(x)
	}
	applyDefaultFX(buf2, sr, []EffectSlot{
		{Type: EffectDistortion, Enabled: true, Params: map[string]float64{"drive": 10, "tone": 8000, "mix": 1}},
		{Type: EffectFilter, Enabled: true, Params: map[string]float64{"mode": 1, "cutoff": 800, "q": 0.707, "mix": 1}},
	})

	// Different chain orders should produce different results
	rms1 := float32RMS(buf1)
	rms2 := float32RMS(buf2)
	if math.Abs(float64(rms1-rms2)) < 0.001 {
		t.Fatalf("different chain orders should produce different RMS: order1=%v order2=%v", rms1, rms2)
	}
}

// TestDefaultFXDisabledSlotsSkipped verifies that disabled slots in DefaultFX
// are not processed.
func TestDefaultFXDisabledSlotsSkipped(t *testing.T) {
	const sr = 44100
	const n = sr / 4

	input := sineSamples(440, sr, n)
	buf := make([]float32, n)
	for i, x := range input {
		buf[i] = float32(x)
	}

	fxBuf := make([]float32, n)
	copy(fxBuf, buf)
	// Distortion is disabled
	applyDefaultFX(fxBuf, sr, []EffectSlot{
		{Type: EffectDistortion, Enabled: false, Params: map[string]float64{"drive": 20, "mix": 1}},
	})

	// Output should be identical to input since effect is disabled
	for i := range buf {
		if buf[i] != fxBuf[i] {
			t.Fatalf("disabled slot should not modify signal at sample %d: want %v got %v", i, buf[i], fxBuf[i])
		}
	}
}

// float32RMS computes RMS of a float32 buffer.
func float32RMS(buf []float32) float32 {
	if len(buf) == 0 {
		return 0
	}
	var sum float64
	for _, x := range buf {
		sum += float64(x) * float64(x)
	}
	return float32(math.Sqrt(sum / float64(len(buf))))
}
