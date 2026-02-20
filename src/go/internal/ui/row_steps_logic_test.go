package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// helper to bootstrap a simple game with bounds and start node at (0,0)
func newGameWithStart(t *testing.T) *Game {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n0.Start = true
	g.start = n0
	g.graph.StartNodeID = n0.ID
	return g
}

func TestRowStepsExcludeSilentInvisible(t *testing.T) {
	assertDefaultParityState(t)
	g := newGameWithStart(t)
	a := g.start
	s := g.tryAddNode(1, 0, model.NodeTypeSilent)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, s)
	g.addEdge(s, b)
	g.addEdge(b, a) // close loop
	g.updateBeatInfos()

	steps := g.drum.Rows[0].Steps
	// Check first few positions based on constructed path with a seam:
	// idx0:A=true, idx1:Silent=false, idx2:B=true, idx3:seam invisible=false, idx4:A=true
	if !steps[0] || steps[1] || !steps[2] || steps[3] || !steps[4] {
		t.Fatalf("unexpected early steps: %v", steps[:5])
	}
}

func TestRowStepsEveryNTriggers(t *testing.T) {
	assertDefaultParityState(t)
	g := newGameWithStart(t)
	a := g.start
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	// A triggers every 2nd time only
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()
	steps := g.drum.Rows[0].Steps
	// Key check: first A suppressed
	if steps[0] != false {
		t.Fatalf("every_n_triggers expected A at idx0 suppressed, got %v", steps[:4])
	}
}

func TestRowStepsSkipEveryN(t *testing.T) {
	assertDefaultParityState(t)
	g := newGameWithStart(t)
	a := g.start
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	// B skip every 1 (always skip)
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 1
		g.graph.SetNodeParams(b.ID, p)
	}
	g.updateBeatInfos()
	steps := g.drum.Rows[0].Steps
	// B should be suppressed; A remains true
	if steps[0] != true || steps[1] != false {
		t.Fatalf("skip_every_n unexpected early steps %v", steps[:3])
	}
}

func TestRowStepsPrevTriggeredAndPrevSkipped(t *testing.T) {
	assertDefaultParityState(t)
	g := newGameWithStart(t)
	a := g.start
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, a)
	// Case 1: C requires previous triggered; with A,B regular, expect [true,true,true]
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(c.ID, p)
	}
	g.updateBeatInfos()
	steps := append([]bool(nil), g.drum.Rows[0].Steps...)
	// Find first C occurrence and ensure it's audible when preceded by triggered B
	found := false
	for i := 0; i < 6 && i < len(steps); i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == c.ID {
			if !steps[i] {
				t.Fatalf("prev_triggered: C not audible at idx %d: %v", i, steps[:i+1])
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("prev_triggered: C not found in first 6 steps")
	}
	// Case 2: Make B always skipped; then C requires previous skipped => true only at C
	if n, ok := g.graph.GetNodeByID(b.ID); ok {
		p := n.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 1
		g.graph.SetNodeParams(b.ID, p)
	}
	if n, ok := g.graph.GetNodeByID(c.ID); ok {
		p := n.Params
		p.LogicKind = "trigger_if_prev_skipped"
		g.graph.SetNodeParams(c.ID, p)
	}
	g.updateBeatInfos()
	steps2 := g.drum.Rows[0].Steps
	found2 := false
	for i := 0; i < 6 && i < len(steps2); i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == c.ID {
			if !steps2[i] {
				t.Fatalf("prev_skipped: C not audible at idx %d: %v", i, steps2[:i+1])
			}
			found2 = true
			break
		}
	}
	if !found2 {
		t.Fatalf("prev_skipped: C not found in first 6 steps")
	}
}

func TestRowStepsProbabilityEdgeCases(t *testing.T) {
	assertDefaultParityState(t)
	g := newGameWithStart(t)
	a := g.start
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	// A p=0 => false, B regular => true
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 0
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()
	steps := g.drum.Rows[0].Steps
	// Ensure A does not trigger in the first few occurrences
	for i := 0; i < 6 && i < len(steps); i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == a.ID && steps[i] {
			t.Fatalf("p=0: A triggered at %d", i)
		}
	}
	// A p=1 => true
	if n, ok := g.graph.GetNodeByID(a.ID); ok {
		p := n.Params
		p.LogicKind = "probability"
		p.LogicP = 1
		g.graph.SetNodeParams(a.ID, p)
	}
	g.updateBeatInfos()
	steps2 := g.drum.Rows[0].Steps
	// Ensure A triggers at its first two occurrences
	count := 0
	for i := 0; i < 8 && i < len(steps2); i++ {
		bi := g.beatInfoAtRow(0, i)
		if bi.NodeID == a.ID {
			if !steps2[i] {
				t.Fatalf("p=1: A not triggered at %d", i)
			}
			count++
			if count == 2 {
				break
			}
		}
	}
}
