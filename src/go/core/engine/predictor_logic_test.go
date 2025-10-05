package engine

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestPredictorMuteEveryNTriggers(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	start := graph.AddNode(0, 0, model.NodeTypeRegular)
	mute := graph.AddNode(1, 0, model.NodeTypeMute)
	tail := graph.AddNode(2, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{start, mute}] = struct{}{}
	graph.Edges[[2]model.NodeID{mute, tail}] = struct{}{}
	graph.Edges[[2]model.NodeID{tail, start}] = struct{}{}
	graph.StartNodeID = start

	if node, ok := graph.GetNodeByID(mute); ok {
		params := node.Params
		params.LogicKind = "every_n_triggers"
		params.LogicN = 2
		graph.SetNodeParams(mute, params)
	} else {
		t.Fatalf("missing mute node")
	}

	pred := NewPredictor(graph)
	path, isLoop, loopStart := graph.CalculateBeatRow()
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart})
	if node, ok := graph.GetNodeByID(mute); ok {
		pred.UpdateNode(mute, node)
	}
	if node, ok := pred.nodes[mute]; !ok {
		t.Fatalf("predictor missing mute node")
	} else {
		if got := node.Params.LogicKind; got != "every_n_triggers" {
			t.Fatalf("unexpected logic kind: %s", got)
		}
		if node.Params.LogicN != 2 {
			t.Fatalf("unexpected logic N: %d", node.Params.LogicN)
		}
	}

	muteIdx := -1
	tailIdx := -1
	for i, bi := range path {
		switch bi.NodeID {
		case mute:
			if muteIdx < 0 {
				muteIdx = i
			}
		case tail:
			if tailIdx < 0 {
				tailIdx = i
			}
		}
	}
	if muteIdx < 0 || tailIdx < 0 {
		t.Fatalf("failed to locate mute (%d) or tail (%d) in path", muteIdx, tailIdx)
	}

	cycleLen := len(path)
	need := cycleLen * 4
	pred.Ensure(need)

	manualCounts := make(map[model.NodeID]int)
	manualLastTrig := make(map[model.NodeID]bool)
	manualLastFired := model.InvalidNodeID
	manualPattern := make([]bool, 0, 4)
	for cycle := 0; cycle < 4; cycle++ {
		idx := muteIdx + cycle*cycleLen
		bi := pred.beatInfoAtRow(0, idx)
		n, ok := pred.nodes[bi.NodeID]
		if !ok {
			t.Fatalf("missing node %d in predictor map", bi.NodeID)
		}
		trig := pred.shouldTriggerNode(0, idx, bi, n, manualCounts, &manualLastFired, manualLastTrig)
		manualLastTrig[bi.NodeID] = trig
		if trig && bi.NodeType == model.NodeTypeRegular {
			manualLastFired = bi.NodeID
		}
		manualPattern = append(manualPattern, trig)
	}
	if got := manualPattern; len(got) != 4 || got[0] || !got[1] || got[2] || !got[3] {
		t.Fatalf("manual predictor pattern mismatch: %v", got)
	}

	expectedTriggers := []bool{false, true, false, true}
	pred.RebaseAt(0)
	pred.Ensure(need)
	for cycle := 0; cycle < len(expectedTriggers); cycle++ {
		idx := muteIdx + cycle*cycleLen
		if state := pred.TriggeredAt(0, idx); state != expectedTriggers[cycle] {
			t.Fatalf("triggered pattern mismatch at cycle %d: got=%v want=%v", cycle, state, expectedTriggers[cycle])
		}
	}

	expectedVisible := []bool{true, false, true, false}
	for cycle := 0; cycle < len(expectedVisible); cycle++ {
		idx := tailIdx + cycle*cycleLen
		got := pred.VisibleAt(0, idx)
		if got != expectedVisible[cycle] {
			t.Fatalf("visible pattern mismatch at cycle %d: got=%v want=%v", cycle, got, expectedVisible[cycle])
		}
	}
}
