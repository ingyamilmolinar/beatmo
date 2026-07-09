//go:build !test && !js

package audio

import "testing"

// TestPrewarmVoiceKeyMatchesDispatch proves the linchpin: warming (id, pitch)
// populates the exact cache slot the dispatch read (newRecipeAwareVoicePitched)
// later hits — so a pre-warmed melody note is a cache HIT at dispatch, never a
// cold inline render on the audioLoop goroutine.
func TestPrewarmVoiceKeyMatchesDispatch(t *testing.T) {
	const (
		id = "organ-church" // melodic, pitch-aware (the instrument that crashed at pitch -2)
		sr = 44100
	)
	if !pitchAwareRecipe(RecipeForInstrument(id)) {
		t.Fatalf("precondition: %q must be a pitch-aware recipe", id)
	}
	globalVoiceCache.Clear()
	ResetRenderLatencyMetrics()

	// Warm the exact pitch the melody will play.
	PrewarmVoice(id, -2)
	m := GetRenderLatencyMetrics()
	if m.CacheMisses < 1 {
		t.Fatalf("PrewarmVoice should render once (cache miss); got %+v", m)
	}
	missesAfterWarm := m.CacheMisses

	// Dispatch reads the SAME (id, pitch) via the function playParamsIdentity
	// calls. It must be a cache HIT (warm key == dispatch key), no re-render.
	if v := newRecipeAwareVoicePitched(id, bpm, sr, -2); v == nil {
		t.Fatal("dispatch render returned nil")
	}
	m2 := GetRenderLatencyMetrics()
	if m2.CacheHits < 1 {
		t.Fatalf("dispatch of a pre-warmed pitch must be a cache hit; got %+v", m2)
	}
	if m2.CacheMisses != missesAfterWarm {
		t.Fatalf("dispatch must not re-render a warmed pitch: misses %d -> %d", missesAfterWarm, m2.CacheMisses)
	}
}

// TestVoiceCachedTracksDispatchKeyAcrossInvalidation is the regression for the
// stale-seen bug: VoiceCached must report the CURRENT cache state so the UI
// prewarmer re-warms after a param edit clears the entry. Without this, a
// cleared voice stayed marked "warmed" and cold-rendered on the dispatch
// goroutine -> overdue panic.
func TestVoiceCachedTracksDispatchKeyAcrossInvalidation(t *testing.T) {
	const id = "organ-church"
	if !pitchAwareRecipe(RecipeForInstrument(id)) {
		t.Fatalf("precondition: %q must be a pitch-aware recipe", id)
	}
	globalVoiceCache.Clear()

	// Cold: not cached yet.
	if VoiceCached(id, 5) {
		t.Fatalf("VoiceCached true before any warm")
	}
	// Warm it -> cached.
	PrewarmVoice(id, 5)
	if !VoiceCached(id, 5) {
		t.Fatalf("VoiceCached false right after PrewarmVoice")
	}
	// A param edit invalidates the instrument's cache (ClearInstrument via the
	// SetInstrumentParam hook). VoiceCached must now report cold so the UI
	// re-warms instead of trusting a stale seen-set.
	SetInstrumentParam(id, "post_gain", 0.9)
	defer ResetInstrumentParams(id)
	if VoiceCached(id, 5) {
		t.Fatalf("VoiceCached still true after a param edit cleared the cache — the stale-seen bug")
	}
	// Re-warm under the new params -> cached again, and dispatch would hit.
	PrewarmVoice(id, 5)
	if !VoiceCached(id, 5) {
		t.Fatalf("VoiceCached false after re-warm under edited params")
	}
}

// TestClearInstrumentFiresInvalidateObserver verifies the audio->UI wiring: a
// cache invalidation (param edit path) notifies the registered observer with the
// instrument id, so the prewarmer can proactively re-warm.
func TestClearInstrumentFiresInvalidateObserver(t *testing.T) {
	prev := VoiceCacheInvalidateObserver
	defer func() { VoiceCacheInvalidateObserver = prev }()

	var got string
	VoiceCacheInvalidateObserver = func(id string) { got = id }

	globalVoiceCache.ClearInstrument("organ-church")
	if got != "organ-church" {
		t.Fatalf("observer not fired with instrument id on ClearInstrument; got %q", got)
	}

	got = "unset"
	globalVoiceCache.Clear()
	if got != "" {
		t.Fatalf("observer should fire with \"\" on whole-cache Clear; got %q", got)
	}
}
