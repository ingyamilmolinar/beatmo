package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestSequencerSnapshotSkipsDuringRowCountMismatchThenResumes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	next := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if start == nil || next == nil {
		t.Fatalf("failed to create nodes for row mismatch test")
	}
	g.addEdge(start, next)
	g.addEdge(next, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })
	g.SetPlaying(true)

	scheduleAbsForTest(g, 0)
	if plays == 0 {
		t.Fatalf("expected initial scheduling to play")
	}

	plays = 0
	g.drum.AddRow()
	g.drum.Rows[1].Origin = start.ID
	g.drum.Rows[1].Node = start

	// Snapshot rows mismatch; sequencer should skip scheduling.
	scheduleAbsForTest(g, 1)
	if plays != 0 {
		t.Fatalf("expected no plays while snapshot rows mismatch")
	}

	// Refresh paths + snapshot; scheduling should resume.
	g.updateBeatInfos()
	scheduleAbsForTest(g, 2)
	if plays == 0 {
		t.Fatalf("expected scheduling to resume after snapshot update")
	}
}
