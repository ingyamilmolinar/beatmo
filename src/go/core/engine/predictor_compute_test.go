package engine

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRebaseAtEmptyPaths verifies that RebaseAt does not panic when the
// predictor has no paths configured (empty SetPaths).
func TestRebaseAtEmptyPaths(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	pred := NewPredictor(graph, nil)
	pred.SetPaths(nil, nil, nil, nil)

	// Should not panic with index 0.
	pred.RebaseAt(0)

	// Should not panic with a positive index beyond any buffer.
	pred.RebaseAt(10)
}

// TestRebaseAtPreservesHistory verifies that predictions computed before a
// rebase point are preserved, and that extending the horizon after rebase
// produces consistent results.
func TestRebaseAtPreservesHistory(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	n1 := graph.AddNode(1, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{n0, n1}] = struct{}{}
	graph.Edges[[2]model.NodeID{n1, n0}] = struct{}{}
	graph.StartNodeID = n0

	path := []model.BeatInfo{
		{NodeID: n0, NodeType: model.NodeTypeRegular},
		{NodeID: n1, NodeType: model.NodeTypeRegular},
	}
	nodes := map[model.NodeID]model.Node{
		n0: graph.Nodes[n0],
		n1: graph.Nodes[n1],
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.Ensure(8)

	if !pred.AudibleAt(0, 0) {
		t.Fatalf("expected AudibleAt(0,0)=true before rebase")
	}
	if !pred.AudibleAt(0, 1) {
		t.Fatalf("expected AudibleAt(0,1)=true before rebase")
	}

	// Rebase at index 4: predictions 0-3 should be reconstructed.
	pred.RebaseAt(4)

	if !pred.AudibleAt(0, 0) {
		t.Fatalf("expected AudibleAt(0,0)=true after rebase")
	}
	if !pred.AudibleAt(0, 1) {
		t.Fatalf("expected AudibleAt(0,1)=true after rebase")
	}

	// Extend horizon again.
	pred.Ensure(8)
	for i := 0; i < 8; i++ {
		if !pred.AudibleAt(0, i) {
			t.Fatalf("expected AudibleAt(0,%d)=true after re-Ensure", i)
		}
	}
}

// TestComputeRowMixedNodeTypes verifies prediction results for a path
// containing Regular, Silent, Mute, and Regular nodes in sequence.
func TestComputeRowMixedNodeTypes(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	n1 := graph.AddNode(1, 0, model.NodeTypeSilent)
	n2 := graph.AddNode(2, 0, model.NodeTypeMute)
	n3 := graph.AddNode(3, 0, model.NodeTypeRegular)
	graph.StartNodeID = n0

	path := []model.BeatInfo{
		{NodeID: n0, NodeType: model.NodeTypeRegular},
		{NodeID: n1, NodeType: model.NodeTypeSilent},
		{NodeID: n2, NodeType: model.NodeTypeMute},
		{NodeID: n3, NodeType: model.NodeTypeRegular},
	}
	nodes := map[model.NodeID]model.Node{
		n0: graph.Nodes[n0],
		n1: graph.Nodes[n1],
		n2: graph.Nodes[n2],
		n3: graph.Nodes[n3],
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.Ensure(8)

	// idx 0: Regular node n0 — audible and triggered.
	if !pred.AudibleAt(0, 0) {
		t.Errorf("idx 0: expected AudibleAt=true for regular node")
	}
	if !pred.TriggeredAt(0, 0) {
		t.Errorf("idx 0: expected TriggeredAt=true for regular node")
	}

	// idx 1: Silent node n1 — never audible, never triggered.
	if pred.AudibleAt(0, 1) {
		t.Errorf("idx 1: expected AudibleAt=false for silent node")
	}
	if pred.TriggeredAt(0, 1) {
		t.Errorf("idx 1: expected TriggeredAt=false for silent node")
	}

	// idx 2: Mute node n2 (no logic kind) — not audible, but triggered.
	// With no logic kind, shouldGateMute returns false so the gate is NOT set.
	if pred.AudibleAt(0, 2) {
		t.Errorf("idx 2: expected AudibleAt=false for mute node")
	}
	if !pred.TriggeredAt(0, 2) {
		t.Errorf("idx 2: expected TriggeredAt=true for mute node")
	}

	// idx 3: Regular node n3 — audible because the mute had no logic kind
	// (shouldGateMute=false), so no gate was activated.
	if !pred.AudibleAt(0, 3) {
		t.Errorf("idx 3: expected AudibleAt=true for regular node after ungated mute")
	}
	if !pred.TriggeredAt(0, 3) {
		t.Errorf("idx 3: expected TriggeredAt=true for regular node after ungated mute")
	}
}

// TestEnsureBeyondCurrentHorizon verifies that calling Ensure with a larger
// horizon correctly extends predictions without invalidating prior values.
func TestEnsureBeyondCurrentHorizon(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = n0

	path := []model.BeatInfo{{NodeID: n0, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{n0: graph.Nodes[n0]}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)

	// First ensure to horizon 4.
	pred.Ensure(4)
	for i := 0; i < 4; i++ {
		if !pred.AudibleAt(0, i) {
			t.Fatalf("first Ensure: expected AudibleAt(0,%d)=true", i)
		}
	}

	// Extend to horizon 8.
	pred.Ensure(8)
	for i := 0; i < 8; i++ {
		if !pred.AudibleAt(0, i) {
			t.Fatalf("extended Ensure: expected AudibleAt(0,%d)=true", i)
		}
	}
}

// TestEnsureIdempotent verifies that calling Ensure with the same horizon
// twice produces identical results.
func TestEnsureIdempotent(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	n1 := graph.AddNode(1, 0, model.NodeTypeSilent)
	graph.StartNodeID = n0

	path := []model.BeatInfo{
		{NodeID: n0, NodeType: model.NodeTypeRegular},
		{NodeID: n1, NodeType: model.NodeTypeSilent},
	}
	nodes := map[model.NodeID]model.Node{
		n0: graph.Nodes[n0],
		n1: graph.Nodes[n1],
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)

	pred.Ensure(8)

	// Capture all values.
	type snapshot struct {
		audible, visible, triggered bool
	}
	first := make([]snapshot, 8)
	for i := 0; i < 8; i++ {
		first[i] = snapshot{
			audible:   pred.AudibleAt(0, i),
			visible:   pred.VisibleAt(0, i),
			triggered: pred.TriggeredAt(0, i),
		}
	}

	// Second Ensure at the same horizon.
	pred.Ensure(8)

	for i := 0; i < 8; i++ {
		s := snapshot{
			audible:   pred.AudibleAt(0, i),
			visible:   pred.VisibleAt(0, i),
			triggered: pred.TriggeredAt(0, i),
		}
		if s != first[i] {
			t.Fatalf("idx %d: idempotency broken: first=%+v second=%+v", i, first[i], s)
		}
	}
}

// TestEnsureNegativeHorizon verifies that Ensure with a negative horizon
// does not panic (clamped to 0).
func TestEnsureNegativeHorizon(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = n0

	path := []model.BeatInfo{{NodeID: n0, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{n0: graph.Nodes[n0]}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)

	// Should not panic.
	pred.Ensure(-1)
	pred.Ensure(-100)

	// After a negative Ensure, querying any index should return false
	// (no predictions computed).
	if pred.AudibleAt(0, 0) {
		t.Fatalf("expected AudibleAt(0,0)=false after negative Ensure")
	}
}

// TestRebaseAtNegativeIdx verifies that RebaseAt with a negative index
// does not panic (clamped to 0).
func TestRebaseAtNegativeIdx(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = n0

	path := []model.BeatInfo{{NodeID: n0, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{n0: graph.Nodes[n0]}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.Ensure(4)

	// Should not panic.
	pred.RebaseAt(-5)
	pred.RebaseAt(-1)

	// After rebase at 0 (clamped), horizon is 0 — nothing should be visible.
	if pred.AudibleAt(0, 0) {
		t.Fatalf("expected AudibleAt(0,0)=false after negative rebase")
	}

	// Re-ensure should still work.
	pred.Ensure(4)
	if !pred.AudibleAt(0, 0) {
		t.Fatalf("expected AudibleAt(0,0)=true after re-Ensure")
	}
}

// TestRebaseAtWithMuteNodes verifies that RebaseAt correctly preserves mute
// gate context when a mute node fires near the rebase boundary.
func TestRebaseAtWithMuteNodes(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	start := graph.AddNode(0, 0, model.NodeTypeRegular)
	mute := graph.AddNode(1, 0, model.NodeTypeMute)
	tail := graph.AddNode(2, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{start, mute}] = struct{}{}
	graph.Edges[[2]model.NodeID{mute, tail}] = struct{}{}
	graph.Edges[[2]model.NodeID{tail, start}] = struct{}{}
	graph.StartNodeID = start

	// Give mute node a logic kind so shouldGateMute returns true.
	if node, ok := graph.GetNodeByID(mute); ok {
		params := node.Params
		params.LogicKind = "every_n_triggers"
		params.LogicN = 1
		graph.SetNodeParams(mute, params)
	}

	path, isLoop, loopStart := graph.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, node := range graph.Nodes {
		nodes[id] = node
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	if node, ok := graph.GetNodeByID(mute); ok {
		pred.UpdateNode(mute, node)
	}

	cycleLen := len(path)
	horizon := cycleLen * 4
	pred.Ensure(horizon)

	// Record predictions before rebase.
	beforeVis := make([]bool, horizon)
	for i := 0; i < horizon; i++ {
		beforeVis[i] = pred.VisibleAt(0, i)
	}

	// Rebase at a point after the first mute fires.
	rebasePoint := cycleLen * 2
	pred.RebaseAt(rebasePoint)

	// Extend predictions beyond the rebase point.
	pred.Ensure(horizon)

	// After rebase, predictions should be consistent with the full computation.
	for i := 0; i < horizon; i++ {
		got := pred.VisibleAt(0, i)
		if got != beforeVis[i] {
			t.Errorf("visible mismatch at idx=%d: before=%v after_rebase=%v", i, beforeVis[i], got)
		}
	}
}

// TestEnsureAfterSetPathsDirtyRebuild verifies that calling Ensure after
// SetPaths (which resets all buffers) correctly recomputes predictions
// from scratch, including logic counts.
func TestEnsureAfterSetPathsDirtyRebuild(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	a := graph.AddNode(0, 0, model.NodeTypeRegular)
	b := graph.AddNode(1, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{a, b}] = struct{}{}
	graph.Edges[[2]model.NodeID{b, a}] = struct{}{}
	graph.StartNodeID = a

	// Give node A skip_every_n logic.
	nodeA, _ := graph.GetNodeByID(a)
	nodeA.Params.LogicKind = "skip_every_n"
	nodeA.Params.LogicN = 2
	graph.SetNodeParams(a, nodeA.Params)

	path, isLoop, loopStart := graph.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, node := range graph.Nodes {
		nodes[id] = node
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	pred.UpdateNode(a, graph.Nodes[a])
	pred.Ensure(16)

	// Record predictions.
	before := make([]bool, 16)
	for i := 0; i < 16; i++ {
		before[i] = pred.AudibleAt(0, i)
	}

	// Reset via SetPaths (simulates circuit edit).
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	pred.UpdateNode(a, graph.Nodes[a])

	// Horizon should be 0 after SetPaths.
	_, _, h := pred.Snapshot()
	if h != 0 {
		t.Fatalf("expected horizon=0 after SetPaths, got %d", h)
	}

	// Re-ensure should produce identical predictions.
	pred.Ensure(16)
	for i := 0; i < 16; i++ {
		got := pred.AudibleAt(0, i)
		if got != before[i] {
			t.Errorf("prediction mismatch at idx=%d: before=%v after=%v", i, before[i], got)
		}
	}
}

// TestRebaseAtPreservesContextsForExtension verifies that after RebaseAt,
// subsequent Ensure calls extend predictions consistently.
func TestRebaseAtPreservesContextsForExtension(t *testing.T) {
	g, paths, loops, starts, nodes := buildSingleRow(4)
	pred := NewPredictor(g, nil)
	pred.SetPaths(paths, loops, starts, nodes)

	// Compute a full set of predictions.
	pred.Ensure(32)
	fullAud := make([]bool, 32)
	fullVis := make([]bool, 32)
	for i := 0; i < 32; i++ {
		fullAud[i] = pred.AudibleAt(0, i)
		fullVis[i] = pred.VisibleAt(0, i)
	}

	// Rebase at midpoint.
	pred.RebaseAt(16)

	// Extend back to 32.
	pred.Ensure(32)

	// Results should match the full computation.
	for i := 0; i < 32; i++ {
		if pred.AudibleAt(0, i) != fullAud[i] {
			t.Errorf("audible mismatch at idx=%d: full=%v rebased=%v", i, fullAud[i], pred.AudibleAt(0, i))
		}
		if pred.VisibleAt(0, i) != fullVis[i] {
			t.Errorf("visible mismatch at idx=%d: full=%v rebased=%v", i, fullVis[i], pred.VisibleAt(0, i))
		}
	}
}

// TestEnsureMultiRowIsolation verifies that predictor contexts are isolated
// per row — logic counts and gates from one row don't leak into another.
func TestEnsureMultiRowIsolation(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)

	// Row 0: regular node, always fires.
	a := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{a, a}] = struct{}{}
	graph.StartNodeID = a

	// Row 1: node with probability=0 (never fires).
	b := graph.AddNode(1, 0, model.NodeTypeRegular)
	graph.Edges[[2]model.NodeID{b, b}] = struct{}{}
	nodeB, _ := graph.GetNodeByID(b)
	nodeB.Params.LogicKind = "probability"
	nodeB.Params.LogicP = 0
	graph.SetNodeParams(b, nodeB.Params)

	path0 := []model.BeatInfo{{NodeID: a, NodeType: model.NodeTypeRegular}}
	path1 := []model.BeatInfo{{NodeID: b, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{
		a: graph.Nodes[a],
		b: graph.Nodes[b],
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths(
		[][]model.BeatInfo{path0, path1},
		[]bool{true, true},
		[]int{0, 0},
		nodes,
	)
	pred.UpdateNode(b, graph.Nodes[b])
	pred.Ensure(8)

	// Row 0 should always fire, row 1 never.
	for i := 0; i < 8; i++ {
		if !pred.AudibleAt(0, i) {
			t.Errorf("row 0 idx=%d should be audible", i)
		}
		if pred.AudibleAt(1, i) {
			t.Errorf("row 1 idx=%d should not be audible (probability=0)", i)
		}
	}
}

// TestComputeRowLoopWrapAround verifies that a loop with 3 nodes repeats
// correctly when the horizon extends beyond the path length.
func TestComputeRowLoopWrapAround(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	n1 := graph.AddNode(1, 0, model.NodeTypeRegular)
	n2 := graph.AddNode(2, 0, model.NodeTypeSilent)
	graph.StartNodeID = n0

	// Path: [Regular, Regular, Silent] looping from start.
	path := []model.BeatInfo{
		{NodeID: n0, NodeType: model.NodeTypeRegular},
		{NodeID: n1, NodeType: model.NodeTypeRegular},
		{NodeID: n2, NodeType: model.NodeTypeSilent},
	}
	nodes := map[model.NodeID]model.Node{
		n0: graph.Nodes[n0],
		n1: graph.Nodes[n1],
		n2: graph.Nodes[n2],
	}

	pred := NewPredictor(graph, nil)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.Ensure(9)

	// Expected pattern (period 3): audible, audible, not-audible
	for cycle := 0; cycle < 3; cycle++ {
		base := cycle * 3
		// idx base+0: Regular n0 — audible
		if !pred.AudibleAt(0, base+0) {
			t.Errorf("cycle %d idx %d: expected AudibleAt=true for regular node n0", cycle, base+0)
		}
		// idx base+1: Regular n1 — audible
		if !pred.AudibleAt(0, base+1) {
			t.Errorf("cycle %d idx %d: expected AudibleAt=true for regular node n1", cycle, base+1)
		}
		// idx base+2: Silent n2 — not audible
		if pred.AudibleAt(0, base+2) {
			t.Errorf("cycle %d idx %d: expected AudibleAt=false for silent node n2", cycle, base+2)
		}
	}
}
