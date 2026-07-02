//go:build !test && !js

package audio

import (
	"log"
	"math"
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
//  1. Resolves the recipe id via builtinInstrumentRecipeBindings.
//  2. Merges user params over recipe defaults.
//  3. Looks up the cache key keyed on (instrumentID, bpm, sr, paramsHash).
//  4. On miss, allocates a buffer of the instrument's natural length
//     (CVariantInstrument: Beats × spb; legacy types: ConfigForInstrument
//     DurationSec), calls recipe.Render, normalizes, and caches.
//  5. Returns a *cVoice wrapping a clone of the cached buffer.
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

// melodicRecipeIDs is the set of synth-modular recipe IDs that represent
// melodic instruments (bowed/plucked strings, keys, woodwinds, brass).
// These are pitch-aware: at trigger time the C renderer re-renders the
// voice at the node's semitone pitch so the filter formant stays at an
// absolute Hz (not resampled). Instruments NOT in this set (drums, FM,
// synth-modular base, synth-modular-pad) keep the legacy resample path.
var melodicRecipeIDs = map[string]bool{
	// Bowed strings
	"synth-modular-violin":          true,
	"synth-modular-violin-ensemble": true,
	"synth-modular-cello":           true,
	"synth-modular-cello-warm":      true,
	"synth-modular-organ-church":    true,
	"synth-modular-scifi-lead":      true,
	// Plucked strings
	"synth-modular-guitar-nylon":         true,
	"synth-modular-guitar-nylon-bright":  true,
	"synth-modular-guitar-steel":         true,
	"synth-modular-guitar-steel-warm":    true,
	"synth-modular-guitar-electric":      true,
	"synth-modular-harp":                 true,
	"synth-modular-guitar-electric-neck": true,
	// Keys
	"synth-modular-piano-grand": true,
	"synth-modular-piano-felt":  true,
	// Woodwinds
	"synth-modular-flute":         true,
	"synth-modular-flute-breathy": true,
	"synth-modular-oboe":          true,
	"synth-modular-oboe-full":     true,
	// Brass
	"synth-modular-trumpet":          true,
	"synth-modular-trumpet-mellow":   true,
	"synth-modular-french-horn":      true,
	"synth-modular-french-horn-loud": true,
	// Bass guitar (renamed from synth-bass) + synth bass family — pitched melodic.
	"synth-modular-bass-guitar": true,
	"synth-modular-bass-acid":   true,
	"synth-modular-bass-reese":  true,
	"synth-modular-bass-fm":     true,
	"synth-modular-bass-808":    true,
	// Masterpiece template set — pitched melodic instruments.
	"synth-modular-organ": true,
	"synth-modular-sax":   true,
}

// pitchAwareRecipe returns true when recipeID is a melodic synth-modular
// recipe that benefits from per-pitch re-rendering instead of resampling.
// Returns false for drums, FM, synth-modular (base), and synth-modular-pad.
func pitchAwareRecipe(recipeID string) bool {
	return melodicRecipeIDs[recipeID]
}

// roundPitchForCache rounds a semitone value to the nearest 0.5 st for the
// cache key. This keeps the key space bounded while resolving the audible
// re-render granularity coarser than human pitch-discrimination (~5 cents)
// but finer than a half-step. For practical use (node pitches from the UI
// quantize to integer or half-integer semitones) 0.5-st resolution is exact.
func roundPitchForCache(pitch float64) float64 {
	return math.Round(pitch*2) / 2
}

// newRecipeAwareVoicePitched is the pitch-aware variant of newRecipeAwareVoice.
// For melodic (pitch-aware) recipe instruments it threads the node semitone
// pitch into the render so the voice is produced at the target frequency
// instead of at the base pitch (A3=220 Hz) and later resampled. For all
// other instruments it falls through to newRecipeAwareVoice with no change.
func newRecipeAwareVoicePitched(id string, bpm, sampleRate int, pitch float64) Voice {
	recipeID := RecipeForInstrument(id)
	if pitchAwareRecipe(recipeID) {
		if v, ok := tryRecipeVoicePitched(id, bpm, sampleRate, pitch); ok {
			if debugSynthDispatch {
				log.Printf("[SYNTH-DISPATCH] melodic at-pitch path id=%q recipe=%q pitch=%.2f", id, recipeID, pitch)
			}
			return v
		}
	}
	return newRecipeAwareVoice(id, bpm, sampleRate)
}

// tryRecipeVoicePitched renders a melodic recipe at the given node pitch (in
// semitones), sets "pitch" in the merged params before Render, and uses a
// cache key that includes the rounded pitch. Returns (nil, false) if any
// precondition fails (unknown id, no recipe, etc.).
func tryRecipeVoicePitched(id string, bpm, sampleRate int, pitch float64) (Voice, bool) {
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		return nil, false
	}
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		return nil, false
	}
	params := GetInstrumentParams(id)
	samples, bpmKey := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return nil, false
	}
	merged := MergeRecipeDefaults(recipeID, params)

	// Override the modular "pitch" param with the node's semitone value so
	// the C renderer uses freq=220·2^(pitch/12) as its fundamental, keeping
	// the filter cutoff at an absolute Hz (not slid by resampling).
	merged["pitch"] = pitch

	ph := hashRecipeParams(merged)
	roundedPitch := roundPitchForCache(pitch)
	key := voiceCacheKey{
		instrumentID: id,
		bpm:          bpmKey,
		sampleRate:   sampleRate,
		paramsHash:   ph,
		pitch:        roundedPitch,
	}
	if buf, ok := globalVoiceCache.Get(key); ok {
		return &cVoice{buf: buf}, true
	}

	// Account for sample-edit descriptor in the cache key (same as
	// tryRecipeVoiceOpts) but only fold it in if present.
	edit, hasEdit := SampleEditFor(id)
	if hasEdit {
		// Recompute key with edit hash folded in (matching tryRecipeVoiceOpts
		// so same render parameters produce the same cache slot).
		foldedPh := ph ^ hashSampleEdit(edit)
		foldedKey := voiceCacheKey{
			instrumentID: id,
			bpm:          bpmKey,
			sampleRate:   sampleRate,
			paramsHash:   foldedPh,
			pitch:        roundedPitch,
		}
		if buf, ok := globalVoiceCache.Get(foldedKey); ok {
			return &cVoice{buf: buf}, true
		}
		// Render and store under the folded key.
		buf := make([]float32, samples)
		recipe.Render(buf, sampleRate, samples, 0, merged)
		normalizeAndScale(buf, baseInstrumentID(id))
		buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
		globalVoiceCache.Put(foldedKey, buf)
		return &cVoice{buf: buf}, true
	}

	buf := make([]float32, samples)
	recipe.Render(buf, sampleRate, samples, 0, merged)
	normalizeAndScale(buf, baseInstrumentID(id))
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}, true
}

