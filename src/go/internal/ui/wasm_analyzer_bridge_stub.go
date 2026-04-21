//go:build !js || !wasm || test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// BuildAnalyzerStateFromSnapshots is a no-op on non-WASM builds — desktop uses
// the real analyzer service instead. Returns nil so the callback's nil-fallback
// pattern keeps the desktop code path unchanged.
func BuildAnalyzerStateFromSnapshots(activeID string, rows []*DrumRow, sampleRate int) *analyzer.State {
	return nil
}

// BuildScopeStateFromSnapshots is a no-op on non-WASM builds — desktop uses the
// real scope service instead.
func BuildScopeStateFromSnapshots(instID string, tapA, tapB scope.Stage) *scope.State {
	return nil
}
