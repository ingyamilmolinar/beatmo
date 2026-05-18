//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The Chn (Channel) tab renders A/B overlay traces from six DSP-stage taps:
// Synth → AntiPop → FX → EQ → Bus → Master. Each stage is fed by a separate
// JS-side AnalyserNode (or its stub counterpart); the bridge in
// wasm_analyzer_bridge.go reads them with SynthAnalyzerSnapshot(id),
// PreEQAnalyzerSnapshot(id), and ChannelAnalyzerSnapshot(id). If any of
// the three enables is missing for a per-instrument id, the corresponding
// stage's snapshot returns empty and the renderer in drawChainOverlay
// suppresses both the trace AND the legend (state.TapX.Active == false).
//
// The original symptom: viewing "Kick-1" with A=Synth, B=FX showed only
// one trace because EnableSynthAnalyzer was never called for "kick-1" —
// only for "main". These tests assert the enable contract for the three
// call sites that touch analysers: applyMasterEQ, applyRowEQ, and
// setEQActiveChannel.

func TestChnTab_MasterAppliesAllThreeAnalysers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	audio.ResetAnalyserEnableCounts()

	g.drum.applyMasterEQ()

	if got := audio.ChannelAnalyzerEnableCount("main"); got < 1 {
		t.Errorf("ChannelAnalyzer for \"main\" enabled %d times; want >= 1", got)
	}
	if got := audio.PreEQAnalyzerEnableCount("main"); got < 1 {
		t.Errorf("PreEQAnalyzer for \"main\" enabled %d times; want >= 1", got)
	}
	if got := audio.SynthAnalyzerEnableCount("main"); got < 1 {
		t.Errorf("SynthAnalyzer for \"main\" enabled %d times; want >= 1", got)
	}
}

func TestChnTab_ApplyRowEQAppliesAllThreeAnalysers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.Rows[0].Instrument = "kick"
	g.drum.ensureRowEQ(0)

	audio.ResetAnalyserEnableCounts()

	g.drum.applyRowEQ(0)

	if got := audio.ChannelAnalyzerEnableCount("kick"); got < 1 {
		t.Errorf("ChannelAnalyzer for \"kick\" enabled %d times; want >= 1", got)
	}
	if got := audio.PreEQAnalyzerEnableCount("kick"); got < 1 {
		t.Errorf("PreEQAnalyzer for \"kick\" enabled %d times; want >= 1", got)
	}
	// This is the failing assertion in the original bug: per-instrument
	// SynthAnalyzer was never enabled, so on WASM the Synth/AntiPop taps
	// returned empty snapshots and the second trace silently dropped.
	if got := audio.SynthAnalyzerEnableCount("kick"); got < 1 {
		t.Errorf("SynthAnalyzer for \"kick\" enabled %d times; want >= 1 — "+
			"the Chn tab's Synth/AntiPop traces depend on this", got)
	}
}

func TestChnTab_SetActiveChannelAppliesAllThreeAnalysers(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.Rows[0].Instrument = "snare"
	g.drum.ensureRowEQ(0)

	audio.ResetAnalyserEnableCounts()

	g.drum.setEQActiveChannel("snare")

	if got := audio.ChannelAnalyzerEnableCount("snare"); got < 1 {
		t.Errorf("ChannelAnalyzer for \"snare\" enabled %d times; want >= 1", got)
	}
	if got := audio.PreEQAnalyzerEnableCount("snare"); got < 1 {
		t.Errorf("PreEQAnalyzer for \"snare\" enabled %d times; want >= 1", got)
	}
	if got := audio.SynthAnalyzerEnableCount("snare"); got < 1 {
		t.Errorf("SynthAnalyzer for \"snare\" enabled %d times; want >= 1 — "+
			"the Chn tab's Synth/AntiPop traces depend on this", got)
	}
}
