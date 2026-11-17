package ui

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Test that baseline edges remain visible once the connection animation
// completes (t >= 1) in the default cached path.
func TestEdgeVisibleWithCache(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a simple edge (0,0) -> (1,0)
	a := g.tryAddNode(0, 0, 0)
	b := g.tryAddNode(1, 0, 0)
	if a == nil || b == nil {
		t.Fatalf("failed to add nodes")
	}
	g.addEdge(a, b)
	if len(g.edges) == 0 {
		t.Fatalf("no edges added")
	}

	// Force connection animation to complete
	g.edges[0].t = 1

	// Intercept edge line draws
	draws := 0
	oldDraw := drawEdgeLine
	drawEdgeLine = func(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM, col color.Color, thick float64) {
		draws++
	}
	defer func() { drawEdgeLine = oldDraw }()

	g.Draw(ebiten.NewImage(800, 600))
	if draws == 0 {
		t.Fatalf("expected baseline edge draw with cache disabled; got %d", draws)
	}
}
