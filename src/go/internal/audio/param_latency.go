package audio

import (
	"sync"
	"sync/atomic"
	"time"
)

// Makes "is a slider drag reaching the audio in time for the next trigger?" falsifiable. Durations in nanoseconds.
type ParamLatencyMetrics struct {
	// Only calls that mutated state: rejected and no-op writes are counted separately.
	Updates int64
	// NaN/Inf inputs: surfaces upstream bugs without polluting the dispatch average.
	Rejected int64
	// Same-value writes skipped; stationary drag bursts dominate it.
	Coalesced int64
	// One full call: clamp, write, hook publish, platform callback.
	DispatchAvgNS int64
	DispatchMaxNS int64
}

// One producer; the RWMutex keeps the snapshot from tearing.
type paramLatencyCounters struct {
	mu          sync.RWMutex
	updates     atomic.Int64
	rejected    atomic.Int64
	coalesced   atomic.Int64
	dispatchSum atomic.Int64
	dispatchMax atomic.Int64
}

var paramLatency = &paramLatencyCounters{}

// Two atomics and a CAS, so observability costs nothing while unread.
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

// Separate so the dispatch average is not biased toward the early return.
func recordParamRejected() {
	paramLatency.rejected.Add(1)
}

func recordParamCoalesced() {
	paramLatency.coalesced.Add(1)
}

// Callers divide the raw sums themselves: Updates may advance between two atomic loads.
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

// Between sampling windows (tests, the perfStats reset hook).
func ResetParamLatencyMetrics() {
	paramLatency.updates.Store(0)
	paramLatency.rejected.Store(0)
	paramLatency.coalesced.Store(0)
	paramLatency.dispatchSum.Store(0)
	paramLatency.dispatchMax.Store(0)
}
