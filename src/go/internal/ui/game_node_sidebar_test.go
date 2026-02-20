package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ─── Open/Close Lifecycle ────────────────────────────────────────────────────

// TestSidebarOpenCloseCycle verifies the sidebar open/close lifecycle:
// Open sets IsOpen=true and Node!=nil; Close sets IsOpen=false, Node=nil,
// and ClosedGuard=2.
func TestSidebarOpenCloseCycle(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}

	// Initially not open.
	if g.sidebar.IsOpen() {
		t.Fatal("sidebar should not be open initially")
	}
	if g.sidebar.Node() != nil {
		t.Fatal("sidebar node should be nil initially")
	}

	// Open.
	g.sidebar.Open(n)
	if !g.sidebar.IsOpen() {
		t.Fatal("sidebar should be open after Open()")
	}
	if g.sidebar.Node() == nil {
		t.Fatal("sidebar node should not be nil after Open()")
	}
	if g.sidebar.Node().ID != n.ID {
		t.Fatalf("sidebar node ID mismatch: got %v, want %v", g.sidebar.Node().ID, n.ID)
	}

	// Close.
	g.sidebar.Close()
	if g.sidebar.IsOpen() {
		t.Fatal("sidebar should not be open after Close()")
	}
	if g.sidebar.Node() != nil {
		t.Fatal("sidebar node should be nil after Close()")
	}
	if g.sidebar.ClosedGuard() != 2 {
		t.Fatalf("ClosedGuard should be 2 after Close(), got %d", g.sidebar.ClosedGuard())
	}
}

// TestSidebarClosedGuardDecrement verifies that DecrementClosedGuard counts
// down to 0 and does not go negative.
func TestSidebarClosedGuardDecrement(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.Close()

	if g.sidebar.ClosedGuard() != 2 {
		t.Fatalf("ClosedGuard should be 2 after Close(), got %d", g.sidebar.ClosedGuard())
	}

	g.sidebar.DecrementClosedGuard()
	if g.sidebar.ClosedGuard() != 1 {
		t.Fatalf("ClosedGuard should be 1 after one decrement, got %d", g.sidebar.ClosedGuard())
	}

	g.sidebar.DecrementClosedGuard()
	if g.sidebar.ClosedGuard() != 0 {
		t.Fatalf("ClosedGuard should be 0 after two decrements, got %d", g.sidebar.ClosedGuard())
	}

	// Should not go negative.
	g.sidebar.DecrementClosedGuard()
	if g.sidebar.ClosedGuard() != 0 {
		t.Fatalf("ClosedGuard should remain 0 after extra decrement, got %d", g.sidebar.ClosedGuard())
	}
}

// TestSidebarOpenResetsSections verifies that reopening the sidebar on a
// different node resets all sections to collapsed.
func TestSidebarOpenResetsSections(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(4, 0, model.NodeTypeRegular)

	// Open first node and expand all sections.
	g.sidebar.Open(n1)
	g.sidebar.ExpandAllSections()

	// Verify sections are expanded.
	for _, sec := range []string{"vol", "pit", "dur", "logic", "groove"} {
		if !g.sidebar.sectionOpen[sec] {
			t.Fatalf("section %q should be open after ExpandAllSections", sec)
		}
	}

	// Close and reopen on a different node.
	g.sidebar.Close()
	g.sidebar.Open(n2)

	// All sections should be collapsed.
	for _, sec := range []string{"vol", "pit", "dur", "logic", "groove"} {
		if g.sidebar.sectionOpen[sec] {
			t.Fatalf("section %q should be closed after reopening on new node", sec)
		}
	}
}

// ─── Layout ──────────────────────────────────────────────────────────────────

// TestSidebarLayoutWidthClamping verifies that sidebar width is clamped to
// [sidebarMinW, sidebarMaxW] after layout.
func TestSidebarLayoutWidthClamping(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)

	// Set width below minimum.
	g.sidebar.width = 100
	g.sidebar.layout()
	if g.sidebar.width < sidebarMinW {
		t.Fatalf("width should be clamped to sidebarMinW (%d), got %d", sidebarMinW, g.sidebar.width)
	}

	// Set width above maximum.
	g.sidebar.width = 600
	g.sidebar.layout()
	if g.sidebar.width > sidebarMaxW {
		t.Fatalf("width should be clamped to sidebarMaxW (%d), got %d", sidebarMaxW, g.sidebar.width)
	}

	// Set width within range.
	g.sidebar.width = 300
	g.sidebar.layout()
	if g.sidebar.width != 300 {
		t.Fatalf("width should remain 300, got %d", g.sidebar.width)
	}
}

