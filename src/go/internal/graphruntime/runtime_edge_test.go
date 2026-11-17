package graphruntime

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestApplyAllEmptySlice verifies that ApplyAll with zero commands succeeds.
func TestApplyAllEmptySlice(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)
	if err := rt.ApplyAll(); err != nil {
		t.Fatalf("ApplyAll with no commands should succeed: %v", err)
	}
}

// TestRemoveEdgeIdempotent verifies that removing a non-existent edge does not error.
func TestRemoveEdgeIdempotent(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)
	// Remove an edge that was never added.
	if err := rt.Apply(&RemoveEdgeCommand{From: 999, To: 888}); err != nil {
		t.Fatalf("removing non-existent edge should not error: %v", err)
	}
}

// TestSnapshotEmpty verifies that a snapshot of an empty graph returns empty nodes and edges.
func TestSnapshotEmpty(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)
	snap := rt.Snapshot()
	if len(snap.Nodes) != 0 {
		t.Fatalf("expected 0 nodes, got %d", len(snap.Nodes))
	}
	if len(snap.Edges) != 0 {
		t.Fatalf("expected 0 edges, got %d", len(snap.Edges))
	}
}

// TestAddNodeWithoutCaptureID verifies that AddNodeCommand works when outID is nil.
func TestAddNodeWithoutCaptureID(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)
	// Don't call CaptureID -- outID stays nil.
	cmd := &AddNodeCommand{I: 3, J: 4, Type: model.NodeTypeRegular}
	if err := rt.Apply(cmd); err != nil {
		t.Fatalf("add node without capture: %v", err)
	}
	snap := rt.Snapshot()
	if len(snap.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(snap.Nodes))
	}
	if snap.Nodes[0].Node.I != 3 || snap.Nodes[0].Node.J != 4 {
		t.Fatalf("unexpected node position: %+v", snap.Nodes[0].Node)
	}
}

// TestGraphAccessor verifies that rt.Graph() returns the same graph pointer.
func TestGraphAccessor(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)
	if rt.Graph() != graph {
		t.Fatal("Graph() should return the same graph pointer")
	}
}
