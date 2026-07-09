//go:build test

package audio

import "testing"

// TestMergedInstrumentParamsRO_InvalidatesOnEveryMutationPath guards the
// revision plumbing behind the merged-params cache: every mutation that can
// change MergeRecipeDefaults' output must be visible through the cached
// read-only accessor on the next call. A missed bump would freeze the Synth
// tab's previews/mirror at stale values (the cache backs every per-frame
// read — see merged_params_cache.go).
func TestMergedInstrumentParamsRO_InvalidatesOnEveryMutationPath(t *testing.T) {
	const inst = "dnb-kick"
	recipeID := RecipeForInstrument(inst)
	if recipeID == "" {
		t.Fatal("dnb-kick has no recipe binding")
	}
	t.Cleanup(func() {
		ResetInstrumentParams(inst)
		ResetRecipeToShipped(recipeID)
		BindInstrumentToRecipe(inst, recipeID)
	})

	base := MergedInstrumentParamsRO(inst)
	if len(base) == 0 {
		t.Fatal("empty merged map for a bound modular instrument")
	}
	// Same revs → the cached read repeats identically (no re-merge needed).
	if again := MergedInstrumentParamsRO(inst); len(again) != len(base) {
		t.Fatal("second read changed shape with no mutation")
	}

	// 1. SetInstrumentParam must invalidate.
	orig := base["gen1_kick_attack"]
	SetInstrumentParam(inst, "gen1_kick_attack", orig+0.25)
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig+0.25 {
		t.Errorf("after SetInstrumentParam: merged gen1_kick_attack=%v, want %v", got, orig+0.25)
	}

	// 2. ResetInstrumentParams must invalidate.
	ResetInstrumentParams(inst)
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig {
		t.Errorf("after ResetInstrumentParams: merged gen1_kick_attack=%v, want default %v", got, orig)
	}

	// 3. A recipe-defaults Save (UpdateRecipeDefaultsAndInvalidate) must
	// invalidate even with an empty per-instrument overlay.
	if !UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{"gen1_kick_attack": orig + 0.5}) {
		t.Fatal("UpdateRecipeDefaultsAndInvalidate reported no change")
	}
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig+0.5 {
		t.Errorf("after defaults Save: merged gen1_kick_attack=%v, want %v", got, orig+0.5)
	}
	if !ResetRecipeToShipped(recipeID) {
		t.Fatal("ResetRecipeToShipped reported no change")
	}
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig {
		t.Errorf("after ResetRecipeToShipped: merged gen1_kick_attack=%v, want shipped %v", got, orig)
	}

	// 4. Re-binding to a different recipe must invalidate.
	otherRecipe := RecipeForInstrument("snare")
	if otherRecipe == "" || otherRecipe == recipeID {
		t.Fatalf("need a distinct second recipe for the rebind check, got %q", otherRecipe)
	}
	BindInstrumentToRecipe(inst, otherRecipe)
	rebound := MergedInstrumentParamsRO(inst)
	if _, hasKick := rebound["gen1_kick_attack"]; hasKick && len(rebound) == len(base) {
		t.Error("after rebind: merged map still looks like the old recipe's schema")
	}
}
