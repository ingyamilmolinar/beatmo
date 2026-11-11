//go:build test

// Routing test for Phase 2A + 2B. Locks the Meters-tab path to the
// scalar-only AnalyzerMetricsOnly callback so a regression that
// reroutes it through the heavy AnalyzerState path immediately fails
// the per-Draw expectation.

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// TestMetersTabRoutesThroughMetricsOnly verifies EQPanelZone.Draw on
// TabMeters calls AnalyzerMetricsOnly and never AnalyzerState. This is
// the contract the long-session OOM fix depends on: routing TabMeters
// back through the heavy path would resurrect the leak.
func TestMetersTabRoutesThroughMetricsOnly(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	buildSoakScene(t, g, 3, 4)

	if g.drum.eqPanelZone == nil {
		t.Fatal("eqPanelZone nil; harness assumption broken")
	}

	heavyCalls := 0
	lightCalls := 0
	g.drum.eqPanelZone.callbacks.AnalyzerState = func() *analyzer.State {
		heavyCalls++
		return &analyzer.State{}
	}
	g.drum.eqPanelZone.callbacks.AnalyzerMetricsOnly = func() *analyzer.State {
		lightCalls++
		return &analyzer.State{}
	}

	scratch := ebiten.NewImage(1280, 720)
	g.drum.eqPanelZone.tabState.SetActiveTab(TabMeters)

	const frames = 10
	for i := 0; i < frames; i++ {
		g.drum.eqPanelZone.Draw(scratch)
	}

	if heavyCalls != 0 {
		t.Errorf("Meters tab triggered AnalyzerState %d times across %d frames "+
			"(want 0; the heavy slice-allocating path must NOT fire on Meters)",
			heavyCalls, frames)
	}
	if lightCalls != frames {
		t.Errorf("Meters tab triggered AnalyzerMetricsOnly %d times across %d frames "+
			"(want exactly %d; per-Draw memo should keep it to 1 per frame)",
			lightCalls, frames, frames)
	}
}

// TestNonMetersTabsRouteThroughAnalyzerState verifies the OTHER tabs
// (Wave, Spectrum) keep using the full AnalyzerState builder. Asserts
// the Phase 2A change didn't accidentally drop slice data needed by
// non-Meters renderers.
func TestNonMetersTabsRouteThroughAnalyzerState(t *testing.T) {
	withDefaultAudio(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("set subdivisions: %v", err)
	}
	buildSoakScene(t, g, 3, 4)

	heavyCalls := 0
	lightCalls := 0
	g.drum.eqPanelZone.callbacks.AnalyzerState = func() *analyzer.State {
		heavyCalls++
		return &analyzer.State{
			Master: analyzer.ChannelMetrics{Active: true, FFTBins: []float64{0.1, 0.2}, FreqBins: []float64{100, 200}, Waveform: []float64{0.1}},
		}
	}
	g.drum.eqPanelZone.callbacks.AnalyzerMetricsOnly = func() *analyzer.State {
		lightCalls++
		return &analyzer.State{}
	}
	g.drum.eqPanelZone.callbacks.DrawWaveform = func(dst *ebiten.Image) {}

	scratch := ebiten.NewImage(1280, 720)

	for _, tab := range []PanelTab{TabWave, TabSpectrum} {
		heavyCalls, lightCalls = 0, 0
		g.drum.eqPanelZone.tabState.SetActiveTab(tab)
		g.drum.eqPanelZone.Draw(scratch)

		if heavyCalls == 0 {
			t.Errorf("tab %v did not invoke AnalyzerState (want >= 1)", tab)
		}
		if lightCalls != 0 {
			t.Errorf("tab %v invoked AnalyzerMetricsOnly %d times (want 0; light path is Meters-only)",
				tab, lightCalls)
		}
	}
}