// TestSidebarLayoutNodeTypeSections verifies that sections are shown/hidden
// based on the node type: Regular shows all, Silent hides vol/pit/dur/logic/groove,
// Mute hides vol/pit/dur.
func TestSidebarLayoutNodeTypeSections(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Regular node: all sections present.
	nReg := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(nReg)
	g.sidebar.layout()

	for _, sec := range []string{"sec-vol", "sec-pit", "sec-dur", "sec-logic", "sec-groove"} {
		if r, ok := g.sidebar.rects[sec]; !ok || r.Empty() {
			t.Fatalf("Regular node: section rect %q should exist and be non-empty", sec)
		}
	}

	// Silent node: vol/pit/dur/logic/groove hidden.
	nSilent := g.tryAddNode(4, 0, model.NodeTypeSilent)
	g.sidebar.Open(nSilent)
	g.sidebar.layout()

	for _, sec := range []string{"sec-vol", "sec-pit", "sec-dur", "sec-logic", "sec-groove"} {
		if _, ok := g.sidebar.rects[sec]; ok {
			t.Fatalf("Silent node: section rect %q should NOT exist", sec)
		}
	}
	// Audible section should still be present.
	if r, ok := g.sidebar.rects["sec-aud"]; !ok || r.Empty() {
		t.Fatal("Silent node: sec-aud should exist")
	}

	// Mute node: vol/pit/dur hidden, logic/groove present.
	nMute := g.tryAddNode(8, 0, model.NodeTypeMute)
	g.sidebar.Open(nMute)
	g.sidebar.layout()

	for _, sec := range []string{"sec-vol", "sec-pit", "sec-dur"} {
		if _, ok := g.sidebar.rects[sec]; ok {
			t.Fatalf("Mute node: section rect %q should NOT exist", sec)
		}
	}
	for _, sec := range []string{"sec-logic", "sec-groove"} {
		if r, ok := g.sidebar.rects[sec]; !ok || r.Empty() {
			t.Fatalf("Mute node: section rect %q should exist and be non-empty", sec)
		}
	}
}

// ─── Hit Testing ─────────────────────────────────────────────────────────────

// TestSidebarHitInsidePanel verifies that Hit returns true for a point inside
// the panel rect.
func TestSidebarHitInsidePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	panel := g.sidebar.rects["panel"]
	if panel.Empty() {
		t.Fatal("panel rect should not be empty")
	}
	cx := (panel.Min.X + panel.Max.X) / 2
	cy := (panel.Min.Y + panel.Max.Y) / 2

	if !g.sidebar.Hit(cx, cy) {
		t.Fatalf("Hit(%d,%d) should return true for point inside panel %v", cx, cy, panel)
	}
}

// TestSidebarHitOutsidePanel verifies that Hit returns false for a point well
// outside the panel rect.
func TestSidebarHitOutsidePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Well outside the panel on the right side.
	if g.sidebar.Hit(700, 300) {
		t.Fatal("Hit should return false for point well outside panel")
	}
}

// TestSidebarHitCollapsed verifies that when collapsed, Hit returns true only
// for the expand tab area.
func TestSidebarHitCollapsed(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.collapsed = true

	tabRect := g.sidebar.expandTabRect()
	if tabRect.Empty() {
		t.Fatal("expand tab rect should not be empty")
	}

	// Inside the tab area.
	cx := (tabRect.Min.X + tabRect.Max.X) / 2
	cy := (tabRect.Min.Y + tabRect.Max.Y) / 2
	if !g.sidebar.Hit(cx, cy) {
		t.Fatalf("Hit(%d,%d) should return true inside expand tab %v", cx, cy, tabRect)
	}

	// Outside the tab area (far right).
	if g.sidebar.Hit(500, 300) {
		t.Fatal("Hit should return false outside expand tab when collapsed")
	}
}

