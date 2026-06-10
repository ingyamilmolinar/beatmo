//go:build !test && !js

package audio

import "testing"

// recipeParamsToModular must read every modular schema name by key and fall
// back to the built-in identity default for absent keys.
func TestRecipeParamsToModular_IdentityRoundTrip(t *testing.T) {
	ident := ModularParamSchemaIdentity()
	rp := RecipeParams{}
	for k, v := range ident {
		rp[k] = v
	}
	m := recipeParamsToModular(rp)
	if m.OscType != ident["osc_type"] {
		t.Errorf("OscType = %v, want %v", m.OscType, ident["osc_type"])
	}
	if m.FilterCutoff != ident["filter_cutoff"] {
		t.Errorf("FilterCutoff = %v, want %v", m.FilterCutoff, ident["filter_cutoff"])
	}
	if m.Gain != ident["gain"] {
		t.Errorf("Gain = %v, want %v", m.Gain, ident["gain"])
	}
	if m.FMOp1Level != ident["fm_op1_level"] {
		t.Errorf("FMOp1Level = %v, want %v", m.FMOp1Level, ident["fm_op1_level"])
	}
}

func TestRecipeParamsToModular_AbsentUsesIdentity(t *testing.T) {
	m := recipeParamsToModular(RecipeParams{}) // empty map
	ident := ModularParamSchemaIdentity()
	if m.AmpDecay != ident["amp_decay"] {
		t.Errorf("AmpDecay fallback = %v, want %v", m.AmpDecay, ident["amp_decay"])
	}
	if m.FilterResonance != ident["filter_resonance"] {
		t.Errorf("FilterResonance fallback = %v, want %v", m.FilterResonance, ident["filter_resonance"])
	}
}

func TestRecipeParamsToModular_OverridesApply(t *testing.T) {
	rp := RecipeParams{
		"osc_type":      4,
		"filter_cutoff": 1234,
		"fm_op3_depth":  5,
		"gain":          0.5,
		"amp_attack":    0.02,
	}
	m := recipeParamsToModular(rp)
	if m.OscType != 4 {
		t.Errorf("OscType = %v, want 4", m.OscType)
	}
	if m.FilterCutoff != 1234 {
		t.Errorf("FilterCutoff = %v, want 1234", m.FilterCutoff)
	}
	if m.FMOp3Depth != 5 {
		t.Errorf("FMOp3Depth = %v, want 5", m.FMOp3Depth)
	}
	if m.Gain != 0.5 {
		t.Errorf("Gain = %v, want 0.5", m.Gain)
	}
	if m.AmpAttack != 0.02 {
		t.Errorf("AmpAttack = %v, want 0.02", m.AmpAttack)
	}
}

func TestRecipeParamsToModular_RendersNonSilent(t *testing.T) {
	const sr = 48000
	n := sr / 4
	rp := RecipeParams{}
	for k, v := range ModularParamSchemaIdentity() {
		rp[k] = v
	}
	buf := make([]float32, n)
	renderModularP(buf, sr, n, recipeParamsToModular(rp))
	if peakAbs(buf) <= 0 {
		t.Fatalf("recipe-param modular render is silent")
	}
}
