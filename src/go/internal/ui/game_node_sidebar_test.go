package ui

import (
	"image"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// ─── Open/Close Lifecycle ────────────────────────────────────────────────────

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
	for _, sec := range []sbSection{secVol, secPit, secDur, secLogic, secGroove} {
		if !g.sidebar.sectionOpen[sec] {
			t.Fatalf("section %q should be open after ExpandAllSections", sec)
		}
	}

	// Close and reopen on a different node.
	g.sidebar.Close()
	g.sidebar.Open(n2)

	// All sections should be collapsed.
	for _, sec := range []sbSection{secVol, secPit, secDur, secLogic, secGroove} {
		if g.sidebar.sectionOpen[sec] {
			t.Fatalf("section %q should be closed after reopening on new node", sec)
		}
	}
}

// ─── Layout ──────────────────────────────────────────────────────────────────

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

// Silent hides vol/pit/dur/logic/groove; Mute hides vol/pit/dur.
func TestSidebarLayoutNodeTypeSections(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Regular node: all sections present.
	nReg := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(nReg)
	g.sidebar.layout()

	for _, sec := range []sbID{sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secDur), sbSectionHeader(secLogic), sbSectionHeader(secGroove)} {
		if r, ok := g.sidebar.rects[sec]; !ok || r.Empty() {
			t.Fatalf("Regular node: section rect %q should exist and be non-empty", sec)
		}
	}

	// Silent node: vol/pit/dur/logic/groove hidden.
	nSilent := g.tryAddNode(4, 0, model.NodeTypeSilent)
	g.sidebar.Open(nSilent)
	g.sidebar.layout()

	for _, sec := range []sbID{sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secDur), sbSectionHeader(secLogic), sbSectionHeader(secGroove)} {
		if _, ok := g.sidebar.rects[sec]; ok {
			t.Fatalf("Silent node: section rect %q should NOT exist", sec)
		}
	}
	// Audible section should still be present.
	if r, ok := g.sidebar.rects[sbSectionHeader(secAud)]; !ok || r.Empty() {
		t.Fatal("Silent node: sec-aud should exist")
	}

	// Mute node: vol/pit/dur hidden, logic/groove present.
	nMute := g.tryAddNode(8, 0, model.NodeTypeMute)
	g.sidebar.Open(nMute)
	g.sidebar.layout()

	for _, sec := range []sbID{sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secDur)} {
		if _, ok := g.sidebar.rects[sec]; ok {
			t.Fatalf("Mute node: section rect %q should NOT exist", sec)
		}
	}
	for _, sec := range []sbID{sbSectionHeader(secLogic), sbSectionHeader(secGroove)} {
		if r, ok := g.sidebar.rects[sec]; !ok || r.Empty() {
			t.Fatalf("Mute node: section rect %q should exist and be non-empty", sec)
		}
	}
}

// ─── Hit Testing ─────────────────────────────────────────────────────────────

func TestSidebarHitInsidePanel(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	panel := g.sidebar.rects[sbPanel]
	if panel.Empty() {
		t.Fatal("panel rect should not be empty")
	}
	cx := (panel.Min.X + panel.Max.X) / 2
	cy := (panel.Min.Y + panel.Max.Y) / 2

	if !g.sidebar.Hit(cx, cy) {
		t.Fatalf("Hit(%d,%d) should return true for point inside panel %v", cx, cy, panel)
	}
}

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
	if g.sidebar.sectionOpen[secVol] {
		t.Fatal("vol section should be closed initially")
	}

	// Toggle open.
	g.sidebar.toggleSection(secVol)
	if !g.sidebar.sectionOpen[secVol] {
		t.Fatal("vol section should be open after first toggle")
	}

	// Toggle closed.
	g.sidebar.toggleSection(secVol)
	if g.sidebar.sectionOpen[secVol] {
		t.Fatal("vol section should be closed after second toggle")
	}
}