// TestSidebarHitNotOpen verifies that Hit returns false when the sidebar is
// not open.
func TestSidebarHitNotOpen(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Sidebar never opened.
	if g.sidebar.Hit(50, 50) {
		t.Fatal("Hit should return false when sidebar is not open")
	}
}

// ─── Input Handling ──────────────────────────────────────────────────────────

// TestSidebarHandleInputCollapsedExpands verifies that pressing inside the
// collapsed expand tab sets collapsed=false and open=true.
func TestSidebarHandleInputCollapsedExpands(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.collapsed = true

	// HandleInput with pressed=true while collapsed.
	result := g.sidebar.HandleInput(5, 50, true)
	if result == InputIgnored {
		t.Fatal("HandleInput should not return InputIgnored when collapsed")
	}
	if g.sidebar.collapsed {
		t.Fatal("sidebar should not be collapsed after press")
	}
	if !g.sidebar.open {
		t.Fatal("sidebar should be open after expanding from collapsed state")
	}
}

// TestSidebarHandleInputNotOpenReturnsIgnored verifies that HandleInput returns
// InputIgnored when the sidebar is not open and not collapsed.
func TestSidebarHandleInputNotOpenReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	result := g.sidebar.HandleInput(50, 50, true)
	if result != InputIgnored {
		t.Fatalf("HandleInput should return InputIgnored when not open, got %v", result)
	}
}

// TestSidebarHandleInputResizeDrag verifies that dragging on the resize handle
// changes the sidebar width.
func TestSidebarHandleInputResizeDrag(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	initialWidth := g.sidebar.width

	// Get the resize handle rect (right edge of panel).
	handleR := g.sidebar.resizeHandleRect()
	if handleR.Empty() {
		t.Fatal("resize handle rect should not be empty")
	}

	// Press on the resize handle center.
	hx := (handleR.Min.X + handleR.Max.X) / 2
	hy := (handleR.Min.Y + handleR.Max.Y) / 2

	result := g.sidebar.HandleInput(hx, hy, true)
	if result != InputCaptured {
		t.Fatalf("press on resize handle should return InputCaptured, got %v", result)
	}
	if !g.sidebar.resizing {
		t.Fatal("sidebar should be in resizing state after press on handle")
	}

	// Drag to the right by 50 pixels.
	dragDelta := 50
	result = g.sidebar.HandleInput(hx+dragDelta, hy, true)
	if result != InputCaptured {
		t.Fatalf("drag should return InputCaptured, got %v", result)
	}

	if g.sidebar.width != initialWidth+dragDelta {
		t.Fatalf("width should change by %d: expected %d, got %d",
			dragDelta, initialWidth+dragDelta, g.sidebar.width)
	}

	// Release.
	g.sidebar.HandleInput(hx+dragDelta, hy, false)
	if g.sidebar.resizing {
		t.Fatal("sidebar should not be resizing after release")
	}
}

// ─── Sections & Dropdowns ────────────────────────────────────────────────────

// TestSidebarToggleSection verifies that toggling a section opens and closes it.
func TestSidebarToggleSection(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Initially closed.
	if g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should be closed initially")
	}

	// Toggle open.
	g.sidebar.toggleSection("vol")
	if !g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should be open after first toggle")
	}

	// Toggle closed.
	g.sidebar.toggleSection("vol")
	if g.sidebar.sectionOpen["vol"] {
		t.Fatal("vol section should be closed after second toggle")
	}
}

// TestSidebarLogicDropdownToggle verifies that the logic dropdown can be
// toggled open via the logic button's OnClick callback.
func TestSidebarLogicDropdownToggle(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.layout()

	// Verify logic button exists.
	btn, ok := g.sidebar.btns["logic"]
	if !ok || btn == nil {
		t.Fatal("logic button should exist when logic section is open")
	}

	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be closed initially")
	}

	// Fire the button callback to toggle dropdown open.
	btn.OnClick()
	// Drain the UI queue.
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if !g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be open after button click")
	}

	// Re-layout to get dropdown rects, then fire again to close.
	g.sidebar.layout()
	btn2 := g.sidebar.btns["logic"]
	if btn2 == nil {
		t.Fatal("logic button should still exist")
	}
	btn2.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be closed after second button click")
	}
}

