//go:build js && wasm

package main

import "github.com/ingyamilmolinar/beatmo/internal/async"

// wasmFriendlyRuntimeOpts returns the runtime tuning we want as defaults
// on WASM. The browser tab is capped at ~2 GB of WebAssembly linear
// memory; an unbounded Go heap will OOM the runtime long before the user
// stops a long session. The defaults below trip GC earlier and bound the
// soft heap cap so the runtime sweeps proactively. Both knobs can still
// be overridden via BEATMO_MEMORY_LIMIT_MB / BEATMO_GC_PERCENT envs —
// the env paths take precedence inside async.ConfigureRuntime when set.
//
// Rationale for the chosen numbers (tuned against the synth-tab profile
// scenario at 60 Hz churn; see bench-results/synth-browser-*):
//
//   - MemoryLimit 1500 MB: leaves ~500 MB of headroom under the 2 GB
//     ceiling for short-lived spikes (vector tessellation, scope drain,
//     etc.) without ever brushing the OOM line.
//   - GCPercent 50: GC triggers when heap doubles by 50% (vs the 100%
//     default). Two-thirds the per-cycle work, ~50% more cycles, but
//     each cycle reclaims sooner and the heap high-water mark stays
//     correspondingly lower. The WASM GC is single-threaded so faster
//     cycles also reduce the longest pause.
func wasmFriendlyRuntimeOpts() async.RuntimeOptions {
	return async.RuntimeOptions{
		MemoryLimitMB: 1500,
		GCPercent:     50,
	}
}
