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
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, node := range graph.Nodes {
		nodes[id] = node
	}
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
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

func TestPredictorProbabilityLogicDeterministic(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	start := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = start

	path := []model.BeatInfo{{NodeID: start, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{start: graph.Nodes[start]}

	pred := NewPredictor(graph)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)

	node := nodes[start]
	node.Params.LogicKind = "probability"
	node.Params.LogicP = 0.25
	graph.SetNodeParams(start, node.Params)
	pred.UpdateNode(start, node)
	pred.Ensure(16)

	for idx := 0; idx < 8; idx++ {
		roll := pred.deterministicRoll(0, idx, start)
		want := roll <= 0.25
		if got := pred.VisibleAt(0, idx); got != want {
			t.Fatalf("probability mismatch idx=%d roll=%.4f got=%v want=%v", idx, roll, got, want)
		}
	}

	// p=0 should never trigger
	node.Params.LogicP = 0
	graph.SetNodeParams(start, node.Params)
	pred.UpdateNode(start, node)
	pred.Ensure(8)
	for idx := 0; idx < 4; idx++ {
		if pred.VisibleAt(0, idx) {
			t.Fatalf("expected probability=0 to suppress idx=%d", idx)
		}
	}

	// p=1 should always trigger
	node.Params.LogicP = 1
	graph.SetNodeParams(start, node.Params)
	pred.UpdateNode(start, node)
	pred.Ensure(8)
	for idx := 0; idx < 4; idx++ {
		if !pred.VisibleAt(0, idx) {
			t.Fatalf("expected probability=1 to fire idx=%d", idx)
		}
	}
}

func TestPredictorPrevTriggerLogic(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	a := graph.AddNode(0, 0, model.NodeTypeRegular)
	b := graph.AddNode(1, 0, model.NodeTypeRegular)
	graph.StartNodeID = a

	path := []model.BeatInfo{
		{NodeID: a, NodeType: model.NodeTypeRegular},
		{NodeID: b, NodeType: model.NodeTypeRegular},
	}
	nodes := map[model.NodeID]model.Node{
		a: graph.Nodes[a],
		b: graph.Nodes[b],
	}

	// A skips every 2nd trigger; B fires if prev triggered.
	nodeA := nodes[a]
	nodeA.Params.LogicKind = "skip_every_n"
	nodeA.Params.LogicN = 2
	graph.SetNodeParams(a, nodeA.Params)
	nodes[a] = nodeA

	nodeB := nodes[b]
	nodeB.Params.LogicKind = "trigger_if_prev_triggered"
	graph.SetNodeParams(b, nodeB.Params)
	nodes[b] = nodeB

	pred := NewPredictor(graph)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.UpdateNode(a, nodeA)
	pred.UpdateNode(b, nodeB)
	pred.Ensure(8)

	// B should fire on indices 1,5... and skip on 3,7...
	expect := []bool{true, false, true, false}
	for i, want := range expect {
		idx := 1 + i*2
		if got := pred.VisibleAt(0, idx); got != want {
			t.Fatalf("trigger_if_prev_triggered idx=%d got=%v want=%v", idx, got, want)
		}
	}

	// Switch B to trigger_if_prev_skipped.
	nodeB.Params.LogicKind = "trigger_if_prev_skipped"
	graph.SetNodeParams(b, nodeB.Params)
	pred.UpdateNode(b, nodeB)
	pred.Ensure(8)

	expectSkipped := []bool{false, true, false, true}
	for i, want := range expectSkipped {
		idx := 1 + i*2
		if got := pred.VisibleAt(0, idx); got != want {
			t.Fatalf("trigger_if_prev_skipped idx=%d got=%v want=%v", idx, got, want)
		}
	}
}

func TestPredictorMuteNodeLogicCallback(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	start := graph.AddNode(0, 0, model.NodeTypeRegular)
	mute := graph.AddNode(1, 0, model.NodeTypeMute)
	tail := graph.AddNode(2, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{start, mute}] = struct{}{}
	graph.Edges[[2]model.NodeID{mute, tail}] = struct{}{}
	graph.Edges[[2]model.NodeID{tail, start}] = struct{}{}
	graph.StartNodeID = start

	seenCounts := make([]int, 0, 8)
	graph.SetNodeLogic(mute, func(ctx model.NodeContext) model.NodeDecision {
		seenCounts = append(seenCounts, ctx.TriggerCount)
		if ctx.TriggerCount%2 == 0 {
			disabled := false
			return model.NodeDecision{Enabled: &disabled}
		}
		return model.NodeDecision{}
	})

	path, isLoop, loopStart := graph.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, node := range graph.Nodes {
		nodes[id] = node
	}
	if node, ok := nodes[mute]; !ok || node.Params.Logic == nil {
		t.Fatalf("expected mute node logic present in nodes snapshot (ok=%v)", ok)
	}

	pred := NewPredictor(graph)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	if node, ok := pred.nodes[mute]; !ok || node.Params.Logic == nil {
		t.Fatalf("expected predictor to retain mute node logic (ok=%v)", ok)
	}

	muteIdx := -1
	for i, bi := range path {
		if bi.NodeID == mute {
			muteIdx = i
			break
		}
	}
	if muteIdx < 0 {
		t.Fatalf("mute node not found in path")
	}

	cycleLen := loopSegmentLen(path, loopStart)
	if cycleLen <= 0 {
		cycleLen = len(path)
	}
	if cycleLen <= 0 {
		t.Fatalf("invalid path length")
	}
	cycles := 4
	horizon := cycleLen*cycles + muteIdx + 1
	pred.Ensure(horizon)

	got := make([]bool, 0, cycles)
	for i := 0; i < cycles; i++ {
		idx := muteIdx + i*cycleLen
		if bi := pred.beatInfoAtRow(0, idx); bi.NodeID != mute {
			t.Fatalf("expected mute node at idx=%d (got id=%d type=%v)", idx, bi.NodeID, bi.NodeType)
		}
		got = append(got, pred.TriggeredAt(0, idx))
	}
	if len(seenCounts) < cycles {
		t.Fatalf("expected logic callback per mute occurrence; got %d counts", len(seenCounts))
	}
	for i := 0; i < cycles; i++ {
		if seenCounts[i] != i+1 {
			t.Fatalf("unexpected trigger counts: got=%v wantPrefix=%v", seenCounts, []int{1, 2, 3, 4})
		}
	}
	want := []bool{true, false, true, false}
	if len(got) != len(want) {
		t.Fatalf("unexpected trigger pattern length: got=%d want=%d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mute logic callback mismatch at cycle %d: got=%v want=%v (pattern=%v)", i, got[i], want[i], got)
		}
	}
}

