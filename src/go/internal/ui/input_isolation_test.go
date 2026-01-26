//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestInputIsolation_EQPanelDoesNotTriggerSplitter(t *testing.T) {
	// Setup: splitter at Y=300, drum pane below (includes EQ panel)
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	// Simulate click in the EQ panel area (well below the splitter)
	eqPanelY := 500 // Below the splitter in the drum pane
	result := s.HandleInput(400, eqPanelY, true)

	if result != InputIgnored {
		t.Errorf("splitter should ignore clicks from EQ panel area, got %v", result)
	}
	if s.dragging {
		t.Error("splitter should not start dragging from EQ panel clicks")
	}
}

func TestInputIsolation_DrumViewBlocksAtSpatialCheck(t *testing.T) {
	dv := &DrumView{
		Bounds: image.Rect(0, 300, 800, 600),
	}

	// Point outside drum view bounds (in grid pane)
	if dv.BlocksAt(400, 100) {
		t.Error("DrumView should not block at points outside its bounds")
	}

	// Point inside drum view bounds, no overlay open
	if dv.BlocksAt(400, 400) {
		t.Error("DrumView should not block when no overlays are open")
	}

	// Point inside drum view bounds, overlay open
	dv.instMenuOpen = true
	if !dv.BlocksAt(400, 400) {
		t.Error("DrumView should block when instrument menu is open")
	}

	// Point outside bounds even with overlay open
	if dv.BlocksAt(400, 100) {
		t.Error("DrumView should not block outside bounds even with overlay open")
	}
}

func TestInputIsolation_SplitterRespectsGrabZone(t *testing.T) {
	s := NewSplitter(600)
	s.Y = 300
	s.winW = 800

	tests := []struct {
		name     string
		y        int
		expected InputResult
	}{
		{"above grab zone", 200, InputIgnored},          // Too far above
		{"at grab zone top", 295, InputCaptured},        // Within 5px grab
		{"on divider", 300, InputCaptured},              // Exactly on divider
		{"at grab zone bottom", 305, InputCaptured},     // Within 5px grab
		{"below grab zone", 310, InputIgnored},          // Just outside (below)
		{"well below in drum pane", 400, InputIgnored},  // In drum pane
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s.dragging = false // Reset state
			result := s.HandleInput(400, tc.y, true)
			if result != tc.expected {
				t.Errorf("at y=%d: expected %v, got %v", tc.y, tc.expected, result)
			}
		})
	}
}

// TestInputIsolation_AnyDropdownOpen tests the anyDropdownOpen() helper method
// that consolidates all dropdown menu state checks.
func TestInputIsolation_AnyDropdownOpen(t *testing.T) {
	dv := &DrumView{}

	// No dropdowns open
	if dv.anyDropdownOpen() {
		t.Error("anyDropdownOpen() should be false when no dropdowns are open")
	}

	// Test each dropdown individually
	dropdownTests := []struct {
		name  string
		setup func()
	}{
		{"subdiv menu", func() { dv.subdivMenuOpen = true }},
		{"instrument menu", func() { dv.instMenuOpen = true }},
		{"color menu", func() { dv.colorMenuOpen = true }},
		{"eq channel menu", func() { dv.eqChannelOpen = true }},
	}

	for _, tc := range dropdownTests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset all
			dv.subdivMenuOpen = false
			dv.instMenuOpen = false
			dv.colorMenuOpen = false
			dv.eqChannelOpen = false

			tc.setup()

			if !dv.anyDropdownOpen() {
				t.Errorf("anyDropdownOpen() should be true when %s is open", tc.name)
			}
		})
	}
}

func TestInputIsolation_OverlayBlocksInput(t *testing.T) {
	dv := &DrumView{
		Bounds: image.Rect(0, 300, 800, 600),
	}

	overlayTests := []struct {
		name   string
		setup  func()
		blocks bool
	}{
		{"no overlay", func() {}, false},
		{"instrument menu", func() { dv.instMenuOpen = true }, true},
		{"color menu", func() { dv.colorMenuOpen = true }, true},
		{"subdiv menu", func() { dv.subdivMenuOpen = true }, true},
		{"eq channel menu", func() { dv.eqChannelOpen = true }, true},
		{"inst hold", func() { dv.instHold = true }, true},
		{"rename box", func() { dv.renameBox = &TextInput{} }, true},
		{"naming", func() { dv.naming = true }, true},
	}

	for _, tc := range overlayTests {
		t.Run(tc.name, func(t *testing.T) {
			// Reset state
			dv.instMenuOpen = false
			dv.colorMenuOpen = false
			dv.subdivMenuOpen = false
			dv.eqChannelOpen = false
			dv.instHold = false
			dv.renameBox = nil
			dv.naming = false

			tc.setup()

			// Test point inside drum view bounds
			if dv.BlocksAt(400, 400) != tc.blocks {
				t.Errorf("expected BlocksAt to return %v", tc.blocks)
			}
		})
	}
}

