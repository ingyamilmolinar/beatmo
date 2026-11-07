package audio

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
)

// The AnalyserNode updates at about 30 Hz, so fetching more often only burns Go heap and js.Value allocations.
const snapshotCacheTTL = 33 * time.Millisecond

// Callers within the TTL share the same backing slices: they must not mutate Spectrum/Waveform, and none
// retains them past one Draw.
type snapshotCache struct {
	mu      sync.Mutex
	entries map[channel.ID]*snapshotCacheEntry
}

type snapshotCacheEntry struct {
	lastFetch time.Time
	snap      AnalyzerSnapshot
}

func newSnapshotCache() *snapshotCache {
	return &snapshotCache{entries: make(map[channel.ID]*snapshotCacheEntry)}
}

func (c *snapshotCache) getOrFetch(id channel.ID, ttl time.Duration, fetch func() AnalyzerSnapshot) AnalyzerSnapshot {
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

	// Outside the lock, since the bridge call can be slow; a concurrent fetch of the same id is benign, last write wins.
	snap := fetch()

	c.mu.Lock()
	entry.snap = snap
	entry.lastFetch = time.Now()
	c.mu.Unlock()
	return snap
}

// Used on circuit load, so no stale within-TTL frame is returned.
func (c *snapshotCache) clear() {
	c.mu.Lock()
	c.entries = make(map[channel.ID]*snapshotCacheEntry)
	c.mu.Unlock()
}

// Destinations for js.CopyBytesToGo, reinterpreted as []float32; one pool serves spectrum and wave because
// getSnapshotByteBuf rejects entries that are too small.
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
