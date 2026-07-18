//go:build !test && !js

package audio

import (
	"log"
	"math"
	"os"
	"sync/atomic"
	"time"
)

// debugSynthDispatch is set from the BEATMO_SYNTH_DISPATCH_DEBUG env var
// at package init. When enabled, every newRecipeAwareVoice call emits a
// one-liner to the standard logger announcing which path was taken
// (recipe vs legacy), the merged paramsHash, and the instrument id.
// Helps users debug live "the slider does nothing" reports by proving
// the trigger flowed through the expected branch.
var debugSynthDispatch = os.Getenv("BEATMO_SYNTH_DISPATCH_DEBUG") != ""

// prewarmDebug (BEATMO_PREWARM_DEBUG=1) logs voice-cache HIT/RENDER for the
// pitched (melodic) path with the full cache key, so a warm→dispatch key
// mismatch or a cold dispatch render is directly observable. Temporary
// diagnostic for the overdue-panic investigation.
var prewarmDebug = os.Getenv("BEATMO_PREWARM_DEBUG") != ""

// dispatchMustNotRender arms invariant I3: when true, a cache miss on the
// dispatch path must NOT call recipe.Render (a ~24ms CGo call that would delay
// every following beat). Instead it serves the best available buffer ON TIME
// and hands the exact render off-thread. Armed by the UI once the prewarmer
// exists; left false in tests and tools that render synchronously on purpose.
var dispatchMustNotRender atomic.Bool

// asyncRenderSubmit hands an exact (id, pitch, params) render to an off-thread
// pool. Set by internal/ui to the prewarm pool; nil-cleared at teardown. It is
// an atomic.Pointer because it is READ on the audio dispatch goroutine while the
// UI goroutine may Store(nil) during Game.Close() BEFORE the audioLoop is joined
// (see game_close.go's own "closing audio.* before receivers exit would race"
// note). A plain func var here is a data race that -race misses because no test
// drives a concurrent audioLoop through the fallback during Close().
var asyncRenderSubmit atomic.Pointer[func(id string, pitch float64, params RecipeParams)]

// SetDispatchMustNotRender arms/disarms invariant I3 (see dispatchMustNotRender).
func SetDispatchMustNotRender(on bool) { dispatchMustNotRender.Store(on) }

// SetAsyncRenderSubmit installs (or, with nil, clears) the off-thread render
// hand-off (see asyncRenderSubmit).
func SetAsyncRenderSubmit(f func(id string, pitch float64, params RecipeParams)) {
	if f == nil {
		asyncRenderSubmit.Store(nil)
		return
	}
	asyncRenderSubmit.Store(&f)
}

// serveBestAvailable implements the miss policy for a non-blocking dispatch:
// play the closest already-rendered buffer at the correct time, count it, and
// submit the exact render off-thread so the NEXT trigger is correct. Returns
// (nil,false) when NOTHING is cached for this instrument at all — the caller
// then falls through to a normal (dispatch-goroutine) render, because a LATE
// first note beats a SILENT one (user decision). That fall-through is bounded
// to an instrument's very first trigger: edit-induced misses hit tier 1/2.
//
// Two tiers, each strictly better than a synchronous ~24ms CGo render:
//  1. Same (instrument, pitch) under any params — the common case; a param
//     edit rotated the binding.
//  2. Same instrument at ANY pitch — a brand-new note appearing faster than
//     the prewarmer's horizon. An audibly wrong note ON TIME.
//
// The async submit always targets the exact correct (id, pitch), so a tier-2
// mismatch self-corrects on the very next trigger of this note.
func serveBestAvailable(id string, pitch float64) (Voice, bool) {
	buf, ok := globalVoiceCache.getAnyForInstrumentPitch(id, pitch)
	if !ok {
		buf, ok = globalVoiceCache.getAnyForInstrument(id)
	}
	if !ok {
		return nil, false // nothing cached at all — caller renders (late > silent)
	}
	recordStalePlay()
	if p := asyncRenderSubmit.Load(); p != nil {
		(*p)(id, pitch, GetInstrumentParams(id))
	}
	return &cVoice{buf: buf}, true
}

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
//
// WARNING: this bare wrapper attributes its render to originDispatch. It is
// correct ONLY for callers that actually run on the audioLoop dispatch
// goroutine (playParamsIdentity / PlayBatch). Any NEW caller that is not on
// that goroutine (a UI audition, a background warm, an offline render, …)
// MUST call newRecipeAwareVoiceOrigin explicitly with the right renderOrigin
// — using this wrapper would silently inflate DispatchRenders and false-
// positive the DispatchRenders==0 gate (invariant I3).
func newRecipeAwareVoice(id string, bpm, sampleRate int) Voice {
	return newRecipeAwareVoiceOrigin(id, bpm, sampleRate, originDispatch)
}

