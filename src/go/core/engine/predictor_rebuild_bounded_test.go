package engine

import (
	"io"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// These tests pin the bounded dirty-rebuild optimization: a node-param edit must
// NOT cost O(windowStart) (session length). The cost was the deterministic root
// cause of "choppy when I add node rules" (see internal/audio node-logic cost
// table). The optimization must be byte-identical to the full re-walk.

func buildRebuildCircuit(t *testing.T, profile string) (*Predictor, *model.Graph, model.NodeID) {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	const n = 12
	ids := make([]model.NodeID, n)
	for k := range ids {
		ids[k] = graph.AddNode(0, k, model.NodeTypeRegular)
	}
	for k := range ids {
		graph.Edges[[2]model.NodeID{ids[k], ids[(k+1)%n]}] = struct{}{}
	}
	graph.StartNodeID = ids[0]
	set := func(idx int, kind string, ln int, lp float64) {
		node, _ := graph.GetNodeByID(ids[idx])
		node.Params.LogicKind, node.Params.LogicN, node.Params.LogicP = kind, ln, lp
		graph.SetNodeParams(ids[idx], node.Params)
	}
	switch profile {
	case "static":
	case "every_n":
		set(2, "every_n_triggers", 2, 0)
		set(5, "every_n_triggers", 3, 0)
	case "skip_every_n":
		set(3, "skip_every_n", 2, 0)
		set(7, "skip_every_n", 4, 0)
	case "probability":
		set(4, "probability", 0, 0.5)
		set(9, "probability", 0, 0.25)
	case "prev_chain":
		for k := 1; k < n; k++ {
			set(k, "trigger_if_prev_triggered", 0, 0)
		}
	case "mixed":
		set(2, "every_n_triggers", 2, 0)
		set(4, "skip_every_n", 3, 0)
		set(6, "probability", 0, 0.5)
		set(8, "trigger_if_prev_triggered", 0, 0)
		set(10, "trigger_if_prev_skipped", 0, 0)
	default:
		t.Fatalf("unknown profile %q", profile)
	}

	pred := NewPredictor(graph, nil)
	path, isLoop, loopStart := graph.CalculateBeatRow()
	nodes := make(map[model.NodeID]model.Node, len(graph.Nodes))
	for id, node := range graph.Nodes {
		nodes[id] = node
	}
	pred.SetPaths([][]model.BeatInfo{path}, []bool{isLoop}, []int{loopStart}, nodes)
	for id, node := range graph.Nodes {
		pred.UpdateNode(id, node)
	}
	return pred, graph, ids[0]
}

func dirtyEditEngine(pred *Predictor, graph *model.Graph, id model.NodeID) {
	node, _ := graph.GetNodeByID(id)
	node.Params.LogicN++ // any param change dirties the predictor
	graph.SetNodeParams(id, node.Params)
	pred.UpdateNode(id, node)
}

// advanceDirtyEnsure pushes windowStart to ~ws, dirties one node, and runs the
// rebuild Ensure. Returns the predictor (post-rebuild) and the window bounds.
func advanceDirtyEnsure(t *testing.T, profile string, ws int, forceFull bool) (*Predictor, int, int) {
	t.Helper()
	pred, graph, id := buildRebuildCircuit(t, profile)
	pred.SetForceFullRebuildForTest(forceFull)
	horizon := ws + 4096
	pred.Ensure(horizon) // slide window forward (simulate playback)
	dirtyEditEngine(pred, graph, id)
	pred.Ensure(horizon) // the rebuild under test
	start, end, _ := pred.WindowBoundsForTest()
	return pred, start, end
}

// TestDirtyRebuildEquivalentToFullWalk: the bounded rebuild produces byte-for-
// byte identical audible/visible/triggered buffers as the full [0,windowStart)
// re-walk, across every logic profile and a range of session lengths.
func TestDirtyRebuildEquivalentToFullWalk(t *testing.T) {
	profiles := []string{"static", "every_n", "skip_every_n", "probability", "prev_chain", "mixed"}
	starts := []int{1000, 5000, 20000, 70000}
	for _, prof := range profiles {
		for _, ws := range starts {
			ref, rs, re := advanceDirtyEnsure(t, prof, ws, true)  // full walk
			opt, os, oe := advanceDirtyEnsure(t, prof, ws, false) // bounded
			if rs != os || re != oe {
				t.Fatalf("%s ws=%d: window mismatch full=[%d,%d) opt=[%d,%d)", prof, ws, rs, re, os, oe)
			}
			for idx := os; idx < oe; idx++ {
				if ref.AudibleAt(0, idx) != opt.AudibleAt(0, idx) {
					t.Fatalf("%s ws=%d: AudibleAt(%d) full=%v opt=%v", prof, ws, idx, ref.AudibleAt(0, idx), opt.AudibleAt(0, idx))
				}
				if ref.VisibleAt(0, idx) != opt.VisibleAt(0, idx) {
					t.Fatalf("%s ws=%d: VisibleAt(%d) full=%v opt=%v", prof, ws, idx, ref.VisibleAt(0, idx), opt.VisibleAt(0, idx))
				}
				if ref.TriggeredAt(0, idx) != opt.TriggeredAt(0, idx) {
					t.Fatalf("%s ws=%d: TriggeredAt(%d) full=%v opt=%v", prof, ws, idx, ref.TriggeredAt(0, idx), opt.TriggeredAt(0, idx))
				}
			}
		}
	}
}

// TestDirtyRebuildBoundedByWindowCap: rebuild cost must NOT scale with session
// length. Pre-optimization it was ~14-18x from windowStart 4096→131072; bounded
// it should be roughly flat.
func TestDirtyRebuildBoundedByWindowCap(t *testing.T) {
	const reps = 9
	timeRebuild := func(ws int) time.Duration {
		var best time.Duration = time.Hour
		for r := 0; r < reps; r++ {
			pred, graph, id := buildRebuildCircuit(t, "prev_chain")
			horizon := ws + 4096
			pred.Ensure(horizon)
			dirtyEditEngine(pred, graph, id)
			t0 := time.Now()
			pred.Ensure(horizon)
			if d := time.Since(t0); d < best {
				best = d
			}
		}
		return best
	}
	small := timeRebuild(4096)
	large := timeRebuild(131072)
	t.Logf("dirty rebuild: ws=4096 %v, ws=131072 %v (ratio %.1fx)", small, large, float64(large)/float64(small))
	if large > 4*small {
		t.Fatalf("rebuild scales with windowStart (not bounded): ws=4096 %v vs ws=131072 %v", small, large)
	}
}
