package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
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

// TestSoloOrganDoesNotStackChannelAnalyzers pins the invariant: repeatedly
// soloing/unsoloing the organ must not grow the organ channel's analyzer count.
func TestSoloOrganDoesNotStackChannelAnalyzers(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 5, -2, 7)
	organ := melodicRow(t, g)

	base := audio.ChannelAnalyzerCount(channel.Of(testMelodicInst))
	t.Logf("%s analyzers before any solo: %d", testMelodicInst, base)

	for i := 1; i <= 5; i++ {
		setRowSolo(t, g.drum, organ, true)
		setRowSolo(t, g.drum, organ, false)
		t.Logf("after solo/unsolo cycle %d: %s analyzers=%d", i, testMelodicInst, audio.ChannelAnalyzerCount(channel.Of(testMelodicInst)))
	}

	got := audio.ChannelAnalyzerCount(channel.Of(testMelodicInst))
	if got > 1 {
		t.Errorf("solo stacked analyzers on the melodic channel: count=%d after 5 solo/unsolo cycles (want <=1).\n"+
			"Each one runs a 512-pt FFT on the audio thread every 512 samples and can never be disabled.", got)
	}
}

// TestSelectAudioChannelIsAnalyzerIdempotent isolates the chokepoint: selecting
// the same channel repeatedly (which reconcileChannelWithAudibleSet does on
// every mute/solo toggle) must not stack analyzers.
func TestSelectAudioChannelIsAnalyzerIdempotent(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 5, -2, 7)

	for i := 0; i < 8; i++ {
		g.drum.selectAudioChannel(channel.Of(testMelodicInst))
	}
	if got := audio.ChannelAnalyzerCount(channel.Of(testMelodicInst)); got > 1 {
		t.Errorf("8 selects of the same channel left %d live analyzers (want <=1)", got)
	}
}
