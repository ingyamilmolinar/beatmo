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

// renderOrigin identifies WHICH goroutine paid for a voice render.
//
// This is the whole point of the split: recipe.Render is a ~24ms CGo call
// (organ-church), and PlayBatch (batch_desktop.go) is a plain serial loop on
// the audioLoop dispatch goroutine. A render there delays every subsequent
// beat in the batch and every beat still queued in audioCh, which is what
// makes a knob drag audible as a crack. A render on the prewarm pool costs
// the same CPU but blocks nothing.
//
// Threaded EXPLICITLY through the dispatch helpers. Do not replace this with a
// package-level flag: the prewarm pool runs 2 workers concurrently with
// dispatch, so a global would misattribute.
type renderOrigin int

const (
	// originDispatch: rendered on the audioLoop dispatch goroutine. Any
	// non-zero DispatchRenders during playback is invariant I3 violated.
	originDispatch renderOrigin = iota
	// originPrewarm: rendered off-thread (async.Pool "audio.prewarm").
	originPrewarm
)

// RenderLatencyMetrics is a snapshot of the voice-render counters.
type RenderLatencyMetrics struct {
	// CacheHits: triggers served from globalVoiceCache with no render.
	CacheHits int64
	// CacheMisses: triggers that had to render a voice (the cost a param edit
	// pays — one per distinct pitch/param-hash after an invalidation).
	// Always == DispatchRenders + PrewarmRenders.
	CacheMisses int64
	// RenderAvgNS / RenderMaxNS: cost of one cache-miss voice render (recipe
	// Render + normalize). The Max is the worst single stall on the trigger path.
	RenderAvgNS int64
	RenderMaxNS int64
	// DispatchRenders: renders that ran on the audioLoop dispatch goroutine.
	// MUST be 0 during playback (invariant I3) — each one delays the next beat.
	DispatchRenders int64
	// PrewarmRenders: renders that did NOT run on the audio dispatch path.
	// Despite the name this is broader than the "audio.prewarm" pool: it also
	// covers every other off-dispatch caller that renders a voice —
	// warmInstrumentPlatform (voice_warm_desktop.go, UI goroutine), the
	// Synth/Sampler audition entry points (Play / PlayVol in engine_play.go,
	// also UI goroutine), Sampler capture (sample_capture_notjs.go), and
	// offline song render (offline_song_render.go). All of these are harmless
	// — they don't delay a beat — so they share this one "not dispatch"
	// bucket rather than each getting a dedicated origin. Healthy; unbounded
	// is fine.
	PrewarmRenders int64
	// StalePlays: triggers served by the best-available fallback because the
	// exact binding was not rendered yet. Low is healthy; persistently high
	// means prewarm is not keeping up and edits are landing a trigger late.
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

// recordStalePlay counts one best-available fallback (the exact binding was not
// ready, so an older/nearest render played ON TIME instead of blocking).
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
		// Deliberate fail-conservative choice: an unrecognized/unset origin
		// (including the zero value, originDispatch) counts as dispatch. A
		// caller that forgets to thread origin through, or a future origin
		// added without a case here, shows up as a suspicious DispatchRenders
		// bump rather than silently vanishing into PrewarmRenders.
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
