//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/scope"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestPerTabDrawCallBudget guards the audio-panel per-frame blit count, the
// quantity that drives Ebiten's makeStaleIfDependingOn / mapiternext churn on
// the single WASM thread and — when it gets large — starves the sequencer
// goroutine into choppy audio. It draws each tab twice with the SAME scope
// state, so the second (measured) Chain draw is a cache HIT — the cost of an
// inter-tick frame, which on a 60 Hz+ display is roughly half of all frames
// (the scope data refreshes at ~30 Hz, so Draw outpaces it).
//
// The Chain (Scope) tab once issued ~5500 blits on EVERY frame (two full-width
// traces at ~2 drawRect per pixel column), ~1.8x the next tab and the worst by
// far. The trace cache (chain_trace_cache.go) collapses an unchanged-data frame
// to a single blit, so a cache-hit Chain frame must now be the CHEAPEST data
// tab, not the most expensive. A regression that re-rendered the trace on a
// hit, or added comparable per-frame draw to another tab, trips this budget.
// (Cache-MISS frames still pay the full render — that is the inherent ~30 Hz
// trace-redraw cost, unchanged by the cache.)
func TestPerTabDrawCallBudget(t *testing.T) {
	g := driveScene(t, "crop_chain_default")
	snap := snapshotWithSine()

	// Feed every audio-data path so each tab renders real data.
	post := snap
	pre := snap
	g.drum.eqTestSnapshot = &post
	g.drum.eqTestPreEQSnapshot = &pre
	rowSnaps := make([]RowSnapshot, 0, len(g.drum.Rows))
	for _, r := range g.drum.Rows {
		if r != nil && r.Instrument != "" {
			rowSnaps = append(rowSnaps, RowSnapshot{ID: r.Instrument, Name: r.Name, Snap: snap})
		}
	}
	testAnalyzerStateOverride = SynthesizeAnalyzerState("main", "Master", snap, rowSnaps, nil, "", "", audio.SampleRate())
	testScopeStateOverride = SynthesizeScopeState("main", scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Sends: snap, Master: snap})
	t.Cleanup(func() {
		testAnalyzerStateOverride = nil
		testScopeStateOverride = nil
	})

	screen := ebiten.NewImage(1280, 720)
	tabs := []struct {
		name string
		tab  PanelTab
	}{
		{"Wave", TabWave},
		{"Spectrum", TabSpectrum},
		{"Levels", TabMeters},
		{"Chain", TabScope},
	}
	calls := map[PanelTab]int64{}
	for _, tc := range tabs {
		g.drum.eqPanelZone.SetActiveTab(tc.tab)
		g.Draw(screen) // warm: populate the cache (unchanged-data → cache hit)
		ResetImageMetrics()
		g.Draw(screen)
		n := MetricDrawCallsTotal()
		calls[tc.tab] = n
		t.Logf("tab=%-9s cache-hit drawCalls=%d", tc.name, n)
	}

	// Both the Chain (Scope) and Wave tabs cache their per-pixel-column trace,
	// so an unchanged-data frame collapses to a single blit + chrome. Each
	// once cost thousands of blits EVERY frame (Chain ~5500, Wave ~3100); on a
	// cache hit each must now stay well under that. 1500 leaves generous room
	// for the per-frame chrome and is far below the uncached cost.
	for _, tab := range []struct {
		name string
		tab  PanelTab
	}{{"Chain", TabScope}, {"Wave", TabWave}} {
		if calls[tab.tab] > 1500 {
			t.Errorf("%s cache-hit draw=%d exceeds budget 1500 — trace cache not effective", tab.name, calls[tab.tab])
		}
		// And a cached hit must be cheaper than the uncached Spectrum tab,
		// which renders ~10 bars + a hi-res FFT underlay every frame.
		if calls[tab.tab] >= calls[TabSpectrum] {
			t.Errorf("%s cache-hit draw (%d) should be cheaper than uncached Spectrum (%d) — cache regressed",
				tab.name, calls[tab.tab], calls[TabSpectrum])
		}
	}
}