// TestSidebarGrooveDropdownMutualExclusion verifies that opening the groove
// dropdown closes the logic dropdown, and vice versa.
func TestSidebarGrooveDropdownMutualExclusion(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.layout()

	// Open logic dropdown via button callback.
	logicBtn := g.sidebar.btns["logic"]
	if logicBtn == nil {
		t.Fatal("logic button should exist")
	}
	logicBtn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if !g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be open")
	}

	// Re-layout to wire groove button with current state.
	g.sidebar.layout()

	// Open groove dropdown via button callback.
	grvBtn := g.sidebar.btns["grv"]
	if grvBtn == nil {
		t.Fatal("groove button should exist")
	}
	grvBtn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if !g.sidebar.grooveDropdownOpen {
		t.Fatal("groove dropdown should be open")
	}
	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be closed when groove is opened (mutual exclusion)")
	}

	// Re-layout and open logic again — groove should close.
	g.sidebar.layout()
	logicBtn2 := g.sidebar.btns["logic"]
	if logicBtn2 == nil {
		t.Fatal("logic button should exist")
	}
	logicBtn2.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if !g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be open")
	}
	if g.sidebar.grooveDropdownOpen {
		t.Fatal("groove dropdown should be closed when logic is opened (mutual exclusion)")
	}
}

// TestSidebarCloseDropdownsOutside verifies that closeDropdownsOutside closes
// dropdowns when the point is outside all dropdown rects.
func TestSidebarCloseDropdownsOutside(t *testing.T) {
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
		t.Fatal("logic dropdown should be open before test")
	}

	// Click well outside all rects.
	g.sidebar.closeDropdownsOutside(0, 0)

	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be closed after clicking outside")
	}
}

// ─── Parameter Mutations ─────────────────────────────────────────────────────

// TestSidebarVolumeButtons verifies that vol+ and vol- buttons modify the
// graph node's volume parameter.
func TestSidebarVolumeButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["vol"] = true
	g.sidebar.layout()

	// Get initial volume.
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	initialVol := mn.Params.Volume

	// Fire vol+ callback.
	volPlus := g.sidebar.btns["vol+"]
	if volPlus == nil {
		t.Fatal("vol+ button should exist when vol section is open")
	}
	volPlus.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn2, _ := g.graph.GetNodeByID(n.ID)
	if mn2.Params.Volume <= initialVol {
		t.Fatalf("volume should increase after vol+: before=%v, after=%v", initialVol, mn2.Params.Volume)
	}

	// Fire vol- callback.
	volMinus := g.sidebar.btns["vol-"]
	if volMinus == nil {
		t.Fatal("vol- button should exist")
	}
	savedVol := mn2.Params.Volume
	volMinus.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn3, _ := g.graph.GetNodeByID(n.ID)
	if mn3.Params.Volume >= savedVol {
		t.Fatalf("volume should decrease after vol-: before=%v, after=%v", savedVol, mn3.Params.Volume)
	}
}

// TestSidebarPitchButtons verifies that pit+ and pit- buttons modify the
// graph node's pitch parameter.
func TestSidebarPitchButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["pit"] = true
	g.sidebar.layout()

	// Get initial pitch.
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	initialPitch := mn.Params.Pitch

	// Fire pit+ callback.
	pitPlus := g.sidebar.btns["pit+"]
	if pitPlus == nil {
		t.Fatal("pit+ button should exist when pit section is open")
	}
	pitPlus.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn2, _ := g.graph.GetNodeByID(n.ID)
	if mn2.Params.Pitch != initialPitch+1 {
		t.Fatalf("pitch should increase by 1 after pit+: before=%v, after=%v", initialPitch, mn2.Params.Pitch)
	}

	// Fire pit- callback.
	pitMinus := g.sidebar.btns["pit-"]
	if pitMinus == nil {
		t.Fatal("pit- button should exist")
	}
	pitMinus.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn3, _ := g.graph.GetNodeByID(n.ID)
	if mn3.Params.Pitch != initialPitch {
		t.Fatalf("pitch should return to initial after pit- following pit+: expected %v, got %v",
			initialPitch, mn3.Params.Pitch)
	}
}

