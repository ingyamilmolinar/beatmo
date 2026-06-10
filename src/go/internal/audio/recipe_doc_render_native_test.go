//go:build !test && !js

package audio

import (
	"math"
	"reflect"
	"testing"
)

// Native build replication guarantees, in two complementary tests:
//
//   1. TestRecipeDocRegistrationShapeIdenticalNative — proves the
//      RecipeDoc → ToRegistration projection produces a
//      RecipeRegistration whose Params slice is field-for-field
//      identical to the production-registered one. Since both the
//      legacy and the doc path point at the same C renderer (looked up
//      in builtinRecipeRenderers by recipe id), shape identity here is
//      what guarantees behavioural identity in production —
//      byte-equality of an isolated render call is structurally
//      unverifiable on native (the C renderers share global ma_noise
//      state, so a second invocation produces different bytes even
//      from the same recipe).
//
//   2. TestRecipeDocRendersFiniteAllRecipes — proves the doc-registered
//      shadow recipe (a) registers cleanly through
//      RegisterPluginRecipeFromDoc, (b) renders without panicking, (c)
//      produces a buffer of the requested length with all finite
//      samples and at least one non-zero sample. This is proof-of-life:
//      it catches a doc projection that registers but silently
//      mis-routes its renderer.
//
// Combined with TestRecipeDocRoundtripAllRecipes (which iterates the
// registry under -tags test) and the existing parity goldens
// (parity_fixture_*.json + xplat_audio_compare.browser.test.js, which
// pin native byte output through the production registry path), this
// is the strongest replication guarantee the test architecture can
// achieve on native. The byte-identity claim the original plan made
// was unachievable with a delegating-renderer test; explicit registration
// shape + render proof-of-life + production parity goldens cover the
// same property in three layers.

// TestRecipeDocRegistrationShapeIdenticalNative asserts that every
// shipped builtin recipe's doc projection materialises to a
// RecipeRegistration whose Params slice is field-for-field identical
// to the production-registered one. Catches drift between
// BuildBuiltinRecipeDocs and the registry. Native-only because the
// production registration of FM extras (fm-bass body, fm-bell/lead/
// epiano brightness) lives in synth_recipe_wired.go and only fires
// under !test.
func TestRecipeDocRegistrationShapeIdenticalNative(t *testing.T) {
	regs := RecipeRegistrations()
	docs := BuildBuiltinRecipeDocs()
	if got, want := len(docs), len(builtinRecipeDescriptors); got != want {
		t.Fatalf("BuildBuiltinRecipeDocs len = %d, want %d", got, want)
	}
	for _, doc := range docs {
		t.Run(doc.ID, func(t *testing.T) {
			reg := regs[doc.ID]
			if reg == nil {
				t.Fatalf("registry has no entry for %q", doc.ID)
			}
			roundTrip := doc.ToRegistration(func() SynthRecipe { return nil })
			if roundTrip.ID != reg.ID || roundTrip.DisplayName != reg.DisplayName || roundTrip.Category != reg.Category {
				t.Fatalf("shape mismatch: got id=%q display=%q category=%q; want id=%q display=%q category=%q",
					roundTrip.ID, roundTrip.DisplayName, roundTrip.Category,
					reg.ID, reg.DisplayName, reg.Category)
			}
			if !reflect.DeepEqual(roundTrip.Params, reg.Params) {
				t.Fatalf("Params field-for-field mismatch:\n  doc.ToRegistration: %+v\n  registry:           %+v", roundTrip.Params, reg.Params)
			}
		})
	}
}

func TestRecipeDocRendersFiniteAllRecipes(t *testing.T) {
	const (
		sampleRate = 48000
		samples    = 4800
	)
	for _, doc := range BuildBuiltinRecipeDocs() {
		t.Run(doc.ID, func(t *testing.T) {
			source := NewRecipe(doc.ID)
			if source == nil {
				t.Fatalf("NewRecipe(%q) = nil — production registration missing", doc.ID)
			}
			shadowID := doc.ID + ".__doctest"
			t.Cleanup(func() { UnregisterRecipeForTest(shadowID) })
			shadowDoc := doc
			shadowDoc.ID = shadowID
			shadowDoc.BaseRecipe = doc.ID
			shadowDoc.Origin = OriginUser
			if err := RegisterPluginRecipeFromDoc(shadowDoc, recipeProviderAdapter{source: source}); err != nil {
				t.Fatalf("RegisterPluginRecipeFromDoc(%q): %v", shadowID, err)
			}
			shadow := NewRecipe(shadowID)
			if shadow == nil {
				t.Fatalf("NewRecipe(%q) = nil after registration", shadowID)
			}

			defaults := RecipeDefaultParams(doc.ID)
			buf := make([]float32, samples)
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						t.Fatalf("shadow.Render panicked: %v", rec)
					}
				}()
				shadow.Render(buf, sampleRate, samples, 0, defaults)
			}()

			nonZero := 0
			for i, v := range buf {
				f := float64(v)
				if math.IsNaN(f) || math.IsInf(f, 0) {
					t.Fatalf("sample %d is non-finite (%v)", i, v)
				}
				if v != 0 {
					nonZero++
				}
			}
			if nonZero == 0 {
				t.Fatalf("shadow render produced all-zero output — renderer not wired through the doc path")
			}
		})
	}
}

// recipeProviderAdapter wraps an already-registered SynthRecipe so it can
// be re-registered via RegisterPluginRecipeFromDoc as a shadow recipe.
// Duplicated from recipe_doc_render_stub_test.go because that file is
// build-tagged for `test || js`; under !test && !js this file owns the
// adapter type so the same symbol is available in both builds.
type recipeProviderAdapter struct {
	source SynthRecipe
}

func (a recipeProviderAdapter) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	a.source.Render(buf, sampleRate, samples, variant, p)
}
