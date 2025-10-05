package ui

import (
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Test that baseline edges remain visible when the cache is disabled and the
// connection animation has completed (t >= 1). This protects against the bug
// where edges vanished in no-cache/safe modes.
func TestEdgeVisibleWithoutCache(t *testing.T) {
	old := os.Getenv("NO_EDGE_CACHE")
	os.Setenv("NO_EDGE_CACHE", "1")
	defer os.Setenv("NO_EDGE_CACHE", old)

	g := New(testLogger)
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
