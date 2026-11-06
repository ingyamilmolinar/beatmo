//go:build test || js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// VoiceCacheStatsForTest returns zero on stub/WASM builds — the native voice
// cache (voice_cache.go) is `!test && !js` and so its globalVoiceCache does
// not exist here. The signature must match the native build so soak tests
// can call it unconditionally; under stub builds the result is just a
// "we did not exercise the native cache" sentinel rather than a measurement.
func VoiceCacheStatsForTest() (entries int, totalBytes int64) {
	return 0, 0
}

// SetVoiceCacheBudgetBytes is a no-op on stub/WASM builds (no native cache).
func SetVoiceCacheBudgetBytes(n int64) { platformRenderBudget(n) }

// platformRenderBudget hands the budget to the browser's render cache
// (audio.js setRenderCacheBudget); the test build keeps none.
var platformRenderBudget = func(int64) {}

// LatestVoiceSample returns the most recently rendered cached sample
// buffer for the given instrument. Under -tags test / GOOS=js, the
// real voice cache doesn't exist, so this returns nil and UI callers
// fall back to the placeholder waveform. Phase 4 — Synth tab.
func LatestVoiceSample(instrumentID instrument.ID) []float32 {
	return nil
}
