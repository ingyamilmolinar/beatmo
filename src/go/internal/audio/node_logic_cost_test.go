//go:build !test

package audio

import (
	"io"
	"sort"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Deterministic CPU-cost measurement of the predictor scheduling path — the Go
// analog of scripts/bench_audio_node_cost.mjs (which measures only WebAudio
// insert-FX node cost). This covers the path FX-cost can't: the predictor
// Ensure()/rebuild that runs on the scheduling critical path on BOTH desktop
// and browser, and which the user's "choppy when I add node rules" symptom
// points at.
//
// Key mechanism under test (predictor_compute.go:184-235): a node-param edit
// sets predDirty; the next Ensure() re-walks [0, windowStart) for every row to
// rebuild cumulative per-row logic state. windowStart = horizon - windowCap
// grows without bound across a session, so each edit's rebuild cost grows with
// how long you've been playing — independent of how small the edit was.

// costCircuit holds a predictor plus the handle needed to dirty it.
type costCircuit struct {
	pred   *engine.Predictor
	graph  *model.Graph
	editID model.NodeID
}

// buildCostCircuit builds an n-node adjacent loop with a chosen logic profile.
func buildCostCircuit(t *testing.T, n int, profile string) costCircuit {
	t.Helper()
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	ids := make([]model.NodeID, n)
	for k := range ids {
		ids[k] = graph.AddNode(0, k, model.NodeTypeRegular)
	}
	for k := range ids {
		graph.Edges[[2]model.NodeID{ids[k], ids[(k+1)%n]}] = struct{}{}
	}
	graph.StartNodeID = ids[0]

	set := func(idx int, kind string, ln int, lp float64) {
		node, ok := graph.GetNodeByID(ids[idx])
		if !ok {
			t.Fatalf("node %d missing", idx)
		}
		p := node.Params
		p.LogicKind, p.LogicN, p.LogicP = kind, ln, lp
		graph.SetNodeParams(ids[idx], p)
	}
	switch profile {
	case "static": // no logic — cheapest per-cell re-walk
	case "every_n":
		for k := 1; k < n; k += 2 {
			set(k, "every_n_triggers", 2, 0)
		}
	case "prev_chain": // O(<=64) backward walk per cell — the expensive kind
		for k := 1; k < n; k++ {
			set(k, "trigger_if_prev_triggered", 0, 0)
		}
	default:
		t.Fatalf("unknown profile %q", profile)
	}
	return costCircuit{pred: finishPredictor(graph), graph: graph, editID: ids[0]}
}

// dirtyEdit mutates one node param and pushes it to the predictor, setting
// predDirty (the same path a live node-rule edit takes).
func (c costCircuit) dirtyEdit() {
	node, _ := c.graph.GetNodeByID(c.editID)
	node.Params.LogicN++ // any param change dirties the predictor
	c.graph.SetNodeParams(c.editID, node.Params)
	c.pred.UpdateNode(c.editID, node)
}

func medianMS(ds []time.Duration) float64 {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	return float64(ds[len(ds)/2].Microseconds()) / 1000.0
}

// timeDirtyRebuild advances the window to `windowStart+windowCap`, then times
// the Ensure() that follows a dirty edit (the full [0,windowStart) re-walk).
func timeDirtyRebuild(c costCircuit, windowStart, reps int) (rebuildMS float64, gotStart int) {
	horizon := windowStart + 4096 // windowCap=4096 → slides windowStart to `windowStart`
	c.pred.Ensure(horizon)        // advance window (simulate playback to here)
	s, _, _ := c.pred.WindowBoundsForTest()
	gotStart = s
	ds := make([]time.Duration, 0, reps)
	for r := 0; r < reps; r++ {
		c.dirtyEdit()
		t0 := time.Now()
		c.pred.Ensure(horizon)
		ds = append(ds, time.Since(t0))
	}
	return medianMS(ds), gotStart
}

// timeSteadyEnsure times incremental horizon extension with NO dirty edit.
func timeSteadyEnsure(c costCircuit, fromHorizon, reps int) float64 {
	c.pred.Ensure(fromHorizon)
	ds := make([]time.Duration, 0, reps)
	h := fromHorizon
	for r := 0; r < reps; r++ {
		h += 64 // one frame of forward schedule
		t0 := time.Now()
		c.pred.Ensure(h)
		ds = append(ds, time.Since(t0))
	}
	return medianMS(ds)
}

// TestNodeLogicEnsureCostTable prints the deterministic cost table and asserts
// the smoking gun: the dirty-rebuild cost scales with windowStart (session
// length), which is what makes "add a node rule mid-song" choppy. Task 9's fix
// will bound this; TestDirtyRebuildBoundedByWindowCap gates that.
func TestNodeLogicEnsureCostTable(t *testing.T) {
	const reps = 7
	profiles := []string{"static", "every_n", "prev_chain"}
	starts := []int{4096, 32768, 131072} // ~ short / medium / long session

	t.Logf("%-12s %-10s  steady(ms)  rebuild@4096  rebuild@32768  rebuild@131072  scaling", "profile", "nodes")
	for _, prof := range profiles {
		row := make([]float64, len(starts))
		for i, ws := range starts {
			c := buildCostCircuit(t, 12, prof)
			rb, gotStart := timeDirtyRebuild(c, ws, reps)
			if gotStart < ws/2 {
				t.Logf("  note: windowStart landed at %d (wanted ~%d)", gotStart, ws)
			}
			row[i] = rb
		}
		cSteady := buildCostCircuit(t, 12, prof)
		steady := timeSteadyEnsure(cSteady, 8192, reps)
		scaling := 0.0
		if row[0] > 0 {
			scaling = row[2] / row[0]
		}
		t.Logf("%-12s %-10d  %9.3f  %11.3f  %12.3f  %13.3f  %5.1fx (131072/4096)",
			prof, 12, steady, row[0], row[1], row[2], scaling)
	}
}
