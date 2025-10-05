package ui

import (
	"image/color"
	"os"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Test that arrowheads are drawn in the screen-space edge path when the
// connection is complete (t >= 1).
func TestScreenEdgesDrawArrows(t *testing.T) {
	old := os.Getenv("SCREEN_EDGES")
	os.Setenv("SCREEN_EDGES", "1")
	defer os.Setenv("SCREEN_EDGES", old)

	g := New(testLogger)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, 0)
	b := g.tryAddNode(1, 0, 0)
	g.addEdge(a, b)
	g.edges[0].t = 1

	// Count line draws; in SCREEN_EDGES path, baseline uses drawRect, but
	// arrowheads use DrawLineCam (drawEdgeLine).
	draws := 0
	oldDraw := drawEdgeLine
	drawEdgeLine = func(dst *ebiten.Image, x1, y1, x2, y2 float64, cam *ebiten.GeoM, col color.Color, thick float64) {
		draws++
	}
	defer func() { drawEdgeLine = oldDraw }()

	g.Draw(ebiten.NewImage(800, 600))
	if draws < 2 {
		t.Fatalf("expected arrowheads in screen-space path; got %d line draws", draws)
	}
}
