package engine

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newLoopPredictorForTest builds one looping row of 4 regular nodes.
func newLoopPredictorForTest(t *testing.T) (*Predictor, []model.NodeID) {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	var path []model.BeatInfo
	nodes := map[model.NodeID]model.Node{}
	var list []model.NodeID
	for i := 0; i < 4; i++ {
		id := graph.AddNode(i, 0, model.NodeTypeRegular)
		if i == 0 {
			graph.StartNodeID = id
		}
		path = append(path, model.BeatInfo{NodeID: id, NodeType: model.NodeTypeRegular, I: i})
		nodes[id] = graph.Nodes[id]
		list = append(list, id)
	}
	p := NewPredictor(graph, nil)
	p.SetPaths([][]model.BeatInfo{path}, []bool{true}, []int{0}, nodes)
	return p, list
}

// The audio lane fingerprints the predictor by Epoch: it must move on every
// change that can alter an already-computed cell, and must NOT move when
// Ensure merely extends forward (or the lane would re-reconcile every tick).
func TestPredictorEpochBumpsOnChangeNotOnExtend(t *testing.T) {
	p, ids := newLoopPredictorForTest(t)
	e0 := p.Epoch()
	p.Ensure(64)
	if p.Epoch() != e0 {
		t.Fatal("Ensure must not bump the epoch")
	}
	n := p.graph.Nodes[ids[0]]
	n.Params.Volume = 0.5
	p.UpdateNode(ids[0], n)
	if p.Epoch() == e0 {
		t.Fatal("UpdateNode must bump the epoch")
	}
	e1 := p.Epoch()
	p.RebaseAt(8)
	if p.Epoch() == e1 {
		t.Fatal("RebaseAt must bump the epoch")
	}
}

// DecisionAt exposes the custom Params.Logic decision (volume/pitch/duration
// multipliers) the predictor evaluated for an audible cell, so the audio
// lane's pure note function can apply it without re-running stateful logic.
func TestPredictorDecisionAtRecordsCustomLogic(t *testing.T) {
	p, ids := newLoopPredictorForTest(t)
	n := p.graph.Nodes[ids[1]]
	n.Params.Logic = func(c model.NodeContext) model.NodeDecision {
		return model.NodeDecision{VolumeMul: 0.25, PitchDelta: 2}
	}
	p.UpdateNode(ids[1], n)
	p.Ensure(16)
	dec, ok := p.DecisionAt(0, 1)
	if !ok || dec.VolumeMul != 0.25 || dec.PitchDelta != 2 {
		t.Fatalf("DecisionAt(0,1)=%+v ok=%v", dec, ok)
	}
	if _, ok := p.DecisionAt(0, 0); ok {
		t.Fatal("a cell without custom logic must report ok=false")
	}
	// A later disabling decision must clear the recorded one.
	off := false
	n.Params.Logic = func(c model.NodeContext) model.NodeDecision { return model.NodeDecision{Enabled: &off} }
	p.UpdateNode(ids[1], n)
	p.Ensure(16)
	if _, ok := p.DecisionAt(0, 1); ok {
		t.Fatal("a suppressed cell must not keep a stale decision")
	}
}
