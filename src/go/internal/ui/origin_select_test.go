package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// helper to click at screen coordinates over multiple updates
func clickAt(g *Game, sx, sy int) {
	// press
	restore := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
	// release
	restore = SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
}

func TestOriginSelectOnExistingNode(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Create two visible nodes
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(3, 0, model.NodeTypeSilent)
	if n0 == nil || n1 == nil {
		t.Fatalf("nodes not created")
	}
	// Add a second drum row
	g.drum.AddRow()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	// Press origin select for row 1
	btn := g.drum.rowOriginBtns[1]
	r := btn.Rect()
	// Use Button.Handle directly to enqueue request
	_ = btn.Handle((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
	_ = btn.Handle((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, false)
	// Process request in Game.Update
	_ = g.Update()
	if g.pendingStartRow != 1 {
		t.Fatalf("pendingStartRow=%d want 1", g.pendingStartRow)
	}
	// Click on silent node n1 to set as origin for row 1
	x1, y1, x2, y2 := g.nodeScreenRect(n1)
	sx := int(math.Round((x1 + x2) / 2))
	sy := int(math.Round((y1 + y2) / 2))
	clickAt(g, sx, sy)
	if g.pendingStartRow != -1 {
		t.Fatalf("pendingStartRow not cleared: %d", g.pendingStartRow)
	}
	if g.drum.Rows[1].Origin != n1.ID {
		t.Fatalf("row1 origin=%d want %d", g.drum.Rows[1].Origin, n1.ID)
	}
	if g.drum.Rows[1].Node != n1 {
		t.Fatalf("row1 node pointer not set")
	}
	if !n1.Start {
		t.Fatalf("selected node not marked Start")
	}
}

func TestDrumAutoHeightMatchesRows(t *testing.T) {
	g := New(testLogger)
	w, h := 800, 600
	forceAutoSize = true
	g.Layout(w, h)
	want := timelineHeight + (len(g.drum.Rows)+1)*g.drum.rowHeight()
	have := g.winH - g.split.Y
	// Allow clamping at both ends: minimum drum height (120) and maximum (h-120).
	if have != want {
		if !(want < 120 && have == 120) && !(want > h-120 && have == 120) && !(want > h-120 && have == h-120) {
			t.Fatalf("initial drum height=%d want %d", have, want)
		}
	}
	// Add a row and re-layout; height should grow by one rowHeight
	g.drum.AddRow()
	g.Layout(w, h)
	want2 := timelineHeight + (len(g.drum.Rows)+1)*g.drum.rowHeight()
	have2 := g.winH - g.split.Y
	if have2 != want2 {
		if !(want2 < 120 && have2 == 120) && !(want2 > h-120 && have2 == 120) && !(want2 > h-120 && have2 == h-120) {
			t.Fatalf("after add drum height=%d want %d", have2, want2)
		}
	}
	forceAutoSize = false
}
