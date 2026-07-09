//go:build js

package main

// Browser build: profiling is a no-op. Keeping net/http + runtime/pprof out of
// the js/wasm import graph strips net + crypto/tls + pprof's template/profile
// machinery (~4 MiB raw) from main.wasm. See profiling.go for the desktop impls.

func maybeStartPprofServer() {}

func startBenchCPUProfile(_ string) (func(), error) { return func() {}, nil }
