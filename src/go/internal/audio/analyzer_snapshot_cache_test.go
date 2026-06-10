package audio

import (
	"testing"
	"time"
)

// TestSnapshotCache_TTLHonored asserts the per-channel snapshot cache
// returns the stored value for repeated id lookups within the TTL
// window and refetches when TTL expires. This is the core OOM-
// prevention mechanism for the WASM analyzer bridge: the Draw loop
// runs at 60+ fps but the JS-side AnalyserNode updates at ~30 Hz, so
// cache turn-over must be bounded by the TTL not by the Draw rate.
//
// The cache file lives in the no-build-tag path so this test runs
// alongside the rest of the audio suite; the cache implementation
// itself has no js.Value dependency.
func TestSnapshotCache_TTLHonored(t *testing.T) {
	c := newSnapshotCache()
	var fetches int
	probe := AnalyzerSnapshot{
		RMS:      0.42,
		Spectrum: []float64{1, 2, 3},
		Waveform: []float64{4, 5, 6},
	}
	fetch := func() AnalyzerSnapshot {
		fetches++
		return probe
	}

	// First call: cold cache, must fetch.
	if got := c.getOrFetch("kick", time.Hour, fetch); got.RMS != 0.42 {
		t.Fatalf("first fetch returned RMS=%v, want 0.42", got.RMS)
	}
	if fetches != 1 {
		t.Fatalf("expected 1 fetch after cold call, got %d", fetches)
	}

	// Many calls within TTL: must hit cache.
	for i := 0; i < 100; i++ {
		_ = c.getOrFetch("kick", time.Hour, fetch)
	}
	if fetches != 1 {
		t.Fatalf("expected fetch count to stay at 1 within TTL, got %d "+
			"— cache TTL is not gating the refetch path", fetches)
	}

	// Different id: must fetch independently.
	_ = c.getOrFetch("snare", time.Hour, fetch)
	if fetches != 2 {
		t.Fatalf("expected separate fetch for new id, got fetches=%d", fetches)
	}

	// Tiny TTL: subsequent call must refetch.
	c2 := newSnapshotCache()
	fetches = 0
	_ = c2.getOrFetch("kick", time.Nanosecond, fetch)
	time.Sleep(time.Millisecond) // ensure TTL elapses
	_ = c2.getOrFetch("kick", time.Nanosecond, fetch)
	if fetches != 2 {
		t.Fatalf("expected refetch after TTL elapsed, got fetches=%d", fetches)
	}
}

// TestSnapshotCache_HitAllocBudget pins the per-cached-call allocation
// count. A cache hit must not allocate at all (returning the stored
// snapshot value is a struct copy). Pre-fix the cache did not exist and
// every getOrFetch call ran the full JS bridge → ~600 allocs/call.
func TestSnapshotCache_HitAllocBudget(t *testing.T) {
	c := newSnapshotCache()
	probe := AnalyzerSnapshot{
		RMS:      0.42,
		Spectrum: make([]float64, 64),
		Waveform: make([]float64, 512),
	}
	fetchCount := 0
	fetch := func() AnalyzerSnapshot {
		fetchCount++
		return probe
	}

	// Prime the cache.
	_ = c.getOrFetch("kick", time.Hour, fetch)
	if fetchCount != 1 {
		t.Fatalf("setup: expected 1 fetch, got %d", fetchCount)
	}

	// Cache-hit budget: 0 allocs. Returns a copy of the AnalyzerSnapshot
	// value, which is small and stack-allocatable.
	const budget = 0.0
	allocs := testing.AllocsPerRun(1000, func() {
		_ = c.getOrFetch("kick", time.Hour, fetch)
	})
	if allocs > budget {
		t.Fatalf("snapshot cache hit allocs/call = %.1f exceeds budget %.0f "+
			"— a code path is allocating on the cache-hit return", allocs, budget)
	}
	if fetchCount != 1 {
		t.Fatalf("cache-hit fired %d additional fetches; should be 0",
			fetchCount-1)
	}
	t.Logf("snapshot cache-hit allocs/call = %.1f (budget %.0f) — "+
		"pre-fix every call ran the full JS bridge (~600 js.Value allocs)",
		allocs, budget)
}

