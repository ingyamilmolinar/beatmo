//go:build js && wasm

package main

import "github.com/ingyamilmolinar/beatmo/internal/async"

// The tab's linear memory caps out near 2 GB, and an unbounded Go heap OOMs the runtime in a long session. A 1500 MB
// soft limit leaves headroom for short spikes; GOGC 50 reclaims sooner and lowers the high-water mark and the longest
// pause of the single-threaded GC. BEATMO_MEMORY_LIMIT_MB / BEATMO_GC_PERCENT override.
func wasmFriendlyRuntimeOpts() async.RuntimeOptions {
	return async.RuntimeOptions{
		MemoryLimitMB: 1500,
		GCPercent:     50,
	}
}