// newRecipeAwareVoiceOrigin is newRecipeAwareVoice with explicit render
// attribution. The key computation is IDENTICAL regardless of origin — origin
// affects only the metric, never the cache key.
func newRecipeAwareVoiceOrigin(id string, bpm, sampleRate int, origin renderOrigin) Voice {
	if v, ok := tryRecipeVoiceOptsOrigin(id, bpm, sampleRate, false, origin); ok {
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

// melodicRecipeIDs and pitchAwareRecipe moved to melodic_recipes.go (untagged)
// so the UI note-display path (all build tags) and this native render path
// share one source of truth for which recipes are melodic.

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
//
// WARNING: bare wrapper — attributes originDispatch. See the WARNING on
// newRecipeAwareVoice; a non-dispatch caller must use
// newRecipeAwareVoicePitchedOrigin explicitly.
func newRecipeAwareVoicePitched(id string, bpm, sampleRate int, pitch float64) Voice {
	return newRecipeAwareVoicePitchedOrigin(id, bpm, sampleRate, pitch, originDispatch)
}

// newRecipeAwareVoicePitchedOrigin is newRecipeAwareVoicePitched with explicit
// render attribution.
func newRecipeAwareVoicePitchedOrigin(id string, bpm, sampleRate int, pitch float64, origin renderOrigin) Voice {
	recipeID := RecipeForInstrument(id)
	if pitchAwareRecipe(recipeID) {
		if v, ok := tryRecipeVoicePitchedOrigin(id, bpm, sampleRate, pitch, origin); ok {
			if debugSynthDispatch {
				log.Printf("[SYNTH-DISPATCH] melodic at-pitch path id=%q recipe=%q pitch=%.2f", id, recipeID, pitch)
			}
			return v
		}
	}
	return newRecipeAwareVoiceOrigin(id, bpm, sampleRate, origin)
}

// tryRecipeVoicePitched renders a melodic recipe at the given node pitch (in
// semitones), sets "pitch" in the merged params before Render, and uses a
// cache key that includes the rounded pitch. Returns (nil, false) if any
// precondition fails (unknown id, no recipe, etc.).
//
// WARNING: bare wrapper — attributes originDispatch. See the WARNING on
// newRecipeAwareVoice; a non-dispatch caller must use
// tryRecipeVoicePitchedOrigin explicitly.
func tryRecipeVoicePitched(id string, bpm, sampleRate int, pitch float64) (Voice, bool) {
	return tryRecipeVoicePitchedOrigin(id, bpm, sampleRate, pitch, originDispatch)
}

// tryRecipeVoicePitchedOrigin is tryRecipeVoicePitched with explicit render
// attribution.
func tryRecipeVoicePitchedOrigin(id string, bpm, sampleRate int, pitch float64, origin renderOrigin) (Voice, bool) {
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
		recordVoiceCacheHit()
		if prewarmDebug {
			log.Printf("[pw] HIT id=%s pitch=%.2f bpmKey=%d sr=%d ph=%x", id, roundedPitch, bpmKey, sampleRate, ph)
		}
		return &cVoice{buf: buf}, true
	}
	if prewarmDebug {
		log.Printf("[pw] RENDER-MISS id=%s pitch=%.2f bpmKey=%d sr=%d ph=%x", id, roundedPitch, bpmKey, sampleRate, ph)
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
			recordVoiceCacheHit()
			return &cVoice{buf: buf}, true
		}
		if origin == originDispatch && dispatchMustNotRender.Load() {
			if v, ok := serveBestAvailable(id, pitch); ok {
				return v, true
			}
			// Nothing cached at all — fall through to the render below. A late
			// first note beats a silent one (user decision). Bounded to an
			// instrument's first-ever trigger.
		}
		// Render and store under the folded key (cache miss — timed).
		buf := make([]float32, samples)
		renderStart := time.Now()
		recipe.Render(buf, sampleRate, samples, 0, merged)
		normalizeAndScale(buf, baseInstrumentID(id))
		recordVoiceRender(time.Since(renderStart), origin)
		buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
		globalVoiceCache.Put(foldedKey, buf)
		return &cVoice{buf: buf}, true
	}

	if origin == originDispatch && dispatchMustNotRender.Load() {
		if v, ok := serveBestAvailable(id, pitch); ok {
			return v, true
		}
		// Nothing cached at all — fall through to the render below. A late
		// first note beats a silent one (user decision). Bounded to an
		// instrument's first-ever trigger.
	}

	buf := make([]float32, samples)
	renderStart := time.Now()
	recipe.Render(buf, sampleRate, samples, 0, merged)
	normalizeAndScale(buf, baseInstrumentID(id))
	recordVoiceRender(time.Since(renderStart), origin)
	globalVoiceCache.Put(key, buf)
	return &cVoice{buf: buf}, true
}

