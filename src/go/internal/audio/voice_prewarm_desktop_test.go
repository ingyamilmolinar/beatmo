//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// So a pre-warmed note is a HIT at dispatch, never a cold inline render.
func TestPrewarmVoiceKeyMatchesDispatch(t *testing.T) {
	const id = roster.OrganChurch // melodic, pitch-aware (the instrument that crashed at pitch -2)
	sr := SampleRate()            // the rate PrewarmVoice renders at
	if !IsMelodicInstrument(id) {
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

	// The function playParamsIdentity calls; it must hit.
	if v := newRecipeAwareVoicePitched(id, sr, -2); v == nil {
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

// Must report the CURRENT state: a voice left marked "warmed" after an edit cold-rendered on the dispatch goroutine.
func TestVoiceCachedTracksDispatchKeyAcrossInvalidation(t *testing.T) {
	const id = roster.OrganChurch
	if !IsMelodicInstrument(id) {
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
	// After the edit VoiceCached must report cold, so the UI re-warms instead of trusting a stale seen-set.
	SetInstrumentParam(id, "gain", 0.9)
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

// So the prewarmer can re-warm proactively.
func TestClearInstrumentFiresInvalidateObserver(t *testing.T) {
	prev := VoiceCacheInvalidateObserver
	defer func() { VoiceCacheInvalidateObserver = prev }()

	var got instrument.ID
	VoiceCacheInvalidateObserver = func(id instrument.ID) { got = id }

	globalVoiceCache.ClearInstrument(roster.OrganChurch)
	if got != roster.OrganChurch {
		t.Fatalf("observer not fired with instrument id on ClearInstrument; got %q", got)
	}

	got = "unset"
	globalVoiceCache.Clear()
	if got != "" {
		t.Fatalf("observer should fire with \"\" on whole-cache Clear; got %q", got)
	}
}
