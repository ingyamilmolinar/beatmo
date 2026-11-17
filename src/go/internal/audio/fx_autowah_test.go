package audio

import (
	"math"
	"testing"
)

func TestAutoWahModifiesSignal(t *testing.T) {
	aw := newAutoWah(44100, map[string]float64{
		"sensitivity": 0.5, "rate": 2, "depth": 0.7, "mix": 0.5,
	})
	assertEffectModifiesSignal(t, aw, 440, 44100, 44100)
}

func TestAutoWahMixZero(t *testing.T) {
	aw := newAutoWah(44100, map[string]float64{
		"sensitivity": 0.5, "rate": 2, "depth": 0.7, "mix": 1,
	})
	assertDryWhenMixZero(t, aw, 440, 44100, 44100)
}

func TestAutoWahReset(t *testing.T) {
	aw := newAutoWah(44100, map[string]float64{
		"sensitivity": 0.5, "rate": 2, "depth": 0.7, "mix": 1,
	})
	assertResetClearsState(t, aw)
}

func TestAutoWahNoNaN(t *testing.T) {
	aw := newAutoWah(44100, map[string]float64{
		"sensitivity": 1, "rate": 20, "depth": 1, "mix": 1,
	})
	out := make([]float64, 2000)
	for i := range out {
		out[i] = aw.ProcessSample(0)
	}
	assertNoNaNOrInf(t, out)
}

func TestAutoWahEnvelopeResponse(t *testing.T) {
	// Loud input should produce different frequency emphasis than quiet input,
	// because the envelope follower drives the filter cutoff higher for loud signals.
	const sr = 44100
	const n = 44100

	// Process loud signal (0.8 amplitude)
	awLoud := newAutoWah(sr, map[string]float64{
		"sensitivity": 0.8, "rate": 0.5, "depth": 0, "mix": 1,
	})
	loudInput := make([]float64, n)
	for i := range loudInput {
		loudInput[i] = 0.8 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}
	loudOut := make([]float64, n)
	for i, x := range loudInput {
		loudOut[i] = awLoud.ProcessSample(x)
	}

	// Process quiet signal (0.01 amplitude)
	awQuiet := newAutoWah(sr, map[string]float64{
		"sensitivity": 0.8, "rate": 0.5, "depth": 0, "mix": 1,
	})
	quietInput := make([]float64, n)
	for i := range quietInput {
		quietInput[i] = 0.01 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}
	quietOut := make([]float64, n)
	for i, x := range quietInput {
		quietOut[i] = awQuiet.ProcessSample(x)
	}

	// The spectral shape should differ. Measure energy at a higher frequency
	// relative to the fundamental. Loud signal should open the filter more,
	// letting more high frequency through relative to the fundamental level.
	loudRMS := rmsEnergy(loudOut)
	quietRMS := rmsEnergy(quietOut)

	// At minimum, the RMS ratio should not be the same as the input ratio (80:1).
	// The filter's envelope response should compress or expand the ratio.
	inputRatio := 0.8 / 0.01
	outputRatio := loudRMS / (quietRMS + 1e-20)

	if math.Abs(outputRatio-inputRatio) < 1.0 {
		t.Errorf("envelope should affect frequency response differently for loud vs quiet: "+
			"inputRatio=%v outputRatio=%v", inputRatio, outputRatio)
	}
}

func TestAutoWahSetParam(t *testing.T) {
	const sr = 44100
	const n = 22050

	// Process signal with low sensitivity
	aw1 := newAutoWah(sr, map[string]float64{
		"sensitivity": 0.1, "rate": 2, "depth": 0.5, "mix": 1,
	})
	input := sineSamples(440, sr, n)
	out1 := make([]float64, n)
	for i, x := range input {
		out1[i] = aw1.ProcessSample(x)
	}

	// Process same signal with high sensitivity
	aw2 := newAutoWah(sr, map[string]float64{
		"sensitivity": 0.1, "rate": 2, "depth": 0.5, "mix": 1,
	})
	aw2.SetParam("sensitivity", 1.0)
	out2 := make([]float64, n)
	for i, x := range input {
		out2[i] = aw2.ProcessSample(x)
	}

	// Outputs should differ
	var diffEnergy float64
	for i := range out1 {
		d := out1[i] - out2[i]
		diffEnergy += d * d
	}
	diffRMS := math.Sqrt(diffEnergy / float64(n))
	if diffRMS < 0.001 {
		t.Errorf("changing sensitivity should change output, diffRMS=%v", diffRMS)
	}
}