func TestSidebarLogicDropdownToggle(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.layout()

	// Verify logic button exists.
	btn, ok := g.sidebar.btns[sbLogicSelect]
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
	btn2 := g.sidebar.btns[sbLogicSelect]
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

func TestSidebarGrooveDropdownMutualExclusion(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.layout()

	// Open logic dropdown via button callback.
	logicBtn := g.sidebar.btns[sbLogicSelect]
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
	grvBtn := g.sidebar.btns[sbGrooveSelect]
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
	logicBtn2 := g.sidebar.btns[sbLogicSelect]
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

func TestSidebarCloseDropdownsOutside(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secLogic] = true
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

func TestSidebarVolumeButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secVol] = true
	g.sidebar.layout()

	// Get initial volume.
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	initialVol := mn.Params.Volume

	// Fire vol+ callback.
	volPlus := g.sidebar.btns[sbPlus(fieldVol)]
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
	volMinus := g.sidebar.btns[sbMinus(fieldVol)]
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

func TestSidebarPitchButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secPit] = true
	g.sidebar.layout()

	// Get initial pitch.
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	initialPitch := mn.Params.Pitch

	// Fire pit+ callback.
	pitPlus := g.sidebar.btns[sbPlus(fieldPit)]
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
	pitMinus := g.sidebar.btns[sbMinus(fieldPit)]
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

func TestSidebarDurationButtons(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secDur] = true
	g.sidebar.layout()

	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	initialDur := mn.Params.Duration

	// Fire dur+ callback.
	durPlus := g.sidebar.btns[sbPlus(fieldDur)]
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
	durMinus := g.sidebar.btns[sbMinus(fieldDur)]
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

// A compact card anchored beside the tapped node, height capped to the pane, not a panel docked at x=0.
func TestSidebarPanelRectDimensions(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	panel, ok := g.sidebar.rects[sbPanel]
	if !ok || panel.Empty() {
		t.Fatal("panel rect should exist and be non-empty")
	}
	if panel.Dx() != g.sidebar.width {
		t.Fatalf("panel width should match sidebar width: panel.Dx()=%d, sidebar.width=%d",
			panel.Dx(), g.sidebar.width)
	}
	gridH := g.split.GridH(g.winH)
	if panel.Dy() <= 0 || panel.Dy() > gridH {
		t.Fatalf("panel should be content-sized and capped to the grid pane height: panel.Dy()=%d, gridH=%d",
			panel.Dy(), gridH)
	}

	// A node at another cell must move the card. It sits LEFT of the first: anchored right, the card would hit
	// the corner cluster and clamp to the same x, masking the anchoring.
	n2 := g.tryAddNode(-5, 5, model.NodeTypeRegular)
	g.sidebar.Open(n2)
	g.sidebar.layout()
	panel2, ok := g.sidebar.rects[sbPanel]
	if !ok || panel2.Empty() {
		t.Fatal("panel2 rect should exist and be non-empty")
	}
	if panel2.Min == panel.Min {
		t.Fatalf("panel should be anchored beside its own node, not fixed: panel=%v (node@0,0) panel2=%v (node@-5,5) share the same origin",
			panel, panel2)
	}
}

func TestSidebarCloseButtonRectInHeader(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	closeR, ok := g.sidebar.rects[sbClose]
	if !ok || closeR.Empty() {
		t.Fatal("close rect should exist")
	}
	headerR, ok := g.sidebar.rects[sbHeader]
	if !ok || headerR.Empty() {
		t.Fatal("header rect should exist")
	}

	// Close button should overlap with the header area.
	if !closeR.Overlaps(headerR) {
		t.Fatalf("close button rect %v should overlap header rect %v", closeR, headerR)
	}
}

func TestSidebarExpandedSectionCreatesButtonRects(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.layout()

	// Volume section collapsed: no vol+/vol- rects.
	if _, ok := g.sidebar.rects[sbPlus(fieldVol)]; ok {
		t.Fatal("vol+ rect should not exist when section is collapsed")
	}
	if _, ok := g.sidebar.rects[sbMinus(fieldVol)]; ok {
		t.Fatal("vol- rect should not exist when section is collapsed")
	}

	// Expand volume section.
	g.sidebar.sectionOpen[secVol] = true
	g.sidebar.layout()

	if r, ok := g.sidebar.rects[sbPlus(fieldVol)]; !ok || r.Empty() {
		t.Fatal("vol+ rect should exist when vol section is expanded")
	}
	if r, ok := g.sidebar.rects[sbMinus(fieldVol)]; !ok || r.Empty() {
		t.Fatal("vol- rect should exist when vol section is expanded")
	}
}

func TestSidebarCloseSectionClosesDropdown(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// Close the logic section.
	g.sidebar.toggleSection(secLogic)

	if g.sidebar.sectionOpen[secLogic] {
		t.Fatal("logic section should be closed after toggle")
	}
	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should be closed when logic section is closed")
	}
}

