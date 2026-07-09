//go:build !test && !js

// gen-modular-instruments emits src/js/modular_instruments.gen.js from the
// config-first table modularInstrumentDefs declared in internal/audio
// (modular_instruments.go). It mirrors cmd/gen-synth-abi and cmd/gen-chain-spec:
// a single Go source of truth drives a generated JS module so audio.js never
// needs a hand-maintained per-instrument entry.
//
// Browser playback resolves an instrument through audio.js's RENDER /
// RENDER_INFO tables. Every modular instrument in the table is emitted here and
// Object.assign()d into those tables, so ADDING or CLONING a modular instrument
// is a one-row table edit — no manual audio.js change, and no way to ship a
// silent instrument on WASM.
//
// Usage:
//
//	cd src/go && go run ./cmd/gen-modular-instruments > ../js/modular_instruments.gen.js
//
// A drift test (modular_instruments_gen_test.go) regenerates and diffs so
// commits cannot land with a stale gen file.
package main

import (
	"fmt"
	"os"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func main() {
	out := generate(audio.ModularInstrumentDefs())
	if _, err := os.Stdout.WriteString(out); err != nil {
		fmt.Fprintf(os.Stderr, "gen-modular-instruments: write failed: %v\n", err)
		os.Exit(1)
	}
}
