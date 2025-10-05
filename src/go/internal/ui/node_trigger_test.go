package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

func TestNodeTriggeredRespectsMuteLogic(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	mute := g.tryAddNode(1, 0, model.NodeTypeMute)
	tail := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mute)
	g.addEdge(mute, tail)
	g.addEdge(tail, start)

	g.start = start
	g.graph.StartNodeID = start.ID

	if n, ok := g.graph.GetNodeByID(mute.ID); ok {
		n.Params.LogicKind = "every_n_triggers"
		n.Params.LogicN = 2
		g.graph.Nodes[mute.ID] = n
	}

	g.updateBeatInfos()
	g.ensurePredictions(32)

	muteIdx := -1
	cycle := len(g.beatInfosByRow[0])
	for i, bi := range g.beatInfosByRow[0] {
		if bi.NodeID == mute.ID {
			muteIdx = i
			break
		}
	}
	if muteIdx < 0 {
		t.Fatalf("mute index not found")
	}

	if g.nodeTriggered(0, muteIdx, g.beatInfoAtRow(0, muteIdx)) {
		t.Fatalf("mute should be skipped on first cycle")
	}

	second := muteIdx + cycle
	if !g.nodeTriggered(0, second, g.beatInfoAtRow(0, second)) {
		t.Fatalf("mute should trigger on second cycle")
	}
}
