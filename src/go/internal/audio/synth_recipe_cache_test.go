//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// These tests live on the native-build path because they exercise the real
// globalVoiceCache and the voiceCacheInvalidate hook wired up in
// voice_cache.go init(). Under -tags test the cache is stubbed out, so the
// hook is a no-op and these scenarios are vacuously true.

func TestVoiceCache_ClearInstrumentRemovesAllItsEntries(t *testing.T) {
	vc := newTestCache()
	a := voiceCacheKey{instrumentID: roster.Snare, sampleRate: 44100}
	b := voiceCacheKey{instrumentID: roster.Snare, sampleRate: 44100, paramsHash: 42}
	c := voiceCacheKey{instrumentID: roster.Kick, sampleRate: 44100}

	vc.Put(a, []float32{1})
	vc.Put(b, []float32{2})
	vc.Put(c, []float32{3})

	vc.ClearInstrument(roster.Snare)

	if _, ok := vc.Get(a); ok {
		t.Errorf("entry for snare/a still present after ClearInstrument")
	}
	if _, ok := vc.Get(b); ok {
		t.Errorf("entry for snare/b still present after ClearInstrument")
	}
	if _, ok := vc.Get(c); !ok {
		t.Errorf("ClearInstrument(\"snare\") removed kick entry")
	}
}

func TestVoiceCache_ClearInstrumentUnknownIsNoop(t *testing.T) {
	vc := newTestCache()
	k := voiceCacheKey{instrumentID: roster.Snare, sampleRate: 44100}
	vc.Put(k, []float32{1})

	vc.ClearInstrument("nobody-here")

	if _, ok := vc.Get(k); !ok {
		t.Errorf("unrelated ClearInstrument removed snare entry")
	}
}

// TestSetInstrumentParam_DoesNotBlanketInvalidate documents the neighborhood-
// cache behavior change: a single live param edit (the knob-drag hot path) no
// longer clears the whole instrument's cached voices. The edited params map to a
// DIFFERENT cache key (full paramsHash), so dispatch renders the new sound under
// that key while old-value entries are RETAINED (bounded by the LRU byte
// budget). Retaining them is what lets a pre-warmed neighbor survive a drag and
// hit instead of cold-rendering. Blanket-clearing here was the root of the
// 93%-miss knob-drag storm. Contrast ResetInstrumentParams (below), which still
// clears.
func TestSetInstrumentParam_DoesNotBlanketInvalidate(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("test-cache-snare") })

	// Seed the global cache with a snare entry (old-params key).
	k := voiceCacheKey{instrumentID: "test-cache-snare", sampleRate: 44100}
	globalVoiceCache.Put(k, []float32{0.5})
	if _, ok := globalVoiceCache.Get(k); !ok {
		t.Fatal("precondition: cache should hold the seeded entry")
	}

	SetInstrumentParam("test-cache-snare", "pitch", 1)

	if _, ok := globalVoiceCache.Get(k); !ok {
		t.Errorf("a single live param edit must NOT blanket-clear the cache — " +
			"old-value entries are retained (LRU-bounded) so pre-warmed neighbors survive a drag")
	}
}

func TestResetInstrumentParams_InvalidatesGlobalVoiceCache(t *testing.T) {
	if err := InstallInstrumentDocForTest("test-cache-kick", roster.Kick); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { RemoveInstrumentDocForTest("test-cache-kick") })

	// Set a param, then Reset; both should invalidate the cache.
	SetInstrumentParam("test-cache-kick", "amp_decay", 0.5)

	k := voiceCacheKey{instrumentID: "test-cache-kick", sampleRate: 44100}
	globalVoiceCache.Put(k, []float32{0.7})
	if _, ok := globalVoiceCache.Get(k); !ok {
		t.Fatal("precondition: cache should hold the seeded entry")
	}

	ResetInstrumentParams("test-cache-kick")

	if _, ok := globalVoiceCache.Get(k); ok {
		t.Errorf("ResetInstrumentParams did not invalidate globalVoiceCache for instrument")
	}
}
