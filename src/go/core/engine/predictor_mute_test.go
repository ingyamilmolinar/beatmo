package engine

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestPredictorMuteClearsAudioOnly(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	startID := graph.AddNode(0, 0, model.NodeTypeRegular)
	graph.StartNodeID = startID
	muteID := graph.AddNode(1, 0, model.NodeTypeMute)
	tailID := graph.AddNode(2, 0, model.NodeTypeRegular)

	paths := [][]model.BeatInfo{{
		{NodeID: startID, NodeType: model.NodeTypeRegular},
		{NodeID: muteID, NodeType: model.NodeTypeMute},
		{NodeID: tailID, NodeType: model.NodeTypeRegular},
		{NodeID: startID, NodeType: model.NodeTypeRegular},
	}}
	nodes := map[model.NodeID]model.Node{
		startID: graph.Nodes[startID],
		muteID:  graph.Nodes[muteID],
		tailID: graph.Nodes[tailID],
	}

	pred := NewPredictor(graph)
	pred.SetPaths(paths, []bool{true}, []int{0}, nodes)
	pred.Ensure(6)

	cases := []struct {
		idx  int
		want bool
	}{
		{0, true},
		{1, false},
		{2, true},
		{3, true},
		{4, true},
	}

	for _, tc := range cases {
		if got := pred.AudibleAt(0, tc.idx); got != tc.want {
			t.Fatalf("AudibleAt(0,%d)=%v want %v", tc.idx, got, tc.want)
		}
	}
	if !pred.TriggeredAt(0, 1) {
		t.Fatalf("mute node should register as triggered at idx 1")
	}
	if !pred.TriggeredAt(0, 2) {
		t.Fatalf("regular node should trigger immediately after mute")
	}
}
