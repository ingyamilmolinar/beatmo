//go:build test || js

package audio

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

// recipeProviderAdapter wraps an already-registered SynthRecipe so it can
// be re-registered via RegisterPluginRecipeFromDoc as a shadow recipe. The
// shadow recipe delegates rendering back to the original so the test
// proves the doc-registered path produces output identical to the
// production-registered path — which under -tags test (zero-fill stubs)
// means literal byte-equality.
type recipeProviderAdapter struct {
	source SynthRecipe
}

func (a recipeProviderAdapter) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	a.source.Render(buf, sampleRate, samples, variant, p)
}

// TestRecipeDocRendersIdenticalAllRecipes proves that for every shipped
// builtin (drums + FM), registering the same RecipeDoc via the plugin path
// produces a recipe whose Render output is bit-identical to the production
// recipe under -tags test. The stub renderers are deterministic zero-fill,
// so bytes.Equal works here and any divergence indicates a real registration
// drift (wrong recipe id mapping, lost ParamSeed overlay, etc.).
//
// Native byte-identity is structurally unverifiable from a Go test —
// the C renderers share global ma_noise state, so calling source.Render
// then shadow.Render advances the noise generator twice and the two
// buffers diverge by design. The native counterpart in
// recipe_doc_render_native_test.go therefore asserts registration shape
// identity (Params field-for-field equality) + render proof-of-life
// (finite + non-zero samples). Combined with the parity goldens
// (parity_fixture_*.json + xplat_audio_compare.browser.test.js), the
// production byte-identity property remains pinned end-to-end.
//
// Subtest names are the recipe id so the failure log says "fm-bass" when
// it breaks.
func TestRecipeDocRendersIdenticalAllRecipes(t *testing.T) {
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
			shadowDoc.Origin = OriginUser // doctest treats it as a runtime recipe
			if err := RegisterPluginRecipeFromDoc(shadowDoc, recipeProviderAdapter{source: source}); err != nil {
				t.Fatalf("RegisterPluginRecipeFromDoc(%q): %v", shadowID, err)
			}
			shadow := NewRecipe(shadowID)
			if shadow == nil {
				t.Fatalf("NewRecipe(%q) = nil after registration", shadowID)
			}

			defaults := RecipeDefaultParams(doc.ID)
			bufA := make([]float32, samples)
			bufB := make([]float32, samples)
			source.Render(bufA, sampleRate, samples, 0, defaults)
			shadow.Render(bufB, sampleRate, samples, 0, defaults)

			if !bytes.Equal(floatBytes(bufA), floatBytes(bufB)) {
				// Locate first divergence to make stub regressions cheap to debug.
				for i := range bufA {
					if bufA[i] != bufB[i] {
						t.Fatalf("render output diverges at sample %d: source=%g shadow=%g (this should be impossible under -tags test stubs)", i, bufA[i], bufB[i])
					}
				}
				t.Fatalf("render output bytes differ but no sample-level diff found")
			}
		})
	}
}

// floatBytes reinterprets a []float32 as []byte for bytes.Equal. Uses the
// host byte order; only used for in-process buffer comparison, never
// serialised. Same approach as hashRecipeParams uses for the deterministic
// FNV hash, only here without sorting since we want order-sensitive equality.
func floatBytes(buf []float32) []byte {
	out := make([]byte, len(buf)*4)
	for i, v := range buf {
		binary.LittleEndian.PutUint32(out[i*4:], math.Float32bits(v))
	}
	return out
}
