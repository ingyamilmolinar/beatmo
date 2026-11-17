//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

// PrewarmVoice renders (id, pitch) into globalVoiceCache off the caller's hot
// path. It calls the SAME functions the dispatch path uses
// (newRecipeAwareVoicePitched for pitch-aware recipes, newRecipeAwareVoice
// otherwise), so the produced cache key is byte-identical to the one the
// sequencer's PlayBatch -> playParamsIdentity later reads. The returned voice is
// discarded — the render + globalVoiceCache.Put is the whole point.
//
// A cache hit is a cheap no-op, so re-warming an already-warm pitch is safe.
// Reads the sampleRate global exactly like the audioLoop dispatch already
// does; the cache is RWMutex-guarded, so concurrent warm + dispatch is safe.
// See docs/superpowers/specs/2026-07-13-voice-prewarm-off-thread-design.md.
func PrewarmVoice(id instrument.ID, pitch float64) {
	if id == "" {
		return
	}
	if env.PrewarmDebug.Get() {
		log.Debugf("[pw] warm-call id=%s pitch=%.2f", id, pitch)
	}
	if docMelodic(id) {
		newRecipeAwareVoicePitchedOrigin(id, SampleRate(), pitch, originPrewarm)
		return
	}
	newRecipeAwareVoiceOrigin(id, SampleRate(), originPrewarm)
}

// PrewarmVoiceDoc renders (id, pitch) under an EXPLICIT document snapshot into
// globalVoiceCache — the prospective document of a param edit, so the voice the
// next trigger will read is warm before it plays. The key is the dispatch key
// for that document (TestPrewarmVoiceParamsKeyMatchesDispatch).
func PrewarmVoiceDoc(id instrument.ID, pitch float64, d instrument.Doc) {
	if id == "" {
		return
	}
	if !d.Render.Melodic {
		PrewarmVoice(id, pitch)
		return
	}
	renderPitchedIntoCacheForDoc(id, SampleRate(), pitch, d, originPrewarm)
}

// warming — for everything else pitch is a post-render resample (already a hit).
func WarmsPerPitch(id instrument.ID) bool {
	return docMelodic(id)
}

// VoiceReady reports whether a trigger of (id, pitch) would find its voice
// rendered — the audio lane switches a row to an instrument only from its
// first note whose voice is ready (spec 2026-10-31 §1.3). An instrument
// without a document voice (a sample) has nothing to render.
func VoiceReady(id instrument.ID, pitch float64) bool {
	if !hasDocVoice(id) {
		return true
	}
	melodic := IsMelodicInstrument(id)
	if !melodic {
		pitch = 0
	}
	key, ok := liveVoiceKey(id, SampleRate(), pitch, melodic, false)
	return ok && globalVoiceCache.Has(key)
}
