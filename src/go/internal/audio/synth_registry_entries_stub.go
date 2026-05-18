//go:build test || js

package audio

// Under the test and js build tags the audio package has no CGo. The 6
// Phase-1 drum recipes are still registered so the registry-shape tests pass
// uniformly under every build, but their Render is a deterministic no-op
// that simply zeroes the buffer. Audio-correctness for these recipes is
// covered by the native (!test && !js) tests in synth_recipe_parity_test.go
// (Phase 2) and the browser xplat parity test in xplat_synth_recipe_parity.

type stubDrumRecipe struct {
	id       string
	display  string
	category string
	params   []ParamDef
}

func (r *stubDrumRecipe) ID() string             { return r.id }
func (r *stubDrumRecipe) DisplayName() string    { return r.display }
func (r *stubDrumRecipe) Category() string       { return r.category }
func (r *stubDrumRecipe) ParamSchema() []ParamDef { return r.params }

func (r *stubDrumRecipe) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	if samples > len(buf) {
		samples = len(buf)
	}
	for i := 0; i < samples; i++ {
		buf[i] = 0
	}
}

func init() {
	registerBuiltinRecipes(func(id, display, category string, params []ParamDef) func() SynthRecipe {
		return func() SynthRecipe {
			return &stubDrumRecipe{
				id:       id,
				display:  display,
				category: category,
				params:   params,
			}
		}
	})
}
