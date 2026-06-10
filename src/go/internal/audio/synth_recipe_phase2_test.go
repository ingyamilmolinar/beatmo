package audio

import (
	"strings"
	"testing"
)

// Phase 2: assertions over the full 25-recipe + 30-instrument registry.

func TestPhase2_RegisteredRecipeCountAtLeast25(t *testing.T) {
	// Every Phase-1 + Phase-2 descriptor registers exactly one entry.
	// Tests may register additional fakeRecipes transiently, so we allow
	// the count to be >= 25 (the floor).
	want := 25
	got := len(builtinRecipeDescriptors)
	if got < want {
		t.Errorf("builtinRecipeDescriptors has %d entries, want at least %d", got, want)
	}

	regs := RecipeRegistrations()
	for _, d := range builtinRecipeDescriptors {
		if regs[d.ID] == nil {
			t.Errorf("descriptor %q not in RecipeRegistrations()", d.ID)
		}
	}
}

func TestPhase2_AllRecipeIDsFollowNamingConvention(t *testing.T) {
	// Drums use the "drum-" prefix so the recipe namespace is collision-free
	// with future plugin recipes; FM keeps its "fm-" prefix for parity with
	// the instrument ids (which the UI surfaces verbatim).
	for _, d := range builtinRecipeDescriptors {
		switch d.Category {
		case "drum":
			if !strings.HasPrefix(d.ID, "drum-") {
				t.Errorf("drum recipe %q does not start with \"drum-\"", d.ID)
			}
		case "fm":
			if !strings.HasPrefix(d.ID, "fm-") {
				t.Errorf("FM recipe %q does not start with \"fm-\"", d.ID)
			}
		case modularRecipeCategory:
			if !strings.HasPrefix(d.ID, "synth-") {
				t.Errorf("modular recipe %q does not start with \"synth-\"", d.ID)
			}
		default:
			t.Errorf("recipe %q has unrecognised category %q", d.ID, d.Category)
		}
	}
}

func TestPhase2_EveryInstrumentBindingResolvesToRegisteredRecipe(t *testing.T) {
	// With Phase 2 the binding table now covers all 30 user-visible
	// instruments (10 base drums + 4 bass/sub-bass slots + 14 -1/-2/ghost
	// variants + 5 FM base + 5 FM -1 variants). Every binding must point at
	// a recipe in builtinRecipeDescriptors.
	known := make(map[string]bool, len(builtinRecipeDescriptors))
	for _, d := range builtinRecipeDescriptors {
		known[d.ID] = true
	}
	for instID, recipeID := range builtinInstrumentRecipeBindings {
		if !known[recipeID] {
			t.Errorf("instrument %q binds to recipe %q which is not in builtinRecipeDescriptors", instID, recipeID)
		}
	}
}

func TestPhase2_EveryBuiltinInstrumentIDHasABinding(t *testing.T) {
	// BuiltinInstrumentIDs is the catalog of shipped instruments. Every one
	// must have a recipe binding so user-edited synth params propagate at
	// trigger time. (Future: WAV/sample-based instruments may be exempt.)
	for _, id := range BuiltinInstrumentIDs {
		if _, ok := builtinInstrumentRecipeBindings[id]; !ok {
			t.Errorf("builtin instrument %q has no entry in builtinInstrumentRecipeBindings", id)
		}
	}
}

func TestPhase2_AllRecipesRenderAtDefaultsWithoutPanic(t *testing.T) {
	// Smoke: every registered recipe accepts default params + a 1024-sample
	// buffer at 44.1 kHz without panicking. Under -tags test Render is a
	// no-op stub; native build exercises the real C dispatch.
	for _, id := range RecipeOrder() {
		r := NewRecipe(id)
		if r == nil {
			t.Errorf("NewRecipe(%q) returned nil", id)
			continue
		}
		buf := make([]float32, 1024)
		params := RecipeDefaultParams(id)
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Errorf("recipe %q panicked at defaults: %v", id, rec)
				}
			}()
			r.Render(buf, 44100, len(buf), 0, params)
		}()
	}
}