func TestPredictorRegularNodeLogicCallback(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	start := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = start
	graph.Edges[[2]model.NodeID{start, start}] = struct{}{}

	seenCounts := make([]int, 0, 8)
	graph.SetNodeLogic(start, func(ctx model.NodeContext) model.NodeDecision {
		seenCounts = append(seenCounts, ctx.TriggerCount)
		if ctx.TriggerCount%2 == 0 {
			disabled := false
			return model.NodeDecision{Enabled: &disabled}
		}
		return model.NodeDecision{}
	})

	path, isLoop, loopStart := graph.CalculateBeatRow()
	if !isLoop {
		t.Fatalf("expected loop for self-edge; len=%d start=%d", len(path), loopStart)
	}
	if len(path) < 4 {
		t.Fatalf("expected loop-expanded path length >=4, got %d", len(path))
	}
	nodes := map[model.NodeID]model.Node{start: graph.Nodes[start]}
	if node, ok := nodes[start]; !ok || node.Params.Logic == nil {
		t.Fatalf("expected node logic present in nodes snapshot (ok=%v)", ok)
	}

	pred := NewPredictor(graph)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	if node, ok := pred.nodes[start]; !ok || node.Params.Logic == nil {
		t.Fatalf("expected predictor to retain regular node logic (ok=%v)", ok)
	}

	pred.Ensure(8)
	want := []bool{true, false, true, false}
	gotAud := make([]bool, len(want))
	gotVis := make([]bool, len(want))
	gotIDs := make([]model.NodeID, len(want))
	for idx := range want {
		gotIDs[idx] = pred.beatInfoAtRow(0, idx).NodeID
		gotAud[idx] = pred.AudibleAt(0, idx)
		gotVis[idx] = pred.VisibleAt(0, idx)
	}
	for idx, expected := range want {
		if gotAud[idx] != expected {
			t.Fatalf("audible pattern mismatch got=%v want=%v counts=%v ids=%v", gotAud, want, seenCounts, gotIDs)
		}
		if gotVis[idx] != expected {
			t.Fatalf("visible pattern mismatch got=%v want=%v counts=%v ids=%v", gotVis, want, seenCounts, gotIDs)
		}
	}

	if len(seenCounts) < len(want) {
		t.Fatalf("expected logic callback per trigger; got %d counts", len(seenCounts))
	}
	for i := 0; i < len(want); i++ {
		if seenCounts[i] != i+1 {
			t.Fatalf("unexpected trigger counts: got=%v wantPrefix=%v", seenCounts, []int{1, 2, 3, 4})
		}
	}
}
