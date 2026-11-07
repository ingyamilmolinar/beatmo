package ui

import (
	"sync"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// The AnalyserNodes update at ~30 Hz, so rebuilding faster only churns the allocator. Matches the snapshot
// cache's TTL so both layers refresh together.
const stateCacheTTL = 33 * time.Millisecond

// No build tag, so tests exercise the cache without WASM; the WASM bridge injects the fetch function.

type analyzerStateCacheEntry struct {
	mu         sync.Mutex
	lastBuild  time.Time
	state      *analyzer.State
	activeID   channel.ID
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
	instID    channel.ID
	tapA      scope.Stage
	tapB      scope.Stage
}

var (
	analyzerStateCache   = &analyzerStateCacheEntry{}
	analyzerMetricsCache = &analyzerMetricsCacheEntry{}
	scopeStateCache      = &scopeStateCacheEntry{}
)

// A rename or instrument swap must force a fresh build even within the TTL.
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

// The same signature on every build, so tests and the WASM bridge share one path.
func cachedAnalyzerState(activeID channel.ID, rowsFP uint64, sampleRate int, build func() *analyzer.State) *analyzer.State {
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
func cachedScopeState(instID channel.ID, tapA, tapB scope.Stage, build func() *scope.State) *scope.State {
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

// Tests that change state in quick succession call this to avoid stale-cache flakes.
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
