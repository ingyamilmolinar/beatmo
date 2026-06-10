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

// TestVoiceCacheGetSharesBackingArray pins the post-OOM-fix contract:
// Get returns the cached backing array directly (no clone). cVoice
// playback never writes to v.buf (drums_c.go: Sample/SampleBlock only
// READ and advance v.i), so concurrent voices can share the slice
// safely. Removing the ~80 KB clone-on-get was a key step in cutting the
// PlayParams cumulative allocation rate observed in the synth-tab
// profile.
//
// CONTRACT REGRESSION GUARD: if a future change reintroduces a clone in
// Get, the test fails because &got[0] and &refetched[0] would differ.
// Equally, if a caller mutates the returned slice (violating the contract
// the new API documents), every concurrent voice would observe the
// mutation — this test would still pass, but the synth-tab soak test
// would surface the corruption as audio glitches.
func TestVoiceCacheGetSharesBackingArray(t *testing.T) {
	vc := newTestCache()
	key := testKey("kick")
	vc.Put(key, []float32{1, 2, 3})

	got, ok := vc.Get(key)
	if !ok {
		t.Fatal("Get returned false for stored key")
	}
	got2, ok := vc.Get(key)
	if !ok {
		t.Fatal("second Get returned false")
	}
	if len(got) == 0 || len(got2) == 0 {
		t.Fatalf("empty buffers: |got|=%d |got2|=%d", len(got), len(got2))
	}
	if &got[0] != &got2[0] {
		t.Errorf("Get must return the SAME backing array across calls; "+
			"got &[0]=%p, got2 &[0]=%p — a clone has snuck back into Get",
			&got[0], &got2[0])
	}
}

// TestVoiceCachePutSharesBackingArray pins the post-OOM-fix contract:
// Put stores the caller-provided slice directly (no clone). Callers
// (tryRecipeVoice and the CVariantInstrument render paths) must not
// mutate the buffer after calling Put; the cache and any concurrent Get
// reader share the backing array. Removing the ~80 KB clone-on-put was
// the second leg of the per-trigger cache allocation reduction.
func TestVoiceCachePutSharesBackingArray(t *testing.T) {
	vc := newTestCache()
	key := testKey("hihat")
	buf := []float32{1, 2, 3}
	vc.Put(key, buf)

	got, ok := vc.Get(key)
	if !ok {
		t.Fatal("Get returned false for stored key")
	}
	if len(got) == 0 || len(buf) == 0 {
		t.Fatalf("empty buffers: |got|=%d |buf|=%d", len(got), len(buf))
	}
	if &got[0] != &buf[0] {
		t.Errorf("Put must store the SAME backing array the caller supplied; "+
			"got &[0]=%p, buf &[0]=%p — a clone has snuck back into Put",
			&got[0], &buf[0])
	}
}

// TestVoiceCacheGetPutAllocBudget pins the per-call allocation count for
// the cache hot path. Pre-fix this was ~2 allocs per Get (the clone +
// its slice header) and ~2 allocs per Put. After the fix Get is zero-
// alloc on cache-hit and Put is bounded by map-growth amortized cost.
func TestVoiceCacheGetPutAllocBudget(t *testing.T) {
	vc := newTestCache()
	key := testKey("budget")
	buf := make([]float32, 22050) // realistic snare-sized buffer
	vc.Put(key, buf)

	// Warm up.
	for i := 0; i < 5; i++ {
		_, _ = vc.Get(key)
	}

	const getBudget = 0.0
	getAllocs := testing.AllocsPerRun(1000, func() {
		_, _ = vc.Get(key)
	})
	if getAllocs > getBudget {
		t.Errorf("Get allocs/call = %.1f exceeds budget %.0f — a clone or other "+
			"alloc has re-entered the cache hit path", getAllocs, getBudget)
	}
	t.Logf("Get allocs/call = %.1f (budget %.0f, %d-sample buffer) — "+
		"pre-fix was ~2 allocs/call (clone + slice header)",
		getAllocs, getBudget, len(buf))
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
	key1 := voiceCacheKey{instrumentID: "snare", sampleRate: 44100, paramsHash: hashRecipeParams(SynthParams{}.ToRecipeParams())}
	key2 := voiceCacheKey{instrumentID: "snare", sampleRate: 44100, paramsHash: hashRecipeParams(SynthParams{Pitch: 2.0}.ToRecipeParams())}

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
