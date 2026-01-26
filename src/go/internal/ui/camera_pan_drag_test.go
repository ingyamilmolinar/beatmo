package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestCameraPanDrag moves the camera by dragging and verifies that node and
// edge screen positions translate by the same delta across the full Update path.
func TestCameraPanDrag(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	n0 := g.tryAddNode(6, 4, model.NodeTypeRegular)
	n1 := g.tryAddNode(10, 4, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	// Capture initial screen center of n0
	x1, y1, x2, y2 := g.nodeScreenRect(n0)
	cx := (x1 + x2) * 0.5
	cy := (y1 + y2) * 0.5

	// Simulate press and drag by (+15,+9) pixels in the top pane.
	mx, my := int(math.Round(cx)), int(math.Round(cy))
	dx, dy := 15, 9
	pressed := true
	cur := func() (int, int) { return mx, my }
	mouse := func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft }
	restore := SetInputForTest(cur, mouse, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	defer restore()

	// Mouse down frame
	_ = g.Update()
	// Move cursor while held
	mx += dx
	my += dy
	_ = g.Update()
	// Release
	pressed = false
	_ = g.Update()

	nx1, ny1, nx2, ny2 := g.nodeScreenRect(n0)
	ncx := (nx1 + nx2) * 0.5
	ncy := (ny1 + ny2) * 0.5
	if math.Abs((ncx-cx)-float64(dx)) > 0.6 || math.Abs((ncy-cy)-float64(dy)) > 0.6 {
		t.Fatalf("pan drag mismatch: moved by (%.2f,%.2f) want (%d,%d)", ncx-cx, ncy-cy, dx, dy)
	}

	// Verify edge endpoint stayed colocated with n0 after drag.
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	camScale := unitPx / g.grid.Unit()
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	ex, ey := n0.X*camScale+offX, n0.Y*camScale+offY+float64(topOffset)
	if math.Abs(ex-ncx) > 0.6 || math.Abs(ey-ncy) > 0.6 {
		t.Fatalf("edge/node diverged after pan: edge(%.2f,%.2f) node(%.2f,%.2f)", ex, ey, ncx, ncy)
	}
}
