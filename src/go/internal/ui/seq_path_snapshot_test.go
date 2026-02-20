package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

func TestSequencerUsesPathSnapshotIsolation(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mid := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if start == nil || mid == nil {
		t.Fatalf("failed to create nodes for snapshot test")
	}
	g.addEdge(start, mid)
	g.addEdge(mid, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	scheduleAbsForMuteTest(g, 0)
	if plays == 0 {
		t.Fatalf("expected initial scheduling to play at abs=0")
	}

	plays = 0
	for i := range g.beatInfosByRow[0] {
		g.beatInfosByRow[0][i].NodeType = model.NodeTypeInvisible
	}

	scheduleAbsForMuteTest(g, 1)
	if plays == 0 {
		t.Fatalf("expected sequencer to keep using snapshot after live mutation")
	}
}

func TestSequencerPathSnapshotUpdatesOnBeatInfoChange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mid := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if start == nil || mid == nil {
		t.Fatalf("failed to create nodes for snapshot update test")
	}
	g.addEdge(start, mid)
	g.addEdge(mid, start)
	g.start = start
	g.graph.StartNodeID = start.ID
	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	g.updateBeatInfos()

	// Flip the mid node to invisible and rebuild paths (updates snapshot).
	if node, ok := g.graph.GetNodeByID(mid.ID); ok {
		node.Type = model.NodeTypeInvisible
		g.graph.Nodes[mid.ID] = node
	} else {
		t.Fatalf("missing mid node for update")
	}
	g.updateBeatInfos()

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	// Pretend the sequencer already advanced past abs=0 so we only evaluate abs=1.
	g.seqNextIdxs = []int{1}
	g.nextBeatIdxs = []int{1}
	scheduleAbsForMuteTest(g, 1)
	if plays != 0 {
		t.Fatalf("expected no play at abs=1 after node became invisible, got %d", plays)
	}
}
