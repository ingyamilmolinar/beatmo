//go:build !test && !js

package audio

// nativeDrumRecipe wraps a CGo cParamRenderer (renderHiHatP, renderCowbellP, …)
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

func (r *nativeDrumRecipe) ID() string              { return r.id }
func (r *nativeDrumRecipe) DisplayName() string     { return r.display }
func (r *nativeDrumRecipe) Category() string        { return r.category }
func (r *nativeDrumRecipe) ParamSchema() []ParamDef { return r.params }

func (r *nativeDrumRecipe) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	// Family-parameterized engines receive the full RecipeParams so the
	// curated family knobs (FM operator ratios, kick harmonics, …) reach
	// the C param block; recipes whose family phase hasn't landed yet keep
	// the generic 7-knob path.
	if fr := builtinFamilyRenderers[r.id]; fr != nil {
		fr(buf, sampleRate, samples, p)
		return
	}
	r.render(buf, sampleRate, samples, recipeParamsToSynth(p))
}

// cRecipeRenderer renders with the full merged RecipeParams map (family-
// parameterized engines). Sibling of cParamRenderer, which only carries the
// generic 7-knob synth_params surface.
type cRecipeRenderer func(buf []float32, sampleRate, samples int, p RecipeParams)

// builtinFamilyRenderers maps recipe id → family-aware renderer. A recipe
// present here renders its curated family knobs; absent recipes fall back to
// the legacy generic-only path in nativeDrumRecipe.Render. Grown family by
// family as the native-deprecation migration lands.
var builtinFamilyRenderers = map[string]cRecipeRenderer{
	// FM family — rebound onto the modular engine in fm_modular_binding.go's init()
	// (Phase-7, the LAST family); no legacy fm-* entry here (render_fm_*_p C paths
	// + the Go FMParams adapter deleted in the Phase-7 cutover, mirroring
	// bass/kick/tom/snare/cymbal).
	// Kick family — rebound onto the modular engine in kick_modular_binding.go's
	// init(); no legacy kick entry here (render_kick* C paths deleted in the
	// Phase-3 cutover, mirroring bass).
	// Tom family — rebound onto the modular engine in tom_modular_binding.go's
	// init(); no legacy tom entry here (render_tom* C paths deleted in the Phase-4
	// cutover, mirroring bass/kick).
	// Snare family — rebound onto the modular engine in snare_modular_binding.go's
	// init(); no legacy snare entry here (render_snare* / render_clap C paths
	// deleted in the Phase-5 cutover, mirroring bass/kick/tom).
	// Cymbal family — rebound onto the modular engine in
	// cymbal_modular_binding.go's init(); no legacy cymbal entry here
	// (render_hihat* / render_open_hihat* / render_cowbell* / render_shaker* /
	// render_ride* / render_crash* C paths deleted in the Phase-6 cutover,
	// mirroring bass/kick/tom/snare).
	// Bass family — rebound onto the modular engine in bass_modular_binding.go's
	// init(); no legacy bass entry here (render_bass_guitar* / render_sub_bass*
	// C paths deleted in the Phase-2 cutover).
	// Kick family — see the comment above; rebound in kick_modular_binding.go.
}

// cymbalFamilyRenderer deleted with the cymbal-family migration (Phase-6): the
// cymbal recipes render via the modular binding (cymbalRecipeToModular), rebound
// in cymbal_modular_binding.go's init(), not through a legacy CymbalParams
// adapter.

// snareFamilyRenderer deleted with the snare-family migration (Phase-5): the
// snare recipes render via the modular binding (snareRecipeToModular), rebound in
// snare_modular_binding.go's init(), not through a legacy SnareParams adapter.

// tomFamilyRenderer deleted with the tom-family migration (Phase-4): the tom
// recipes render via the modular binding (tomRecipeToModular), rebound in
// tom_modular_binding.go's init(), not through a legacy TomParams adapter.

