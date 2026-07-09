package audio

// VoiceCacheInvalidateObserver, when set, is notified with the instrument id
// whenever the voice cache is invalidated (ClearInstrument), or "" when the
// whole cache is cleared (Clear). The UI voice-prewarmer registers it to
// proactively re-render the affected instrument's known pitches off the
// audioLoop dispatch goroutine — so a synth param edit during playback does not
// leave the next beat to cold-render inline (the overdue-panic window). The
// callback must be cheap and non-blocking; it is invoked from cache-mutation
// paths. Untagged so every build (native/test/js) can reference the var; only
// the native cache actually fires it.
var VoiceCacheInvalidateObserver func(id string)

// notifyVoiceCacheInvalidated fires the observer if one is registered.
func notifyVoiceCacheInvalidated(id string) {
	if f := VoiceCacheInvalidateObserver; f != nil {
		f(id)
	}
}
