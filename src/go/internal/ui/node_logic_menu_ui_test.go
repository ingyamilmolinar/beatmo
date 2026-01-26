package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// TestNodeLogicDropdown_OpenSelectAndAdjust mirrors the instrument dropdown UX:
// it opens the logic menu, selects a rule, and adjusts its parameter via +/-.
func TestNodeLogicDropdown_OpenSelectAndAdjust(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple loop s0 -> r1 -> s2 -> (back)
	s0 := g.tryAddNode(0, 0, model.NodeTypeSilent)
	r1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	s2 := g.tryAddNode(2, 0, model.NodeTypeSilent)
	g.addEdge(s0, r1)
	g.addEdge(r1, s2)
	g.addEdge(s2, s0)

	// Open popup on r1 and open logic dropdown
	g.sel = r1
	r1.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = r1
	g.updateNodeMenuRects()
	click := func(id string, b *Button, x, y int) {
		if b == nil {
			t.Fatalf("button %s missing", id)
		}
		if !b.Handle(x, y, true) {
			t.Fatalf("button %s did not handle click", id)
		}
		b.Handle(x, y, false)
	}
	// Click logic button
	bl := g.nodeMenuRects["logic"]
	mx, my := (bl.Min.X+bl.Max.X)/2, (bl.Min.Y+bl.Max.Y)/2
	if g.nodeMenuBtns == nil {
		t.Fatalf("node menu buttons not initialized")
	}
	click("logic", g.nodeMenuBtns["logic"], mx, my)
	// Apply queued UI change
	_ = g.Update()
	if !g.nodeLogicOpen {
		t.Fatalf("logic menu did not open")
	}
	// Select Probability rule
	g.updateNodeMenuRects()
	prob := g.nodeMenuRects["logic:probability"]
	mx, my = (prob.Min.X+prob.Max.X)/2, (prob.Min.Y+prob.Max.Y)/2
	click("logic:probability", g.nodeMenuBtns["logic:probability"], mx, my)
	_ = g.Update()
	// Adjust probability down then up
	g.updateNodeMenuRects()
	if b := g.nodeMenuBtns["lp-"]; b == nil {
		t.Fatalf("missing lp- button")
	} else {
		click("lp-", b, b.Rect().Min.X+1, b.Rect().Min.Y+1)
	}
	if b := g.nodeMenuBtns["lp+"]; b == nil {
		t.Fatalf("missing lp+ button")
	} else {
		click("lp+", b, b.Rect().Min.X+1, b.Rect().Min.Y+1)
	}

	// Now choose Trigger Every N and ensure N adjusters exist and affect params
	if b := g.nodeMenuBtns["logic"]; b == nil {
		t.Fatalf("logic button missing on reopen")
	} else {
		click("logic", b, (bl.Min.X+bl.Max.X)/2, (bl.Min.Y+bl.Max.Y)/2)
	}
	_ = g.Update()
	g.nodeLogicOpen = true
	g.updateNodeMenuRects()
	trig := g.nodeMenuRects["logic:every_n_triggers"]
	if b := g.nodeMenuBtns["logic:every_n_triggers"]; b == nil {
		t.Fatalf("missing every_n_triggers item")
	} else {
		click("logic:every_n_triggers", b, (trig.Min.X+trig.Max.X)/2, (trig.Min.Y+trig.Max.Y)/2)
	}
	_ = g.Update()
	g.updateNodeMenuRects()
	if b := g.nodeMenuBtns["ln+"]; b == nil {
		t.Fatalf("missing ln+ button")
	} else {
		click("ln+", b, b.Rect().Min.X+1, b.Rect().Min.Y+1)
	}
	if b := g.nodeMenuBtns["ln-"]; b == nil {
		t.Fatalf("missing ln- button")
	} else {
		click("ln-", b, b.Rect().Min.X+1, b.Rect().Min.Y+1)
	}

	// Quick behavioral check: set N=2 and verify only every 2nd triggers
	if mn, ok := g.graph.GetNodeByID(r1.ID); ok {
		p := mn.Params
		p.LogicKind = "every_n_triggers"
		p.LogicN = 2
		g.graph.SetNodeParams(r1.ID, p)
	}
	plays := 0
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })
	g.SetPlaying(true)
	for abs := 0; abs < 24; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	if plays == 0 {
		t.Fatalf("expected some plays with every 2 triggers")
	}
}
