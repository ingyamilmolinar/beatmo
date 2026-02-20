package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestMuteSequencerMatchesPredictorLogicCallback(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	if start == nil || mute == nil || tail == nil {
		t.Fatalf("failed to create nodes for mute logic test")
	}

	g.graph.SetNodeLogic(mute.ID, func(ctx model.NodeContext) model.NodeDecision {
		if ctx.TriggerCount%2 == 0 {
			disabled := false
			return model.NodeDecision{Enabled: &disabled}
		}
		return model.NodeDecision{}
	})

	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start

	g.updateBeatInfos()
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated")
	}

	cycleLen := len(g.beatInfosByRow[0])
	horizon := cycleLen * 4
	if horizon < 1 {
		t.Fatalf("invalid horizon")
	}
	g.drum.SetLength(horizon)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Ensure audio path isn't blocked by missing instruments.
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.SetPlaying(true)

	g.engine.Predictor.Ensure(horizon)
	expected := make(map[int]bool)
	hasTrue := false
	hasFalse := false
	for abs := 0; abs < horizon; abs++ {
		info := g.beatInfoAtRow(0, abs)
		if info.NodeType != model.NodeTypeMute {
			continue
		}
		val := g.engine.Predictor.TriggeredAt(0, abs)
		expected[abs] = val
		if val {
			hasTrue = true
		} else {
			hasFalse = true
		}
	}
	if len(expected) == 0 {
		t.Fatalf("expected mute beats in path")
	}
	if !hasTrue || !hasFalse {
		t.Fatalf("expected both true and false mute triggers; got true=%v false=%v (%v)", hasTrue, hasFalse, expected)
	}

	g.parityMu.Lock()
	g.paritySeqDecisions = make(map[int]map[int]paritySeqDecision)
	g.parityMu.Unlock()
	g.seqNextIdxs = make([]int, len(g.drum.Rows))
	g.nextBeatIdxs = make([]int, len(g.drum.Rows))

	for abs := 0; abs < horizon; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}

	g.parityMu.Lock()
	decisions := make(map[int]paritySeqDecision)
	if rowDecisions, ok := g.paritySeqDecisions[0]; ok {
		for abs, dec := range rowDecisions {
			decisions[abs] = dec
		}
	}
	g.parityMu.Unlock()

	for abs, want := range expected {
		dec, ok := decisions[abs]
		if !ok {
			t.Fatalf("missing sequencer decision for abs=%d", abs)
		}
		if dec.Audible != want {
			t.Fatalf("mute decision mismatch at abs=%d: got=%v want=%v", abs, dec.Audible, want)
		}
	}
}
