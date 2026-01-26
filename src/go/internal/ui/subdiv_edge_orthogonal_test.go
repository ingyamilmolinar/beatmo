package ui

import (
	"fmt"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	assets_pkg "github.com/ingyamilmolinar/tunkul/internal/assets"
)

// edgeInfo captures edge state for comparison before/after subdivision changes.
type edgeInfo struct {
	FromID, ToID model.NodeID
	FromI, FromJ int
	ToI, ToJ     int
}

// isOrthogonal returns true if the edge is horizontal or vertical (not diagonal).
func (e edgeInfo) isOrthogonal() bool {
	return e.FromI == e.ToI || e.FromJ == e.ToJ
}

// timingDistance returns the distance along the primary axis (the one that differs).
// For horizontal edges, this is abs(ToI - FromI); for vertical, abs(ToJ - FromJ).
func (e edgeInfo) timingDistance() int {
	di := e.ToI - e.FromI
	if di < 0 {
		di = -di
	}
	dj := e.ToJ - e.FromJ
	if dj < 0 {
		dj = -dj
	}
	if di > dj {
		return di
	}
	return dj
}

// captureEdges extracts all edges from the game's graph.
func captureEdges(g *Game) []edgeInfo {
	var edges []edgeInfo
	for pair := range g.graph.Edges {
		fromID, toID := pair[0], pair[1]
		fromNode, okFrom := g.graph.Nodes[fromID]
		toNode, okTo := g.graph.Nodes[toID]
		if !okFrom || !okTo {
			continue
		}
		edges = append(edges, edgeInfo{
			FromID: fromID,
			ToID:   toID,
			FromI:  fromNode.I,
			FromJ:  fromNode.J,
			ToI:    toNode.I,
			ToJ:    toNode.J,
		})
	}
	return edges
}

// formatEdges returns a human-readable string for debugging.
func formatEdges(edges []edgeInfo) string {
	var s string
	for _, e := range edges {
		ortho := "orthogonal"
		if !e.isOrthogonal() {
			ortho = "DIAGONAL"
		}
		s += fmt.Sprintf("  %d->%d: (%d,%d)->(%d,%d) [%s]\n",
			e.FromID, e.ToID, e.FromI, e.FromJ, e.ToI, e.ToJ, ortho)
	}
	return s
}

// TestSubdivisionResizePreservesOrthogonalEdges verifies that changing
// subdivision from 8 to 16 keeps all edges orthogonal.
// This test is expected to FAIL until the bug in SetSubdivisions is fixed.
func TestSubdivisionResizePreservesOrthogonalEdges(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Import the startup demo (subdiv=8)
	if len(assets_pkg.StartupDemoJSON) == 0 {
		t.Fatalf("startup demo JSON missing")
	}
	if err := g.Import(assets_pkg.StartupDemoJSON); err != nil {
		t.Fatalf("import startup demo: %v", err)
	}

	// Verify initial subdivision is 8
	if div := g.grid.MaxDiv(); div != 8 {
		t.Fatalf("expected initial subdiv=8, got %d", div)
	}

	// Capture all edges before subdivision change
	edgesBefore := captureEdges(g)
	if len(edgesBefore) == 0 {
		t.Fatalf("no edges found in startup demo")
	}

	// Verify all edges are orthogonal before the change
	for _, e := range edgesBefore {
		if !e.isOrthogonal() {
			t.Fatalf("edge %d->%d is not orthogonal BEFORE subdivision change: (%d,%d)->(%d,%d)",
				e.FromID, e.ToID, e.FromI, e.FromJ, e.ToI, e.ToJ)
		}
	}

	// Build a map of timing distances (by edge ID pair) for comparison
	timingBefore := make(map[[2]model.NodeID]int)
	for _, e := range edgesBefore {
		key := [2]model.NodeID{e.FromID, e.ToID}
		timingBefore[key] = e.timingDistance()
	}

	// Stop playback if running (required for SetSubdivisions)
	stopPlaybackForTest(g)

	// Change subdivision from 8 to 16
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("SetSubdivisions(16) failed: %v", err)
	}

	// Verify subdivision changed
	if div := g.grid.MaxDiv(); div != 16 {
		t.Fatalf("expected subdiv=16 after change, got %d", div)
	}

	// Capture edges after subdivision change
	edgesAfter := captureEdges(g)
	if len(edgesAfter) != len(edgesBefore) {
		t.Fatalf("edge count changed: before=%d, after=%d", len(edgesBefore), len(edgesAfter))
	}

	// Verify all edges remain orthogonal after the change
	var diagonalEdges []edgeInfo
	for _, e := range edgesAfter {
		if !e.isOrthogonal() {
			diagonalEdges = append(diagonalEdges, e)
		}
	}

	if len(diagonalEdges) > 0 {
		t.Errorf("found %d diagonal edge(s) after subdivision change 8->16:\n%s",
			len(diagonalEdges), formatEdges(diagonalEdges))
		t.Logf("all edges after change:\n%s", formatEdges(edgesAfter))
	}

	// Verify timing distances scaled correctly (should be 2x for 8->16)
	scaleFactor := 2 // 16/8
	for _, e := range edgesAfter {
		key := [2]model.NodeID{e.FromID, e.ToID}
		before, ok := timingBefore[key]
		if !ok {
			t.Errorf("edge %d->%d not found in before map", e.FromID, e.ToID)
			continue
		}
		expected := before * scaleFactor
		actual := e.timingDistance()
		if actual != expected {
			t.Errorf("edge %d->%d timing distance: expected %d (was %d * %d), got %d",
				e.FromID, e.ToID, expected, before, scaleFactor, actual)
		}
	}
}

