//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Native only: under -tags test the cache is stubbed and these would pass vacuously.

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

// The edited params map to a DIFFERENT key, so old entries are retained within the LRU budget: that is what lets a
// pre-warmed neighbour survive a drag. Blanket clearing caused the knob-drag miss storm.
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
