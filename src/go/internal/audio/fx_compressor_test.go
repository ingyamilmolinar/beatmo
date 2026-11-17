//go:build test

package audio

import (
	"math"
	"testing"
)

const compressorSR = 44100

func newTestCompressorFX(params map[string]float64) *compressorFX {
	return newCompressorFX(compressorSR, params)
}

// TestCompressorFXReducesLoudSignal verifies that a loud signal (amplitude 1.0)
// with a low threshold (-10 dB) is compressed, producing lower RMS output than input.
func TestCompressorFXReducesLoudSignal(t *testing.T) {
	c := newTestCompressorFX(map[string]float64{
		"threshold": -10,
		"ratio":     8,
		"attack":    1,
		"release":   50,
		"makeup":    0,
		"mix":       1,
	})

	input := sineSamples(440, compressorSR, compressorSR)
	out := make([]float64, len(input))
	for i, x := range input {
		out[i] = c.ProcessSample(x)
	}

	inRMS := rmsEnergy(input)
	outRMS := rmsEnergy(out)

	if outRMS >= inRMS {
		t.Errorf("expected compression to reduce RMS: input=%.4f output=%.4f", inRMS, outRMS)
	}
	// Should be meaningfully reduced, not just barely.
	if outRMS > inRMS*0.95 {
		t.Errorf("compression too weak: input=%.4f output=%.4f (ratio=%.2f)", inRMS, outRMS, outRMS/inRMS)
	}
}

// TestCompressorFXPassesQuietSignal verifies that a very quiet signal with a
// high threshold (0 dB) passes through essentially unmodified (no makeup gain).
func TestCompressorFXPassesQuietSignal(t *testing.T) {
	c := newTestCompressorFX(map[string]float64{
		"threshold": 0,
		"ratio":     4,
		"attack":    10,
		"release":   100,
		"makeup":    0,
		"mix":       1,
	})

	// Very quiet signal: amplitude 0.01 (-40 dB), well below 0 dB threshold.
	input := make([]float64, compressorSR)
	for i := range input {
		input[i] = 0.01 * math.Sin(2*math.Pi*440*float64(i)/float64(compressorSR))
	}
	out := make([]float64, len(input))
	for i, x := range input {
		out[i] = c.ProcessSample(x)
	}

	inRMS := rmsEnergy(input)
	outRMS := rmsEnergy(out)

	// Output should be very close to input since signal is far below threshold.
	ratio := outRMS / inRMS
	if math.Abs(ratio-1.0) > 0.05 {
		t.Errorf("quiet signal should pass through: input=%.6f output=%.6f ratio=%.4f", inRMS, outRMS, ratio)
	}
}

// TestCompressorFXMakeupGain verifies that adding makeup gain changes the output
// energy compared to zero makeup.
func TestCompressorFXMakeupGain(t *testing.T) {
	params := map[string]float64{
		"threshold": -20,
		"ratio":     4,
		"attack":    5,
		"release":   100,
		"makeup":    0,
		"mix":       1,
	}
	c1 := newTestCompressorFX(params)

	input := sineSamples(440, compressorSR, compressorSR)
	out1 := make([]float64, len(input))
	for i, x := range input {
		out1[i] = c1.ProcessSample(x)
	}

	// Same but with 10 dB makeup.
	params["makeup"] = 10
	c2 := newTestCompressorFX(params)
	out2 := make([]float64, len(input))
	for i, x := range input {
		out2[i] = c2.ProcessSample(x)
	}

	rms1 := rmsEnergy(out1)
	rms2 := rmsEnergy(out2)

	if rms2 <= rms1 {
		t.Errorf("expected makeup gain to increase energy: noMakeup=%.4f withMakeup=%.4f", rms1, rms2)
	}
}

// TestCompressorFXMixZeroDry verifies that mix=0 produces exact passthrough.
// We create the effect with mix=0 upfront so the smoothParam starts at zero
// (assertDryWhenMixZero calls SetParam which triggers the smooth ramp, but
// since we start at 0 the ramp is a no-op).
func TestCompressorFXMixZeroDry(t *testing.T) {
	c := newTestCompressorFX(map[string]float64{
		"threshold": -20,
		"ratio":     4,
		"attack":    10,
		"release":   100,
		"makeup":    0,
		"mix":       0,
	})
	assertDryWhenMixZero(t, c, 440, compressorSR, compressorSR)
}

// TestCompressorFXReset verifies that Reset() clears internal envelope state,
// so processing silence after reset produces silence.
func TestCompressorFXReset(t *testing.T) {
	c := newTestCompressorFX(map[string]float64{
		"threshold": -20,
		"ratio":     4,
		"attack":    10,
		"release":   100,
		"makeup":    0,
		"mix":       1,
	})
	assertResetClearsState(t, c)
}

// TestCompressorFXNoNaNInf verifies no sample is NaN or Inf when processing
// a full-amplitude signal through the compressor.
func TestCompressorFXNoNaNInf(t *testing.T) {
	c := newTestCompressorFX(map[string]float64{
		"threshold": -10,
		"ratio":     8,
		"attack":    1,
		"release":   50,
		"makeup":    12,
		"mix":       1,
	})
	out := processFX(c, 440, compressorSR, compressorSR)
	assertNoNaNOrInf(t, out)
}

// TestCompressorFXModifiesSignal verifies that the compressor effect changes
// the signal (i.e., it is not a passthrough).
func TestCompressorFXModifiesSignal(t *testing.T) {
	c := newTestCompressorFX(map[string]float64{
		"threshold": -10,
		"ratio":     8,
		"attack":    1,
		"release":   50,
		"makeup":    0,
		"mix":       1,
	})
	assertEffectModifiesSignal(t, c, 440, compressorSR, compressorSR)
}
