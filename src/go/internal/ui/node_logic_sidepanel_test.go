package ui

import (
	"github.com/ingyamilmolinar/tunkul/core/model"
	"image"
	"testing"
)

// Ensure LOG button aligns with other rows and rule text does not overlap button.
func TestLogicButtonAlignedAndTextNotOverlap(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()

	rLogic := g.nodeMenuRects["logic"]
	rVolMinus := g.nodeMenuRects["vol-"]
	if rLogic.Min.X != rVolMinus.Min.X {
		t.Fatalf("logic button misaligned: logic=%v vol-=%v", rLogic, rVolMinus)
	}
	// Current rule text is drawn at panel.Min.X+60 with width based on TextSprite.
	panel := g.nodeMenuRects["panel"]
	valueX := panel.Min.X + 60
	text := "Prev Triggered"
	w := debugCharW * len(text)
	h := debugCharH
	curRect := image.Rect(valueX, rLogic.Min.Y, valueX+w, rLogic.Min.Y+h)
	if curRect.Overlaps(rLogic) {
		t.Fatalf("rule text overlaps logic button: text=%v logic=%v", curRect, rLogic)
	}
}

// Logic dropdown renders as a side panel to the right of the main popup.
func TestLogicDropdownSidePanel(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()
	panel := g.nodeMenuRects["panel"]
	h0 := panel.Dy()
	// Open logic menu
	g.nodeLogicOpen = true
	g.updateNodeMenuRects()
	panel2 := g.nodeMenuRects["panel"]
	if panel2.Dy() != h0 {
		t.Fatalf("panel height changed when opening logic dropdown: %d -> %d", h0, panel2.Dy())
	}
	// Find at least one side item and ensure it lies to the right of the panel
	found := false
	for id, r := range g.nodeMenuRects {
		if len(id) > 6 && id[:6] == "logic:" {
			if r.Min.X < panel.Max.X {
				t.Fatalf("logic side item not to the right: %s rect=%v panel=%v", id, r, panel)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no logic side items found")
	}
}

// Groove dropdown also renders as side panel.
func TestGrooveDropdownSidePanel(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.nodeMenuOpen = true
	g.nodeMenuNode = n
	g.updateNodeMenuRects()
	panel := g.nodeMenuRects["panel"]
	h0 := panel.Dy()
	// Open groove menu
	g.nodeGrooveOpen = true
	g.updateNodeMenuRects()
	panel2 := g.nodeMenuRects["panel"]
	if panel2.Dy() != h0 {
		t.Fatalf("panel height changed when opening groove dropdown: %d -> %d", h0, panel2.Dy())
	}
	found := false
	for id, r := range g.nodeMenuRects {
		if len(id) > 7 && id[:7] == "groove:" {
			if r.Min.X < panel.Max.X {
				t.Fatalf("groove side item not to the right: %s rect=%v panel=%v", id, r, panel)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no groove side items found")
	}
}
