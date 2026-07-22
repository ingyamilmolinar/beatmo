//go:build test

package audio

import (
	"math"
	"testing"
)

// TestBuildInstrumentRecipeDocRoundTrips proves the exported doc (base + seed)
// reconstructs the instrument's effective tone via the correct import path —
// the thing the legacy exportUserRecipe could not do.
func TestBuildInstrumentRecipeDocRoundTrips(t *testing.T) {
	const inst = "test-inst-recipedoc"
	const base = "drum-kick"
	reg := RecipeRegistrations()[base]
	if reg == nil || len(reg.Params) == 0 {
		t.Skipf("%s not available", base)
	}
	BindInstrumentToRecipe(inst, base)
	t.Cleanup(func() { ResetInstrumentParams(inst) })

	// Edit a param to a value that differs from the shipped default.
	p := reg.Params[0]
	shipped := RecipeShippedDefaults(base)
	newVal := p.Min
	if math.Abs(newVal-shipped[p.Name]) < 1e-9 {
		newVal = p.Max
	}
	SetInstrumentParam(inst, p.Name, newVal)

	doc, err := BuildInstrumentRecipeDoc(inst)
	if err != nil {
		t.Fatalf("build doc: %v", err)
	}
	if doc.BaseRecipe != base {
		t.Errorf("base recipe = %q, want %q", doc.BaseRecipe, base)
	}
	if _, ok := doc.ParamSeed[p.Name]; !ok {
		t.Errorf("edited param %q not in seed: %v", p.Name, doc.ParamSeed)
	}

	// Reconstruct as a fresh recipe and verify the effective tone matches.
	const newID = "user.test.recipedoc"
	if _, err := RegisterUserRecipeFromBase(newID, "Test", doc.BaseRecipe, doc.ParamSeed); err != nil {
		t.Fatalf("register from base: %v", err)
	}
	t.Cleanup(func() { UnregisterRecipeForTest(newID) })

	got := RecipeDefaultParams(newID)
	want := MergeRecipeDefaults(base, GetInstrumentParams(inst))
	if math.Abs(got[p.Name]-want[p.Name]) > 1e-9 {
		t.Errorf("reconstructed %q = %v, want %v", p.Name, got[p.Name], want[p.Name])
	}

	// And the base is tracked for the reconstructed recipe.
	if RecipeBaseOf(newID) != base {
		t.Errorf("RecipeBaseOf(%q) = %q, want %q", newID, RecipeBaseOf(newID), base)
	}
}
