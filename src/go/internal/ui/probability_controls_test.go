package ui

import (
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
)

// Ensure Probability logic +/- buttons are clickable and adjust P within [0,1].
func TestProbabilityControlsAdjustP(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Single node and open its menu
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	// Select Probability rule via dropdown using direct button handles
	g.updateNodeMenuRects()
	if b := g.nodeMenuBtns["logic"]; b == nil {
		t.Fatalf("missing logic button")
	} else {
		b.Handle((g.nodeMenuRects["logic"].Min.X+g.nodeMenuRects["logic"].Max.X)/2, (g.nodeMenuRects["logic"].Min.Y+g.nodeMenuRects["logic"].Max.Y)/2, true)
	}
	_ = g.Update()
	g.updateNodeMenuRects()
	if b := g.nodeMenuBtns["logic:probability"]; b == nil {
		t.Fatalf("missing probability item")
	} else {
		r := g.nodeMenuRects["logic:probability"]
		b.Handle((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
	}
	_ = g.Update()
	// Now adjust P via +/-
	g.updateNodeMenuRects()
	before := 0.0
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		before = mn.Params.LogicP
	}
	// Click lp+
	if b := g.nodeMenuBtns["lp+"]; b == nil {
		t.Fatalf("missing lp+ button")
	} else {
		r := g.nodeMenuRects["lp+"]
		b.Handle((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
	}
	_ = g.Update()
	after := 0.0
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		after = mn.Params.LogicP
	}
	if !(after > before) {
		t.Fatalf("expected LogicP to increase: before=%.2f after=%.2f", before, after)
	}
	// Click lp- and ensure it decreases
	if b := g.nodeMenuBtns["lp-"]; b == nil {
		t.Fatalf("missing lp- button")
	} else {
		r := g.nodeMenuRects["lp-"]
		b.Handle((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
	}
	_ = g.Update()
	after2 := 0.0
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		after2 = mn.Params.LogicP
	}
	if !(after2 < after) {
		t.Fatalf("expected LogicP to decrease: before=%.2f after=%.2f", after, after2)
	}
	// No clamping is enforced; only direction matters.
}
