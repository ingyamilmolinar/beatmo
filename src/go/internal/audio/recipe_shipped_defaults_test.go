package audio

import "testing"

// TestRecipeShippedDefaults_RoundTrip covers the shipped-defaults snapshot and
// the customized/reset helpers that the Synth-tab Save and Reset flows rely on.
func TestRecipeShippedDefaults_RoundTrip(t *testing.T) {
	const recipeID = "drum-snare"
	shipped := RecipeShippedDefaults(recipeID)
	if len(shipped) == 0 {
		t.Fatalf("no shipped defaults captured for %q (fixture broken)", recipeID)
	}

	// Force a clean baseline; restore on cleanup (process-global registry).
	ResetRecipeToShipped(recipeID)
	t.Cleanup(func() { ResetRecipeToShipped(recipeID) })

	if RecipeDefaultsCustomized(recipeID) {
		t.Fatal("fresh recipe should not report customized")
	}

	// Customizing a default must flip the flag.
	pick := "decay"
	orig := shipped[pick]
	if !UpdateRecipeDefaultsAndInvalidate(recipeID, map[string]float64{pick: orig + 1.0}) {
		t.Fatalf("UpdateRecipeDefaultsAndInvalidate reported no change for %q", pick)
	}
	if !RecipeDefaultsCustomized(recipeID) {
		t.Error("recipe should report customized after a default was changed")
	}
	// The shipped snapshot must be unaffected by the customization.
	if got := RecipeShippedDefaults(recipeID)[pick]; got != orig {
		t.Errorf("shipped snapshot changed after customization: %q = %v, want %v", pick, got, orig)
	}

	// Reset restores the shipped values and clears the customized flag.
	ResetRecipeToShipped(recipeID)
	if RecipeDefaultsCustomized(recipeID) {
		t.Error("recipe still reports customized after ResetRecipeToShipped")
	}
	if got := RecipeDefaultParams(recipeID)[pick]; got != orig {
		t.Errorf("after reset, default %q = %v, want shipped %v", pick, got, orig)
	}
}

// TestRecipeDefaultsCustomized_KickFundamentalIsNotCustomized guards the
// parity-safety property: drum-kick ships a non-identity extra default
// (fundamental=55), yet its as-shipped state must read as NOT customized so the
// render dispatcher keeps it on the cheap legacy path (and parity goldens stay
// byte-identical). This is why the gate compares against the shipped snapshot,
// not a generic identity baseline.
func TestRecipeDefaultsCustomized_KickFundamentalIsNotCustomized(t *testing.T) {
	const recipeID = "drum-kick"
	if len(RecipeShippedDefaults(recipeID)) == 0 {
		t.Skipf("%q not registered in this build", recipeID)
	}
	ResetRecipeToShipped(recipeID)
	t.Cleanup(func() { ResetRecipeToShipped(recipeID) })

	if RecipeDefaultsCustomized(recipeID) {
		t.Errorf("as-shipped %q reports customized; its non-identity default (fundamental) must read as baseline", recipeID)
	}
}
