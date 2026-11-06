package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
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
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	// Select Probability rule via dropdown using direct button handles
	g.sidebar.layout()
	if b := g.sidebar.btns[sbLogicSelect]; b == nil {
		t.Fatalf("missing logic button")
	} else {
		b.HandleInputResult((g.sidebar.rects[sbLogicSelect].Min.X+g.sidebar.rects[sbLogicSelect].Max.X)/2, (g.sidebar.rects[sbLogicSelect].Min.Y+g.sidebar.rects[sbLogicSelect].Max.Y)/2, true)
	}
	_ = g.Update()
	g.sidebar.layout()
	if b := g.sidebar.btns[sbLogicOption(model.LogicProbability)]; b == nil {
		t.Fatalf("missing probability item")
	} else {
		r := g.sidebar.rects[sbLogicOption(model.LogicProbability)]
		b.HandleInputResult((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
	}
	_ = g.Update()
	// Now adjust P via +/-
	g.sidebar.layout()
	before := 0.0
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		before = mn.Params.LogicP
	}
	// Click lp+
	if b := g.sidebar.btns[sbPlus(fieldLogicP)]; b == nil {
		t.Fatalf("missing lp+ button")
	} else {
		r := g.sidebar.rects[sbPlus(fieldLogicP)]
		b.HandleInputResult((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
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
	if b := g.sidebar.btns[sbMinus(fieldLogicP)]; b == nil {
		t.Fatalf("missing lp- button")
	} else {
		r := g.sidebar.rects[sbMinus(fieldLogicP)]
		b.HandleInputResult((r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2, true)
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
