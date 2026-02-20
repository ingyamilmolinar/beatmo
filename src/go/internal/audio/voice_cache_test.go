//go:build !test

package audio

import (
	"sync"
	"testing"
)

func newTestCache() *voiceCache {
	return &voiceCache{
		entries: make(map[voiceCacheKey]*roundRobinEntry),
		maxSize: 256,
	}
}

func testKey(id string) voiceCacheKey {
	return voiceCacheKey{instrumentID: id, bpm: 0, sampleRate: 44100}
}

func TestVoiceCacheGetMiss(t *testing.T) {
	vc := newTestCache()
	buf, ok := vc.Get(testKey("missing"))
	if ok || buf != nil {
		t.Errorf("Get on empty cache: buf=%v ok=%v, want nil/false", buf, ok)
	}
}

func TestVoiceCachePutAndGet(t *testing.T) {
	vc := newTestCache()
	key := testKey("snare")
	data := []float32{0.1, 0.2, 0.3}
	vc.Put(key, data)

	got, ok := vc.Get(key)
	if !ok {
		t.Fatal("Get after Put returned false")
	}
	if len(got) != len(data) {
		t.Fatalf("got len %d, want %d", len(got), len(data))
	}
	for i, v := range got {
		if v != data[i] {
			t.Errorf("sample[%d] = %f, want %f", i, v, data[i])
		}
	}
}

func TestVoiceCacheGetReturnsClone(t *testing.T) {
	vc := newTestCache()
	key := testKey("kick")
	vc.Put(key, []float32{1, 2, 3})

	got, _ := vc.Get(key)
	got[0] = 999 // mutate the returned slice

	got2, _ := vc.Get(key)
	if got2[0] == 999 {
		t.Error("mutating Get result corrupted cache")
	}
}

func TestVoiceCachePutStoresCopy(t *testing.T) {
	vc := newTestCache()
	key := testKey("hihat")
	buf := []float32{1, 2, 3}
	vc.Put(key, buf)

	buf[0] = 999 // mutate original

	got, _ := vc.Get(key)
	if got[0] == 999 {
		t.Error("mutating original after Put corrupted cache")
	}
}

func TestVoiceCacheRoundRobin(t *testing.T) {
	vc := newTestCache()
	key := testKey("tom")

	// Put 3 distinct variants.
	for v := 0; v < RoundRobinVariants; v++ {
		vc.Put(key, []float32{float32(v)})
	}

	// Get should cycle through all 3 (counter starts at 0, Add(1) gives 1,2,3...).
	seen := map[float32]bool{}
	for i := 0; i < RoundRobinVariants*2; i++ {
		got, ok := vc.Get(key)
		if !ok {
			t.Fatalf("Get returned false on iteration %d", i)
		}
		seen[got[0]] = true
	}
	if len(seen) != RoundRobinVariants {
		t.Errorf("round-robin saw %d unique variants, want %d", len(seen), RoundRobinVariants)
	}
}

func TestVoiceCacheRoundRobinWraps(t *testing.T) {
	vc := newTestCache()
	key := testKey("clap")

	for v := 0; v < RoundRobinVariants; v++ {
		vc.Put(key, []float32{float32(v * 10)})
	}

	// Get many times to verify wrapping.
	for i := 0; i < 1000; i++ {
		got, ok := vc.Get(key)
		if !ok {
			t.Fatalf("Get returned false on iteration %d", i)
		}
		// Value should be 0, 10, or 20.
		valid := got[0] == 0 || got[0] == 10 || got[0] == 20
		if !valid {
			t.Errorf("iteration %d: unexpected value %f", i, got[0])
			break
		}
	}
}

func TestVoiceCacheIsFullFalseForMissing(t *testing.T) {
	vc := newTestCache()
	if vc.IsFull(testKey("none")) {
		t.Error("IsFull returned true for missing key")
	}
}

func TestVoiceCacheIsFullFalsePartial(t *testing.T) {
	vc := newTestCache()
	key := testKey("partial")
	vc.Put(key, []float32{1})
	if vc.IsFull(key) {
		t.Error("IsFull returned true with only 1 variant")
	}
}

func TestVoiceCacheIsFullTrue(t *testing.T) {
	vc := newTestCache()
	key := testKey("full")
	for i := 0; i < RoundRobinVariants; i++ {
		vc.Put(key, []float32{float32(i)})
	}
	if !vc.IsFull(key) {
		t.Error("IsFull returned false after all variants stored")
	}
}

func TestVoiceCachePutIgnoresExtraVariants(t *testing.T) {
	vc := newTestCache()
	key := testKey("extra")
	for i := 0; i < RoundRobinVariants+1; i++ {
		vc.Put(key, []float32{float32(i)})
	}

	vc.mu.RLock()
	entry := vc.entries[key]
	vc.mu.RUnlock()

	if len(entry.variants) != RoundRobinVariants {
		t.Errorf("variants count = %d, want %d (extra should be ignored)", len(entry.variants), RoundRobinVariants)
	}
}

func TestVoiceCacheMaxSizeEvictionBlock(t *testing.T) {
	vc := &voiceCache{
		entries: make(map[voiceCacheKey]*roundRobinEntry),
		maxSize: 4,
	}

	// Fill to max.
	for i := 0; i < 4; i++ {
		key := voiceCacheKey{instrumentID: string(rune('a' + i)), sampleRate: 44100}
		vc.Put(key, []float32{1})
	}

	// 5th key should be silently dropped.
	overflow := voiceCacheKey{instrumentID: "overflow", sampleRate: 44100}
	vc.Put(overflow, []float32{1})

	if _, ok := vc.Get(overflow); ok {
		t.Error("overflow key should not be stored when cache is at maxSize")
	}
}

func TestVoiceCacheClear(t *testing.T) {
	vc := newTestCache()
	key := testKey("clear")
	vc.Put(key, []float32{1, 2, 3})

	vc.Clear()

	if _, ok := vc.Get(key); ok {
		t.Error("Get returned true after Clear")
	}
	if vc.IsFull(key) {
		t.Error("IsFull returned true after Clear")
	}
}

func TestVoiceCacheDifferentSynthParamsKey(t *testing.T) {
	vc := newTestCache()
	key1 := voiceCacheKey{instrumentID: "snare", sampleRate: 44100, synthParams: SynthParams{}}
	key2 := voiceCacheKey{instrumentID: "snare", sampleRate: 44100, synthParams: SynthParams{Pitch: 2.0}}

	vc.Put(key1, []float32{1})
	vc.Put(key2, []float32{2})

	got1, _ := vc.Get(key1)
	got2, _ := vc.Get(key2)
	if got1[0] == got2[0] {
		t.Error("different SynthParams should produce separate cache entries")
	}
}

func TestVoiceCacheConcurrentGetPut(t *testing.T) {
	vc := newTestCache()
	var wg sync.WaitGroup

	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			key := voiceCacheKey{instrumentID: "conc", bpm: id, sampleRate: 44100}
			for i := 0; i < 100; i++ {
				vc.Put(key, []float32{float32(i)})
				vc.Get(key)
				vc.IsFull(key)
			}
		}(g)
	}
	wg.Wait()
	// No race/panic = pass. Run with -race to verify.
}
