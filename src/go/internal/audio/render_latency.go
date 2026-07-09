package audio

import (
	"sync/atomic"
	"time"
)

// render_latency.go is the render-side companion to param_latency.go. Param
// latency measures the CHEAP part of a live edit (the SetInstrumentParam map
// write + cache invalidate + hook). The EXPENSIVE part is the voice RE-RENDER
// that a cache-miss triggers on the next trigger — the ~tens-of-ms
// `recipe.Render` (a CGo render_modular_p call) that, on a param-storm, runs for
// every distinct pitch. These counters make that cost falsifiable ("measure
// first"), so an off-thread / base-pitch-resample optimization can be judged
// against numbers rather than guesses.
//
// Nanoseconds internally (callers convert to ms). Reset by ResetRenderLatencyMetrics.

// RenderLatencyMetrics is a snapshot of the voice-render counters.
type RenderLatencyMetrics struct {
	// CacheHits: triggers served from globalVoiceCache with no render.
	CacheHits int64
	// CacheMisses: triggers that had to render a voice (the cost a param edit
	// pays — one per distinct pitch/param-hash after an invalidation).
	CacheMisses int64
	// RenderAvgNS / RenderMaxNS: cost of one cache-miss voice render (recipe
	// Render + normalize). The Max is the worst single stall on the trigger path.
	RenderAvgNS int64
	RenderMaxNS int64
}

type renderLatencyCounters struct {
	hits      atomic.Int64
	misses    atomic.Int64
	renderSum atomic.Int64
	renderMax atomic.Int64
}

var renderLatency = &renderLatencyCounters{}

// recordVoiceCacheHit is called when a trigger is served from the voice cache.
func recordVoiceCacheHit() { renderLatency.hits.Add(1) }

// recordVoiceRender records one cache-miss render's duration (and bumps misses).
// Fast path: two atomic adds + one CAS loop for the max.
func recordVoiceRender(d time.Duration) {
	renderLatency.misses.Add(1)
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
		CacheHits:   renderLatency.hits.Load(),
		CacheMisses: misses,
		RenderAvgNS: avg,
		RenderMaxNS: renderLatency.renderMax.Load(),
	}
}

// ResetRenderLatencyMetrics zeroes all counters (per sampling window).
func ResetRenderLatencyMetrics() {
	renderLatency.hits.Store(0)
	renderLatency.misses.Store(0)
	renderLatency.renderSum.Store(0)
	renderLatency.renderMax.Store(0)
}
