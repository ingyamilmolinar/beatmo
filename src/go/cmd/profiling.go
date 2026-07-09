//go:build !js

package main

import (
	"net/http"
	_ "net/http/pprof" // registers /debug/pprof handlers on the default mux
	"os"
	"runtime/pprof"
)

// maybeStartPprofServer starts the net/http pprof server on localhost:6060 when
// PPROF=1 is set. Desktop-only: the browser build gets a no-op (profiling_js.go)
// so net/http + crypto/tls never enter the WASM binary.
func maybeStartPprofServer() {
	if os.Getenv("PPROF") != "1" {
		return
	}
	go func() {
		_ = http.ListenAndServe("localhost:6060", nil)
	}()
}

// startBenchCPUProfile begins a CPU profile written to path and returns a stop
// function that flushes and closes it. Used only by desktop benchmark mode.
func startBenchCPUProfile(path string) (func(), error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return nil, err
	}
	return func() {
		pprof.StopCPUProfile()
		f.Close()
	}, nil
}
