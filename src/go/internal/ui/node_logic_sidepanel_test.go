package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure LOG button aligns with other rows and rule text does not overlap button.
func TestLogicButtonAlignedAndTextNotOverlap(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		p := mn.Params
		p.LogicKind = "trigger_if_prev_triggered"
		g.graph.SetNodeParams(n.ID, p)
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	rLogic := g.sidebar.rects["logic"]
	panel := g.sidebar.rects["panel"]
	// Logic button is now full-width within the panel. Verify it spans the
	// panel content area (padded edges).
	if rLogic.Min.X != panel.Min.X+sidebarPad {
		t.Fatalf("logic button should start at panel left+pad: logic.Min.X=%d expected=%d", rLogic.Min.X, panel.Min.X+sidebarPad)
	}
	if rLogic.Max.X != panel.Max.X-sidebarPad {
		t.Fatalf("logic button should end at panel right-pad: logic.Max.X=%d expected=%d", rLogic.Max.X, panel.Max.X-sidebarPad)
	}
	// Logic button should be below the volume buttons
	rVolMinus := g.sidebar.rects["vol-"]
	if rLogic.Min.Y <= rVolMinus.Max.Y {
		t.Fatalf("logic button should be below volume buttons: logic.Min.Y=%d vol-.Max.Y=%d", rLogic.Min.Y, rVolMinus.Max.Y)
	}
}

// Logic dropdown renders inline within the popup panel.
func TestLogicDropdownInlinePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	// Open logic menu — dropdown items should appear inline
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()
	panel2 := g.sidebar.rects["panel"]
	// All logic items must be horizontally within the panel
	found := false
	for id, r := range g.sidebar.rects {
		if len(id) > 6 && id[:6] == "logic:" {
			if r.Min.X < panel2.Min.X || r.Max.X > panel2.Max.X {
				t.Fatalf("logic item outside panel horizontally: %s rect=%v panel=%v", id, r, panel2)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no logic items found")
	}
}

// Groove dropdown renders inline within the popup panel.
func TestGrooveDropdownInlinePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()
	// Open groove menu — dropdown items should appear inline
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()
	panel2 := g.sidebar.rects["panel"]
	// All groove items must be horizontally within the panel
	found := false
	for id, r := range g.sidebar.rects {
		if len(id) > 7 && id[:7] == "groove:" {
			if r.Min.X < panel2.Min.X || r.Max.X > panel2.Max.X {
				t.Fatalf("groove item outside panel horizontally: %s rect=%v panel=%v", id, r, panel2)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no groove items found")
	}
}

// TestSidebarSectionToggleNoFlicker verifies that holding the mouse on a
// section header for multiple frames does NOT cause repeated toggles, and
// that a complete tap (press+release) toggles exactly once via the deferred
// tap pattern.
func TestSidebarSectionToggleNoFlicker(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Volume section should start closed
	if g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should start closed")
	}

	// Find the section header rect
	secRect := g.sidebar.rects["sec-vol"]
	if secRect.Empty() {
		t.Fatal("sec-vol rect should exist")
	}
	cx := (secRect.Min.X + secRect.Max.X) / 2
	cy := (secRect.Min.Y + secRect.Max.Y) / 2

	// Simulate 5 frames of held press — should NOT toggle (deferred tap waits for release)
	for i := 0; i < 5; i++ {
		g.sidebar.HandleInput(cx, cy, true)
	}
	if g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should stay closed during held press (deferred tap)")
	}

	// Release → deferred tap fires → section toggles to open
	g.sidebar.HandleInput(cx, cy, false)
	if !g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should be open after tap (press+release)")
	}

	// Second tap: press+release → should toggle back to closed
	g.sidebar.HandleInput(cx, cy, true)
	g.sidebar.HandleInput(cx, cy, false)

	if g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should be closed after second tap")
	}
}

// TestSidebarScrollBehaviorWheel verifies that wheel events adjust the
// scroll offset when content overflows the viewport.
func TestSidebarScrollBehaviorWheel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Use a small grid pane so content overflows
	g.Layout(640, 250)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// Verify scrolling is enabled
	if !g.sidebar.scroll.HasScroll() {
		t.Fatal("scroll should be enabled when all sections expanded in small pane")
	}

	// Initial offset should be 0
	if g.sidebar.scroll.VS.First != 0 {
		t.Fatalf("initial scroll offset should be 0, got %d", g.sidebar.scroll.VS.First)
	}

	// Scroll down (wheel down = negative steps)
	g.sidebar.HandleWheel(0, 0, -1)

	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatal("scroll offset should increase after wheel down")
	}

	saved := g.sidebar.scroll.VS.First

	// Scroll back up (wheel up = positive steps)
	g.sidebar.HandleWheel(0, 0, 1)

	if g.sidebar.scroll.VS.First >= saved {
		t.Fatal("scroll offset should decrease after wheel up")
	}
}

// TestSidebarScrollbarDrag verifies that dragging the scrollbar thumb
// adjusts the scroll offset.
func TestSidebarScrollbarDrag(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 250)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	if !g.sidebar.scroll.HasScroll() {
		t.Fatal("scroll should be enabled")
	}

	// Get scrollbar thumb rect
	thumb := g.sidebar.scroll.ThumbRect()
	if thumb.Empty() {
		t.Fatal("thumb rect should not be empty when scroll is enabled")
	}

	// Start drag at thumb center
	thumbCY := (thumb.Min.Y + thumb.Max.Y) / 2
	thumbCX := (thumb.Min.X + thumb.Max.X) / 2

	// Simulate press on thumb
	g.sidebar.HandleInput(thumbCX, thumbCY, true)

	if !g.sidebar.scroll.Dragging() {
		t.Fatal("should be dragging after press on thumb")
	}

	// Drag down
	g.sidebar.HandleInput(thumbCX, thumbCY+30, true)

	if g.sidebar.scroll.VS.First <= 0 {
		t.Fatal("scroll offset should increase after drag down")
	}

	// Release
	g.sidebar.HandleInput(thumbCX, thumbCY+30, false)

	if g.sidebar.scroll.Dragging() {
		t.Fatal("should not be dragging after release")
	}
}

// TestSidebarScrollUsesScrollBehavior verifies the sidebar uses ScrollBehavior
// and the View rect is properly set.
func TestSidebarScrollUsesScrollBehavior(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	// View rect should be set and non-empty
	view := g.sidebar.scroll.VS.View
	if view.Empty() {
		t.Fatal("scroll View rect should be set after layout")
	}

	// View should be within the sidebar panel
	panel := g.sidebar.rects["panel"]
	if !image.Pt(view.Min.X, view.Min.Y).In(panel) {
		t.Fatalf("scroll view min should be within panel: view=%v panel=%v", view, panel)
	}
}
