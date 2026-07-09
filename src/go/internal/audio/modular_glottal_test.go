//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// osc_type 12 (glottal) must render non-silent, finite, and differ from both
// saw (osc_type 1) and sine (osc_type 0) — the latter comparison is what
// makes this genuinely RED before the wavetable is wired: osc_type 12
// currently falls through modular_table_for's default case to the sine
// table, so a naive glottal-vs-saw-only diff check would pass by accident.
func TestGlottalOsc_RendersDistinctFinite(t *testing.T) {
	glottal := RecipeParams{"osc_type": 12, "osc_enabled": 1}
	saw := RecipeParams{"osc_type": 1, "osc_enabled": 1}
	sine := RecipeParams{"osc_type": 0, "osc_enabled": 1}
	g := renderModularSeed(t, glottal, 0.5)
	s := renderModularSeed(t, saw, 0.5)
	sn := renderModularSeed(t, sine, 0.5)
	peak, diffSaw, diffSine := float32(0), false, false
	for i := range g {
		if math.IsNaN(float64(g[i])) || math.IsInf(float64(g[i]), 0) {
			t.Fatalf("non-finite glottal sample %d", i)
		}
		if a := float32(math.Abs(float64(g[i]))); a > peak {
			peak = a
		}
		if g[i] != s[i] {
			diffSaw = true
		}
		if g[i] != sn[i] {
			diffSine = true
		}
	}
	if peak < 0.01 {
		t.Fatal("glottal render silent")
	}
	if !diffSaw {
		t.Fatal("glottal identical to saw — table not wired")
	}
	if !diffSine {
		t.Fatal("glottal identical to sine — table not wired (falling through to default)")
	}
}
