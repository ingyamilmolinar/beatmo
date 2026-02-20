package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// screenPosForGrid returns a screen-space coordinate that maps to the given
// grid subdivision (i,j) under the current camera/grid.
func screenPosForGrid(g *Game, i, j int) (x, y int) {
	unit := g.grid.Unit()
	wx := float64(i) * unit
	wy := float64(j) * unit
	sx := int(g.cam.OffsetX + g.cam.Scale*wx)
	sy := int(g.cam.OffsetY + float64(gridTopOffset()) + g.cam.Scale*wy)
	return sx, sy
}

// After lowering subdivisions, adding and removing nodes via mouse should still work.
func TestNodeAddRemoveAfterSubdivChange(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.SetSubdivisions(32); err != nil {
		t.Fatalf("set 32: %v", err)
	}
	// Aligned base so we can lower to 8 safely.
	g.pendingStartRow = 0
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("set 8: %v", err)
	}

	// Compute screen coords for a new node at (4,0) in 8-sub grid.
	tx, ty := screenPosForGrid(g, 4, 0)
	// Simulate left click press and release to create node.
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return tx, ty },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)
	_ = g.Update() // mouse down
	pressed = false
	_ = g.Update() // mouse up -> add
	restore()
	if n := g.nodeAt(4, 0); n == nil {
		t.Fatalf("expected node added at (4,0) after subdiv change")
	}
	// Close the sidebar so it doesn't absorb the right-click.
	g.sidebar.Close()
	g.sidebar.closedGuard = 0 // clear guard so next click isn't blocked
	// Now delete it via right-click (press-only is enough for delete path).
	restore2 := SetInputForTest(
		func() (int, int) { return tx, ty },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonRight },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore2)
	_ = g.Update()
	restore2()
	_ = g.Update()
	if n := g.nodeAt(4, 0); n != nil {
		t.Fatalf("expected node removed at (4,0) after right-click")
	}
}
