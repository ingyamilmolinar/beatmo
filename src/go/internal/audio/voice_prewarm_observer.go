package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Called with the id on ClearInstrument, "" on Clear, so the prewarmer re-renders the instrument's pitches off the
// dispatch goroutine. Must be cheap and non-blocking: invoked from cache mutations. Only the native cache fires it.
var VoiceCacheInvalidateObserver func(id instrument.ID)

// notifyVoiceCacheInvalidated fires the observer if one is registered.
func notifyVoiceCacheInvalidated(id instrument.ID) {
	if f := VoiceCacheInvalidateObserver; f != nil {
		f(id)
	}
}
