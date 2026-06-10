//go:build !test && !js

package audio

import (
	"testing"
)

// Phase-1 schema-vs-implementation contract test. Extends the existing
// TestPhase2Native_RecipeRenderHonorsParamMutations (which only mutates
// `decay`) to cover EVERY param the recipe's WiredParamsForRecipe whitelist
// claims is wired. The failure modes this catches:
//
//   - A recipe declares a param in recipeWiredParams but the C _p() variant
//     never reads it (silent no-op — the user-facing bug that motivated
//     [[project_synth_pipeline_architecture]]).
//   - Phase 3 adds a new wired param (e.g. drum-kick "fundamental_hz") to
//     the recipe and forgets to wire it on the C side; this test fails until
//     the C renderer reads the new param.
//
// We render at the default param set and at a non-identity value for each
// wired param. Outputs must differ by at least sumSq > epsilon. The
// non-identity values come from the ParamDef itself (Min when the default
// is at/above midpoint, else Max) so we test what the user can actually dial
// in via the slider.
func TestRecipeRender_EveryWiredParamMutatesOutput(t *testing.T) {
	const samples = 44100 / 8
	const sumSqEpsilon = 1e-8

	for _, recID := range RecipeOrder() {
		recID := recID
		t.Run(recID, func(t *testing.T) {
			r := NewRecipe(recID)
			if r == nil {
				t.Fatalf("NewRecipe(%q) returned nil", recID)
			}
			if r.Category() == modularRecipeCategory {
				// The modular voice uses ModularSynthParamDefs (not the wired-
				// generic set) and its FM-stage params only affect output when
				// osc_type == FM, so the unconditional "every wired param
				// mutates output" contract doesn't apply. Context-aware
				// coverage lives in TestRenderModularP_EveryParamMutatesInContext.
				t.Skip("modular voice covered by dedicated context-aware param-effect test")
			}
			defaults := RecipeDefaultParams(recID)

			// Default render baseline.
			base := make([]float32, samples)
			r.Render(base, 44100, samples, 0, defaults)

			wired := WiredParamsForRecipe(recID)
			if len(wired) == 0 {
				t.Fatalf("recipe %q: WiredParamsForRecipe returned no params (declared in recipeWiredParams?)", recID)
			}

			collision := recipeExistingStageCollisionSet(recID)
			for _, def := range wired {
				def := def
				// Phase-8A: the appended modular stage params (osc/env/filter/drive +
				// toggles) have CONTEXT-DEPENDENT effects — osc_type does nothing until
				// osc_enabled=1, filter_cutoff nothing until filter_enabled=1 — so the
				// unconditional "every value mutates output" contract doesn't apply.
				// Their context-aware coverage lives in stage_params_test.go. A stage
				// NAME that is actually a family/generic knob on this recipe (the generic
				// `drive`; FM `fm_op*`) is a collision exclusion, NOT an appended stage
				// param, so it stays in this sweep.
				if isModularStageParamName(def.Name) && !collision[def.Name] {
					continue
				}
				t.Run(def.Name, func(t *testing.T) {
					target := pickNonDefaultValue(def)
					if target == def.Default {
						t.Skipf("param %q: Min, Max, and Default coincide — nothing to mutate", def.Name)
					}
					mutated := cloneRecipeParams(defaults)
					mutated[def.Name] = target

					out := make([]float32, samples)
					r.Render(out, 44100, samples, 0, mutated)

					var diffSq float64
					for i := range out {
						d := float64(out[i] - base[i])
						diffSq += d * d
					}
					if diffSq < sumSqEpsilon {
						t.Errorf("recipe %q param %q: mutating %v -> %v did not change output (diffSq=%g)",
							recID, def.Name, def.Default, target, diffSq)
					}
				})
			}
		})
	}
}

// pickNonDefaultValue returns a value inside [Min, Max] that differs from
// Default. Prefers Min when Default sits in the upper half (so the mutation
// is large), otherwise Max. Returns Default unchanged when Min == Max (a
// degenerate but legal ParamDef where the test will skip).
func pickNonDefaultValue(d ParamDef) float64 {
	if d.Min == d.Max {
		return d.Default
	}
	mid := (d.Min + d.Max) / 2
	if d.Default >= mid {
		return d.Min
	}
	return d.Max
}
