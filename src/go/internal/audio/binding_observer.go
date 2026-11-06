package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// binding_observer.go — notification that an instrument's VOICE BINDING changed
// (its params rotated the cache key), so an interested party can render the new
// binding off-thread BEFORE the next trigger asks for it.
//
// Why not reuse VoiceCacheInvalidateObserver: SetInstrumentParam deliberately
// does NOT invalidate (synth_recipe.go:256-267), so that observer never fires
// on the knob-drag hot path — the exact path that needs warming.
//
// Why not reuse platformInstrumentParamsChanged: that is the JS bridge. Hanging
// desktop prewarm off a platform bridge conflates two unrelated concerns.
//
// Set by internal/ui at startup; nil in tests that don't need it.
var InstrumentBindingChangedObserver func(id instrument.ID, d instrument.Doc)

// notifyInstrumentBindingChanged fires the observer with the instrument's NEW
// document. Called after the store lock is released, so the observer may
// re-enter the audio package safely.
func notifyInstrumentBindingChanged(id instrument.ID, d instrument.Doc) {
	if f := InstrumentBindingChangedObserver; f != nil {
		f(id, d)
	}
}
