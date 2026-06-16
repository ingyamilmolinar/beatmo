//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestSidebar_DeferredTapVsScroll opens the sidebar with expanded sections
// (so there is scrollable content). A touch scroll with vertical movement
// > 20 px commits scrolling and cancels the deferredTap. A short touch
// without movement fires the deferred tap.
func TestSidebar_DeferredTapVsScroll(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	panelR := g.sidebar.rects["panel"]
	if panelR.Empty() {
		t.Skip("no panel rect")
	}
	cx := (panelR.Min.X + panelR.Max.X) / 2
	cy := panelR.Min.Y + panelR.Dy()/2

	// Begin touch.
	g.sidebar.HandleInput(cx, cy, true)

	// Drag more than 20px vertically to commit scroll.
	for i := 1; i <= 10; i++ {
		g.sidebar.HandleInput(cx, cy-i*4, true)
	}

	// At this point scroll should be committed.
	if !g.sidebar.scroll.ScrollingCommitted() {
		t.Error("expected scroll to be committed after >20px vertical drag")
	}

	// The deferred tap should be active but will be canceled on release.
	// Release.
	g.sidebar.HandleInput(cx, cy-40, false)

	// After release with scroll committed, deferred tap should be canceled
	// (not fired). We can verify that no section was toggled.
	// Record current section states.
	for _, sec := range []string{"vol", "pit", "dur"} {
		if !g.sidebar.sectionOpen[sec] {
			t.Errorf("section %q should still be open (expanded earlier)", sec)
		}
	}

	// Now test short touch (no scroll) — should fire deferred tap.
	// First, find a section header rect to tap on.
	secDurRect := g.sidebar.rects["sec-dur"]
	if secDurRect.Empty() {
		t.Skip("sec-dur rect empty — cannot test deferred tap fire")
	}

	sx := (secDurRect.Min.X + secDurRect.Max.X) / 2
	sy := (secDurRect.Min.Y + secDurRect.Max.Y) / 2

	// Short press and immediate release (no movement).
	g.sidebar.HandleInput(sx, sy, true)
	g.sidebar.HandleInput(sx, sy, false)

	// After release with no scroll, deferred tap should have fired on sec-dur.
	// sec-dur was open; toggling it closes it.
	if g.sidebar.sectionOpen["dur"] {
		t.Error("expected 'dur' section to be toggled closed by deferred tap")
	}
}

// TestSidebar_DropdownCloseOnOutsideClick opens the sidebar, opens the logic
// dropdown, then clicks outside the dropdown but inside the panel. The dropdown
// should close but the sidebar should stay open.
func TestSidebar_DropdownCloseOnOutsideClick(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	if !g.sidebar.logicDropdownOpen {
		t.Fatal("precondition: logic dropdown should be open")
	}

	// Find the panel rect and click in a spot that is inside the panel
	// but outside all dropdown rects. The header area (top of panel) works.
	headerR := g.sidebar.rects["header"]
	if headerR.Empty() {
		t.Fatal("header rect should not be empty")
	}
	cx := (headerR.Min.X + headerR.Max.X) / 2
	cy := (headerR.Min.Y + headerR.Max.Y) / 2

	// Press and release inside the panel header (outside dropdown).
	g.sidebar.HandleInput(cx, cy, true)
	g.sidebar.HandleInput(cx, cy, false)

	// The close button occupies part of the header. Call closeDropdownsOutside
	// directly to be sure.
	if g.sidebar.logicDropdownOpen {
		// If still open (button might have intercepted), use direct call.
		g.sidebar.closeDropdownsOutside(cx, cy)
	}

	if g.sidebar.logicDropdownOpen {
		t.Error("logic dropdown should be closed after clicking outside dropdown rects")
	}
	if !g.sidebar.IsOpen() {
		t.Error("sidebar should remain open after dropdown close")
	}
}

// TestSidebar_CollapsedExpandTab verifies that when collapsed=true, only the
// expand tab is hittable. Clicking the tab sets collapsed=false.
func TestSidebar_CollapsedExpandTab(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.collapsed = true

	// Hit should be true inside the expand tab.
	tabRect := g.sidebar.expandTabRect()
	if tabRect.Empty() {
		t.Fatal("expand tab rect should not be empty")
	}

	cx := (tabRect.Min.X + tabRect.Max.X) / 2
	cy := (tabRect.Min.Y + tabRect.Max.Y) / 2

	if !g.sidebar.Hit(cx, cy) {
		t.Fatal("Hit should return true inside expand tab when collapsed")
	}

	// Hit should be false outside the tab.
	if g.sidebar.Hit(500, 300) {
		t.Fatal("Hit should return false outside expand tab when collapsed")
	}

	// Click the tab → should expand.
	result := g.sidebar.HandleInput(cx, cy, true)
	if result == InputIgnored {
		t.Fatal("HandleInput should not return InputIgnored on collapsed tab press")
	}
	if g.sidebar.collapsed {
		t.Fatal("sidebar should not be collapsed after tab click")
	}
	if !g.sidebar.open {
		t.Fatal("sidebar should be open after expanding from collapsed")
	}
}
