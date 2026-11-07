//go:build !test && !js

package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

// Through the SAME functions dispatch uses, so the key is byte-identical to the one PlayBatch later reads. A hit is a
// cheap no-op; the cache is RWMutex-guarded, so warming beside dispatch is safe.
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

// A param edit's prospective document, so the next trigger's voice is warm before it plays.
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

// The lane switches a row to an instrument only from its first note whose voice is ready. A sample has nothing to render.
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