// TestSubdivisionResizeGrooveNodesSpecific creates an isolated test case
// matching the problematic nodes 25 and 26 from the startup demo.
// These nodes have different groove settings that cause diagonal edges
// after subdivision change.
func TestSubdivisionResizeGrooveNodesSpecific(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Set initial subdivision to 8
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions(8) failed: %v", err)
	}

	// Create two nodes matching startup demo nodes 25 and 26:
	// - Node 25: i=-33, j=-1, groove_kind="rush", groove_pct=0 (default)
	// - Node 26: i=-31, j=-1, groove_kind="delay", groove_pct=0.3
	// Both share j=-1 (horizontal edge)
	g.pendingStartRow = 0

	nodeA := g.tryAddNode(-33, -1, model.NodeTypeRegular)
	if nodeA == nil {
		t.Fatalf("failed to create nodeA")
	}
	// Set node A params: rush groove with pct=0 (default behavior)
	if na, ok := g.graph.Nodes[nodeA.ID]; ok {
		na.Params.GrooveKind = "rush"
		na.Params.GroovePct = 0
		g.graph.Nodes[nodeA.ID] = na
	}

	nodeB := g.tryAddNode(-31, -1, model.NodeTypeRegular)
	if nodeB == nil {
		t.Fatalf("failed to create nodeB")
	}
	// Set node B params: delay groove with pct=0.3
	if nb, ok := g.graph.Nodes[nodeB.ID]; ok {
		nb.Params.GrooveKind = "delay"
		nb.Params.GroovePct = 0.3
		g.graph.Nodes[nodeB.ID] = nb
	}

	// Create edge A -> B (horizontal)
	g.addEdge(nodeA, nodeB)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Verify edge is orthogonal before change
	nA, _ := g.graph.Nodes[nodeA.ID]
	nB, _ := g.graph.Nodes[nodeB.ID]
	t.Logf("Before change: A(%d,%d) -> B(%d,%d)", nA.I, nA.J, nB.I, nB.J)

	if nA.J != nB.J {
		t.Fatalf("edge is not horizontal before change: A.J=%d, B.J=%d", nA.J, nB.J)
	}

	// Stop playback and change subdivision
	stopPlaybackForTest(g)
	if err := g.SetSubdivisions(16); err != nil {
		t.Fatalf("SetSubdivisions(16) failed: %v", err)
	}

	// Check edge after change
	nA, _ = g.graph.Nodes[nodeA.ID]
	nB, _ = g.graph.Nodes[nodeB.ID]
	t.Logf("After change: A(%d,%d) -> B(%d,%d)", nA.I, nA.J, nB.I, nB.J)

	// The edge should remain orthogonal (both nodes should have same J)
	if nA.J != nB.J {
		t.Errorf("edge became diagonal after subdivision change: A.J=%d, B.J=%d (expected same J)",
			nA.J, nB.J)
	}
	if nA.I == nB.I {
		t.Errorf("nodes collapsed to same I coordinate: A.I=%d, B.I=%d", nA.I, nB.I)
	}
}
