//go:build !test && !js

package audio

import (
	"math"
	"sort"
	"testing"
)

// family_push_binding_parity_test.go is the PUSH-vs-BINDING render-equality guard
// for every modular-migrated family. It proves the 3-way literal agreement —
// C kp_get fallback / native-binding NaN sentinel / push spelled-out literal —
// that nothing else asserts in-process.
//
// Both render paths feed the SAME C render_modular_p; they differ only in HOW the
// family fields reach the engine:
//
//   - PATH A (push / browser seam): ModularPushParams resolves every kp_get
//     fallback to a SPELLED-OUT literal in Go (NaN cannot survive JSON / the JS
//     block-fill, so the browser needs concrete values). recipeParamsToModular
//     then fills the sparse push map exactly as the JS block-fill does (absent
//     keys → modular schema identity). This reproduces what the browser renders.
//
//   - PATH B (native binding): NewRecipe(id).Render routes through the rebound
//     builtinFamilyRenderers closure, which passes NaN sentinels into the ABI so
//     C's kp_get(field, literal) recovers the literal.
//
// WHY NOT EXACT-HASH: the two paths are deliberately NOT byte-identical (the
// CLAUDE.md gotcha + bass_modular_push.go header both say "<1 ULP"). The seam is
// kp_get itself (src/c/synth_params.h):
//
//	static inline double kp_get(float v, double fallback) {
//	    return (v != v) ? fallback : (double)v;   // NaN → exact DOUBLE literal
//	}
//
// PATH B passes NaN, so C recovers the fallback at FULL DOUBLE precision
// (e.g. M_PI*0.5, 0.08, 1.2, 0.95). PATH A spells the same literal as a Go double
// that the modular ABI quantizes to float32 BEFORE kp_get widens it back — so a
// non-float-exact literal arrives ~1 float32-ULP off. Measured inherent diff at
// default: maxAbs ~1.2e-7, relative RMS ~1e-7 (a single float32 ULP near unity).
//
// A LITERAL DRIFT — the thing this guard exists to catch — is a ~1% change
// (0.95→0.94), five-plus orders of magnitude above that float-quantization noise
// floor. So the guard asserts per-sample drift ≤ pushBindMaxAbsTol = 1e-5,
// SCALED by the local signal magnitude (max(1, |bind|)): ~80× above the observed
// float-seam noise, ~1000× below any real literal drift. This is the same
// "correlate, don't byte-compare" contract the xplat gate uses, applied
// in-process where both paths are deterministic.
//
// WHY SCALED-BY-MAGNITUDE: the pre-output signal is not bounded to [-1,1]. A
// long-tail family rendered with a GROWING post-decay envelope (decay=max →
// post_decay=4 → exp(+6t), the legacy decay-slider semantics) amplifies the
// tail to magnitude ~66 deep in the buffer (it hard-clips to ±1 downstream).
// The float32-ULP seam (~6e-8 at the source) rides that amplification linearly,
// so the ABSOLUTE diff scales with the signal (tom-low decay=max: 1.5e-5 on a
// -66 sample → 2.3e-7 RELATIVE, a single float32 ULP). An absolute tolerance
// would false-positive on the amplified ULP while a real literal drift (which
// scales the whole signal ~1%) stays ~1000× above the scaled floor. The earlier
// (bass/kick) families never reached that amplification, so this scaling is
// byte-neutral for them and only relaxes the pathological growing-tail case.
const pushBindMaxAbsTol = 1e-5

