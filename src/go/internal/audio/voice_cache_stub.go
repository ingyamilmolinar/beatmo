//go:build test || js

package audio

// VoiceCacheStatsForTest returns zero on stub/WASM builds — the native voice
// cache (voice_cache.go) is `!test && !js` and so its globalVoiceCache does
// not exist here. The signature must match the native build so soak tests
// can call it unconditionally; under stub builds the result is just a
// "we did not exercise the native cache" sentinel rather than a measurement.
func VoiceCacheStatsForTest() (entries int, totalBytes int64) {
	return 0, 0
}

// LatestVoiceSample returns the most recently rendered cached sample
// buffer for the given instrument. Under -tags test / GOOS=js, the
// real voice cache doesn't exist, so this returns nil and UI callers
// fall back to the placeholder waveform. Phase 4 — Synth tab.
func LatestVoiceSample(instrumentID string) []float32 {
	return nil
}