// TestSnapshotByteBufPool_Reuse pins the bytes-per-cycle budget for the
// pooled scratch buffer used by readSnapshotF64FromTypedArray. After
// priming, get/put cycles should allocate near zero (sync.Pool reuse).
// The pool's interface{} boxing accounts for the residual 0.5/cycle
// budget — it is unavoidable without internal Go runtime changes.
func TestSnapshotByteBufPool_Reuse(t *testing.T) {
	const n = 2048
	for i := 0; i < 10; i++ {
		b := getSnapshotByteBuf(n)
		putSnapshotByteBuf(b)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		b := getSnapshotByteBuf(n)
		putSnapshotByteBuf(b)
	})
	// Budget = 1.5 to allow for the interface{} boxing sync.Pool does on
	// Put (1 alloc/cycle observed empirically). Pre-fix the WASM path
	// allocated 2048-byte buffers fresh on every Snapshot call instead
	// of reusing — at ~210 snapshots/sec that's ~440 KB/sec of allocator
	// churn directly attributable to the JS bridge readback.
	const budget = 1.5
	if allocs > budget {
		t.Errorf("snapshot byte-buf pool allocs/cycle = %.2f exceeds "+
			"budget %.2f — the pool's recycle path is not warming up; "+
			"check getSnapshotByteBuf", allocs, budget)
	}
	t.Logf("snapshot byte-buf pool allocs/cycle = %.2f (budget %.2f, "+
		"buffer size %d bytes) — sync.Pool boxes via interface{} which "+
		"costs one alloc per cycle; the underlying %d-byte buffer itself "+
		"is reused.", allocs, budget, n, n)
}

// TestSnapshotCache_BoundedAtDrawRate models the production usage
// pattern: the EQ Draw loop calls Snapshot for each of N channels at a
// high frame rate. With the cache in place the number of underlying
// fetches must stay bounded by (channels × elapsed / TTL), regardless
// of how often Snapshot is invoked.
func TestSnapshotCache_BoundedAtDrawRate(t *testing.T) {
	c := newSnapshotCache()
	const channels = 7
	const drawsPerSec = 1200         // 10x faster than reality to keep the test brisk
	const totalDraws = 600           // simulates 5 seconds of Draws at 120 fps in 0.5s wall
	const ttl = 3 * time.Millisecond // scaled to match drawsPerSec acceleration

	fetches := 0
	fetch := func() AnalyzerSnapshot {
		fetches++
		return AnalyzerSnapshot{}
	}

	ids := []string{"main", "ch1", "ch2", "ch3", "ch4", "ch5", "ch6"}
	if len(ids) != channels {
		t.Fatalf("setup: ids/channels mismatch")
	}

	start := time.Now()
	for d := 0; d < totalDraws; d++ {
		for _, id := range ids {
			_ = c.getOrFetch(id, ttl, fetch)
		}
		// Throttle the inner loop so wall-clock advances ~consistently.
		time.Sleep(time.Second / drawsPerSec)
	}
	elapsed := time.Since(start)

	// Maximum theoretical fetches: channels × (elapsed / TTL) + channels
	// (the +channels accounts for the cold-cache fetch on each id).
	maxFetches := channels*int(elapsed/ttl) + channels
	// Allow 20% slack for scheduler jitter.
	maxFetches = maxFetches * 12 / 10
	if fetches > maxFetches {
		t.Errorf("fetches=%d exceeds cap %d over elapsed=%v (channels=%d ttl=%v) "+
			"— cache is not gating fetches at the TTL", fetches, maxFetches,
			elapsed, channels, ttl)
	}
	t.Logf("fetches=%d over %d draws (%v elapsed, %d channels, ttl=%v) "+
		"— cap %d, ratio %.1fx",
		fetches, totalDraws, elapsed, channels, ttl, maxFetches,
		float64(fetches)/float64(channels*totalDraws))
}
