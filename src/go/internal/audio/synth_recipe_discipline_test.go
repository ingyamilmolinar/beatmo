package audio

import (
	"testing"
)

// TestAllRegisteredRecipes_HaveValidRegistration asserts every recipe in
// the registry (built-ins plus anything registered by tests) is well-formed.
// This catches accidental registrations with empty ID, missing factory,
// duplicate param names, etc. — the v1 freezing surface for plugin
// registrations is encoded here.
func TestAllRegisteredRecipes_HaveValidRegistration(t *testing.T) {
	regs := RecipeRegistrations()
	if len(regs) == 0 {
		t.Fatal("registry is empty; built-in recipes failed to register via init()")
	}
	for id, reg := range regs {
		if reg.ID == "" {
			t.Errorf("recipe with key %q has empty ID", id)
		}
		if reg.ID != id {
			t.Errorf("registry key %q does not match RecipeRegistration.ID=%q", id, reg.ID)
		}
		if reg.DisplayName == "" {
			t.Errorf("recipe %q has empty DisplayName", id)
		}
		if reg.Category == "" {
			t.Errorf("recipe %q has empty Category", id)
		}
		if reg.New == nil {
			t.Errorf("recipe %q has nil New factory", id)
		}
		if len(reg.Params) == 0 {
			t.Errorf("recipe %q declares no ParamDefs", id)
		}
		paramNames := make(map[string]bool, len(reg.Params))
		for _, p := range reg.Params {
			if paramNames[p.Name] {
				t.Errorf("recipe %q has duplicate param name %q", id, p.Name)
			}
			paramNames[p.Name] = true
			if err := validateParamDef(p); err != nil {
				t.Errorf("recipe %q param %q failed validation: %v", id, p.Name, err)
			}
		}
	}
}

// TestRecipeOrderMatchesRegistrationMap asserts every recipe ID in
// RecipeOrder is also in RecipeRegistrations (no orphans) and vice versa
// (no missing-from-order). This catches concurrency bugs in the registry
// that would otherwise let an entry exist in only one of the two views.
func TestRecipeOrderMatchesRegistrationMap(t *testing.T) {
	order := RecipeOrder()
	regs := RecipeRegistrations()
	if len(order) != len(regs) {
		t.Fatalf("RecipeOrder has %d entries; RecipeRegistrations has %d", len(order), len(regs))
	}
	for _, id := range order {
		if regs[id] == nil {
			t.Errorf("RecipeOrder contains %q but RecipeRegistrations does not", id)
		}
	}
	seen := make(map[string]bool, len(order))
	for _, id := range order {
		if seen[id] {
			t.Errorf("RecipeOrder has duplicate entry %q", id)
		}
		seen[id] = true
	}
	for id := range regs {
		if !seen[id] {
			t.Errorf("RecipeRegistrations contains %q but RecipeOrder does not", id)
		}
	}
}

// TestNewRecipe_FactoryReturnsRecipeWithMatchingID asserts every factory
// constructs an object whose ID() agrees with the registry key. A factory
// that returns a recipe with a divergent ID would silently break the
// instrument → recipe binding.
func TestNewRecipe_FactoryReturnsRecipeWithMatchingID(t *testing.T) {
	for _, id := range RecipeOrder() {
		r := NewRecipe(id)
		if r == nil {
			t.Errorf("NewRecipe(%q) returned nil but recipe is in RecipeOrder", id)
			continue
		}
		if r.ID() != id {
			t.Errorf("NewRecipe(%q).ID()=%q — factory returns a recipe whose ID disagrees with its registry key", id, r.ID())
		}
	}
}

// TestPhase1InstrumentBindings_PointAtRegisteredRecipes asserts every
// Phase-1 instrument-to-recipe binding refers to a recipe that exists
// in the registry. Catches drift between builtinInstrumentRecipeBindings
// and builtinRecipeDescriptors at compile-time (well, test-time).
func TestPhase1InstrumentBindings_PointAtRegisteredRecipes(t *testing.T) {
	regs := RecipeRegistrations()
	for instID, recipeID := range builtinInstrumentRecipeBindings {
		if regs[recipeID] == nil {
			t.Errorf("instrument %q binds to recipe %q which is not registered", instID, recipeID)
		}
	}
}

// TestEveryPhase1RecipeHasAtLeastOneInstrument asserts no Phase-1 recipe
// is registered but never bound to an instrument. A registered-but-orphan
// recipe is dead code; the discipline check forces us to either bind it
// or unregister it.
func TestEveryPhase1RecipeHasAtLeastOneInstrument(t *testing.T) {
	boundRecipes := make(map[string]bool, len(builtinInstrumentRecipeBindings))
	for _, recipeID := range builtinInstrumentRecipeBindings {
		boundRecipes[recipeID] = true
	}
	for _, d := range builtinRecipeDescriptors {
		if !boundRecipes[d.ID] {
			t.Errorf("recipe %q is registered but no instrument binds to it (orphan)", d.ID)
		}
	}
}
