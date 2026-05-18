package audio

import "testing"

// Phase 1.5: assert ResetInstruments wires each Phase-1 drum to its
// SynthRecipe via BindInstrumentToRecipe so user-edited per-instrument
// params can flow through the registry. Phase 2 will migrate the legacy
// Snare{} / Kick{} types to CVariantInstrument and route the trigger path
// through the recipe; the binding done here is the prerequisite.

var instrumentToRecipeBindings = map[string]string{
	"snare":   "drum-snare",
	"kick":    "drum-kick",
	"hihat":   "drum-hihat",
	"clap":    "drum-clap",
	"tom":     "drum-tom",
	"cowbell": "drum-cowbell",
}

func TestResetInstruments_BindsCoreDrumsToRecipes(t *testing.T) {
	ResetInstruments()
	for instID, recipeID := range instrumentToRecipeBindings {
		got := RecipeForInstrument(instID)
		if got != recipeID {
			t.Errorf("RecipeForInstrument(%q)=%q want %q", instID, got, recipeID)
		}
	}
}

func TestResetInstruments_IdempotentBindings(t *testing.T) {
	// Calling ResetInstruments twice shouldn't double-bind or accumulate state.
	ResetInstruments()
	ResetInstruments()
	for instID, recipeID := range instrumentToRecipeBindings {
		if got := RecipeForInstrument(instID); got != recipeID {
			t.Errorf("after double Reset, RecipeForInstrument(%q)=%q want %q", instID, got, recipeID)
		}
	}
}
