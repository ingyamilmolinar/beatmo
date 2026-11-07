//go:build !js || !wasm || test

package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	scope "github.com/ingyamilmolinar/beatmo/internal/scope"
)

// Lets fast tests inject a state without an audio engine; nothing outside tests writes it.
var testAnalyzerStateOverride *analyzer.State

var testScopeStateOverride *scope.State

// nil on desktop, which reads the real analyzer service; tests inject through testAnalyzerStateOverride.
func BuildAnalyzerStateFromSnapshots(activeID channel.ID, rows []*DrumRow, sampleRate int) *analyzer.State {
	return testAnalyzerStateOverride
}

// Desktop's analyzer.Service already fills the scalars; tests inject through testAnalyzerStateOverride.
func BuildAnalyzerMetricsOnly(rows []*DrumRow) *analyzer.State {
	return testAnalyzerStateOverride
}

// nil on desktop, which reads the real scope service.
func BuildScopeStateFromSnapshots(instID channel.ID, tapA, tapB scope.Stage) *scope.State {
	return testScopeStateOverride
}

// The screenshot binary never starts the scope service, so without this every Chain capture shows an empty scope.
func SetSceneScopeState(s *scope.State) { testScopeStateOverride = s }
