//go:build test || js

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// No native cache on stub/WASM builds; same signature so soak tests call it unconditionally.
func VoiceCacheStatsForTest() (entries int, totalBytes int64) {
	return 0, 0
}

// SetVoiceCacheBudgetBytes is a no-op on stub/WASM builds (no native cache).
func SetVoiceCacheBudgetBytes(n int64) { platformRenderBudget(n) }

// The browser's render cache takes the budget; the test build keeps none.
var platformRenderBudget = func(int64) {}

// No real cache under -tags test or js: callers fall back to a placeholder.
func LatestVoiceSample(instrumentID instrument.ID) []float32 {
	return nil
}
