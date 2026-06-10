//go:build !js || !wasm || test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// testAnalyzerStateOverride lets fast Go tests inject a synthetic
// analyzer.State without standing up a live audio engine. Read by
// BuildAnalyzerStateFromSnapshots when AnalyzerService is nil. Stays
// nil in every production binary because nothing outside test code
// writes it.
var testAnalyzerStateOverride *analyzer.State

// testScopeStateOverride is the scope-side counterpart of
// testAnalyzerStateOverride. See its docstring for the rationale.
var testScopeStateOverride *scope.State

// BuildAnalyzerStateFromSnapshots is a no-op on non-WASM builds — desktop uses
// the real analyzer service instead. Returns nil so the callback's nil-fallback
// pattern keeps the desktop code path unchanged. Tests may inject a
// pre-built state via testAnalyzerStateOverride.
func BuildAnalyzerStateFromSnapshots(activeID string, rows []*DrumRow, sampleRate int) *analyzer.State {
	return testAnalyzerStateOverride
}

// BuildAnalyzerMetricsOnly is the no-op companion to
// BuildAnalyzerStateFromSnapshots on non-WASM builds. Desktop callers
// route through the real analyzer.Service which already populates the
// scalar fields; tests inject via testAnalyzerStateOverride.
func BuildAnalyzerMetricsOnly(rows []*DrumRow) *analyzer.State {
	return testAnalyzerStateOverride
}

// BuildScopeStateFromSnapshots is a no-op on non-WASM builds — desktop uses the
// real scope service instead. Tests may inject via testScopeStateOverride.
func BuildScopeStateFromSnapshots(instID string, tapA, tapB scope.Stage) *scope.State {
	return testScopeStateOverride
}