// tryRecipeVoice returns (voice, true) when the recipe path applies, else
// (nil, false). Split out from newRecipeAwareVoice so the legacy path
// stays a single function call deep in the common (no-user-params) case.
func tryRecipeVoice(id string, bpm, sampleRate int) (Voice, bool) {
	return tryRecipeVoiceOpts(id, bpm, sampleRate, false)
}

// tryRecipeVoiceOpts is tryRecipeVoice with an ignoreEdit escape hatch used
// by RenderInstrumentOneShotRaw: the Sampler editor needs the UN-edited
// source render so it can overlay the saved trim/pitch itself.
func tryRecipeVoiceOpts(id string, bpm, sampleRate int, ignoreEdit bool) (Voice, bool) {
	params := GetInstrumentParams(id)
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		return nil, false
	}
	edit, hasEdit := SampleEditFor(id)
	if ignoreEdit {
		hasEdit = false
	}
	// Take the recipe-aware render path when the user has live per-instrument
	// overrides OR the recipe's registered defaults have been customized away
	// from what it shipped with (Synth-tab Save, or a saved override reloaded
	// from disk at startup) OR a non-destructive Sampler edit descriptor exists
	// (the instrument stays a synth; the edit is applied to the fresh render
	// below). Without the customized-defaults check, a Save that clears the
	// overlay — or any restart — would drop to the legacy path and silently
	// ignore the saved tone, reverting the sound. The fast legacy path is kept
	// for unedited, as-shipped recipes so parity goldens stay byte-identical.
	if len(params) == 0 && !RecipeDefaultsCustomized(recipeID) && !hasEdit {
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
	ph := hashRecipeParams(merged)
	if hasEdit {
		// Fold the descriptor into the key so an edit change misses the cache
		// exactly like a param change (SetSampleEdit also invalidates, belt-
		// and-suspenders for in-flight keys).
		ph ^= hashSampleEdit(edit)
	}
	key := voiceCacheKey{
		instrumentID: id,
		bpm:          bpmKey,
		sampleRate:   sampleRate,
		paramsHash:   ph,
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
	if hasEdit {
		// Non-destructive Sampler edit: same transform as the Sampler tab's
		// bake (BakeSample), applied to the fresh recipe render. BakeSample
		// allocates its output, so the cached buffer stays exclusively owned.
		buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
	}
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
