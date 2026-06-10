package ui

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// stateCacheTTL caps the rebuild rate for the synthesized analyzer.State /
// scope.State. The Draw loop runs at 60-120 fps but the underlying
// AnalyserNodes update at ~30 Hz, so rebuilding the State more often than
// this just churns allocator pressure for no visible difference. Matches
// the per-snapshot cache TTL in internal/audio/analyzer_snapshot_cache.go
// so both layers refresh on the same wall-clock heartbeat.
const stateCacheTTL = 33 * time.Millisecond

// Cache types are in a no-build-tag file so tests can exercise them
// without spinning up a WASM environment. The cache callers
// (BuildAnalyzerStateFromSnapshots etc.) live in the WASM-only file
// wasm_analyzer_bridge.go and inject the fetch function.

type analyzerStateCacheEntry struct {
	mu         sync.Mutex
	lastBuild  time.Time
	state      *analyzer.State
	activeID   string
	rowsFP     uint64
	sampleRate int
}

type analyzerMetricsCacheEntry struct {
	mu        sync.Mutex
	lastBuild time.Time
	state     *analyzer.State
	rowsFP    uint64
}

type scopeStateCacheEntry struct {
	mu        sync.Mutex
	lastBuild time.Time
	state     *scope.State
	instID    string
	tapA      scope.Stage
	tapB      scope.Stage
}

var (
	analyzerStateCache   = &analyzerStateCacheEntry{}
	analyzerMetricsCache = &analyzerMetricsCacheEntry{}
	scopeStateCache      = &scopeStateCacheEntry{}
)

// rowsFingerprint produces a cheap uint64 hash over the (id, name)
// pairs of the row slice. Used as a cache key so a row rename or
// instrument swap forces a fresh State build even within the TTL
// window. Skips nil/empty rows the way the builder loop does.
func rowsFingerprint(rows []*DrumRow) uint64 {
	var h uint64 = 1469598103934665603 // FNV-1a 64 offset basis
	const prime uint64 = 1099511628211
	for _, r := range rows {
		if r == nil || r.Instrument == "" {
			continue
		}
		for i := 0; i < len(r.Instrument); i++ {
			h ^= uint64(r.Instrument[i])
			h *= prime
		}
		h ^= 0xff
		h *= prime
		for i := 0; i < len(r.Name); i++ {
			h ^= uint64(r.Name[i])
			h *= prime
		}
		h ^= 0xff
		h *= prime
	}
	return h
}

// cachedAnalyzerState returns the cached *analyzer.State for the given
// (activeID, rowsFP, sampleRate) key if it is within stateCacheTTL,
// otherwise calls build and caches the result. Pure Go; no platform
// dependencies — same signature on every build so tests + the WASM
// bridge share the same code path.
func cachedAnalyzerState(activeID string, rowsFP uint64, sampleRate int, build func() *analyzer.State) *analyzer.State {
	c := analyzerStateCache
	c.mu.Lock()
	if c.state != nil && time.Since(c.lastBuild) < stateCacheTTL &&
		c.activeID == activeID && c.rowsFP == rowsFP && c.sampleRate == sampleRate {
		st := c.state
		c.mu.Unlock()
		return st
	}
	c.mu.Unlock()
	st := build()
	c.mu.Lock()
	c.state = st
	c.lastBuild = time.Now()
	c.activeID = activeID
	c.rowsFP = rowsFP
	c.sampleRate = sampleRate
	c.mu.Unlock()
	return st
}

// cachedAnalyzerMetrics is the Meters-tab counterpart.
func cachedAnalyzerMetrics(rowsFP uint64, build func() *analyzer.State) *analyzer.State {
	c := analyzerMetricsCache
	c.mu.Lock()
	if c.state != nil && time.Since(c.lastBuild) < stateCacheTTL && c.rowsFP == rowsFP {
		st := c.state
		c.mu.Unlock()
		return st
	}
	c.mu.Unlock()
	st := build()
	c.mu.Lock()
	c.state = st
	c.lastBuild = time.Now()
	c.rowsFP = rowsFP
	c.mu.Unlock()
	return st
}

// cachedScopeState is the scope counterpart.
func cachedScopeState(instID string, tapA, tapB scope.Stage, build func() *scope.State) *scope.State {
	c := scopeStateCache
	c.mu.Lock()
	if c.state != nil && time.Since(c.lastBuild) < stateCacheTTL &&
		c.instID == instID && c.tapA == tapA && c.tapB == tapB {
		st := c.state
		c.mu.Unlock()
		return st
	}
	c.mu.Unlock()
	st := build()
	c.mu.Lock()
	c.state = st
	c.lastBuild = time.Now()
	c.instID = instID
	c.tapA = tapA
	c.tapB = tapB
	c.mu.Unlock()
	return st
}

// InvalidateAnalyzerStateCacheForTest clears the cached analyzer states.
// Tests that toggle freeze / change rows / switch tabs in quick
// succession must call this to avoid stale-cache flakes.
func InvalidateAnalyzerStateCacheForTest() {
	analyzerStateCache.mu.Lock()
	analyzerStateCache.state = nil
	analyzerStateCache.lastBuild = time.Time{}
	analyzerStateCache.mu.Unlock()
	analyzerMetricsCache.mu.Lock()
	analyzerMetricsCache.state = nil
	analyzerMetricsCache.lastBuild = time.Time{}
	analyzerMetricsCache.mu.Unlock()
	scopeStateCache.mu.Lock()
	scopeStateCache.state = nil
	scopeStateCache.lastBuild = time.Time{}
	scopeStateCache.mu.Unlock()
}
