//go:build !test && !js

package audio

import (
	"strings"
	"testing"
)

// Native-build Phase-2 assertions. These run alongside the regular Go
// test suite (no -tags test), via `go test ./internal/audio/`, and verify
// the CGo dispatch tables stay in sync with the registry.

func TestPhase2Native_ParamRenderFuncsCoversEveryDrumRecipe(t *testing.T) {
	// Every drum recipe in builtinRecipeDescriptors has a matching
	// cParamRenderer in ParamRenderFuncs. The lookup key is derived from
	// the recipe id by dropping the "drum-" prefix. FM recipes are exempt
	// because they live in fmsynth_c.go's per-id wrappers (registered via
	// builtinRecipeRenderers in synth_registry_entries_native.go), not in
	// ParamRenderFuncs.
	for _, d := range builtinRecipeDescriptors {
		if d.Category != "drum" {
			continue
		}
		if IsModularMigratedRecipe(d.ID) {
			// Migrated families render through the modular binding
			// (builtinFamilyRenderers, rebound in bass_modular_binding.go), NOT a
			// bespoke render_<id>_p cParamRenderer — their legacy C/Go paths were
			// deleted in the Phase-2 cutover. They are exempt from ParamRenderFuncs
			// for the same reason the modular category is.
			continue
		}
		base := strings.TrimPrefix(d.ID, "drum-")
		if _, ok := ParamRenderFuncs[base]; !ok {
			t.Errorf("ParamRenderFuncs missing entry for drum recipe %q (base name %q)", d.ID, base)
		}
	}
}

func TestPhase2Native_BuiltinRecipeRenderersCoverEveryRegisteredRecipe(t *testing.T) {
	// Every entry in builtinRecipeDescriptors must have a matching
	// cParamRenderer in builtinRecipeRenderers, otherwise the native
	// init() would register a recipe whose Render dereferences a nil
	// function pointer.
	for _, d := range builtinRecipeDescriptors {
		if d.Category == modularRecipeCategory {
			// The modular voice is rendered by nativeModularRecipe (which
			// calls render_modular_p directly), not via a builtinRecipeRenderers
			// cParamRenderer, so it is intentionally absent from that map.
			continue
		}
		if IsModularMigratedRecipe(d.ID) {
			// Migrated families render through builtinFamilyRenderers (the modular
			// binding), not a builtinRecipeRenderers cParamRenderer — the legacy
			// renderBassGuitarP / renderSubBassP wrappers were deleted in the
			// Phase-2 cutover. Same exemption rationale as the modular voice.
			continue
		}
		if _, ok := builtinRecipeRenderers[d.ID]; !ok {
			t.Errorf("builtinRecipeRenderers missing entry for %q", d.ID)
		}
	}
	// And conversely: no orphan renderers in builtinRecipeRenderers that
	// aren't declared in builtinRecipeDescriptors.
	known := make(map[string]bool, len(builtinRecipeDescriptors))
	for _, d := range builtinRecipeDescriptors {
		known[d.ID] = true
	}
	for id := range builtinRecipeRenderers {
		if !known[id] {
			t.Errorf("builtinRecipeRenderers has orphan entry %q (no descriptor)", id)
		}
	}
}

func TestPhase2Native_RecipeRenderProducesNonZeroRMS(t *testing.T) {
	// Every registered recipe, at its declared defaults, must produce a
	// non-silent buffer when rendered. Catches a regression where the C
	// _p() variant was wired to the wrong base renderer.
	for _, id := range RecipeOrder() {
		r := NewRecipe(id)
		if r == nil {
			t.Errorf("NewRecipe(%q) returned nil", id)
			continue
		}
		buf := make([]float32, 44100/4) // 0.25 s @ 44.1 kHz — enough for every recipe's attack
		r.Render(buf, 44100, len(buf), 0, RecipeDefaultParams(id))

		var sumSq float64
		for _, v := range buf {
			sumSq += float64(v) * float64(v)
		}
		if sumSq == 0 {
			t.Errorf("recipe %q rendered all-zero buffer at defaults", id)
		}
	}
}

func TestPhase2Native_RecipeRenderHonorsParamMutations(t *testing.T) {
	// Setting any non-default param must produce a buffer different from
	// the default render. Catches a regression where the recipe's Render
	// silently drops params. We mutate `decay` because every Phase-1 and
	// Phase-2 _p() variant wires the decay envelope; not every variant
	// wires pitch (e.g. hihat / shaker / ride / crash post-processors
	// deliberately omit pitch shifting because it produces unmusical
	// results on noise-driven percussion).
	for _, id := range RecipeOrder() {
		r := NewRecipe(id)
		if r == nil {
			continue
		}
		const samples = 44100 / 8
		bufA := make([]float32, samples)
		bufB := make([]float32, samples)
		defaults := RecipeDefaultParams(id)
		r.Render(bufA, 44100, samples, 0, defaults)

		mutated := cloneRecipeParams(defaults)
		// The modular voice has no generic "decay" multiplier. Mutate its
		// post-stage "gain" instead of an envelope time: gain scales every
		// rendered sample, so the effect is independent of how the preset's
		// envelope timing happens to fall inside this short test buffer (a
		// long-attack pad preset can otherwise outlast a decay-only change).
		// Per-modular-param coverage lives in
		// TestRenderModularP_EveryParamMutatesInContext.
		if _, ok := defaults["gain"]; ok {
			mutated["gain"] = 0.5
		} else {
			mutated["decay"] = 0.3 // shorter than identity (1.0)
		}
		r.Render(bufB, 44100, samples, 0, mutated)

		same := true
		for i := range bufA {
			if bufA[i] != bufB[i] {
				same = false
				break
			}
		}
		if same {
			t.Errorf("recipe %q produced identical output for default vs decay=0.3", id)
		}
	}
}