// TestSidebarDurationButtons verifies that dur+ and dur- buttons modify the
// graph node's duration parameter.
func TestSidebarDurationButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["dur"] = true
	g.sidebar.layout()

	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	initialDur := mn.Params.Duration

	// Fire dur+ callback.
	durPlus := g.sidebar.btns["dur+"]
	if durPlus == nil {
		t.Fatal("dur+ button should exist when dur section is open")
	}
	durPlus.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn2, _ := g.graph.GetNodeByID(n.ID)
	expectedDur := initialDur + 0.1
	if math.Abs(mn2.Params.Duration-expectedDur) > 0.001 {
		t.Fatalf("duration should increase by 0.1 after dur+: expected ~%v, got %v",
			expectedDur, mn2.Params.Duration)
	}

	// Fire dur- callback.
	durMinus := g.sidebar.btns["dur-"]
	if durMinus == nil {
		t.Fatal("dur- button should exist")
	}
	durMinus.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn3, _ := g.graph.GetNodeByID(n.ID)
	if math.Abs(mn3.Params.Duration-initialDur) > 0.001 {
		t.Fatalf("duration should return to initial after dur- following dur+: expected ~%v, got %v",
			initialDur, mn3.Params.Duration)
	}
}

// ─── Additional Layout & Rect Tests ─────────────────────────────────────────

// TestSidebarPanelRectDimensions verifies that the panel rect is left-anchored
// with the correct width and spans the full grid height.
func TestSidebarPanelRectDimensions(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	panel, ok := g.sidebar.rects["panel"]
	if !ok || panel.Empty() {
		t.Fatal("panel rect should exist and be non-empty")
	}
	if panel.Min.X != 0 {
		t.Fatalf("panel should be left-anchored: Min.X=%d, want 0", panel.Min.X)
	}
	if panel.Dx() != g.sidebar.width {
		t.Fatalf("panel width should match sidebar width: panel.Dx()=%d, sidebar.width=%d",
			panel.Dx(), g.sidebar.width)
	}
	gridH := g.split.GridH(g.winH)
	if panel.Dy() != gridH {
		t.Fatalf("panel should span full grid height: panel.Dy()=%d, gridH=%d",
			panel.Dy(), gridH)
	}
}

// TestSidebarCloseButtonRectInHeader verifies that the close button rect is
// positioned within the header area.
func TestSidebarCloseButtonRectInHeader(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	closeR, ok := g.sidebar.rects["close"]
	if !ok || closeR.Empty() {
		t.Fatal("close rect should exist")
	}
	headerR, ok := g.sidebar.rects["header"]
	if !ok || headerR.Empty() {
		t.Fatal("header rect should exist")
	}

	// Close button should overlap with the header area.
	if !closeR.Overlaps(headerR) {
		t.Fatalf("close button rect %v should overlap header rect %v", closeR, headerR)
	}
}

// TestSidebarExpandedSectionCreatesButtonRects verifies that expanding a section
// causes the +/- button rects to be created in the layout.
func TestSidebarExpandedSectionCreatesButtonRects(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Volume section collapsed: no vol+/vol- rects.
	if _, ok := g.sidebar.rects["vol+"]; ok {
		t.Fatal("vol+ rect should not exist when section is collapsed")
	}
	if _, ok := g.sidebar.rects["vol-"]; ok {
		t.Fatal("vol- rect should not exist when section is collapsed")
	}

	// Expand volume section.
	g.sidebar.sectionOpen["vol"] = true
	g.sidebar.layout()

	if r, ok := g.sidebar.rects["vol+"]; !ok || r.Empty() {
		t.Fatal("vol+ rect should exist when vol section is expanded")
	}
	if r, ok := g.sidebar.rects["vol-"]; !ok || r.Empty() {
		t.Fatal("vol- rect should exist when vol section is expanded")
	}
}

