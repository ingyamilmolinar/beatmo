package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"image"
	"testing"
)

// TestNodeLogicLayout_NoOverlap ensures logic selector and +/- buttons never overlap
// and align with other rows' buttons.
func TestNodeLogicLayout_NoOverlap(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	// set a logic that uses N
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "skip_every_n"
		p.LogicN = 4
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()

	rLogic := g.nodeMenuRects["logic"]
	rLnMinus := g.nodeMenuRects["ln-"]
	rLnPlus := g.nodeMenuRects["ln+"]
	rVolMinus := g.nodeMenuRects["vol-"]
	rVolPlus := g.nodeMenuRects["vol+"]

	if rLogic.Overlaps(rLnMinus) || rLogic.Overlaps(rLnPlus) {
		t.Fatalf("logic button overlaps +/- controls: logic=%v ln-=%v ln+=%v", rLogic, rLnMinus, rLnPlus)
	}
	// Label rect should not overlap the logic button
	panel := g.nodeMenuRects["panel"]
	labelW := debugCharW * len("LOGIC")
	labelH := debugCharH
	labelRect := image.Rect(panel.Min.X+8, rLogic.Min.Y, panel.Min.X+8+labelW, rLogic.Min.Y+labelH)
	if labelRect.Overlaps(rLogic) {
		t.Fatalf("LOGIC label overlaps logic button: label=%v logic=%v", labelRect, rLogic)
	}
	// Align +/- x positions with volume +/- for consistent grid
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
		p.LogicKind = "probability"
		p.LogicP = 0.5
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()

	rLogic := g.nodeMenuRects["logic"]
	rLpMinus := g.nodeMenuRects["lp-"]
	rLpPlus := g.nodeMenuRects["lp+"]
	rVolMinus := g.nodeMenuRects["vol-"]
	rVolPlus := g.nodeMenuRects["vol+"]
	// No overlap with logic selector
	if rLogic.Overlaps(rLpMinus) || rLogic.Overlaps(rLpPlus) {
		t.Fatalf("logic button overlaps probability +/- controls: logic=%v lp-=%v lp+=%v", rLogic, rLpMinus, rLpPlus)
	}
	// Aligned with other rows
	if rLpMinus.Min.X != rVolMinus.Min.X || rLpPlus.Min.X != rVolPlus.Min.X {
		t.Fatalf("probability +/- not aligned: lp-=%v vol-=%v lp+=%v vol+=%v", rLpMinus, rVolMinus, rLpPlus, rVolPlus)
	}
}
