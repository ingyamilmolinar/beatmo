//go:build js

package main

// Keeps net/http and runtime/pprof (net, crypto/tls, template/profile) out of main.wasm.

func maybeStartPprofServer() {}

func startBenchCPUProfile(_ string) (func(), error) { return func() {}, nil }
