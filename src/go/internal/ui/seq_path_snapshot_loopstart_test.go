package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestSequencerSnapshotHonorsLoopStartSegment(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a loop with a non-zero loop start:
	// A -> B -> C -> B (loop starts at B)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	if a == nil || b == nil || c == nil {
		t.Fatalf("failed to create nodes for loop start test")
	}
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, b)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated")
	}

	// The computed loopStart should point at B within the row path.
	startIdx := g.loopStartByRow[0]
	if startIdx <= 0 {
		t.Fatalf("expected loopStart > 0 (got %d)", startIdx)
	}
	if g.beatInfosByRow[0][startIdx].NodeID != b.ID {
		t.Fatalf("expected loopStart node to be B (got %d)", g.beatInfosByRow[0][startIdx].NodeID)
	}

	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })
	g.SetPlaying(true)

	// Seed counters so the next scheduled index is in the loop segment.
	target := startIdx + 1
	g.seqNextIdxs = []int{target}
	g.nextBeatIdxs = []int{target}

	plays = 0
	scheduleAbsForMuteTest(g, target)
	if plays == 0 {
		t.Fatalf("expected a play at abs=%d in loop segment", target)
	}

	// Mutate live beatInfos to invisible (simulating an unsafe concurrent write).
	for i := range g.beatInfosByRow[0] {
		g.beatInfosByRow[0][i].NodeType = model.NodeTypeInvisible
	}

	plays = 0
	next := target + 1
	scheduleAbsForMuteTest(g, next)
	if plays == 0 {
		t.Fatalf("expected snapshot to preserve loop segment scheduling after mutation")
	}
}
