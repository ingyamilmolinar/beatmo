//go:build !test && !js

package audio

// nativeDrumRecipe wraps a CGo cParamRenderer (renderSnareP, renderKickP, …)
// behind the SynthRecipe interface. The C renderer carries its own noise
// state via ma_noise, so subsequent calls produce naturally-different output
// — that property is what drives the 3-variant round-robin in voice_cache.
// `variant` is therefore informational on this path; plugin recipes (v2+)
// will use it as an explicit seed.
type nativeDrumRecipe struct {
	id       string
	display  string
	category string
	params   []ParamDef
	render   cParamRenderer
}

func (r *nativeDrumRecipe) ID() string             { return r.id }
func (r *nativeDrumRecipe) DisplayName() string    { return r.display }
func (r *nativeDrumRecipe) Category() string       { return r.category }
func (r *nativeDrumRecipe) ParamSchema() []ParamDef { return r.params }

func (r *nativeDrumRecipe) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	r.render(buf, sampleRate, samples, recipeParamsToSynth(p))
}

// recipeParamsToSynth materializes a SynthParams struct from a RecipeParams
// map. Missing keys take the identity value defined in
// synth_params.h:sp_decay/sp_attack/etc — i.e. the value sp_*(nil) would
// substitute. This keeps the recipe → C bridge bit-identical to the legacy
// "pass nil for hardcoded defaults" path when the recipe is at its declared
// defaults.
func recipeParamsToSynth(p RecipeParams) SynthParams {
	get := func(name string, identity float64) float64 {
		if v, ok := p[name]; ok {
			return v
		}
		return identity
	}
	return SynthParams{
		Pitch:      get("pitch", 0),
		Decay:      get("decay", 1),
		Tone:       get("tone", 0),
		Drive:      get("drive", 0),
		Body:       get("body", 0),
		Brightness: get("brightness", 0),
	}
}

// builtinRecipeRenderers maps recipe id → CGo wrapper. Owned by this file
// so the stub build (test||js) is free of any CGo symbol references. Phase
// 2 extends this to all 25 recipes (20 drums + 5 FM); Phase 1 shipped only
// the first 6 drums.
var builtinRecipeRenderers = map[string]cParamRenderer{
	// Phase 1 drums.
	"drum-snare":   renderSnareP,
	"drum-kick":    renderKickP,
	"drum-hihat":   renderHiHatP,
	"drum-clap":    renderClapP,
	"drum-tom":     renderTomP,
	"drum-cowbell": renderCowbellP,
	// Phase 2 drums (14).
	"drum-open-hihat":      renderOpenHihatP,
	"drum-tom-high":        renderTomHighP,
	"drum-tom-low":         renderTomLowP,
	"drum-bass-guitar":     renderBassGuitarP,
	"drum-sub-bass":        renderSubBassP,
	"drum-snare-rimshot":   renderSnareRimshotP,
	"drum-snare-sidestick": renderSnareSidestickP,
	"drum-kick-deep":       renderKickDeepP,
	"drum-kick-punchy":     renderKickPunchyP,
	"drum-kick-lofi":       renderKickLofiP,
	"drum-kick-tight":      renderKickTightP,
	"drum-shaker":          renderShakerP,
	"drum-ride":            renderRideP,
	"drum-crash":           renderCrashP,
	// Phase 2 FM (5).
	"fm-bass":   renderFMBassP,
	"fm-bell":   renderFMBellP,
	"fm-lead":   renderFMLeadP,
	"fm-epiano": renderFMEPianoP,
	"fm-pluck":  renderFMPluckP,
}

func init() {
	registerBuiltinRecipes(func(id, display, category string, params []ParamDef) func() SynthRecipe {
		render := builtinRecipeRenderers[id]
		return func() SynthRecipe {
			return &nativeDrumRecipe{
				id:       id,
				display:  display,
				category: category,
				params:   params,
				render:   render,
			}
		}
	})
}
