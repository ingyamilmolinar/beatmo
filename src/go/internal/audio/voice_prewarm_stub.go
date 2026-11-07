//go:build test

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// UI tests observe the decision through the prewarmer's warmFn seam.
func PrewarmVoice(id instrument.ID, pitch float64) {}

// No cache under -tags test, so every pitch needs a warm: the safe direction.
func VoiceCached(id instrument.ID, pitch float64) bool { return false }

// PrewarmVoiceParams is a no-op under -tags test (the C renderer is stubbed).
func PrewarmVoiceDoc(id instrument.ID, pitch float64, d instrument.Doc) {}

// UI neighbourhood tests drive the enumeration directly; a test needing true overrides the predicate.
func WarmsPerPitch(id instrument.ID) bool { return false }

// ClearVoiceCacheForTest is a no-op under -tags test (no real voice cache exists).
func ClearVoiceCacheForTest() {}

// SetDispatchMustNotRender is a no-op under -tags test (the C renderer is stubbed).
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op under -tags test.
func SetAsyncRenderSubmit(f func(id instrument.ID, pitch float64, d instrument.Doc)) {}

// VoiceReady: the test build renders nothing, so nothing waits.
func VoiceReady(id instrument.ID, pitch float64) bool { return true }
