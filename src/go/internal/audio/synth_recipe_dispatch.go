//go:build !test && !js

package audio

import (
	"log"
	"os"
	"time"
)

// debugSynthDispatch is set from the BEATMO_SYNTH_DISPATCH_DEBUG env var
// at package init. When enabled, every newRecipeAwareVoice call emits a
// one-liner to the standard logger announcing which path was taken
// (recipe vs legacy), the merged paramsHash, and the instrument id.
// Helps users debug live "the slider does nothing" reports by proving
// the trigger flowed through the expected branch.
var debugSynthDispatch = os.Getenv("BEATMO_SYNTH_DISPATCH_DEBUG") != ""

// newRecipeAwareVoice is the Phase-2.6 dispatcher that decides whether to
// render via the SynthRecipe (when the user has edited per-instrument
// params) or fall through to the legacy Instrument.NewVoice path. Called
// by Play / PlayVol / PlayParams in engine_play.go in place of the direct
// inst.NewVoice(bpm, sampleRate) call.
//
// The recipe path:
//   1. Resolves the recipe id via builtinInstrumentRecipeBindings.
//   2. Merges user params over recipe defaults.
//   3. Looks up the cache key keyed on (instrumentID, bpm, sr, paramsHash).
//   4. On miss, allocates a buffer of the instrument's natural length
//      (CVariantInstrument: Beats × spb; legacy types: ConfigForInstrument
//      DurationSec), calls recipe.Render, normalizes, and caches.
//   5. Returns a *cVoice wrapping a clone of the cached buffer.
//
// Fallback to the legacy path happens whenever any precondition is missing
// (no user params, no binding, no recipe, no instrument).
func newRecipeAwareVoice(id string, bpm, sampleRate int) Voice {
	if v, ok := tryRecipeVoice(id, bpm, sampleRate); ok {
		if debugSynthDispatch {
			log.Printf("[SYNTH-DISPATCH] recipe path id=%q recipe=%q paramsHash=%x", id, RecipeForInstrument(id), hashRecipeParams(GetInstrumentParams(id)))
		}
		return v
	}
	if debugSynthDispatch {
		log.Printf("[SYNTH-DISPATCH] legacy path id=%q (no user params or no recipe binding)", id)
	}
	return legacyNewVoice(id, bpm, sampleRate)
}

// tryRecipeVoice returns (voice, true) when the recipe path applies, else
// (nil, false). Split out from newRecipeAwareVoice so the legacy path
// stays a single function call deep in the common (no-user-params) case.
func tryRecipeVoice(id string, bpm, sampleRate int) (Voice, bool) {
	params := GetInstrumentParams(id)
	if len(params) == 0 {
		return nil, false
	}
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		return nil, false
	}
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		return nil, false
	}
	samples, bpmKey := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return nil, false
	}
	merged := MergeRecipeDefaults(recipeID, params)
	key := voiceCacheKey{
		instrumentID: id,
		bpm:          bpmKey,
		sampleRate:   sampleRate,
		paramsHash:   hashRecipeParams(merged),
	}
	if buf, ok := globalVoiceCache.Get(key); ok {
		// cVoice only READS from buf (drums_c.go:251-275 — Sample/SampleBlock
		// never write). Safe to share the cached buffer across concurrent
		// voices instead of cloning it; saves ~80 KB per cache-hit trigger.
		return &cVoice{buf: buf}, true
	}
	buf := make([]float32, samples)
	recipe.Render(buf, sampleRate, samples, 0, merged)
	normalizeAndScale(buf, baseInstrumentID(id))
	globalVoiceCache.Put(key, buf)
	// Same read-only invariant on the freshly-rendered buffer — no clone needed.
	return &cVoice{buf: buf}, true
}

// legacyNewVoice routes through the existing instrument-table dispatch.
// Returns nil for unknown ids — matches the silent-drop behavior of
// Play / PlayVol / PlayParams under those conditions.
func legacyNewVoice(id string, bpm, sampleRate int) Voice {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return nil
	}
	return inst.NewVoice(bpm, sampleRate)
}

// instrumentDurationSamples returns (sampleCount, bpmForCacheKey) for an
// instrument id. CVariantInstrument variants with Beats > 0 are BPM-derived
// (sampleCount = Beats × 60/bpm × sampleRate, bpm carried in the key).
// Legacy types and CVariantInstrument variants without Beats use the
// canonical ConfigForInstrument DurationSec (bpm = 0 in the key so all
// BPMs share the same cache entry).
func instrumentDurationSamples(id string, bpm, sampleRate int) (int, int) {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if ok {
		if v, isVariant := inst.(CVariantInstrument); isVariant && v.Beats > 0 {
			spb := 60.0 / float64(bpm)
			dur := time.Duration(spb * v.Beats * float64(time.Second))
			samples := int(float64(sampleRate) * dur.Seconds())
			if samples < 1 {
				samples = 1
			}
			return samples, bpm
		}
	}
	cfg := ConfigForInstrument(id)
	samples := int(float64(sampleRate) * cfg.DurationSec)
	if samples < 1 {
		samples = 1
	}
	return samples, 0
}

// baseInstrumentID strips the "-1" / "-2" / "-ghost" / "-tight" / "-pedal"
// suffix used by variant instruments, so normalizeAndScale picks the
// right per-instrument scaling factor. Mirrors the suffix-stripping logic
// in CVariantInstrument.renderAndCache (variants.go:66-69).
func baseInstrumentID(id string) string {
	if len(id) > 2 && (id[len(id)-2:] == "-1" || id[len(id)-2:] == "-2") {
		return id[:len(id)-2]
	}
	// Other variant suffixes don't get stripped today because their
	// normalize-lookup key matches the variant id (no per-base override).
	return id
}
