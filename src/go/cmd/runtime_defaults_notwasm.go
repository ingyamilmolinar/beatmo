//go:build !js || !wasm

package main

import "github.com/ingyamilmolinar/beatmo/internal/async"

// Empty: the heap cap and GOGC tuning matter only under WASM's bounded linear memory.
func wasmFriendlyRuntimeOpts() async.RuntimeOptions {
	return async.RuntimeOptions{}
}
