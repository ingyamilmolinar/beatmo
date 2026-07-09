//go:build js

package main

import game_log "github.com/ingyamilmolinar/beatmo/internal/log"

// Browser build stubs. Both entry points are reachable only via desktop CLI
// flags (-record-bench, -hooks-config) that never exist in the browser, so the
// js build gets no-ops. This keeps runtime/pprof (and its internal/profile +
// template deps) out of main.wasm. See beatmo_record_bench.go for the impls.

func startRecordBench(_ *game_log.Logger) (string, func(), error) {
	return "", func() {}, nil
}

func loadHooksConfig(_ string, _ *game_log.Logger) (func(), error) {
	return func() {}, nil
}
