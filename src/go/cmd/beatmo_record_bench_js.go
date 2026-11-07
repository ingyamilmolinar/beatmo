//go:build js

package main

import game_log "github.com/ingyamilmolinar/beatmo/internal/log"

// No-ops: -record-bench and -hooks-config never exist in the browser, and this keeps runtime/pprof (and its template
// deps) out of main.wasm.

func startRecordBench(_ *game_log.Logger) (string, func(), error) {
	return "", func() {}, nil
}

func loadHooksConfig(_ string, _ *game_log.Logger) (func(), error) {
	return func() {}, nil
}
