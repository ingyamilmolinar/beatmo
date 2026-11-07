//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Each of the six stage taps is a separate AnalyserNode; an enable missing for an instrument id empties that
// stage's snapshot and drops its trace and legend. Pins the enables at applyMasterEQ, applyRowEQ and
// setEQActiveChannel.

func TestChnTab_MasterAppliesAllThreeAnalysers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	audio.ResetAnalyserEnableCounts()

	g.drum.applyMasterEQ()

	if got := audio.ChannelAnalyzerEnableCount(channel.Master); got < 1 {
		t.Errorf("ChannelAnalyzer for \"main\" enabled %d times; want >= 1", got)
	}
	if got := audio.PreEQAnalyzerEnableCount(channel.Master); got < 1 {
		t.Errorf("PreEQAnalyzer for \"main\" enabled %d times; want >= 1", got)
	}
	if got := audio.SynthAnalyzerEnableCount(channel.Master); got < 1 {
		t.Errorf("SynthAnalyzer for \"main\" enabled %d times; want >= 1", got)
	}
}

func TestChnTab_ApplyRowEQAppliesAllThreeAnalysers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.Rows[0].Instrument = roster.Kick

	audio.ResetAnalyserEnableCounts()

	g.drum.applyRowEQ(0)

	if got := audio.ChannelAnalyzerEnableCount(channel.Of(roster.Kick)); got < 1 {
		t.Errorf("ChannelAnalyzer for \"kick\" enabled %d times; want >= 1", got)
	}
	if got := audio.PreEQAnalyzerEnableCount(channel.Of(roster.Kick)); got < 1 {
		t.Errorf("PreEQAnalyzer for \"kick\" enabled %d times; want >= 1", got)
	}
	// Without the per-instrument synth analyser the Synth/AntiPop taps read empty on WASM and the second trace drops.
	if got := audio.SynthAnalyzerEnableCount(channel.Of(roster.Kick)); got < 1 {
		t.Errorf("SynthAnalyzer for \"kick\" enabled %d times; want >= 1 — "+
			"the Chn tab's Synth/AntiPop traces depend on this", got)
	}
}

func TestChnTab_SetActiveChannelAppliesAllThreeAnalysers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.Rows[0].Instrument = roster.Snare

	audio.ResetAnalyserEnableCounts()

	g.drum.setEQActiveChannel(channel.Of(roster.Snare))

	if got := audio.ChannelAnalyzerEnableCount(channel.Of(roster.Snare)); got < 1 {
		t.Errorf("ChannelAnalyzer for \"snare\" enabled %d times; want >= 1", got)
	}
	if got := audio.PreEQAnalyzerEnableCount(channel.Of(roster.Snare)); got < 1 {
		t.Errorf("PreEQAnalyzer for \"snare\" enabled %d times; want >= 1", got)
	}
	if got := audio.SynthAnalyzerEnableCount(channel.Of(roster.Snare)); got < 1 {
		t.Errorf("SynthAnalyzer for \"snare\" enabled %d times; want >= 1 — "+
			"the Chn tab's Synth/AntiPop traces depend on this", got)
	}
}
