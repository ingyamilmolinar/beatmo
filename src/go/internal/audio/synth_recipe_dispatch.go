//go:build !test && !js

package audio

import (
	"math"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/env"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// synth_recipe_dispatch.go — the native voice dispatch. Every synth
// instrument renders through ONE path (docVoiceBuffer, doc_voice_native.go):
// the project's instrument document, flattened. Instruments with no document
// (user samples, catalog WAVs) and synth ids currently overridden by a user
// sample take the instrument table (legacyNewVoice).

// dispatchMustNotRender arms invariant I3: when true, a cache miss on the
// dispatch path must NOT render (a ~24ms CGo call that would delay every
// following beat). Instead it serves the best available buffer ON TIME and
// hands the exact render off-thread. Armed by the UI once the prewarmer
// exists; left false in tests and tools that render synchronously on purpose.
var dispatchMustNotRender atomic.Bool

// asyncRenderSubmit hands an exact (id, pitch, document) render to an
// off-thread pool. Set by internal/ui to the prewarm pool; nil-cleared at
// teardown. It is an atomic.Pointer because it is READ on the audio dispatch
// goroutine while the UI goroutine may Store(nil) during Game.Close() BEFORE
// the audioLoop is joined.
var asyncRenderSubmit atomic.Pointer[func(id instrument.ID, pitch float64, d instrument.Doc)]

// SetDispatchMustNotRender arms/disarms invariant I3 (see dispatchMustNotRender).
func SetDispatchMustNotRender(on bool) { dispatchMustNotRender.Store(on) }

// SetAsyncRenderSubmit installs (or, with nil, clears) the off-thread render
// hand-off (see asyncRenderSubmit).
func SetAsyncRenderSubmit(f func(id instrument.ID, pitch float64, d instrument.Doc)) {
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
// first note beats a SILENT one (user decision).
//
// Two tiers, each strictly better than a synchronous ~24ms CGo render:
//  1. Same (instrument, pitch) under any document — a param edit rotated the key.
//  2. Same instrument at ANY pitch — a brand-new note appearing faster than
//     the prewarmer's horizon. An audibly wrong note ON TIME.
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

// hasDocVoice reports whether id renders from its instrument document: it has
// one, and no user sample currently overrides it.
func hasDocVoice(id instrument.ID) bool {
	return !IsUserSample(id) && ensureDoc(id)
}

// newRecipeAwareVoice is the voice dispatcher called by Play / PlayVol /
// PlayParams (engine_play.go): the document path for synth instruments, the
// instrument table for everything else.
//
// WARNING: this bare wrapper attributes its render to originDispatch. It is
// correct ONLY for callers that actually run on the audioLoop dispatch
// goroutine. Any other caller must use newRecipeAwareVoiceOrigin explicitly.
func newRecipeAwareVoice(id instrument.ID, sampleRate int) Voice {
	return newRecipeAwareVoiceOrigin(id, sampleRate, originDispatch)
}

// newRecipeAwareVoiceOrigin is newRecipeAwareVoice with explicit render
// attribution. Origin affects only the metric, never the cache key.
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

// roundPitchForCache rounds a semitone value to the nearest 0.5 st for the
// cache key (node pitches quantize to integer or half-integer semitones).
func roundPitchForCache(pitch float64) float64 {
	return math.Round(pitch*2) / 2
}

// newRecipeAwareVoicePitched is the pitch-aware dispatcher: a melodic document
// renders AT the node pitch; everything else falls through unchanged.
//
// WARNING: bare wrapper — attributes originDispatch (see newRecipeAwareVoice).
func newRecipeAwareVoicePitched(id instrument.ID, sampleRate int, pitch float64) Voice {
	return newRecipeAwareVoicePitchedOrigin(id, sampleRate, pitch, originDispatch)
}

// newRecipeAwareVoicePitchedOrigin is newRecipeAwareVoicePitched with explicit
// render attribution.
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

// tryRecipeVoicePitched renders id's document at the node pitch.
//
// WARNING: bare wrapper — attributes originDispatch (see newRecipeAwareVoice).
func tryRecipeVoicePitched(id instrument.ID, sampleRate int, pitch float64) (Voice, bool) {
	return tryRecipeVoicePitchedOrigin(id, sampleRate, pitch, originDispatch)
}

// tryRecipeVoicePitchedOrigin is tryRecipeVoicePitched with explicit render
// attribution. The block carries the EXACT pitch; the key its 0.5-st rounding.
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

// pitchedVoiceKey is the EFFECTIVE globalVoiceCache key dispatch reads for a
// pitch-aware (id, pitch) trigger, recomputed from the CURRENT document
// (guarded by TestVoiceCachedMatchesDispatch).
func pitchedVoiceKey(id instrument.ID, sampleRate int, pitch float64) (voiceCacheKey, bool) {
	return liveVoiceKey(id, sampleRate, pitch, true, false)
}

// pitchedVoiceKeyForDoc is pitchedVoiceKey for an explicit document snapshot
// (prewarm of a prospective edit). Byte-identical to the dispatch key for the
// same document (TestPrewarmVoiceParamsKeyMatchesDispatch).
func pitchedVoiceKeyForDoc(id instrument.ID, sampleRate int, pitch float64, d instrument.Doc) voiceCacheKey {
	key, _ := docVoiceKey(id, d, sampleRate, pitch, true, false)
	return key
}

// renderPitchedIntoCacheForDoc renders (id, pitch) under document d into
// globalVoiceCache. No-op when already cached; false when d is not melodic.
func renderPitchedIntoCacheForDoc(id instrument.ID, sampleRate int, pitch float64, d instrument.Doc, origin renderOrigin) bool {
	if !d.Render.Melodic {
		return false
	}
	renderDocIntoCache(id, d, sampleRate, pitch, true, false, origin)
	return true
}

// VoiceCached reports whether the pitch-aware voice for (id, pitch) is already
// present under the key the dispatch path will read.
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

// tryRecipeVoiceOpts is tryRecipeVoice with an ignoreEdit escape hatch used by
// RenderInstrumentOneShotRaw: the Sampler editor needs the UN-edited source.
//
// WARNING: bare wrapper — attributes originDispatch (see newRecipeAwareVoice).
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

// legacyNewVoice routes through the instrument table. Returns nil for unknown
// ids — matches the silent-drop behavior of Play / PlayVol / PlayParams.
func legacyNewVoice(id instrument.ID, sampleRate int) Voice {
	instMu.RLock()
	inst, ok := instruments[id]
	instMu.RUnlock()
	if !ok {
		return nil
	}
	return inst.NewVoice(sampleRate)
}
