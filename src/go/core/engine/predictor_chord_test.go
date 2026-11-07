package engine

import (
	"io"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The third node depends on the second through trigger_if_prev_triggered and the second skips every 2nd trigger, so
// the logic walk reads the second node's type.
func chordLoopPredictor(t *testing.T, typ model.NodeType) *Predictor {
	t.Helper()
	graph := model.NewGraph(game_log.New(io.Discard, game_log.LevelError))
	a := graph.AddNode(0, 0, model.NodeTypeRegular)
	b := graph.AddNode(1, 0, typ)
	c := graph.AddNode(2, 0, model.NodeTypeRegular)
	d := graph.AddNode(3, 0, model.NodeTypeMute)
	graph.Edges[[2]model.NodeID{a, b}] = struct{}{}
	graph.Edges[[2]model.NodeID{b, c}] = struct{}{}
	graph.Edges[[2]model.NodeID{c, d}] = struct{}{}
	graph.Edges[[2]model.NodeID{d, a}] = struct{}{}
	graph.StartNodeID = a
	graph.SetNodeParams(b, model.NodeParams{Volume: 0.7, Pitch: 3, Duration: 1,
		LogicKind: model.LogicSkipEveryN, LogicN: 2, Chord: model.ChordSpec{Quality: "m7"}})
	graph.SetNodeParams(c, model.NodeParams{Volume: 1, Duration: 1, LogicKind: model.LogicIfPrevTriggered})

	pred := NewPredictor(graph, nil)
	path, isLoop, loopStart := graph.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, n := range graph.Nodes {
		nodes[id] = n
	}
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	pred.Ensure(64)
	return pred
}

// The timeline cannot tell a chord from a regular node: every decision the
// predictor makes must be identical (the voice stack only matters where a
// note becomes sound).
func TestPredictorTreatsChordExactlyLikeRegular(t *testing.T) {
	reg := chordLoopPredictor(t, model.NodeTypeRegular)
	chord := chordLoopPredictor(t, model.NodeTypeChord)
	audible := 0
	for abs := 0; abs < 64; abs++ {
		if reg.AudibleAt(0, abs) != chord.AudibleAt(0, abs) ||
			reg.VisibleAt(0, abs) != chord.VisibleAt(0, abs) ||
			reg.TriggeredAt(0, abs) != chord.TriggeredAt(0, abs) {
			t.Fatalf("abs %d: regular (aud=%v vis=%v trig=%v) vs chord (aud=%v vis=%v trig=%v)", abs,
				reg.AudibleAt(0, abs), reg.VisibleAt(0, abs), reg.TriggeredAt(0, abs),
				chord.AudibleAt(0, abs), chord.VisibleAt(0, abs), chord.TriggeredAt(0, abs))
		}
		rd, rok := reg.DecisionAt(0, abs)
		cd, cok := chord.DecisionAt(0, abs)
		if rok != cok || rd.VolumeMul != cd.VolumeMul || rd.PitchDelta != cd.PitchDelta || rd.DurationMul != cd.DurationMul {
			t.Fatalf("abs %d: decisions differ: %+v/%v vs %+v/%v", abs, rd, rok, cd, cok)
		}
		if reg.AudibleAt(0, abs) {
			audible++
		}
	}
	if audible == 0 {
		t.Fatal("vacuous: the loop never sounded")
	}
}
