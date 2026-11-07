package audio

import "sync"

// Transient float64 conversion buffers on the per-trigger dispatch path: Schedule's float64 copy for the scope and
// export pushes is unreachable once it returns. A buffer smaller than asked for is dropped and a fresh one allocated:
// requests are near-uniform in size, so no size classes are needed.
var f64BufPool sync.Pool

// Contents are NOT zeroed; callers overwrite every element they read.
func getF64Buf(n int) []float64 {
	if n <= 0 {
		return nil
	}
	if v := f64BufPool.Get(); v != nil {
		b := v.([]float64)
		if cap(b) >= n {
			return b[:n]
		}
	}
	return make([]float64, n)
}

// Callers must not retain b's backing array after this.
func putF64Buf(b []float64) {
	if b == nil || cap(b) == 0 {
		return
	}
	//nolint:staticcheck // sync.Pool stores interface{}; slice header copy is fine
	f64BufPool.Put(b[:0])
}