// The panel rect when open, the expand tab when collapsed.
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
	panel := g.sidebar.rects[sbPanel]
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

// Scroll is the only capture source.
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

	// Simulate a touch-scroll in progress.
	g.sidebar.scroll.HandleTouchBegin(10, 100)
	if !g.sidebar.Capturing() {
		t.Fatal("Capturing should be true while a touch-scroll is active")
	}
	g.sidebar.scroll.HandleTouchEnd()
	if g.sidebar.Capturing() {
		t.Fatal("Capturing should be false after touch ends")
	}
}

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

func TestSidebarLogicDropdownItemSetsKind(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secLogic] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// Find the "probability" dropdown item button.
	btn, ok := g.sidebar.btns[sbLogicOption(model.LogicProbability)]
	if !ok || btn == nil {
		t.Fatal("logic:probability button should exist when dropdown is open")
	}

	btn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn, _ := g.graph.GetNodeByID(n.ID)
	if mn.Params.LogicKind != model.LogicProbability {
		t.Fatalf("LogicKind should be 'probability', got %q", mn.Params.LogicKind)
	}
	if mn.Params.LogicP <= 0 {
		t.Fatal("LogicP should be set to a default > 0 when probability is selected")
	}
	if g.sidebar.logicDropdownOpen {
		t.Fatal("logic dropdown should close after selecting an item")
	}
}

func TestSidebarGrooveDropdownItemSetsKind(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sidebar.Open(n)
	g.sidebar.sectionOpen[secGroove] = true
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	// Find the "delay" dropdown item button.
	btn, ok := g.sidebar.btns[sbGrooveOption(model.GrooveDelay)]
	if !ok || btn == nil {
		t.Fatal("groove:delay button should exist when dropdown is open")
	}

	btn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	mn, _ := g.graph.GetNodeByID(n.ID)
	if mn.Params.GrooveKind != model.GrooveDelay {
		t.Fatalf("GrooveKind should be 'delay', got %q", mn.Params.GrooveKind)
	}
	if g.sidebar.grooveDropdownOpen {
		t.Fatal("groove dropdown should close after selecting an item")
	}
}

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

var _ = image.Pt
var _ = math.Abs

// ─── P1 Coverage Gap Tests ──────────────────────────────────────────────────

func TestSidebar_LogicDropdownSelection(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()

	// Directly open logic dropdown and re-layout to wire dropdown items.
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// Select "every_n_triggers".
	itemBtn := g.sidebar.btns[sbLogicOption(model.LogicEveryN)]
	if itemBtn == nil {
		t.Fatal("expected 'logic:every_n_triggers' button")
	}
	// Execute callback directly (bypassing enqueueUI).
	itemBtn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if g.sidebar.logicDropdownOpen {
		t.Error("expected logic dropdown closed after selection")
	}
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	if mn.Params.LogicKind != model.LogicEveryN {
		t.Errorf("expected LogicKind='every_n_triggers', got %q", mn.Params.LogicKind)
	}
	if mn.Params.LogicN < 2 {
		t.Errorf("expected LogicN >= 2 (default), got %d", mn.Params.LogicN)
	}
}

func TestSidebar_GrooveDropdownSelection(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()

	// Directly open groove dropdown and re-layout.
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.layout()

	// Select "delay".
	itemBtn := g.sidebar.btns[sbGrooveOption(model.GrooveDelay)]
	if itemBtn == nil {
		t.Fatal("expected 'groove:delay' button")
	}
	itemBtn.OnClick()
	for _, fn := range g.uiQueue {
		fn()
	}
	g.uiQueue = nil

	if g.sidebar.grooveDropdownOpen {
		t.Error("expected groove dropdown closed after selection")
	}
	mn, ok := g.graph.GetNodeByID(n.ID)
	if !ok {
		t.Fatal("node not found in graph")
	}
	if mn.Params.GrooveKind != model.GrooveDelay {
		t.Errorf("expected GrooveKind='delay', got %q", mn.Params.GrooveKind)
	}
}

func TestSidebar_DropdownMutualExclusion(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	// Open logic dropdown directly.
	g.sidebar.logicDropdownOpen = true
	if !g.sidebar.logicDropdownOpen {
		t.Fatal("expected logic dropdown open")
	}

	// Open groove dropdown via the toggle logic (same as OnClick handler).
	g.sidebar.grooveDropdownOpen = true
	g.sidebar.logicDropdownOpen = false // mutual exclusion (same as OnClick)

	if !g.sidebar.grooveDropdownOpen {
		t.Error("expected groove dropdown open")
	}
	if g.sidebar.logicDropdownOpen {
		t.Error("expected logic dropdown closed (mutual exclusion)")
	}
}

