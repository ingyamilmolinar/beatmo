package engine

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestPredictorDeleteNodeClearsAudible(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = n0

	path := []model.BeatInfo{{NodeID: n0, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{n0: graph.Nodes[n0]}

	pred := NewPredictor(graph)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.Ensure(4)
	if !pred.AudibleAt(0, 0) {
		t.Fatalf("expected audible before delete")
	}

	pred.DeleteNode(n0)
	pred.Ensure(4)
	if pred.AudibleAt(0, 0) {
		t.Fatalf("expected audible cleared after delete")
	}
}

func TestPredictorOutOfRangeReturnsFalse(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	n0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = n0
	path := []model.BeatInfo{{NodeID: n0, NodeType: model.NodeTypeRegular}}
	nodes := map[model.NodeID]model.Node{n0: graph.Nodes[n0]}

	pred := NewPredictor(graph)
	pred.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	pred.Ensure(2)

	if pred.VisibleAt(-1, 0) || pred.VisibleAt(0, -1) || pred.VisibleAt(0, 5) {
		t.Fatalf("expected VisibleAt out-of-range to be false")
	}
	if pred.AudibleAt(-1, 0) || pred.AudibleAt(0, -1) || pred.AudibleAt(0, 5) {
		t.Fatalf("expected AudibleAt out-of-range to be false")
	}
	if pred.TriggeredAt(-1, 0) || pred.TriggeredAt(0, -1) || pred.TriggeredAt(0, 5) {
		t.Fatalf("expected TriggeredAt out-of-range to be false")
	}
}