// TestSidebarCloseSectionClosesDropdown verifies that closing the logic section
// also closes the logic dropdown.
func TestSidebarCloseSectionClosesDropdown(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// Close the logic section.
	g.sidebar.toggleSection("logic")

	if g.sidebar.sectionOpen["logic"] {
		t.Fatal("logic section should be closed after toggle")
	}
	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be closed when logic section is closed")
	}
}

// TestSidebarInputBounds verifies InputBounds returns the panel rect when open,
// and the expand tab rect when collapsed.
func TestSidebarInputBounds(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	// When open, InputBounds should match panel rect.
	bounds := g.sidebar.InputBounds()
	panel := g.sidebar.rects["panel"]
	if bounds != panel {
		t.Fatalf("InputBounds should match panel rect when open: got %v, want %v", bounds, panel)
	}

	// When collapsed, InputBounds should match expand tab rect.
	g.sidebar.collapsed = true
	bounds = g.sidebar.InputBounds()
	tabRect := g.sidebar.expandTabRect()
	if bounds != tabRect {
		t.Fatalf("InputBounds should match expand tab rect when collapsed: got %v, want %v",
			bounds, tabRect)
	}
}

// TestSidebarCapturing verifies that Capturing returns true only when resizing
// or scroll is active.
func TestSidebarCapturing(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	if g.sidebar.Capturing() {
		t.Fatal("Capturing should be false when idle")
	}

	// Simulate resize start.
	g.sidebar.resizing = true
	if !g.sidebar.Capturing() {
		t.Fatal("Capturing should be true when resizing")
	}
	g.sidebar.resizing = false
}

// TestSidebarHandleWheelWhenNotOpen verifies that HandleWheel returns
// InputIgnored when the sidebar is not open.
func TestSidebarHandleWheelWhenNotOpen(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	result := g.sidebar.HandleWheel(0, 0, 1)
	if result != InputIgnored {
		t.Fatalf("HandleWheel should return InputIgnored when not open, got %v", result)
	}
}

// TestSidebarLogicDropdownItemSetsKind verifies that clicking a logic dropdown
// item sets the node's LogicKind parameter.
func TestSidebarLogicDropdownItemSetsKind(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// Find the "probability" dropdown item button.
	btn, ok := g.sidebar.btns["logic:probability"]
	if !ok || btn == nil {
		t.Fatal("logic:probability button should exist when dropdown is open")
	}

	btn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn, _ := g.graph.GetNodeByID(n.ID)
	if mn.Params.LogicKind != "probability" {
		t.Fatalf("LogicKind should be 'probability', got %q", mn.Params.LogicKind)
	}
	if mn.Params.LogicP <= 0 {
		t.Fatal("LogicP should be set to a default > 0 when probability is selected")
	}
	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should close after selecting an item")
	}
}

// TestSidebarGrooveDropdownItemSetsKind verifies that clicking a groove dropdown
// item sets the node's GrooveKind parameter.
func TestSidebarGrooveDropdownItemSetsKind(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen["groove"] = true
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	// Find the "delay" dropdown item button.
	btn, ok := g.sidebar.btns["groove:delay"]
	if !ok || btn == nil {
		t.Fatal("groove:delay button should exist when dropdown is open")
	}

	btn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn, _ := g.graph.GetNodeByID(n.ID)
	if mn.Params.GrooveKind != "delay" {
		t.Fatalf("GrooveKind should be 'delay', got %q", mn.Params.GrooveKind)
	}
	if g.sidebar.grooveDropdownOpen {
		t.Fatal("groove dropdown should close after selecting an item")
	}
}

// TestSidebarDefaultWidth verifies that a newly created sidebar uses the
// default width.
func TestSidebarDefaultWidth(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	if g.sidebar.width != sidebarDefaultW {
		t.Fatalf("default sidebar width should be %d, got %d", sidebarDefaultW, g.sidebar.width)
	}
}

// TestSidebarZIndex verifies the z-index value.
func TestSidebarZIndex(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	if g.sidebar.ZIndex() != 200 {
		t.Fatalf("ZIndex should be 200, got %d", g.sidebar.ZIndex())
	}
}

// Ensure math import is used. The compiler will verify this; this is a
// compile-time guard.
var _ = image.Pt
var _ = math.Abs
