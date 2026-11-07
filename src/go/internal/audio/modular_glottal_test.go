//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Also differs from sine, the table an unwired osc_type falls through to, so a saw-only check cannot pass by accident.
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
