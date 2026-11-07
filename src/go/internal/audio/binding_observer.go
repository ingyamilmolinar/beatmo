package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Fires when params rotate an instrument's cache key, so the new binding renders off-thread before the next
// trigger. SetInstrumentParam does not invalidate, so VoiceCacheInvalidateObserver never fires on a knob drag; the
// JS bridge is a different concern. nil when unused.
var InstrumentBindingChangedObserver func(id instrument.ID, d instrument.Doc)

// Called after the store lock is released, so the observer may re-enter the package.
func notifyInstrumentBindingChanged(id instrument.ID, d instrument.Doc) {
	if f := InstrumentBindingChangedObserver; f != nil {
		f(id, d)
	}
}
