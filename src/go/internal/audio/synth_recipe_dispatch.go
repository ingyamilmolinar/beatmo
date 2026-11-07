//go:build !test && !js

package audio

import (
	"math"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Every synth renders through ONE path: its flattened project document. Instruments with no document, and synths a
// user sample overrides, take the instrument table.

// Invariant I3: a miss on the dispatch path serves the best available buffer ON TIME and renders off-thread, since a
// render there delays every following beat. False in tests and tools that render synchronously on purpose.
var dispatchMustNotRender atomic.Bool

// Atomic: READ on the audio dispatch goroutine while Game.Close may Store(nil) before the audioLoop is joined.
var asyncRenderSubmit atomic.Pointer[func(id instrument.ID, pitch float64, d instrument.Doc)]

// SetDispatchMustNotRender arms/disarms invariant I3 (see dispatchMustNotRender).
func SetDispatchMustNotRender(on bool) { dispatchMustNotRender.Store(on) }

// nil clears it.
func SetAsyncRenderSubmit(f func(id instrument.ID, pitch float64, d instrument.Doc)) {
	if f == nil {
		asyncRenderSubmit.Store(nil)
		return
	}
	asyncRenderSubmit.Store(&f)
}

// Plays the closest rendered buffer on time and renders the exact one off-thread for the NEXT trigger: same pitch under
// any document first, then any pitch. With nothing cached it returns false: a LATE first note beats a SILENT one.
func serveBestAvailable(id instrument.ID, pitch float64) (Voice, bool) {
	buf, ok := globalVoiceCache.getAnyForInstrumentPitch(id, pitch)
	if !ok {
		buf, ok = globalVoiceCache.getAnyForInstrument(id)
	}
	if !ok {
		return nil, false // nothing cached at all — caller renders (late > silent)
	}
	recordStalePlay()
	if p := asyncRenderSubmit.Load(); p != nil {
		if d, ok := InstrumentDoc(id); ok {
			(*p)(id, pitch, d)
		}
	}
	return &cVoice{buf: buf}, true
}

// It has a document and no user sample overrides it.
func hasDocVoice(id instrument.ID) bool {
	return !IsUserSample(id) && ensureDoc(id)
}

// Attributes its render to originDispatch, so it is correct ONLY on the audioLoop dispatch goroutine; anything else uses
// the Origin variant.
func newRecipeAwareVoice(id instrument.ID, sampleRate int) Voice {
	return newRecipeAwareVoiceOrigin(id, sampleRate, originDispatch)
}

// Origin affects only the metric, never the cache key.
func newRecipeAwareVoiceOrigin(id instrument.ID, sampleRate int, origin renderOrigin) Voice {
	if v, ok := tryRecipeVoiceOptsOrigin(id, sampleRate, false, origin); ok {
		if env.SynthDispatchDebug.Get() {
			log.Debugf("[synth-dispatch] document path id=%q", id)
		}
		return v
	}
	if env.SynthDispatchDebug.Get() {
		log.Debugf("[synth-dispatch] instrument-table path id=%q (no document or a user sample)", id)
	}
	return legacyNewVoice(id, sampleRate)
}

// Node pitches are integer or half-integer semitones.
func roundPitchForCache(pitch float64) float64 {
	return math.Round(pitch*2) / 2
}

// A melodic document renders AT the node pitch. Bare wrapper: attributes originDispatch.
func newRecipeAwareVoicePitched(id instrument.ID, sampleRate int, pitch float64) Voice {
	return newRecipeAwareVoicePitchedOrigin(id, sampleRate, pitch, originDispatch)
}

func newRecipeAwareVoicePitchedOrigin(id instrument.ID, sampleRate int, pitch float64, origin renderOrigin) Voice {
	if docMelodic(id) {
		if v, ok := tryRecipeVoicePitchedOrigin(id, sampleRate, pitch, origin); ok {
			if env.SynthDispatchDebug.Get() {
				log.Debugf("[synth-dispatch] melodic at-pitch path id=%q pitch=%.2f", id, pitch)
			}
			return v
		}
	}
	return newRecipeAwareVoiceOrigin(id, sampleRate, origin)
}

// Bare wrapper: attributes originDispatch.
func tryRecipeVoicePitched(id instrument.ID, sampleRate int, pitch float64) (Voice, bool) {
	return tryRecipeVoicePitchedOrigin(id, sampleRate, pitch, originDispatch)
}

// The block carries the EXACT pitch; the key its 0.5-st rounding.
func tryRecipeVoicePitchedOrigin(id instrument.ID, sampleRate int, pitch float64, origin renderOrigin) (Voice, bool) {
	if !hasDocVoice(id) {
		return nil, false
	}
	buf, _, ok := docVoiceBuffer(id, sampleRate, pitch, true, false, origin)
	if !ok {
		return nil, false
	}
	return &cVoice{buf: buf}, true
}

// Recomputed from the CURRENT document, as dispatch reads it.
func pitchedVoiceKey(id instrument.ID, sampleRate int, pitch float64) (voiceCacheKey, bool) {
	return liveVoiceKey(id, sampleRate, pitch, true, false)
}

// For prewarming a prospective edit: byte-identical to the dispatch key for the same document.
func pitchedVoiceKeyForDoc(id instrument.ID, sampleRate int, pitch float64, d instrument.Doc) voiceCacheKey {
	key, _ := docVoiceKey(id, d, sampleRate, pitch, true, false)
	return key
}

// A no-op when cached; false when d is not melodic.
func renderPitchedIntoCacheForDoc(id instrument.ID, sampleRate int, pitch float64, d instrument.Doc, origin renderOrigin) bool {
	if !d.Render.Melodic {
		return false
	}
	renderDocIntoCache(id, d, sampleRate, pitch, true, false, origin)
	return true
}

// Under the key dispatch will read.
func VoiceCached(id instrument.ID, pitch float64) bool {
	key, ok := pitchedVoiceKey(id, SampleRate(), pitch)
	if !ok {
		return false
	}
	return globalVoiceCache.Has(key)
}

// tryRecipeVoice returns (voice, true) when id renders from its document.
func tryRecipeVoice(id instrument.ID, sampleRate int) (Voice, bool) {
	return tryRecipeVoiceOptsOrigin(id, sampleRate, false, originDispatch)
}

// ignoreEdit gives the Sampler editor the UN-edited source. Bare wrapper: attributes originDispatch.
func tryRecipeVoiceOpts(id instrument.ID, sampleRate int, ignoreEdit bool) (Voice, bool) {
	return tryRecipeVoiceOptsOrigin(id, sampleRate, ignoreEdit, originDispatch)
}

// tryRecipeVoiceOptsOrigin is tryRecipeVoiceOpts with explicit attribution.
func tryRecipeVoiceOptsOrigin(id instrument.ID, sampleRate int, ignoreEdit bool, origin renderOrigin) (Voice, bool) {
	if !hasDocVoice(id) {
		return nil, false
	}
	buf, _, ok := docVoiceBuffer(id, sampleRate, 0, false, ignoreEdit, origin)
	if !ok {
		return nil, false
	}
	return &cVoice{buf: buf}, true
}

// nil for unknown ids: the silent drop Play relies on.
func legacyNewVoice(id instrument.ID, sampleRate int) Voice {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return nil
	}
	return inst.NewVoice(sampleRate)
}
