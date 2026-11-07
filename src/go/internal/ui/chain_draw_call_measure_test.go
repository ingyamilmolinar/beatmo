//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/scope"

	"github.com/hajimehoshi/ebiten/v2"
)

// Per-frame blits drive Ebiten's stale-image churn on the one WASM thread and starve the sequencer. Each tab
// draws twice with the same scope state, so the measured Chain frame is a cache hit, the inter-tick cost.
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

	// A flat per-frame budget for every data tab, and none an order of magnitude above its peers.
	for _, tab := range []struct {
		name string
		tab  PanelTab
	}{{"Chain", TabScope}, {"Wave", TabWave}, {"Spectrum", TabSpectrum}, {"Levels", TabMeters}} {
		if calls[tab.tab] > 1500 {
			t.Errorf("%s draw=%d exceeds budget 1500 — a per-frame trace/chrome cache is not effective",
				tab.name, calls[tab.tab])
		}
	}

	// Spectrum has no ~30 Hz data gate, so it pays in full on every frame of a 60/120 Hz display.
	if calls[TabSpectrum] > 2*calls[TabMeters] {
		t.Errorf("Spectrum draw=%d is more than 2x Levels=%d — static chrome is being repainted per frame again",
			calls[TabSpectrum], calls[TabMeters])
	}
}
