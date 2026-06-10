package audio

import (
	"strings"
	"testing"
)

// builtinDrumRecipeIDs is the list of Phase 1 drum recipes that must be
// registered via init() under every build tag. Phase 2 will extend this
// list to cover all 23 instruments plus 5 FM recipes.
var builtinDrumRecipeIDs = []string{
	"drum-snare",
	"drum-kick",
	"drum-hihat",
	"drum-clap",
	"drum-tom",
	"drum-cowbell",
}

// genericSynthParams is the 8-knob set that every drum recipe inherits.
// Order matters: same order as the C synth_params struct so the table
// drift between the two stays trivially auditable.
var genericSynthParamNames = []string{
	"pitch", "decay", "tone", "attack", "drive", "body", "color", "brightness",
}

func TestBuiltinDrumRecipes_RegisteredViaInit(t *testing.T) {
	regs := RecipeRegistrations()
	for _, id := range builtinDrumRecipeIDs {
		if regs[id] == nil {
			t.Errorf("recipe %q not registered via init()", id)
		}
	}
}

func TestBuiltinDrumRecipes_DeclareOnlyWiredGenericKnobs(t *testing.T) {
	// Post Synth-tab redesign, recipes declare ONLY the generic params their
	// C renderer actually reads (see recipeWiredParams). This test asserts
	// the registry filter matches the wired list — a missing wired param is
	// a silent UX bug (slider disappears); an extra declared param is a
	// silent no-op (slider drawn, does nothing). Both fail this test.
	regs := RecipeRegistrations()
	for _, id := range builtinDrumRecipeIDs {
		r := regs[id]
		if r == nil {
			continue
		}
		have := make(map[string]bool, len(r.Params))
		for _, p := range r.Params {
			have[p.Name] = true
		}
		wantWired := map[string]bool{}
		for _, name := range recipeWiredParams[id] {
			wantWired[name] = true
		}
		for name := range wantWired {
			if !have[name] {
				t.Errorf("recipe %q missing wired param %q", id, name)
			}
		}
		// Phase-3 per-recipe extras are NOT in recipeWiredParams (which only
		// tracks the generic-knob subset). Allow a declared param if it's
		// either in recipeWiredParams OR in recipeExtraParams[id].
		extras := map[string]bool{}
		for _, ex := range recipeExtraParams[id] {
			extras[ex.Name] = true
		}
		for name := range have {
			// Phase-8A: migrated recipes additionally expose the unified modular
			// pipeline stage params (osc/env/filter + gain + per-stage toggles +
			// post_enabled), appended by WiredParamsForRecipe — NOT silent no-ops:
			// the binding/push read them (proven by stage_params_test.go +
			// family_push_binding_parity_test.go). They are not in the generic
			// wired/extras tables, so allow them explicitly.
			if isAppendedStageName(id, name) {
				continue
			}
			if !wantWired[name] && !extras[name] {
				t.Errorf("recipe %q declares %q which is not in recipeWiredParams or recipeExtraParams (silent no-op slider)", id, name)
			}
		}
	}
}

func TestBuiltinDrumRecipes_GenericKnobsGroupedAsGeneric(t *testing.T) {
	regs := RecipeRegistrations()
	for _, id := range builtinDrumRecipeIDs {
		r := regs[id]
		if r == nil {
			continue
		}
		for _, p := range r.Params {
			isGeneric := false
			for _, g := range genericSynthParamNames {
				if p.Name == g {
					isGeneric = true
					break
				}
			}
			if isGeneric && p.Group != "generic" {
				t.Errorf("recipe %q param %q has Group=%q, want \"generic\"", id, p.Name, p.Group)
			}
		}
	}
}

func TestBuiltinDrumRecipes_FactoryReturnsRecipeWithMatchingID(t *testing.T) {
	for _, id := range builtinDrumRecipeIDs {
		r := NewRecipe(id)
		if r == nil {
			t.Errorf("NewRecipe(%q) returned nil", id)
			continue
		}
		if r.ID() != id {
			t.Errorf("NewRecipe(%q).ID() = %q", id, r.ID())
		}
		if r.Category() != "drum" {
			t.Errorf("NewRecipe(%q).Category() = %q want \"drum\"", id, r.Category())
		}
		if !strings.HasPrefix(id, "drum-") {
			t.Errorf("recipe id %q does not follow drum-* convention", id)
		}
	}
}

func TestBuiltinDrumRecipes_DefaultsAreIdentity(t *testing.T) {
	// pitch / tone / drive / body / brightness default to 0 (no change);
	// decay defaults to 1 (multiplier identity). The Synth-tab redesign
	// dropped the silent no-op knobs (attack, color) and started filtering
	// per-recipe via WiredParamsForRecipe, so each recipe declares only the
	// subset its C renderer actually reads. Defaults are still asserted to
	// be the canonical identity values for any param the recipe declares.
	wantDefaults := map[string]float64{
		"pitch":      0,
		"decay":      1,
		"tone":       0,
		"drive":      0,
		"body":       0,
		"brightness": 0,
	}
	for _, id := range builtinDrumRecipeIDs {
		d := RecipeDefaultParams(id)
		wired := map[string]bool{}
		for _, name := range recipeWiredParams[id] {
			wired[name] = true
		}
		for k, want := range wantDefaults {
			got, ok := d[k]
			if !wired[k] {
				if ok {
					t.Errorf("recipe %q default has %q=%v but the C renderer does not read it (recipeWiredParams says no)", id, k, got)
				}
				continue
			}
			if !ok || got != want {
				t.Errorf("recipe %q default[%q]=%v ok=%v; want %v", id, k, got, ok, want)
			}
		}
	}
}

func TestBuiltinDrumRecipes_RenderDoesNotPanic(t *testing.T) {
	// Smoke: every registered recipe must accept the standard buffer + default
	// params without panicking. Under -tags test the Render is a no-op stub
	// but the call path is exercised. Native-build parity is enforced by
	// xvfb-run tests in Phase 2 (synth_recipe_parity_test.go).
	for _, id := range builtinDrumRecipeIDs {
		r := NewRecipe(id)
		if r == nil {
			continue
		}
		buf := make([]float32, 1024)
		params := RecipeDefaultParams(id)
		// Three variants — none should panic, and on stub builds all should
		// leave the buffer in a deterministic state.
		for v := 0; v < 3; v++ {
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Errorf("recipe %q variant %d panicked: %v", id, v, rec)
					}
				}()
				r.Render(buf, 44100, len(buf), v, params)
			}()
		}
	}
}
