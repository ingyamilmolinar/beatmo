//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// On mobile the touch override fires the logic button and sets the suppression; the GestureTap at the same
// point must not fire it again and toggle it back.
func TestNodeMenuTapDoubleFire_LogicButton(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a node and open its popup.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	// Locate the logic button.
	logicRect, ok := g.sidebar.rects[sbLogicSelect]
	if !ok || logicRect.Empty() {
		t.Fatal("logic rect missing or empty")
	}
	lx := (logicRect.Min.X + logicRect.Max.X) / 2
	ly := (logicRect.Min.Y + logicRect.Max.Y) / 2

	// What handleEditor does on the override frame, called directly to skip its grid-pane gate (the button is
	// always in the grid pane on a device).
	hit := g.handleNodeMenuButtons(lx, ly, true)
	if !hit {
		t.Fatal("handleNodeMenuButtons should hit the logic button")
	}

	// Drain the UI queue so the enqueued toggle runs.
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if !g.sidebar.logicDropdownOpen {
		t.Fatal("expected nodeLogicOpen=true after handleNodeMenuButtons press")
	}
	if !suppressClicksUntilRelease {
		t.Fatal("expected suppressClicksUntilRelease=true after ConsumeOnPress button")
	}

	g.handleTapInGrid(lx, ly)

	// Drain any additional UI queue entries.
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	// nodeLogicOpen must remain true — the tap should NOT have toggled it back.
	if !g.sidebar.logicDropdownOpen {
		t.Fatal("nodeLogicOpen was toggled back to false by handleTapInGrid — double-fire bug")
	}
}

// With no suppression (an ultra-fast tap the override missed), the tap still opens the dropdown.
func TestNodeMenuTapFallback_NoSuppress(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add a node and open its popup.
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node at (0,0)")
	}
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	// Logic only, so the button stays inside the visible panel at 640x480.
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.layout()

	// Locate the logic button.
	logicRect, ok := g.sidebar.rects[sbLogicSelect]
	if !ok || logicRect.Empty() {
		t.Fatal("logic rect missing or empty")
	}
	lx := (logicRect.Min.X + logicRect.Max.X) / 2
	ly := (logicRect.Min.Y + logicRect.Max.Y) / 2

	// Ensure suppress is NOT active (simulating override miss).
	suppressClicksUntilRelease = false

	if g.sidebar.logicDropdownOpen {
		t.Fatal("nodeLogicOpen should be false before tap")
	}

	g.handleTapInGrid(lx, ly)

	// Drain the UI queue.
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if !g.sidebar.logicDropdownOpen {
		t.Fatal("expected nodeLogicOpen=true after handleTapInGrid fallback (no suppress)")
	}
}
