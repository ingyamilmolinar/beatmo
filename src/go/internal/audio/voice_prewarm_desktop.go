//go:build !test && !js

package audio

import "log"

// PrewarmVoice renders (id, pitch) into globalVoiceCache off the caller's hot
// path. It calls the SAME functions the dispatch path uses
// (newRecipeAwareVoicePitched for pitch-aware recipes, newRecipeAwareVoice
// otherwise), so the produced cache key is byte-identical to the one the
// sequencer's PlayBatch -> playParamsIdentity later reads. The returned voice is
// discarded — the render + globalVoiceCache.Put is the whole point.
//
// A cache hit is a cheap no-op, so re-warming an already-warm pitch is safe.
// Reads the bpm/sampleRate globals exactly like the audioLoop dispatch already
// does; the cache is RWMutex-guarded, so concurrent warm + dispatch is safe.
// See docs/superpowers/specs/2026-07-13-voice-prewarm-off-thread-design.md.
func PrewarmVoice(id string, pitch float64) {
	if id == "" {
		return
	}
	if prewarmDebug {
		log.Printf("[pw] warm-call id=%s pitch=%.2f bpm=%d", id, pitch, bpm)
	}
	if pitchAwareRecipe(RecipeForInstrument(id)) {
		newRecipeAwareVoicePitched(id, bpm, SampleRate(), pitch)
		return
	}
	newRecipeAwareVoice(id, bpm, SampleRate())
}

// IsMelodicInstrument reports whether id renders at-pitch (its cache key carries
// the node pitch). Only melodic instruments benefit from pitch-neighborhood
// warming — for everything else pitch is a post-render resample (already a hit).
func IsMelodicInstrument(id string) bool {
	return pitchAwareRecipe(RecipeForInstrument(id))
}
