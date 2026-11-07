//go:build test

package audio

import (
	"math"
	"testing"
)

func TestGatePassesLoudSignal(t *testing.T) {
	// 0.5 amplitude sine is about -6 dB, well above -30 dB threshold.
	g := newGate(44100, DefaultParams(EffectGate))
	const sr = 44100
	const n = 4000
	input := make([]float64, n)
	output := make([]float64, n)
	for i := range input {
		input[i] = 0.5 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}
	for i, x := range input {
		output[i] = g.ProcessSample(x)
	}
	// Skip first 200 samples for envelope to settle.
	inRMS := rmsEnergy(input[200:])
	outRMS := rmsEnergy(output[200:])
	ratio := outRMS / inRMS
	if ratio < 0.95 || ratio > 1.05 {
		t.Errorf("loud signal should pass through gate: RMS ratio=%.4f (want ~1.0)", ratio)
	}
}

func TestGateAttenuatesQuietSignal(t *testing.T) {
	// 0.001 amplitude is about -60 dB, well below -30 dB threshold.
	g := newGate(44100, DefaultParams(EffectGate))
	const sr = 44100
	const n = 4000
	input := make([]float64, n)
	output := make([]float64, n)
	for i := range input {
		input[i] = 0.001 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}
	for i, x := range input {
		output[i] = g.ProcessSample(x)
	}
	inRMS := rmsEnergy(input[200:])
	outRMS := rmsEnergy(output[200:])
	ratio := outRMS / inRMS
	// Default range is -90 dB => rangeLin ~ 3.16e-5; ratio should be very small.
	if ratio > 0.01 {
		t.Errorf("quiet signal should be heavily attenuated: RMS ratio=%.6f (want near 0)", ratio)
	}
}

func TestGateReset(t *testing.T) {
	g := newGate(44100, DefaultParams(EffectGate))
	assertResetClearsState(t, g)
}

func TestGateNoNaN(t *testing.T) {
	g := newGate(44100, DefaultParams(EffectGate))
	output := make([]float64, 1000)
	for i := range output {
		output[i] = g.ProcessSample(0)
	}
	assertNoNaNOrInf(t, output)
}

func TestGateSetParam(t *testing.T) {
	// 0.01 (~-40 dB) is gated at -30 dB and passes at -60.
	g := newGate(44100, DefaultParams(EffectGate))
	// Set range to a moderate value so we can distinguish gated vs open.
	g.SetParam("range", -60)

	const sr = 44100
	const n = 4000
	input := make([]float64, n)
	for i := range input {
		input[i] = 0.01 * math.Sin(2*math.Pi*440*float64(i)/float64(sr))
	}

	// First pass: signal is below -30 dB threshold, should be attenuated.
	gated := make([]float64, n)
	for i, x := range input {
		gated[i] = g.ProcessSample(x)
	}
	gatedRMS := rmsEnergy(gated[200:])

	// Reset and lower threshold.
	g.Reset()
	g.SetParam("threshold", -60)

	// Second pass: signal is now above the new -60 dB threshold.
	passed := make([]float64, n)
	for i, x := range input {
		passed[i] = g.ProcessSample(x)
	}
	passedRMS := rmsEnergy(passed[200:])

	if passedRMS <= gatedRMS*2 {
		t.Errorf("lowering threshold should let signal pass: gatedRMS=%.6f passedRMS=%.6f", gatedRMS, passedRMS)
	}
}

func TestGateTimingRelease(t *testing.T) {
	// The envelope decays smoothly rather than closing instantly.
	g := newGate(44100, map[string]float64{
		"threshold": -30,
		"attack":    1,
		"release":   50,
		"range":     -90,
	})

	const sr = 44100

	// Open the gate with loud signal.
	for i := 0; i < 2000; i++ {
		g.ProcessSample(0.5 * math.Sin(2*math.Pi*440*float64(i)/float64(sr)))
	}

	// 50 ms release is ~2205 samples.
	var samplesUntilClosed int
	for i := 0; i < 10000; i++ {
		out := g.ProcessSample(0)
		_ = out
		// Check if envelope has dropped below threshold.
		if g.envelope < g.thresholdLin {
			samplesUntilClosed = i
			break
		}
	}

	// Should take at least a few hundred samples (not instant).
	if samplesUntilClosed < 100 {
		t.Errorf("gate closed too quickly: %d samples (expected gradual release)", samplesUntilClosed)
	}
	// But not more than ~5x the release time constant.
	maxSamples := int(5 * 50 * 0.001 * float64(sr)) // 5 * release_time_in_samples
	if samplesUntilClosed > maxSamples {
		t.Errorf("gate took too long to close: %d samples (max expected %d)", samplesUntilClosed, maxSamples)
	}
}
