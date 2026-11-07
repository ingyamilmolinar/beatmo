//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// The goldens exercise the default algorithm only.

func renderModularFMAlg(t *testing.T, alg float64) []float32 {
	t.Helper()
	rp := RecipeParams{
		"osc_type":     4, // FM
		"fm_algorithm": alg,
		// Give every operator signal so routing differences are audible.
		"fm_op1_level": 1.0,
		"fm_op2_level": 0.6,
		"fm_op3_level": 0.5,
		"fm_op4_level": 0.4,
		"fm_op1_ratio": 1.0,
		"fm_op2_ratio": 2.0,
		"fm_op3_ratio": 3.0,
		"fm_op4_ratio": 5.0,
		"fm_op2_depth": 2.0,
		"fm_op3_depth": 1.5,
		"fm_op4_depth": 1.0,
	}
	const sr = 44100
	buf := make([]float32, sr/4)
	renderModularBlock(buf, sr, len(buf), initBlockForTest(rp))
	for i, v := range buf {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("alg %v: buf[%d] not finite", alg, i)
		}
	}
	return buf
}

func TestModularFMAlgorithmsRouteDistinctly(t *testing.T) {
	outs := map[int][]float32{}
	for alg := 0; alg <= 3; alg++ {
		out := renderModularFMAlg(t, float64(alg))
		var energy float64
		for _, v := range out {
			energy += float64(v) * float64(v)
		}
		if energy == 0 {
			t.Fatalf("alg %d rendered silence", alg)
		}
		outs[alg] = out
	}

	// Each algorithm is a different operator topology.
	for a := 0; a <= 3; a++ {
		for b := a + 1; b <= 3; b++ {
			if buffersEqualF32(outs[a], outs[b]) {
				t.Fatalf("alg %d and alg %d produced identical buffers — routing not applied", a, b)
			}
		}
	}
}

func buffersEqualF32(a, b []float32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
