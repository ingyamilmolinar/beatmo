//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestNodeMenuTapDoubleFire_LogicButton reproduces the mobile double-fire bug:
// the touch-to-mouse override fires the logic button (toggling nodeLogicOpen
// to true and setting suppressClicksUntilRelease), then handleTapInGrid fires
// the same button again (toggling it back to false). The fix makes
// handleTapInGrid skip the menu buttons when suppress is active.
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
	logicRect, ok := g.sidebar.rects["logic"]
	if !ok || logicRect.Empty() {
		t.Fatal("logic rect missing or empty")
	}
	lx := (logicRect.Min.X + logicRect.Max.X) / 2
	ly := (logicRect.Min.Y + logicRect.Max.Y) / 2

	// --- Simulate Frame N: touch-to-mouse override fires logic button ---
	// In the real flow, handleEditor calls handleNodeMenuButtons(x, y, left)
	// which fires the logic button's OnClick and sets suppressClicksUntilRelease
	// via ConsumeOnPress. Simulate this directly to avoid handleEditor's
	// grid-pane gate check (irrelevant to the bug — the override sets cursor
	// position to the button, which is always in the grid pane on real devices).
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

	// --- Simulate Frame N (same frame or next): GestureTap fires at same position ---
	// Before the fix, handleTapInGrid's brute-force loop would fire the logic
	// button again, toggling nodeLogicOpen back to false.
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

// TestNodeMenuTapFallback_NoSuppress verifies that when suppress is NOT active
// (e.g., an ultra-fast tap where the touch-to-mouse override missed the press),
// handleTapInGrid correctly opens the logic dropdown via handleNodeMenuButtons.
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
	// Only expand the logic section (not all) so the button stays within the
	// visible panel height on a 640x480 layout.
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.layout()

	// Locate the logic button.
	logicRect, ok := g.sidebar.rects["logic"]
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

	// handleTapInGrid should fall through to handleNodeMenuButtons and fire
	// the logic button.
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
