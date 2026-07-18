package ui

import (
	"testing"

	assets_pkg "github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// solo_analyzer_stack_test.go — regression guard for "soloing the organ makes
// audio thrash".
//
// Soloing STRICTLY REMOVES work from the scheduler (game_sequencer_schedule.go
// gates muted/solo'd-out rows before evalNodeParamsOnly/scheduleSound), so no
// solo toggle may ever ADD steady-state cost to the audio thread.
//
// It used to. toggleSolo → reconcileChannelWithAudibleSet → selectAudioChannel
// → setEQActiveChannel unconditionally re-issues EnableChannelAnalyzer, and the
// desktop/stub EnableChannelAnalyzer APPENDED a brand-new Analyzer to the
// channel's eqProcs every call. Each stacked analyzer runs a 512-point FFT on
// the audio rendering thread forever and is un-disableable (SetAnalyzerEnabled
// only reaches the registry-current instance), so the organ — the sole audible
// and longest-sustaining channel once soloed — accumulated FFT load with every
// toggle. The JS path already guards against this (see the "Idempotent" comment
// in src/js/audio.go's enableChannelAnalyzer); Go never got the same fix.

// organRowInDemo returns the index of the organ-church row in the startup demo.
func organRowInDemo(t *testing.T, g *Game) int {
	t.Helper()
	for i, r := range g.drum.Rows {
		if r.Instrument == "organ-church" {
			return i
		}
	}
	t.Fatalf("startup demo has no organ-church row; rows=%d", len(g.drum.Rows))
	return -1
}

func loadDemoGame(t *testing.T) *Game {
	t.Helper()
	g := buildTestGame(t)
	if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
		t.Fatalf("import startup demo: %v", err)
	}
	g.updateBeatInfos()
	return g
}

// TestSoloOrganDoesNotStackChannelAnalyzers pins the invariant: repeatedly
// soloing/unsoloing the organ must not grow the organ channel's analyzer count.
func TestSoloOrganDoesNotStackChannelAnalyzers(t *testing.T) {
	g := loadDemoGame(t)
	organ := organRowInDemo(t, g)

	base := audio.ChannelAnalyzerCount("organ-church")
	t.Logf("organ-church analyzers before any solo: %d", base)

	for i := 1; i <= 5; i++ {
		setRowSolo(t, g.drum, organ, true)
		setRowSolo(t, g.drum, organ, false)
		t.Logf("after solo/unsolo cycle %d: organ-church analyzers=%d", i, audio.ChannelAnalyzerCount("organ-church"))
	}

	got := audio.ChannelAnalyzerCount("organ-church")
	if got > 1 {
		t.Errorf("solo stacked analyzers on the organ channel: count=%d after 5 solo/unsolo cycles (want <=1).\n"+
			"Each one runs a 512-pt FFT on the audio thread every 512 samples and can never be disabled.", got)
	}
}

// TestSelectAudioChannelIsAnalyzerIdempotent isolates the chokepoint: selecting
// the same channel repeatedly (which reconcileChannelWithAudibleSet does on
// every mute/solo toggle) must not stack analyzers.
func TestSelectAudioChannelIsAnalyzerIdempotent(t *testing.T) {
	g := loadDemoGame(t)

	for i := 0; i < 8; i++ {
		g.drum.selectAudioChannel("organ-church")
	}
	if got := audio.ChannelAnalyzerCount("organ-church"); got > 1 {
		t.Errorf("8 selects of the same channel left %d live analyzers (want <=1)", got)
	}
}
