package audio

import (
	"math"
	"testing"
)

func TestWaveshaperModifiesSignal(t *testing.T) {
	ws := newWaveshaper(44100, map[string]float64{"curve": 0, "drive": 5, "mix": 1})
	assertEffectModifiesSignal(t, ws, 440, 44100, 44100)
}

func TestWaveshaperMixZero(t *testing.T) {
	ws := newWaveshaper(44100, map[string]float64{"curve": 0, "drive": 5, "mix": 1})
	assertDryWhenMixZero(t, ws, 440, 44100, 44100)
}

func TestWaveshaperReset(t *testing.T) {
	ws := newWaveshaper(44100, map[string]float64{"curve": 0, "drive": 5, "mix": 1})
	assertResetClearsState(t, ws)
}

func TestWaveshaperNoNaN(t *testing.T) {
	ws := newWaveshaper(44100, map[string]float64{"curve": 2, "drive": 20, "mix": 1})

	// Test zeros
	out := make([]float64, 100)
	for i := range out {
		out[i] = ws.ProcessSample(0)
	}
	assertNoNaNOrInf(t, out)

	// Test impulses
	out2 := make([]float64, 100)
	for i := range out2 {
		v := 0.0
		if i == 0 {
			v = 1.0
		}
		out2[i] = ws.ProcessSample(v)
	}
	assertNoNaNOrInf(t, out2)

	// Test extreme values
	extremes := []float64{100, -100, 1e6, -1e6, math.SmallestNonzeroFloat64}
	outE := make([]float64, len(extremes))
	for i, v := range extremes {
		outE[i] = ws.ProcessSample(v)
	}
	assertNoNaNOrInf(t, outE)
}

func TestWaveshaperHarmonics(t *testing.T) {
	// Wave folding of a sine wave should create odd harmonics.
	// (Symmetric folding preserves odd symmetry, so only odd harmonics appear.)
	const sr = 44100
	const n = 44100
	const freq = 440.0

	ws := newWaveshaper(sr, map[string]float64{"curve": 2, "drive": 3, "mix": 1})
	input := sineSamples(freq, sr, n)
	out := make([]float64, n)
	for i, x := range input {
		out[i] = ws.ProcessSample(x)
	}

	// With wave folding at drive=3, expect strong odd harmonics:
	// 3rd harmonic at 1320 Hz and 5th harmonic at 2200 Hz
	mag1320 := goertzelMagnitude(out, 1320, sr)
	mag2200 := goertzelMagnitude(out, 2200, sr)

	if mag1320 < 0.1 {
		t.Errorf("expected 3rd harmonic at 1320 Hz, magnitude=%v", mag1320)
	}
	if mag2200 < 0.1 {
		t.Errorf("expected 5th harmonic at 2200 Hz, magnitude=%v", mag2200)
	}
}

func TestWaveshaperCurves(t *testing.T) {
	// Each of the 4 curves should produce different output for the same input.
	const sr = 44100
	const n = 4410

	input := sineSamples(440, sr, n)
	outputs := make([][]float64, 4)

	for curve := 0; curve < 4; curve++ {
		ws := newWaveshaper(sr, map[string]float64{
			"curve": float64(curve), "drive": 5, "mix": 1,
		})
		outputs[curve] = make([]float64, n)
		for i, x := range input {
			outputs[curve][i] = ws.ProcessSample(x)
		}
	}

	// Verify each pair of curves produces different output
	for i := 0; i < 4; i++ {
		for j := i + 1; j < 4; j++ {
			var diffEnergy float64
			for k := range outputs[i] {
				d := outputs[i][k] - outputs[j][k]
				diffEnergy += d * d
			}
			diffRMS := math.Sqrt(diffEnergy / float64(n))
			if diffRMS < 0.001 {
				t.Errorf("curves %d and %d should produce different output, diffRMS=%v", i, j, diffRMS)
			}
		}
	}
}
