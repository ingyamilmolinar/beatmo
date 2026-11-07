package audio

import (
	"sync/atomic"
	"time"
)

// The EXPENSIVE half of a live edit: the voice re-render a cache miss triggers, per distinct pitch. Counted so an
// optimisation is judged against numbers. Nanoseconds.

// A render on the audioLoop dispatch goroutine delays every later beat in the batch (an audible crack); on the prewarm
// pool it costs the same CPU and blocks nothing. Threaded EXPLICITLY: a package flag would misattribute while the pool
// runs concurrently.
type renderOrigin int

const (
	// Any during playback violates invariant I3.
	originDispatch renderOrigin = iota
	// originPrewarm: rendered off-thread (async.Pool "audio.prewarm").
	originPrewarm
)

// RenderLatencyMetrics is a snapshot of the voice-render counters.
type RenderLatencyMetrics struct {
	// CacheHits: triggers served from globalVoiceCache with no render.
	CacheHits int64
	// Always DispatchRenders + PrewarmRenders.
	CacheMisses int64
	// The Max is the worst single stall on the trigger path.
	RenderAvgNS int64
	RenderMaxNS int64
	// MUST be 0 during playback: each one delays the next beat.
	DispatchRenders int64
	// Every render off the dispatch path (prewarm, auditions, Sampler capture, offline renders): harmless, since none
	// delays a beat.
	PrewarmRenders int64
	// Persistently high means prewarm is not keeping up and edits land a trigger late.
	StalePlays int64
}

type renderLatencyCounters struct {
	hits            atomic.Int64
	misses          atomic.Int64
	renderSum       atomic.Int64
	renderMax       atomic.Int64
	dispatchRenders atomic.Int64
	prewarmRenders  atomic.Int64
	stalePlays      atomic.Int64
}

var renderLatency = &renderLatencyCounters{}

// recordVoiceCacheHit is called when a trigger is served from the voice cache.
func recordVoiceCacheHit() { renderLatency.hits.Add(1) }

// An older or nearest render played ON TIME instead of blocking.
func recordStalePlay() { renderLatency.stalePlays.Add(1) }

// recordVoiceRender records one cache-miss render's duration (and bumps misses),
// attributed to the goroutine class that paid for it.
// Fast path: three atomic adds + one CAS loop for the max.
func recordVoiceRender(d time.Duration, origin renderOrigin) {
	renderLatency.misses.Add(1)
	switch origin {
	case originPrewarm:
		renderLatency.prewarmRenders.Add(1)
	default:
		// Fail-conservative: an unset or unknown origin counts as dispatch, so a caller that forgets to thread it shows up as a
		// suspicious bump instead of vanishing into PrewarmRenders.
		renderLatency.dispatchRenders.Add(1)
	}
	ns := int64(d)
	renderLatency.renderSum.Add(ns)
	for {
		prev := renderLatency.renderMax.Load()
		if ns <= prev {
			return
		}
		if renderLatency.renderMax.CompareAndSwap(prev, ns) {
			return
		}
	}
}

// GetRenderLatencyMetrics returns a snapshot (atomic loads only).
func GetRenderLatencyMetrics() RenderLatencyMetrics {
	misses := renderLatency.misses.Load()
	sum := renderLatency.renderSum.Load()
	var avg int64
	if misses > 0 {
		avg = sum / misses
	}
	return RenderLatencyMetrics{
		CacheHits:       renderLatency.hits.Load(),
		CacheMisses:     misses,
		RenderAvgNS:     avg,
		RenderMaxNS:     renderLatency.renderMax.Load(),
		DispatchRenders: renderLatency.dispatchRenders.Load(),
		PrewarmRenders:  renderLatency.prewarmRenders.Load(),
		StalePlays:      renderLatency.stalePlays.Load(),
	}
}

// ResetRenderLatencyMetrics zeroes all counters (per sampling window).
func ResetRenderLatencyMetrics() {
	renderLatency.hits.Store(0)
	renderLatency.misses.Store(0)
	renderLatency.renderSum.Store(0)
	renderLatency.renderMax.Store(0)
	renderLatency.dispatchRenders.Store(0)
	renderLatency.prewarmRenders.Store(0)
	renderLatency.stalePlays.Store(0)
}
