//go:build !test && !js

package audio

import "testing"

// All ens_* at identity must leave the unison render byte-identical.
func TestEnsemble_IdentityIsExactBypass(t *testing.T) {
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 4, "unison_detune": 10}
	withZeros := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 4, "unison_detune": 10,
		"ens_scatter": 0, "ens_vib_rate": 0, "ens_vib_depth": 0, "ens_humanize": 0}
	a := renderModularSeed(t, base, 0.5)
	b := renderModularSeed(t, withZeros, 0.5)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d differs at ens identity", i)
		}
	}
}

// Scatter and vibrato must change the output; two renders with the same seed
// must be identical (determinism).
func TestEnsemble_HumanizeChangesOutputDeterministically(t *testing.T) {
	hum := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 5, "unison_detune": 8,
		"ens_scatter": 10, "ens_vib_rate": 5.5, "ens_vib_depth": 8, "ens_humanize": 0.8}
	base := RecipeParams{"osc_type": 1, "osc_enabled": 1, "unison_voices": 5, "unison_detune": 8}
	a := renderModularSeed(t, base, 0.5)
	b1 := renderModularSeed(t, hum, 0.5)
	b2 := renderModularSeed(t, hum, 0.5)
	diff := false
	for i := range b1 {
		if b1[i] != b2[i] {
			t.Fatalf("non-deterministic ensemble render at sample %d", i)
		}
		if a[i] != b1[i] {
			diff = true
		}
	}
	if !diff {
		t.Fatal("ens params set but output identical to base")
	}
}
