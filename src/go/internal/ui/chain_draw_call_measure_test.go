//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
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
			rowSnaps = append(rowSnaps, RowSnapshot{ID: channel.Of(r.Instrument), Name: r.Name, Snap: snap})
		}
	}
	testAnalyzerStateOverride = SynthesizeAnalyzerState(channel.Master, "Master", snap, rowSnaps, nil, channel.ID{}, "", audio.SampleRate())
	testScopeStateOverride = SynthesizeScopeState(channel.Master, scope.StageSynth, scope.StageEQ,
		ScopeStageSnapshots{Synth: snap, PreEQ: snap, PostEQ: snap, Master: snap})
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

	// Every data tab must now stay under a flat per-frame blit budget. Each
	// got there a different way: Chain and Wave cache their per-pixel-column
	// trace (once ~5500 and ~3100 blits EVERY frame), and Spectrum caches its
	// static dB-grid chrome (~832 of its 1032 blits were one drawRect per
	// dash of 4 dashed gridlines, repainted every frame).
	//
	// This used to assert "a cached tab must be cheaper than the UNCACHED
	// Spectrum tab". That ceiling is gone — Spectrum is cached too and is now
	// one of the cheapest — so the guard is a flat budget plus a rule that no
	// tab may quietly become an order of magnitude worse than its peers.
	for _, tab := range []struct {
		name string
		tab  PanelTab
	}{{"Chain", TabScope}, {"Wave", TabWave}, {"Spectrum", TabSpectrum}, {"Levels", TabMeters}} {
		if calls[tab.tab] > 1500 {
			t.Errorf("%s draw=%d exceeds budget 1500 — a per-frame trace/chrome cache is not effective",
				tab.name, calls[tab.tab])
		}
	}

	// Spectrum specifically: its cost is framerate-INVARIANT (no ~30 Hz data
	// gate like the Wave/Chain traces), so on a 60/120 Hz display it pays in
	// full on every frame. It was the single worst tab at 1032 blits and the
	// reported cause of audio lag on the single WASM thread. Keep it in the
	// same league as the other data tabs rather than 5x above them.
	if calls[TabSpectrum] > 2*calls[TabMeters] {
		t.Errorf("Spectrum draw=%d is more than 2x Levels=%d — static chrome is being repainted per frame again",
			calls[TabSpectrum], calls[TabMeters])
	}
}