// Test that the column divider is NOT detected in the EQ panel area where
// the Wave widget spans both columns. This is a regression test for the bug
// where the resizer could be triggered from the EQ panel.
func TestInputIsolation_ColumnDividerNotInEQArea(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), g, logger)
	dv.widgets.SetBounds(dv.Bounds)
	dv.refreshWidgetLayout()

	h := dv.layoutHandler

	// Get the column divider X position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X

	// Get a Y position in the EQ/Wave area (row 2)
	eqY := dv.widgets.rowPos[2] + dv.widgets.offset.Y + 30 // 30px into EQ area

	// Attempt to detect column divider at EQ position
	axis, idx := h.detectColumnDivider(colX, eqY)

	if axis != "" || idx != -1 {
		t.Errorf("Column divider should NOT be detected in EQ area (spans both columns): got axis=%q idx=%d", axis, idx)
	}

	// Also verify via HandleInput
	result := h.HandleInput(colX, eqY, true)
	if result != InputIgnored {
		t.Errorf("HandleInput should return InputIgnored in EQ area, got %v", result)
	}
	if h.Capturing() {
		t.Error("LayoutResizeHandler should not be capturing after click in EQ area")
	}
}

// Test that the column divider IS detected in the Rack area (row 1) where
// no widget spans both columns.
func TestInputIsolation_ColumnDividerValidInRackArea(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), g, logger)
	dv.widgets.SetBounds(dv.Bounds)
	dv.refreshWidgetLayout()

	h := dv.layoutHandler

	// Get the column divider X position
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X

	// Get a Y position in the Rack area (row 1)
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10 // 10px into row 1

	// Verify column divider IS detected in Rack area
	axis, idx := h.detectColumnDivider(colX, rackY)

	if axis != "col" || idx != 0 {
		t.Errorf("Column divider SHOULD be detected in Rack area: got axis=%q idx=%d, expected axis=\"col\" idx=0", axis, idx)
	}

	// Also verify via HandleInput starts a drag
	result := h.HandleInput(colX, rackY, true)
	if result != InputConsumed {
		t.Errorf("HandleInput should return InputConsumed when starting drag in Rack area, got %v", result)
	}
	if !h.Capturing() {
		t.Error("LayoutResizeHandler should be capturing after click on divider in Rack area")
	}
}

// Test capture semantics: drag continues even when cursor moves outside bounds.
func TestInputIsolation_LayoutHandlerCapture(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 300, 800, 600), g, logger)
	dv.widgets.SetBounds(dv.Bounds)
	dv.refreshWidgetLayout()

	h := dv.layoutHandler

	// Get the column divider position in row 1 (valid area)
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10

	// Start drag
	result := h.HandleInput(colX, rackY, true)
	if result != InputConsumed {
		t.Fatalf("Expected InputConsumed when starting drag, got %v", result)
	}
	if !h.Capturing() {
		t.Fatal("Expected Capturing() to be true after starting drag")
	}

	// Move cursor outside DrumView bounds (into grid pane above)
	outsideY := dv.Bounds.Min.Y - 100 // 100px above drum view

	// Should still be captured
	result = h.HandleInput(colX, outsideY, true)
	if result != InputCaptured {
		t.Errorf("Expected InputCaptured during drag outside bounds, got %v", result)
	}
	if !h.Capturing() {
		t.Error("Drag should continue outside bounds")
	}

	// Release mouse
	result = h.HandleInput(colX, outsideY, false)
	if h.Capturing() {
		t.Error("Drag should end after mouse release")
	}

	// After release, click outside bounds should be ignored
	result = h.HandleInput(colX, outsideY, true)
	if result != InputIgnored {
		t.Errorf("After drag ends, click outside bounds should be InputIgnored, got %v", result)
	}
}
