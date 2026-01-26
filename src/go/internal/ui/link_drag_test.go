package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Test that releasing a link drag over an empty intersection (no regular node)
// does not create an edge. In particular, releasing over an invisible pass-through
// node should not create an edge either.
func TestLinkDragIgnoresEmptyAndInvisibleTargets(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a horizontal edge with an invisible pass-through node at (1,0).
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b) // creates invisible at (1,0)

	// Count current edges in graph map (should be 1 original).
	baseEdges := len(g.graph.Edges)
	if baseEdges != 1 {
		t.Fatalf("expected 1 edge, got %d", baseEdges)
	}

	// Begin link drag from node a with shift held.
	shift := true
	left := true
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return left && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool {
			if shift && (k == ebiten.KeyShiftLeft || k == ebiten.KeyShiftRight) {
				return true
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	// Start drag.
	g.handleLinkDrag(true, false, 0, 0, a.I, a.J)

	// Release over invisible pass-through at (1,0) with shift released.
	shift = false
	left = false
	g.handleLinkDrag(false, false, 0, 0, 1, 0)

	// Verify no new edges were created.
	if len(g.graph.Edges) != baseEdges {
		t.Fatalf("edge created to invisible node; edges %d -> %d", baseEdges, len(g.graph.Edges))
	}

	// Start another drag, then release over an empty location (no node).
	shift = true
	left = true
	g.handleLinkDrag(true, false, 0, 0, a.I, a.J)
	shift = false
	left = false
	g.handleLinkDrag(false, false, 0, 0, 5, 5)

	if len(g.graph.Edges) != baseEdges {
		t.Fatalf("edge created to empty space; edges %d -> %d", baseEdges, len(g.graph.Edges))
	}
}