// pitchedVoiceKey computes the EFFECTIVE globalVoiceCache key that
// tryRecipeVoicePitched renders/reads for a pitch-aware (id, pitch) trigger —
// including the sample-edit fold. It recomputes from the CURRENT bpm+params, so
// callers always get the key dispatch will actually use. ok=false when id has no
// renderable recipe. Kept adjacent to tryRecipeVoicePitched so the two key
// computations stay in lockstep (guarded by TestVoiceCachedMatchesDispatch).
func pitchedVoiceKey(id string, bpm, sampleRate int, pitch float64) (voiceCacheKey, bool) {
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		return voiceCacheKey{}, false
	}
	if NewRecipe(recipeID) == nil {
		return voiceCacheKey{}, false
	}
	samples, bpmKey := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return voiceCacheKey{}, false
	}
	merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
	merged["pitch"] = pitch
	ph := hashRecipeParams(merged)
	if edit, hasEdit := SampleEditFor(id); hasEdit {
		ph ^= hashSampleEdit(edit)
	}
	return voiceCacheKey{
		instrumentID: id,
		bpm:          bpmKey,
		sampleRate:   sampleRate,
		paramsHash:   ph,
		pitch:        roundPitchForCache(pitch),
	}, true
}

// pitchedVoiceKeyForParams is pitchedVoiceKey with the params supplied by the
// caller instead of read from the manager. The ONLY difference is the params
// source — every other term (merge, pitch override, hash, edit fold, rounding)
// is computed identically, so a key produced here is byte-identical to the key
// dispatch reads for the same param state. Keep these two functions adjacent
// and edit them together; drift here silently defeats prewarming
// (TestPrewarmVoiceParamsKeyMatchesDispatch guards it).
func pitchedVoiceKeyForParams(id string, bpm, sampleRate int, pitch float64, params RecipeParams) (voiceCacheKey, bool) {
	recipeID := RecipeForInstrument(id)
	if recipeID == "" {
		return voiceCacheKey{}, false
	}
	if NewRecipe(recipeID) == nil {
		return voiceCacheKey{}, false
	}
	samples, bpmKey := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return voiceCacheKey{}, false
	}
	merged := MergeRecipeDefaults(recipeID, params)
	merged["pitch"] = pitch
	ph := hashRecipeParams(merged)
	if edit, hasEdit := SampleEditFor(id); hasEdit {
		ph ^= hashSampleEdit(edit)
	}
	return voiceCacheKey{
		instrumentID: id,
		bpm:          bpmKey,
		sampleRate:   sampleRate,
		paramsHash:   ph,
		pitch:        roundPitchForCache(pitch),
	}, true
}

