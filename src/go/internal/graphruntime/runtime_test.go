package graphruntime

import (
	"fmt"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestSnapshotImmutable(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)

	add := (&AddNodeCommand{I: 0, J: 0, Type: model.NodeTypeRegular})
	if err := rt.Apply(add); err != nil {
		t.Fatalf("apply add node: %v", err)
	}
	snap := rt.Snapshot()

	if len(snap.Nodes) != 1 {
		t.Fatalf("snapshot nodes=%d want 1", len(snap.Nodes))
	}

	// mutate original graph
	rt.graph.Nodes[snap.Nodes[0].ID] = model.Node{I: 5, J: 5, Type: model.NodeTypeMute}

	if snap.Nodes[0].Node.I != 0 || snap.Nodes[0].Node.J != 0 {
		t.Fatalf("snapshot mutated: %+v", snap.Nodes[0].Node)
	}
}

func TestSnapshotIncludesEdges(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)
	var aID, bID model.NodeID
	_ = rt.Apply((&AddNodeCommand{I: 0, J: 0, Type: model.NodeTypeRegular}).CaptureID(&aID))
	_ = rt.Apply((&AddNodeCommand{I: 1, J: 0, Type: model.NodeTypeRegular}).CaptureID(&bID))
	_ = rt.Apply(&AddEdgeCommand{From: aID, To: bID})

	snap := rt.Snapshot()
	if len(snap.Edges) != 1 {
		t.Fatalf("snapshot edges=%d want 1", len(snap.Edges))
	}
	if snap.Edges[0].From != aID || snap.Edges[0].To != bID {
		t.Fatalf("unexpected edge snapshot: %+v", snap.Edges[0])
	}
}

func TestApplyAllCommands(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)

	var nodeID model.NodeID
	cmds := []Command{
		(&AddNodeCommand{I: 1, J: 2, Type: model.NodeTypeRegular}).CaptureID(&nodeID),
		&SetNodeParamsCommand{
			ID: nodeID,
			P:  model.NodeParams{Volume: 0.5, Pitch: 3},
		},
	}
	if err := rt.ApplyAll(cmds...); err != nil {
		t.Fatalf("ApplyAll failed: %v", err)
	}

	if nodeID == model.InvalidNodeID {
		t.Fatalf("nodeID not captured")
	}
	if node, ok := rt.graph.GetNodeByID(nodeID); !ok {
		t.Fatalf("node missing in graph")
	} else if node.Params.Volume != 0.5 || node.Params.Pitch != 3 {
		t.Fatalf("unexpected params: %+v", node.Params)
	}

	// Applying nil should fail fast.
	if err := rt.Apply(nil); err == nil {
		t.Fatalf("expected error when applying nil command")
	}
}

func TestNewRuntimeNilPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic for nil graph")
		}
	}()
	_ = NewRuntime(nil)
}

func TestEdgeCommands(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	graph := model.NewGraph(logger)
	rt := NewRuntime(graph)

	var aID, bID model.NodeID
	if err := rt.Apply((&AddNodeCommand{I: 0, J: 0, Type: model.NodeTypeRegular}).CaptureID(&aID)); err != nil {
		t.Fatalf("add node A: %v", err)
	}
	if err := rt.Apply((&AddNodeCommand{I: 1, J: 0, Type: model.NodeTypeRegular}).CaptureID(&bID)); err != nil {
		t.Fatalf("add node B: %v", err)
	}
	addEdge := &AddEdgeCommand{From: aID, To: bID}
	if err := rt.Apply(addEdge); err != nil {
		t.Fatalf("add edge: %v", err)
	}

	if _, ok := rt.graph.Edges[[2]model.NodeID{aID, bID}]; !ok {
		t.Fatalf("edge not present after add")
	}

	if err := rt.Apply(&RemoveEdgeCommand{From: aID, To: bID}); err != nil {
		t.Fatalf("remove edge: %v", err)
	}
	if _, ok := rt.graph.Edges[[2]model.NodeID{aID, bID}]; ok {
		t.Fatalf("edge should be removed")
	}

	if err := rt.Apply(&RemoveNodeCommand{ID: aID}); err != nil {
		t.Fatalf("remove node: %v", err)
	}
	if _, ok := rt.graph.GetNodeByID(aID); ok {
		t.Fatalf("node A should be removed")
	}
}

func TestApplyAllStopsOnError(t *testing.T) {
	graph := model.NewGraph(game_log.New(nil, game_log.LevelError))
	rt := NewRuntime(graph)

	errCmd := &failingCommand{}
	cmds := []Command{
		&AddNodeCommand{I: 0, J: 0, Type: model.NodeTypeRegular},
		errCmd,
		&AddNodeCommand{I: 1, J: 0, Type: model.NodeTypeRegular},
	}
	if err := rt.ApplyAll(cmds...); err == nil {
		t.Fatalf("expected error from failing command")
	}
	if len(rt.graph.Nodes) != 1 {
		t.Fatalf("ApplyAll should stop after failure; nodes=%d", len(rt.graph.Nodes))
	}
	if errCmd.count != 1 {
		t.Fatalf("failing command should execute once, ran %d times", errCmd.count)
	}
}

type failingCommand struct {
	count int
}

func (f *failingCommand) Apply(*Runtime) error {
	f.count++
	return fmt.Errorf("fail")
}
