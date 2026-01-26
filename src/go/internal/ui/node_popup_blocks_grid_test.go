package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

// Clicking inside the popup panel (not on a button) must not create grid nodes.
func TestNodePopupBlocksGridClicks(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()
	panel := g.nodeMenuRects["panel"]
	// Pick a coordinate inside the panel but outside known buttons.
	// Use a point slightly to the left of the right-aligned +/- buttons.
	x := panel.Min.X + 40
	y := panel.Min.Y + 12 // near first row value area
	before := len(g.nodes)
	// Simulate press+release cycle using SetInputForTest overrides.
	left := false
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return left },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore()
	left = true
	_ = g.Update()
	left = false
	_ = g.Update()
	if len(g.nodes) != before {
		t.Fatalf("unexpected grid node created via popup click: before=%d after=%d", before, len(g.nodes))
	}
}

// Clicking logic +/- must adjust value and not create grid nodes.
func TestNodePopupButtonsClickableNoGrid(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 2
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()
	beforeNodes := len(g.nodes)
	// Click ln+ button
	r := g.nodeMenuRects["ln+"]
	x2 := (r.Min.X + r.Max.X) / 2
	y2 := (r.Min.Y + r.Max.Y) / 2
	left2 := false
	restore2 := SetInputForTest(
		func() (int, int) { return x2, y2 },
		func(ebiten.MouseButton) bool { return left2 },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	defer restore2()
	left2 = true
	_ = g.Update()
	left2 = false
	_ = g.Update()
	// Verify N incremented and no new grid nodes created
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		if mn.Params.LogicN <= 2 {
			t.Fatalf("expected LogicN to increase, got %d", mn.Params.LogicN)
		}
	} else {
		t.Fatalf("node missing in graph")
	}
	if len(g.nodes) != beforeNodes {
		t.Fatalf("unexpected grid node created via ln+ click")
	}
}