func TestFamilyPushMatchesBindingRender(t *testing.T) {
	migrated := sortedModularMigratedRecipeIDs()
	if len(migrated) == 0 {
		t.Fatal("no modular-migrated recipes registered — guard is vacuous")
	}
	for _, recipeID := range migrated {
		recipeID := recipeID
		t.Run(recipeID, func(t *testing.T) {
			instID := instrumentForMigratedRecipe(t, recipeID)
			recipe := NewRecipe(recipeID)
			if recipe == nil {
				t.Fatalf("NewRecipe(%q) returned nil", recipeID)
			}

			for _, c := range parityCases(recipeID, recipe.ParamSchema()) {
				c := c
				t.Run(c.Name, func(t *testing.T) {
					// PATH A: production push seam (instrument id + overlay → merged →
					// translated modular block), then block-filled by recipeParamsToModular
					// exactly as the JS side fills absent keys with schema identity.
					push, ok := modularPushParamsForInstrument(instID, c.Overlay)
					if !ok {
						t.Fatalf("modularPushParamsForInstrument(%q) returned !ok for a migrated recipe", instID)
					}
					bufPush := make([]float32, oracleSamples)
					renderModularP(bufPush, oracleSR, oracleSamples, recipeParamsToModular(push))

					// PATH B: the recipe's actual bound renderer (NaN-sentinel binding).
					merged := MergeRecipeDefaults(recipeID, c.Overlay)
					bufBind := make([]float32, oracleSamples)
					recipe.Render(bufBind, oracleSR, oracleSamples, 0, merged)

					maxRel, at := maxScaledDiff(bufPush, bufBind)
					if maxRel > pushBindMaxAbsTol {
						t.Errorf("push vs binding render drift (literal disagreement?): maxScaledDiff=%.3e at sample %d (tol=%.1e)\n  push[%d]=%v\n  bind[%d]=%v",
							maxRel, at, pushBindMaxAbsTol, at, bufPush[at], at, bufBind[at])
					}
				})
			}
		})
	}
}


// maxScaledDiff returns the largest per-sample absolute difference NORMALIZED by
// the local signal magnitude (max(1, |b|)) and the index where it occurs. For
// near-unity signals this equals the raw abs diff; for amplified pre-clip tails
// (a growing post-decay envelope can push the signal to magnitude ~66) it divides
// out the linear amplification so the float32-ULP seam stays at its true ~1e-7
// relative size instead of false-positiving on the amplified absolute value.
func maxScaledDiff(a, b []float32) (float64, int) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var maxRel float64
	at := -1
	for i := 0; i < n; i++ {
		denom := math.Abs(float64(b[i]))
		if denom < 1.0 {
			denom = 1.0
		}
		d := math.Abs(float64(a[i])-float64(b[i])) / denom
		if d > maxRel {
			maxRel = d
			at = i
		}
	}
	return maxRel, at
}

// parityCases extends the legacy-oracle sweep with the Phase-8A appended modular
// stage params (osc/env/filter/drive/gain + per-stage toggles). The oracle sweep
// deliberately skips those (no legacy bytes to pin against), but the push and the
// native binding MUST still agree on them — both must write the same effective
// modular block when a user toggles/edits a stage. This adds one default+min+max
// case per genuinely-appended stage param (collision exclusions, which are NOT
// appended stage params, stay out — they are already covered by the oracle sweep).
func parityCases(recipeID string, defs []ParamDef) []oracleCase {
	cases := oracleCases(recipeID, defs)
	collision := recipeExistingStageCollisionSet(recipeID)
	for _, d := range defs {
		// The appended stage params ship in the hidden UI group (Phase-8A); they
		// are still real, drivable params the push and binding MUST agree on, so
		// do NOT skip them on group here — only on the appended-stage predicate.
		if !isModularStageParamName(d.Name) || collision[d.Name] {
			continue
		}
		cases = append(cases,
			oracleCase{Name: "stage_" + d.Name + "=min", Overlay: RecipeParams{d.Name: d.Min}},
			oracleCase{Name: "stage_" + d.Name + "=max", Overlay: RecipeParams{d.Name: d.Max}},
		)
	}
	return cases
}

// sortedModularMigratedRecipeIDs returns the migrated recipe ids in deterministic
// order (the registry is a map).
func sortedModularMigratedRecipeIDs() []string {
	ids := make([]string, 0, len(modularMigrations))
	for id := range modularMigrations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// instrumentForMigratedRecipe returns a shipped instrument id whose live binding
// resolves to recipeID, so the test drives modularPushParamsForInstrument through
// the same id→overlay seam production uses. Fails if no instrument binds to the
// migrated recipe (a migration that no instrument plays is dead code).
func instrumentForMigratedRecipe(t *testing.T, recipeID string) string {
	t.Helper()
	// Deterministic: pick the lexicographically-smallest instrument id bound to
	// recipeID, reading the live binding table via RecipeForInstrument (what the
	// production seam consults).
	var ids []string
	for instID := range builtinInstrumentRecipeBindings {
		if RecipeForInstrument(instID) == recipeID {
			ids = append(ids, instID)
		}
	}
	if len(ids) == 0 {
		t.Fatalf("no shipped instrument binds to migrated recipe %q", recipeID)
	}
	sort.Strings(ids)
	return ids[0]
}
