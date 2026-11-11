package audio

import "sync"

// f64BufPool recycles transient []float64 conversion buffers used in the
// per-trigger audio dispatch path. Two sites consume from this pool:
//
//  1. engine_mixer.Schedule converts the freshly-rendered float32 voice
//     buffer to float64 once for scope.PushSamples / export.PushSamples.
//     The slice is unreachable as soon as Schedule returns (scope.ringBuf
//     copies the contents via append), so the pool sees turnover at the
//     trigger rate.
//  2. internal/scope.ringBuf.drain returns transient slices the same way
//     (slices are stored in scope.State which is replaced on each tick),
//     but those are sized larger and use their own pool wired in
//     scope.service.
//
// Returning a buffer with capacity < the requested size is permitted —
// callers will allocate a fresh slice in that case. The pool only
// amortizes the steady-state alloc rate for similarly-sized requests,
// which matches the synth-tab churn scenario where every trigger renders
// the same per-instrument duration.
var f64BufPool sync.Pool

// getF64Buf returns a slice of length n. Reuses a pooled buffer when one
// of sufficient capacity is available; otherwise allocates. The returned
// slice's contents are NOT zeroed; callers must overwrite every element
// they read.
func getF64Buf(n int) []float64 {
	if n <= 0 {
		return nil
	}
	if v := f64BufPool.Get(); v != nil {
		b := v.([]float64)
		if cap(b) >= n {
			return b[:n]
		}
		// Capacity insufficient — drop on the floor; GC reclaims it. A
		// size-class bucket scheme is unnecessary because every site that
		// borrows from this pool uses near-uniform sizes (per-instrument
		// rendered buffers under one BPM).
	}
	return make([]float64, n)
}

// putF64Buf returns b to the pool. Callers must not retain references to
// b's backing array after this call.
func putF64Buf(b []float64) {
	if b == nil || cap(b) == 0 {
		return
	}
	//nolint:staticcheck // sync.Pool stores interface{}; slice header copy is fine
	f64BufPool.Put(b[:0])
}
