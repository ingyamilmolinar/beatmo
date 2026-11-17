//go:build !js || !wasm

package main

import "github.com/ingyamilmolinar/beatmo/internal/async"

// wasmFriendlyRuntimeOpts on non-WASM builds returns the empty options
// (zero-valued fields = "leave defaults"). The aggressive heap cap +
// GOGC tuning is only useful on WASM where linear memory is bounded at
// 2 GB; desktop has the host OS as the only ceiling.
func wasmFriendlyRuntimeOpts() async.RuntimeOptions {
	return async.RuntimeOptions{}
}