// renderPitchedIntoCacheForParams renders (id, pitch) under the binding implied
// by params and stores it in globalVoiceCache. No-op when already cached.
// Returns false when id has no renderable pitch-aware recipe.
func renderPitchedIntoCacheForParams(id string, bpm, sampleRate int, pitch float64, params RecipeParams, origin renderOrigin) bool {
	recipeID := RecipeForInstrument(id)
	if !pitchAwareRecipe(recipeID) {
		return false
	}
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		return false
	}
	key, ok := pitchedVoiceKeyForParams(id, bpm, sampleRate, pitch, params)
	if !ok {
		return false
	}
	if globalVoiceCache.Has(key) {
		return true // already warm — cheap no-op
	}
	samples, _ := instrumentDurationSamples(id, bpm, sampleRate)
	if samples <= 0 {
		return false
	}
	merged := MergeRecipeDefaults(recipeID, params)
	merged["pitch"] = pitch

	buf := make([]float32, samples)
	renderStart := time.Now()
	recipe.Render(buf, sampleRate, samples, 0, merged)
	normalizeAndScale(buf, baseInstrumentID(id))
	recordVoiceRender(time.Since(renderStart), origin)
	if edit, hasEdit := SampleEditFor(id); hasEdit {
		buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
	}
	globalVoiceCache.Put(key, buf)
	return true
}

// VoiceCached reports whether the pitch-aware voice for (id, pitch) is already
// present in globalVoiceCache under the key the dispatch path will read. The UI
// voice-prewarmer uses it to decide whether a pitch still needs warming.
// Because the key is recomputed from current bpm+params on every call, it stays
// correct across param edits / BPM changes / cache invalidations — unlike a
// static seen-set, whose staleness let a cleared voice go un-rewarmed and
// cold-render on the dispatch goroutine (the overdue-panic root cause).
func VoiceCached(id string, pitch float64) bool {
	key, ok := pitchedVoiceKey(id, bpm, SampleRate(), pitch)
	if !ok {
		return false
	}
	return globalVoiceCache.Has(key)
}

// tryRecipeVoice returns (voice, true) when the recipe path applies, else
// (nil, false). Split out from newRecipeAwareVoice so the legacy path
// stays a single function call deep in the common (no-user-params) case.
func tryRecipeVoice(id string, bpm, sampleRate int) (Voice, bool) {
	return tryRecipeVoiceOptsOrigin(id, bpm, sampleRate, false, originDispatch)
}

// tryRecipeVoiceOpts is tryRecipeVoice with an ignoreEdit escape hatch used
// by RenderInstrumentOneShotRaw: the Sampler editor needs the UN-edited
// source render so it can overlay the saved trim/pitch itself.
//
// WARNING: bare wrapper — attributes originDispatch. See the WARNING on
// newRecipeAwareVoice; a non-dispatch caller must use
// tryRecipeVoiceOptsOrigin explicitly.
func tryRecipeVoiceOpts(id string, bpm, sampleRate int, ignoreEdit bool) (Voice, bool) {
	return tryRecipeVoiceOptsOrigin(id, bpm, sampleRate, ignoreEdit, originDispatch)
}

// tryRecipeVoiceOptsOrigin is tryRecipeVoiceOpts with explicit render
// attribution.
func tryRecipeVoiceOptsOrigin(id string, bpm, sampleRate int, ignoreEdit bool, origin renderOrigin) (Voice, bool) {
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
		recordVoiceCacheHit()
		return &cVoice{buf: buf}, true
	}
	buf := make([]float32, samples)
	renderStart := time.Now()
	recipe.Render(buf, sampleRate, samples, 0, merged)
	normalizeAndScale(buf, baseInstrumentID(id))
	recordVoiceRender(time.Since(renderStart), origin)
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