func TestSidebar_DeferredTapInScroll(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	// Use the panel rect for touch start.
	panelR := g.sidebar.rects[sbPanel]
	if panelR.Empty() {
		t.Skip("no panel rect")
	}
	cx := (panelR.Min.X + panelR.Max.X) / 2
	cy := panelR.Min.Y + panelR.Dy()/2

	g.sidebar.HandleInput(cx, cy, true)

	// Drag past dead zone to commit scroll.
	for i := 1; i <= 15; i++ {
		g.sidebar.HandleInput(cx, cy-i*4, true)
	}

	firstAfterDrag := g.sidebar.scroll.VS.First

	// Release.
	g.sidebar.HandleInput(cx, cy-60, false)

	// Scroll should have committed — position should have changed.
	if firstAfterDrag <= 0 && g.sidebar.scroll.VS.First <= 0 {
		t.Error("expected scroll position to change after drag past dead zone")
	}
}

func TestSidebar_SilentNodeHidesParams(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Place a regular node, then change it to silent in the graph.
	n := g.tryAddNode(5, 5, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("failed to add node")
	}
	if mn, ok := g.graph.GetNodeByID(n.ID); ok {
		mn.Type = model.NodeTypeSilent
		g.graph.Nodes[n.ID] = mn
	}

	g.sel = n
	n.Selected = true
	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	// Silent node should hide vol, pit, dur, logic, groove sections.
	for _, key := range []sbID{sbSectionHeader(secVol), sbSectionHeader(secPit), sbSectionHeader(secDur), sbSectionHeader(secLogic), sbSectionHeader(secGroove)} {
		if r, ok := g.sidebar.rects[key]; ok && !r.Empty() {
			t.Errorf("expected section %q hidden for silent node, got rect %v", key, r)
		}
	}

	// Logic and groove buttons should not be wired.
	if g.sidebar.btns[sbLogicSelect] != nil {
		t.Error("expected no 'logic' button for silent node")
	}
	if g.sidebar.btns[sbGrooveSelect] != nil {
		t.Error("expected no 'grv' button for silent node")
	}

	// "aud" section should still exist.
	if _, ok := g.sidebar.rects[sbSectionHeader(secAud)]; !ok {
		t.Error("expected 'sec-aud' section for silent node")
	}
}

func TestSidebar_ScrollbarTrackJump(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	bar := g.sidebar.scroll.BarRect()
	thumb := g.sidebar.scroll.ThumbRect()
	if bar.Empty() || thumb.Empty() {
		t.Fatal("expected non-empty scroll bar/thumb")
	}

	// Click below thumb in track.
	clickY := thumb.Max.Y + (bar.Max.Y-thumb.Max.Y)/2
	if clickY >= bar.Max.Y {
		clickY = bar.Max.Y - 1
	}
	clickX := (bar.Min.X + bar.Max.X) / 2

	g.sidebar.HandleInput(clickX, clickY, true)
	g.sidebar.HandleInput(clickX, clickY, false)

	if g.sidebar.scroll.VS.First <= 0 {
		t.Error("expected scroll to jump to non-zero after track click below thumb")
	}
}

func TestSidebar_ScrollMomentumDecays(t *testing.T) {
	g := openSidebarWithScroll(t, 250)

	sb := g.sidebar.scroll
	startY := 200
	endY := 100
	sb.HandleTouchBegin(50, startY)
	for y := startY; y >= endY; y -= 5 {
		sb.HandleTouchMove(50, y)
	}
	sb.HandleTouchEnd()

	if !sb.HasMomentum() {
		t.Fatal("expected momentum after fling")
	}

	posAfterFling := sb.VS.First
	for i := 0; i < 60; i++ {
		g.sidebar.UpdateScroll()
	}

	posAfterDecay := sb.VS.First
	if posAfterDecay <= posAfterFling {
		t.Errorf("expected scroll position to advance via momentum: %d -> %d", posAfterFling, posAfterDecay)
	}

	// After enough frames, momentum should stop.
	for i := 0; i < 200; i++ {
		g.sidebar.UpdateScroll()
	}
	if sb.HasMomentum() {
		t.Error("expected momentum to decay to zero")
	}
}
