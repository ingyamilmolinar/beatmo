package engine

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// buildSingleRow makes a short single-row path with a loop so Ensure()
// computations are deterministic across horizons.
func buildSingleRow(n int) (*model.Graph, [][]model.BeatInfo, []bool, []int, map[model.NodeID]model.Node) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	a := g.AddNode(0, 0, model.NodeTypeRegular)
	prev := a
	ids := []model.NodeID{a}
	for i := 1; i < n; i++ {
		cur := g.AddNode(i, 0, model.NodeTypeRegular)
		g.Edges[[2]model.NodeID{prev, cur}] = struct{}{}
		ids = append(ids, cur)
		prev = cur
	}
	g.Edges[[2]model.NodeID{ids[n-1], ids[0]}] = struct{}{}
	g.StartNodeID = ids[0]
	row, isLoop, start := g.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(g.Nodes))
	for id, node := range g.Nodes {
		nodes[id] = node
	}
	return g, [][]model.BeatInfo{row}, []bool{isLoop}, []int{start}, nodes
}

func TestPredictorEnsureClearsDirtyFlag(t *testing.T) {
	g, paths, loops, starts, nodes := buildSingleRow(16)
	p := NewPredictor(g)
	p.SetPaths(paths, loops, starts, nodes)

	// Initial prediction to a horizon.
	p.Ensure(64)

	// Mutate a node to mark predDirty and confirm it is set.
	if node, ok := g.GetNodeByID(paths[0][0].NodeID); ok {
		node.Params.Volume = 0.5
		g.SetNodeParams(paths[0][0].NodeID, node.Params)
		p.UpdateNode(paths[0][0].NodeID, node)
	}
	if !p.PredDirtyForTest() {
		t.Fatalf("expected predDirty after UpdateNode")
	}

	// Ensure to the same horizon should clear the dirty flag after recompute.
	p.Ensure(64)
	if p.PredDirtyForTest() {
		t.Fatalf("expected predDirty to be false after Ensure")
	}
}

func TestPredictorDirtyRebuildsExistingHorizon(t *testing.T) {
	g, paths, loops, starts, nodes := buildSingleRow(4)
	p := NewPredictor(g)
	p.SetPaths(paths, loops, starts, nodes)

	path := paths[0]
	if len(path) == 0 {
		t.Fatalf("expected non-empty beat path")
	}

	horizon := 32
	if horizon < len(path)*3 {
		horizon = len(path) * 3
	}
	p.Ensure(horizon)

	nodeID := model.InvalidNodeID
	firstIdx := -1
	for i, bi := range path {
		if bi.NodeType == model.NodeTypeRegular {
			nodeID = bi.NodeID
			firstIdx = i
			break
		}
	}
	if nodeID == model.InvalidNodeID {
		t.Fatalf("failed to locate a regular node in path")
	}
	secondIdx := firstIdx + len(path)
	if secondIdx >= horizon {
		horizon = secondIdx + len(path)
		p.Ensure(horizon)
	}

	// Baseline: every pass through the loop should be visible.
	if !p.VisibleAt(0, firstIdx) {
		t.Fatalf("expected first occurrence visible before logic change (idx=%d)", firstIdx)
	}
	if !p.VisibleAt(0, secondIdx) {
		t.Fatalf("expected second loop occurrence visible before logic change (idx=%d)", secondIdx)
	}

	// Flip logic so the node fires every other visit.
	node, ok := g.GetNodeByID(nodeID)
	if !ok {
		t.Fatalf("missing node %d", nodeID)
	}
	node.Params.LogicKind = "skip_every_n"
	node.Params.LogicN = 2
	g.SetNodeParams(nodeID, node.Params)
	p.UpdateNode(nodeID, node)

	// Re-run prediction at the same horizon; dirty rebuild should refresh existing samples.
	p.Ensure(horizon)
	if !p.VisibleAt(0, firstIdx) {
		t.Fatalf("expected first occurrence to remain visible after logic change (idx=%d)", firstIdx)
	}
	if p.VisibleAt(0, secondIdx) {
		t.Fatalf("expected second occurrence to be skipped after logic change (idx=%d)", secondIdx)
	}
}
