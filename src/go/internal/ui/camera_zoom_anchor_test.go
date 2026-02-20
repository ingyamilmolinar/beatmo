package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestZoomAnchorsAtCursor exercises the full Game.Update input path to ensure
// wheel zoom keeps the world point under the cursor fixed in screen space, and
// that nodes and edges remain colocated after zoom.
func TestZoomAnchorsAtCursor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Simple horizontal edge.
	a := g.tryAddNode(5, 3, model.NodeTypeRegular)
	b := g.tryAddNode(11, 3, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.updateBeatInfos()

	// Initial screen position of node a.
	x1, y1, x2, y2 := g.nodeScreenRect(a)
	cx := int(math.Round((x1 + x2) * 0.5))
	cy := int(math.Round((y1 + y2) * 0.5))

	// Wheel zoom in at the node center. Provide input through SetInputForTest
	// so Game.Update() drives zoom via zoomAtScreen and Camera.HandleMouse.
	wheelY := 1.0
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return cx, cy }, // cursor at node center
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wheelY }, // one notch zoom in
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Apply one update to process the wheel.
	_ = g.Update()

	// After zoom, the node should still be under the cursor (same screen px).
	nx1, ny1, nx2, ny2 := g.nodeScreenRect(a)
	ncx := (nx1 + nx2) * 0.5
	ncy := (ny1 + ny2) * 0.5
	tol := 1.0
	if math.Abs(ncx-float64(cx)) > tol || math.Abs(ncy-float64(cy)) > tol {
		t.Fatalf("anchored zoom failed: node moved to (%.2f,%.2f) want (%d,%d)", ncx, ncy, cx, cy)
	}

	// Also verify the connected edge endpoint projects to the same screen pos.
	// Rebuild the same projection used in Draw.
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	camScale := unitPx / g.grid.Unit()
	ex, ey := a.X*camScale+offX, a.Y*camScale+offY+float64(gridTopOffset())
	if math.Abs(ex-ncx) > tol || math.Abs(ey-ncy) > tol {
		t.Fatalf("edge/node diverged after zoom: edge(%.2f,%.2f) node(%.2f,%.2f)", ex, ey, ncx, ncy)
	}
}
