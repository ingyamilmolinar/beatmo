package audio

import (
	"sync"
	"sync/atomic"
	"time"
)

// ParamLatencyMetrics is the Phase-9 observability surface that makes the
// "is this real-time?" question falsifiable. Without these counters there
// is no way to tell whether a slider drag is reaching the audio layer in
// time for the next trigger.
//
// All durations are nanoseconds (callers convert to ms for display).
// All counters reset to zero on ResetParamLatencyMetrics().
type ParamLatencyMetrics struct {
	// Updates is the total count of accepted SetInstrumentParam calls since
	// the last reset that produced real state mutation. Non-finite/rejected
	// calls (Phase-4 boundary) and no-op writes (Phase-8 coalesce) are NOT
	// counted because they did not invalidate the cache or notify observers.
	Updates int64
	// Rejected counts SetInstrumentParam calls where sanitizeParamValue
	// dropped the input (NaN/Inf). Tracking these separately surfaces
	// upstream bugs (e.g. JSON import sending garbage) without polluting
	// the dispatch latency average.
	Rejected int64
	// Coalesced counts SetInstrumentParam calls that were skipped because
	// the post-sanitize value equalled the currently stored value. The
	// pre-Phase-8 path still invalidated the voice cache + published a hook
	// + crossed the WASM bridge for these zero-information writes. Stationary
	// drag bursts dominate this counter.
	Coalesced int64
	// DispatchAvgNS / DispatchMaxNS measure the cost of one full SetParam
	// call: clamp + map write + cache invalidate + hook publish + platform
	// callback. Hot for the user during a knob drag at 60 Hz.
	DispatchAvgNS int64
	DispatchMaxNS int64
}

// paramLatencyCounters accumulates SetInstrumentParam timing. Designed for
// a single producer (the goroutine driving the slider) but reads under
// RWMutex so the snapshot reader doesn't tear durations.
type paramLatencyCounters struct {
	mu          sync.RWMutex
	updates     atomic.Int64
	rejected    atomic.Int64
	coalesced   atomic.Int64
	dispatchSum atomic.Int64
	dispatchMax atomic.Int64
}

var paramLatency = &paramLatencyCounters{}

// recordParamDispatch records the duration of one accepted SetInstrumentParam
// call. Called from inside SetInstrumentParam after the dispatch completes.
// The fast path is two atomic ops + one CAS so we don't pay for the
// observability when nothing's reading it.
func recordParamDispatch(d time.Duration) {
	paramLatency.updates.Add(1)
	ns := int64(d)
	paramLatency.dispatchSum.Add(ns)
	for {
		prev := paramLatency.dispatchMax.Load()
		if ns <= prev {
			return
		}
		if paramLatency.dispatchMax.CompareAndSwap(prev, ns) {
			return
		}
	}
}

// recordParamRejected bumps the rejected counter when sanitizeParamValue
// drops a non-finite input. Separate from Updates so the dispatch average
// isn't biased toward the early-return path.
func recordParamRejected() {
	paramLatency.rejected.Add(1)
}

// recordParamCoalesced bumps the no-op counter when SetInstrumentParam is
// called with the same value already stored. Cheaper than the full dispatch
// path because we skip cache invalidate + hook + platform callback.
func recordParamCoalesced() {
	paramLatency.coalesced.Add(1)
}

// GetParamLatencyMetrics returns a snapshot of the current counters. Cheap
// (atomic loads only). Callers compute averages from Updates and
// DispatchAvgNS by reading the raw sum and dividing — they MUST NOT divide
// before calling because Updates may advance between two atomic loads.
func GetParamLatencyMetrics() ParamLatencyMetrics {
	updates := paramLatency.updates.Load()
	sum := paramLatency.dispatchSum.Load()
	var avg int64
	if updates > 0 {
		avg = sum / updates
	}
	return ParamLatencyMetrics{
		Updates:       updates,
		Rejected:      paramLatency.rejected.Load(),
		Coalesced:     paramLatency.coalesced.Load(),
		DispatchAvgNS: avg,
		DispatchMaxNS: paramLatency.dispatchMax.Load(),
	}
}

// ResetParamLatencyMetrics zeroes all counters. Tests + the perfStats()
// reset hook on the UI side call this between sampling windows.
func ResetParamLatencyMetrics() {
	paramLatency.updates.Store(0)
	paramLatency.rejected.Store(0)
	paramLatency.coalesced.Store(0)
	paramLatency.dispatchSum.Store(0)
	paramLatency.dispatchMax.Store(0)
}
