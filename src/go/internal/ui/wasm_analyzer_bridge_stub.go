//go:build !js || !wasm || test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
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
func BuildAnalyzerStateFromSnapshots(activeID channel.ID, rows []*DrumRow, sampleRate int) *analyzer.State {
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
func BuildScopeStateFromSnapshots(instID channel.ID, tapA, tapB scope.Stage) *scope.State {
	return testScopeStateOverride
}

// SetSceneScopeState lets a screenshot scene put a synthetic trace on the
// Chain tab. Desktop never stands up the scope service in the screenshot
// binary (ScopeService() is nil), so without this every Chain capture shows
// an empty scope — which is exactly how the trigger marker came to be the
// only mark in the viewport and got mistaken for a stray object.
func SetSceneScopeState(s *scope.State) { testScopeStateOverride = s }