// kickFamilyRenderer deleted with the kick-family migration (Phase-3): the kick
// recipes render via the modular binding (kickRecipeToModular), rebound in
// kick_modular_binding.go's init(), not through a legacy KickParams adapter.

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
		Pitch:       get("pitch", 0),
		Decay:       get("decay", 1),
		Tone:        get("tone", 0),
		Drive:       get("drive", 0),
		Body:        get("body", 0),
		Brightness:  get("brightness", 0),
		Fundamental: get("fundamental", 0),
	}
}

// builtinRecipeRenderers maps recipe id → CGo wrapper. Owned by this file
// so the stub build (test||js) is free of any CGo symbol references. Phase
// 2 extends this to all 25 recipes (20 drums + 5 FM); Phase 1 shipped only
// the first 6 drums.
var builtinRecipeRenderers = map[string]cParamRenderer{
	// drum-snare / drum-clap (+ rimshot / sidestick) render via the modular binding
	// (builtinFamilyRenderers, rebound in snare_modular_binding.go) — no legacy
	// renderSnareP / renderClapP / renderSnare*P entry (those C/Go paths are deleted
	// Phase-5).
	// drum-kick renders via the modular binding (builtinFamilyRenderers, rebound
	// in kick_modular_binding.go) — no legacy renderKickP entry (deleted Phase-3).
	// drum-tom (+ high/low) render via the modular binding (builtinFamilyRenderers,
	// rebound in tom_modular_binding.go) — no legacy renderTomP / renderTom*P entry
	// (those C/Go paths are deleted Phase-4).
	// drum-hihat / open-hihat / cowbell / shaker / ride / crash render via the
	// modular binding (builtinFamilyRenderers, rebound in cymbal_modular_binding.go)
	// — no legacy renderHiHatP / renderOpenHihatP / renderCowbellP / renderShakerP /
	// renderRideP / renderCrashP entry (those C/Go paths are deleted Phase-6).
	// drum-bass-guitar / drum-sub-bass render via the modular binding
	// (builtinFamilyRenderers, rebound in bass_modular_binding.go) — no legacy
	// renderBassGuitarP / renderSubBassP entry (those C/Go paths are deleted).
	// drum-kick-deep / punchy / lofi / tight render via the modular binding
	// (rebound in kick_modular_binding.go) — no legacy renderKick*P entry
	// (deleted Phase-3).
	// fm-bass / fm-bell / fm-lead / fm-epiano / fm-pluck render via the modular
	// binding (builtinFamilyRenderers, rebound in fm_modular_binding.go) — no legacy
	// renderFM*P entry (those C/Go paths are deleted Phase-7, the LAST family).
}

// nativeModularRecipe wraps the unified modular voice (render_modular_p) behind
// the SynthRecipe interface. Unlike nativeDrumRecipe it converts the wide
// RecipeParams block via recipeParamsToModular and renders through the modular
// engine, so the entire pipeline (osc/FM/ADSR/filter/post) is param-driven.
type nativeModularRecipe struct {
	id       string
	display  string
	category string
	params   []ParamDef
}

func (r *nativeModularRecipe) ID() string              { return r.id }
func (r *nativeModularRecipe) DisplayName() string     { return r.display }
func (r *nativeModularRecipe) Category() string        { return r.category }
func (r *nativeModularRecipe) ParamSchema() []ParamDef { return r.params }

func (r *nativeModularRecipe) Render(buf []float32, sampleRate, samples, variant int, p RecipeParams) {
	mp := recipeParamsToModular(p)
	// The noise generator (osc_type 5/6) is deterministic per seed; drive the
	// round-robin variants off the variant index so repeated noise hits vary.
	// For every non-noise voice the seed is read but unused, so all variants
	// stay byte-identical (matching the pre-noise behaviour).
	mp.NoiseSeed = float64(variant)
	renderModularP(buf, sampleRate, samples, mp)
}

func init() {
	registerBuiltinRecipes(func(doc RecipeDoc) func() SynthRecipe {
		id, display, category, params := doc.ID, doc.DisplayName, doc.Category, doc.ParamDefs
		if category == modularRecipeCategory {
			return func() SynthRecipe {
				return &nativeModularRecipe{
					id:       id,
					display:  display,
					category: category,
					params:   params,
				}
			}
		}
		render := builtinRecipeRenderers[doc.ID]
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
