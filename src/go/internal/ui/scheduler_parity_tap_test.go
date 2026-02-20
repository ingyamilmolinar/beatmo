package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure parity tap stays quiet when DrumView slate matches predictor/audio.
func TestSchedulerParityTime_NoMismatch(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.parityWatch = parityWatchPanic

	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.addEdge(root, next)
	g.start = root
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.refreshDrumRow()
	g.ClearParityMismatches()

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)

	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("expected no parity mismatches, got %d", got)
	}
}

// Detect stale DrumView slate when predictor/audio would fire.
func TestSchedulerParityTime_DetectsMismatch(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	g.parityWatch = parityWatchPanic

	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.addEdge(root, next)
	g.start = root
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.refreshDrumRow()

	// Force a stale slate: predictor expects on at abs=0, slate says off.
	if len(g.drum.Rows[0].Steps) == 0 {
		t.Fatalf("unexpected empty drum steps")
	}
	g.drum.Rows[0].Steps[0] = false
	g.ClearParityMismatches()

	g.SetPlaying(true)
	defer func() {
		if r := recover(); r == nil {
			info := g.beatInfoAtRow(0, 0)
			expected := g.parityExpected(0, 0, info)
			visible := false
			if g.engine != nil && g.engine.Predictor != nil {
				g.engine.Predictor.Ensure(1)
				visible = g.engine.Predictor.VisibleAt(0, 0)
			}
			inst := ""
			missing := false
			if g.drum != nil && len(g.drum.Rows) > 0 {
				inst = g.drum.Rows[0].Instrument
				missing = !g.drum.IsInstrumentAvailable(inst)
			}
			t.Fatalf("expected panic, got nil (expected=%v visible=%v slate=%v inst=%q missing=%v seqNextIdxs=%v nextBeatIdxs=%v renderReady=%v)", expected, visible, g.drum.Rows[0].Steps[0], inst, missing, g.seqNextIdxs, g.nextBeatIdxs, g.renderReady)
		}
	}()
	scheduleAbsForMuteTest(g, 0)
}
