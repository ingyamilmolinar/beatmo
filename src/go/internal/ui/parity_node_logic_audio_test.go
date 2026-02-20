package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Regression: NodeLogic callbacks that disable triggers must not produce
// audio_missing parity mismatches for intentionally skipped beats.
func TestParityScanSkipsAudioMissingWhenNodeLogicDisables(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	g.graph.SetNodeLogic(a.ID, func(ctx model.NodeContext) model.NodeDecision {
		if ctx.TriggerCount%2 == 0 {
			disabled := false
			return model.NodeDecision{Enabled: &disabled}
		}
		return model.NodeDecision{}
	})

	g.SetPlaying(true)

	scheduleAbsForMuteTest(g, 0)
	scheduleAbsForMuteTest(g, 1)

	g.parityMu.Lock()
	if m, ok := g.paritySeqDecisions[0]; ok {
		if dec, ok2 := m[1]; ok2 {
			dec.RecordedAt = time.Unix(0, 0).Add(-250 * time.Millisecond)
			m[1] = dec
		}
	}
	g.parityMu.Unlock()

	g.ClearParityMismatches()
	g.parityScan("test-node-logic-disable")

	for _, m := range g.ParityMismatchSnapshot() {
		if m.Kind == "audio_missing" {
			t.Fatalf("unexpected audio_missing mismatch for node-logic disabled beat: %+v", g.ParityMismatchSnapshot())
		}
	}
}

// Regression: NodeLogic parameter changes (vol/pitch/dur) should not create
// scheduler vs view parity mismatches.
func TestParityNodeLogicParamAdjustmentsNoMismatch(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.parityWatch = parityWatchLog
	g.ClearParityMismatches()

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.addEdge(a, a)
	g.updateBeatInfos()
	ensureRowInstrumentsAvailable(t, g)
	g.drum.SetLength(1)
	g.drum.Offset = 0
	g.refreshDrumRow()

	g.graph.SetNodeLogic(a.ID, func(ctx model.NodeContext) model.NodeDecision {
		return model.NodeDecision{VolumeMul: 0.5, PitchDelta: 3, DurationMul: 2}
	})

	g.SetPlaying(true)
	scheduleAbsForMuteTest(g, 0)

	g.ClearParityMismatches()
	g.parityScan("test-node-logic-params")

	if got := g.ParityMismatchSnapshot(); len(got) != 0 {
		t.Fatalf("unexpected parity mismatches after node logic param adjustments: %+v", got)
	}
}
