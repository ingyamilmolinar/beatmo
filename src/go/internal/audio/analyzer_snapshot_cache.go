package audio

import (
	"sync"
	"time"
)

// snapshotCacheTTL caps the per-channel analyzer snapshot refresh rate.
// WebAudio AnalyserNode updates at ~30 Hz max anyway (the audio thread
// only writes new data each ~2-3 ms block), so refreshing more often
// than this just burns Go heap + js.Value allocations for no UI benefit.
// At 30 Hz × 7 channels we cap snapshot fetches at ~210/s instead of
// ~840/s at 120 fps.
const snapshotCacheTTL = 33 * time.Millisecond

// snapshotCache stores the most recent AnalyzerSnapshot returned for
// each channel id, gated by a wall-clock TTL. Callers that re-request
// the same id within the TTL get the stored snapshot (and therefore the
// stored []float64 backing arrays) at zero JS-bridge cost and zero new
// allocations.
//
// SAFETY: the cache shares the returned []float64 slices across all
// callers within one TTL window. Consumers must not mutate the returned
// Spectrum/Waveform slices. The only caller that retains a snapshot
// beyond one Draw is the freeze-cache path (eqPanelZone.go's
// frozenAnalyzer), which captures via SynthesizeAnalyzerState's
// CaptureBuffer.Wave.Samples — that path constructs its own wave
// objects, so the cache's shared slices are not retained past one Draw
// in practice.
type snapshotCache struct {
	mu      sync.Mutex
	entries map[string]*snapshotCacheEntry
}

type snapshotCacheEntry struct {
	lastFetch time.Time
	snap      AnalyzerSnapshot
}

func newSnapshotCache() *snapshotCache {
	return &snapshotCache{entries: make(map[string]*snapshotCacheEntry)}
}

// getOrFetch returns the cached snapshot for id if it is within ttl,
// otherwise invokes fetch and caches the result.
func (c *snapshotCache) getOrFetch(id string, ttl time.Duration, fetch func() AnalyzerSnapshot) AnalyzerSnapshot {
	c.mu.Lock()
	entry, ok := c.entries[id]
	if !ok {
		entry = &snapshotCacheEntry{}
		c.entries[id] = entry
	}
	now := time.Now()
	if ok && now.Sub(entry.lastFetch) < ttl {
		snap := entry.snap
		c.mu.Unlock()
		return snap
	}
	c.mu.Unlock()

	// Fetch outside the lock — the JS bridge call can be slow and we
	// don't want to block other channels' lookups behind this one. A
	// concurrent fetch for the same id is benign: both populate entry
	// with comparable data and only the last write wins.
	snap := fetch()

	c.mu.Lock()
	entry.snap = snap
	entry.lastFetch = time.Now()
	c.mu.Unlock()
	return snap
}

// snapshotByteBufPool recycles []byte scratch buffers used as the
// destination for js.CopyBytesToGo. The bytes are reinterpreted as
// []float32 (the JS side hands us a Uint8Array view over a Float32Array
// buffer). One pool covers spectrum (~256 bytes) and wave (~2 KB)
// because the requested-vs-cached size check in getSnapshotByteBuf
// rejects too-small entries.
var snapshotByteBufPool sync.Pool

func getSnapshotByteBuf(n int) []byte {
	if n <= 0 {
		return nil
	}
	if v := snapshotByteBufPool.Get(); v != nil {
		b := v.([]byte)
		if cap(b) >= n {
			return b[:n]
		}
	}
	return make([]byte, n)
}

func putSnapshotByteBuf(b []byte) {
	if b == nil || cap(b) == 0 {
		return
	}
	//nolint:staticcheck
	snapshotByteBufPool.Put(b[:0])
}
