package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// click helper
func clickGame(g *Game, sx, sy int) {
	r1 := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	r1()
	r2 := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	r2()
}

// Ensure origin selection for a row ignores clicks on nodes belonging to a different circuit.
func TestOriginSelectIgnoresDifferentCircuitNode(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Circuit A (row 0): a0 -> a1
	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a0, a1)

	// Add row for Circuit B
	g.drum.AddRow()

	// Circuit B (row 1): b0 -> b1
	b0 := g.tryAddNode(0, 2, model.NodeTypeRegular)
	b1 := g.tryAddNode(2, 2, model.NodeTypeRegular)
	g.addEdge(b0, b1)
	g.updateBeatInfos()

	// Select origin mode for row 1, set to b0 first.
	g.drum.recalcButtons()
	g.drum.calcLayout()
	ob := g.drum.rowOriginBtns[1]
	or := ob.Rect()
	_ = ob.Handle((or.Min.X+or.Max.X)/2, (or.Min.Y+or.Max.Y)/2, true)
	_ = ob.Handle((or.Min.X+or.Max.X)/2, (or.Min.Y+or.Max.Y)/2, false)
	_ = g.Update()
	if g.pendingStartRow != 1 {
		t.Fatalf("pendingStartRow=%d want 1", g.pendingStartRow)
	}
	x1, y1, x2, y2 := g.nodeScreenRect(b0)
	clickGame(g, int(math.Round((x1+x2)/2)), int(math.Round((y1+y2)/2)))
	if g.drum.Rows[1].Origin != b0.ID {
		t.Fatalf("row1 origin=%d want %d", g.drum.Rows[1].Origin, b0.ID)
	}

	// Re-enter origin mode for row 1, set playing active; clicking a node from circuit A (a0)
	// should NOT change because row 0 is audible and playback is active.
	g.SetPlaying(true)
	_ = ob.Handle((or.Min.X+or.Max.X)/2, (or.Min.Y+or.Max.Y)/2, true)
	_ = ob.Handle((or.Min.X+or.Max.X)/2, (or.Min.Y+or.Max.Y)/2, false)
	_ = g.Update()
	if g.pendingStartRow != 1 {
		t.Fatalf("pendingStartRow=%d want 1 (before wrong click)", g.pendingStartRow)
	}
	ax1, ay1, ax2, ay2 := g.nodeScreenRect(a0)
	clickGame(g, int(math.Round((ax1+ax2)/2)), int(math.Round((ay1+ay2)/2)))
	// Selection should remain active and origin unchanged
	if g.drum.Rows[1].Origin != b0.ID {
		t.Fatalf("row1 origin changed unexpectedly to %d", g.drum.Rows[1].Origin)
	}
	if g.pendingStartRow != 1 {
		t.Fatalf("pendingStartRow cleared on wrong circuit click")
	}

	// Click a valid node in circuit B to commit change
	bx1, by1, bx2, by2 := g.nodeScreenRect(b1)
	clickGame(g, int(math.Round((bx1+bx2)/2)), int(math.Round((by1+by2)/2)))
	if g.drum.Rows[1].Origin != b1.ID {
		t.Fatalf("row1 origin not updated to b1; got %d", g.drum.Rows[1].Origin)
	}
	if g.pendingStartRow != -1 {
		t.Fatalf("pendingStartRow not cleared after valid selection")
	}
}
