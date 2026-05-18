//go:build !test && !js

package audio

import "testing"

// These tests live on the native-build path because they exercise the real
// globalVoiceCache and the voiceCacheInvalidate hook wired up in
// voice_cache.go init(). Under -tags test the cache is stubbed out, so the
// hook is a no-op and these scenarios are vacuously true.

func TestVoiceCache_ClearInstrumentRemovesAllItsEntries(t *testing.T) {
	vc := newTestCache()
	a := voiceCacheKey{instrumentID: "snare", sampleRate: 44100}
	b := voiceCacheKey{instrumentID: "snare", bpm: 120, sampleRate: 44100, paramsHash: 42}
	c := voiceCacheKey{instrumentID: "kick", sampleRate: 44100}

	vc.Put(a, []float32{1})
	vc.Put(b, []float32{2})
	vc.Put(c, []float32{3})

	vc.ClearInstrument("snare")

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
	k := voiceCacheKey{instrumentID: "snare", sampleRate: 44100}
	vc.Put(k, []float32{1})

	vc.ClearInstrument("nobody-here")

	if _, ok := vc.Get(k); !ok {
		t.Errorf("unrelated ClearInstrument removed snare entry")
	}
}

func TestSetInstrumentParam_InvalidatesGlobalVoiceCache(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("test-cache-snare") })

	// Seed the global cache with a snare entry.
	k := voiceCacheKey{instrumentID: "test-cache-snare", sampleRate: 44100}
	globalVoiceCache.Put(k, []float32{0.5})
	if _, ok := globalVoiceCache.Get(k); !ok {
		t.Fatal("precondition: cache should hold the seeded entry")
	}

	SetInstrumentParam("test-cache-snare", "pitch", 1)

	if _, ok := globalVoiceCache.Get(k); ok {
		t.Errorf("SetInstrumentParam did not invalidate globalVoiceCache for instrument")
	}
}

func TestResetInstrumentParams_InvalidatesGlobalVoiceCache(t *testing.T) {
	t.Cleanup(func() { ResetInstrumentParams("test-cache-kick") })

	// Set a param, then Reset; both should invalidate the cache.
	SetInstrumentParam("test-cache-kick", "decay", 0.5)

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
