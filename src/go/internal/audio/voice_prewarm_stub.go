//go:build test

package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// PrewarmVoice is a no-op under the fast (-tags test) build: the C renderer is
// stubbed, and UI tests observe the prewarm decision via voicePrewarmer's
// injectable warmFn seam rather than through a real render.
func PrewarmVoice(id instrument.ID, pitch float64) {}

// VoiceCached always reports false under the fast (-tags test) build: there is
// no real voice cache, so the prewarmer always treats pitches as needing a warm
// (the safe direction — its warmFn is a test recorder, not a real render).
func VoiceCached(id instrument.ID, pitch float64) bool { return false }

// PrewarmVoiceParams is a no-op under -tags test (the C renderer is stubbed).
func PrewarmVoiceDoc(id instrument.ID, pitch float64, d instrument.Doc) {}

// WarmsPerPitch reports false under -tags test (the C renderer is stubbed;
// UI neighborhood tests drive the enumeration directly). A dedicated UI test can
// override the prewarmer's melodic predicate if it needs true.
func WarmsPerPitch(id instrument.ID) bool { return false }

// ClearVoiceCacheForTest is a no-op under -tags test (no real voice cache exists).
func ClearVoiceCacheForTest() {}

// SetDispatchMustNotRender is a no-op under -tags test (the C renderer is stubbed).
func SetDispatchMustNotRender(on bool) {}

// SetAsyncRenderSubmit is a no-op under -tags test.
func SetAsyncRenderSubmit(f func(id instrument.ID, pitch float64, d instrument.Doc)) {}

// VoiceReady: the test build renders nothing, so nothing waits.
func VoiceReady(id instrument.ID, pitch float64) bool { return true }
