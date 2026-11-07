package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Aligned with the other rows' buttons.
func TestNodeLogicLayout_NoOverlap(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	// set a logic that uses N
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = model.LogicSkipEveryN
		p.LogicN = 4
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	rLogic := g.sidebar.rects[sbLogicSelect]
	rLnMinus := g.sidebar.rects[sbMinus(fieldLogicN)]
	rLnPlus := g.sidebar.rects[sbPlus(fieldLogicN)]
	rVolMinus := g.sidebar.rects[sbMinus(fieldVol)]
	rVolPlus := g.sidebar.rects[sbPlus(fieldVol)]

	if rLogic.Overlaps(rLnMinus) || rLogic.Overlaps(rLnPlus) {
		t.Fatalf("logic button overlaps +/- controls: logic=%v ln-=%v ln+=%v", rLogic, rLnMinus, rLnPlus)
	}
	// The label sits inside the full-width logic button; the ± align with volume's.
	if rLnMinus.Min.X != rVolMinus.Min.X || rLnPlus.Min.X != rVolPlus.Min.X {
		t.Fatalf("logic +/- not aligned with other rows: ln-=%v vol-=%v ln+=%v vol+=%v", rLnMinus, rVolMinus, rLnPlus, rVolPlus)
	}
}

// TestNodeLogicLayout_ProbabilityButtonsAligned ensures probability +/- align and do not overlap.
func TestNodeLogicLayout_ProbabilityButtonsAligned(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = model.LogicProbability
		p.LogicP = 0.5
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	rLogic := g.sidebar.rects[sbLogicSelect]
	rLpMinus := g.sidebar.rects[sbMinus(fieldLogicP)]
	rLpPlus := g.sidebar.rects[sbPlus(fieldLogicP)]
	rVolMinus := g.sidebar.rects[sbMinus(fieldVol)]
	rVolPlus := g.sidebar.rects[sbPlus(fieldVol)]
	// No overlap with logic selector
	if rLogic.Overlaps(rLpMinus) || rLogic.Overlaps(rLpPlus) {
		t.Fatalf("logic button overlaps probability +/- controls: logic=%v lp-=%v lp+=%v", rLogic, rLpMinus, rLpPlus)
	}
	// Aligned with other rows
	if rLpMinus.Min.X != rVolMinus.Min.X || rLpPlus.Min.X != rVolPlus.Min.X {
		t.Fatalf("probability +/- not aligned: lp-=%v vol-=%v lp+=%v vol+=%v", rLpMinus, rVolMinus, rLpPlus, rVolPlus)
	}
}
