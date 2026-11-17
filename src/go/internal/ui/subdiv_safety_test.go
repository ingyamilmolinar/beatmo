package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// buildTestGame initializes a game with frozen input for UI tests.
func buildTestGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)
	return g
}

// Ensure lowering from 32->4 is denied when any explicit node does not align
// to the coarser grid (e.g. 1/8 at 32-subdiv is 4 steps, not multiple of 8).
func TestSetSubdivDisallowUnalignedDownscale(t *testing.T) {
	g := buildTestGame(t)
	if err := g.SetSubdivisions(32); err != nil {
		t.Fatalf("set 32: %v", err)
	}
	// Build a simple segment with a node at 1/8th beat (4 steps at 32/subdiv).
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(4, 0, model.NodeTypeRegular) // 1/8th
	g.addEdge(n0, n1)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	// Sanity
	if div := g.grid.MaxDiv(); div != 32 {
		t.Fatalf("pre div=%d want 32", div)
	}
	// Stop playback if it was toggled; ensure not playing
	stopPlaybackForTest(g)
	// Attempt to lower to 4.
	if err := g.SetSubdivisions(4); err == nil {
		t.Fatalf("expected error lowering to 4 with misaligned nodes")
	}
	// Verify grid/timeline unchanged and nodes untouched.
	if g.grid.MaxDiv() != 32 || g.drum.timelineUnitsPerBeat != 32 {
		t.Fatalf("grid/timeline changed on denied change: grid=%d timeline=%d", g.grid.MaxDiv(), g.drum.timelineUnitsPerBeat)
	}
	if nn, ok := g.graph.GetNodeByID(n1.ID); !ok || nn.I != 4 || nn.J != 0 {
		t.Fatalf("node mutated on denied change: %+v", nn)
	}
}

// Lowering from 32->4 is allowed when all explicit nodes are aligned
// to multiples of 8 (factor=32/4).
func TestSetSubdivAllowAlignedDownscale(t *testing.T) {
	g := buildTestGame(t)
	if err := g.SetSubdivisions(32); err != nil {
		t.Fatalf("set 32: %v", err)
	}
	// Build aligned nodes at 0, 8, 16 (multiples of 8 at 32/subdiv).
	g.pendingStartRow = 0
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	c := g.tryAddNode(16, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	// Lower to 4 should succeed.
	if err := g.SetSubdivisions(4); err != nil {
		t.Fatalf("unexpected error lowering to 4: %v", err)
	}
	if g.grid.MaxDiv() != 4 || g.drum.timelineUnitsPerBeat != 4 {
		t.Fatalf("grid/timeline not updated: grid=%d timeline=%d", g.grid.MaxDiv(), g.drum.timelineUnitsPerBeat)
	}
	// Nodes should be scaled exactly: 8->1, 16->2
	if nb, _ := g.graph.GetNodeByID(b.ID); nb.I != 1 {
		t.Fatalf("b.I=%d want 1", nb.I)
	}
	if nc, _ := g.graph.GetNodeByID(c.ID); nc.I != 2 {
		t.Fatalf("c.I=%d want 2", nc.I)
	}
}

// Verify J-axis (vertical) alignment is also enforced when lowering.
func TestSetSubdivDisallowUnalignedVertical(t *testing.T) {
	g := buildTestGame(t)
	if err := g.SetSubdivisions(32); err != nil {
		t.Fatalf("set 32: %v", err)
	}
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	v := g.tryAddNode(0, 4, model.NodeTypeRegular) // 1/8th vertically
	g.addEdge(n0, v)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	if err := g.SetSubdivisions(4); err == nil {
		t.Fatalf("expected error lowering to 4 with vertical misalignment")
	}
	if vv, _ := g.graph.GetNodeByID(v.ID); vv.J != 4 {
		t.Fatalf("v.J changed on denied change: %d", vv.J)
	}
}
