//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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
	// This test exercises the Splitter struct in isolation from any DrumView.
	// Reset the package-level click-suppress global so prior tests that open
	// menus/panels (which call SuppressClicksUntilMouseUp) don't interfere.
	suppressClicksUntilRelease = false
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

	// Also verify via HandleInput starts a drag when clicking on the pill handle
	hr := h.columnHandleRect(idx)
	hcx := (hr.Min.X + hr.Max.X) / 2
	hcy := (hr.Min.Y + hr.Max.Y) / 2
	result := h.HandleInput(hcx, hcy, true)
	if result != InputConsumed {
		t.Errorf("HandleInput should return InputConsumed when starting drag on handle, got %v", result)
	}
	if !h.Capturing() {
		t.Error("LayoutResizeHandler should be capturing after click on handle")
	}
}

// TestPopupBlocksBPMFocus verifies that clicking at the BPM box location
// while the instrument category selector popup is open does NOT focus the
// BPM text input. Popups must fully block input to elements beneath them.
func TestPopupBlocksBPMFocus(t *testing.T) {
	assertDefaultParityState(t)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, game_log.New(nil, game_log.LevelError))
	dv.recalcButtons()
	dv.calcLayout()

	if dv.bpmBox == nil {
		t.Fatal("bpmBox not initialised after calcLayout")
	}

	// Open instrument menu
	dv.instMenuOpen = true

	// Simulate a click at the BPM box center
	r := dv.bpmBox.Rect
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 300 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()

	if dv.bpmBox.Focused() {
		t.Fatal("BPM box gained focus while instrument menu was open — popup failed to block input")
	}
}

// TestPopupBlocksBPMFocus_AllPopups tests that every popup type blocks
// the BPM text input from gaining focus when clicked through.
func TestPopupBlocksBPMFocus_AllPopups(t *testing.T) {
	popups := []struct {
		name  string
		setup func(dv *DrumView)
	}{
		{"instMenu", func(dv *DrumView) { dv.instMenuOpen = true }},
		{"colorMenu", func(dv *DrumView) { dv.colorMenuOpen = true }},
		{"subdivMenu", func(dv *DrumView) { dv.subdivMenuOpen = true }},
		{"fxPanel", func(dv *DrumView) { dv.fxPanelOpen = true }},
		{"eqChannel", func(dv *DrumView) { dv.eqChannelOpen = true }},
		{"contextMenu", func(dv *DrumView) { dv.contextMenuOpen = true }},
		{"overflowMenu", func(dv *DrumView) { dv.overflowMenuOpen = true }},
	}

	for _, tc := range popups {
		t.Run(tc.name, func(t *testing.T) {
			assertDefaultParityState(t)
			audio.ClearAllInsertEffects()
			audio.InitInsertChains(44100)
			t.Cleanup(func() { audio.ClearAllInsertEffects() })

			dv := NewDrumView(image.Rect(0, 0, 800, 300), nil, game_log.New(nil, game_log.LevelError))
			dv.recalcButtons()
			dv.calcLayout()

			if dv.bpmBox == nil {
				t.Fatal("bpmBox not initialised")
			}

			// Open the popup
			tc.setup(dv)

			// Simulate click at BPM box center
			r := dv.bpmBox.Rect
			cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
			restore := SetInputForTest(
				func() (int, int) { return cx, cy },
				func(ebiten.MouseButton) bool { return true },
				func(ebiten.Key) bool { return false },
				func() []rune { return nil },
				func() (float64, float64) { return 0, 0 },
				func() (int, int) { return 800, 300 },
			)
			t.Cleanup(restore)
			dv.Update()
			restore()

			if dv.bpmBox.Focused() {
				t.Fatalf("BPM box gained focus while %s was open", tc.name)
			}
		})
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

	// Get the column divider and use handle center to initiate drag
	colX := dv.widgets.colPos[1] + dv.widgets.offset.X
	rackY := dv.widgets.rowPos[1] + dv.widgets.offset.Y + 10
	_, idx := h.detectColumnDivider(colX, rackY)
	if idx < 0 {
		t.Fatal("column divider not detected")
	}
	hr := h.columnHandleRect(idx)
	hcx := (hr.Min.X + hr.Max.X) / 2
	hcy := (hr.Min.Y + hr.Max.Y) / 2

	// Start drag on handle
	result := h.HandleInput(hcx, hcy, true)
	if result != InputConsumed {
		t.Fatalf("Expected InputConsumed when starting drag on handle, got %v", result)
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

// TestSliderCrossGroupIsolation_MainVolToEQ verifies that while dragging the
// main volume slider, moving the cursor over an EQ slider does not change EQ.
func TestSliderCrossGroupIsolation_MainVolToEQ(t *testing.T) {
	assertDefaultParityState(t)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Warm-up for layout.
	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if dv.mainVolSlider == nil || dv.mainVolSlider.Rect().Empty() {
		t.Skip("main vol slider not visible")
	}

	// Find an EQ slider that is visible.
	eqSliderIdx := -1
	for i, s := range dv.eqSliders {
		if s != nil && !s.Rect().Empty() {
			eqSliderIdx = i
			break
		}
	}
	if eqSliderIdx < 0 {
		t.Skip("no EQ sliders visible")
	}

	// Record initial EQ gain.
	origGain := dv.eqBandGainsDB[eqSliderIdx]
	origMainVol := dv.mainVolSlider.Value

	// Frame 1: Press on main vol slider center.
	mvr := dv.mainVolSlider.Rect()
	mvCx, mvCy := (mvr.Min.X+mvr.Max.X)/2, (mvr.Min.Y+mvr.Max.Y)/2

	r := SetInputForTest(
		func() (int, int) { return mvCx, mvCy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if dv.activeSliderKind != sliderKindMainVol {
		t.Fatalf("expected activeSliderKind=sliderKindMainVol, got %d", dv.activeSliderKind)
	}

	// Frame 2: Drag to the EQ slider's position (still holding mouse).
	eqR := dv.eqSliders[eqSliderIdx].Rect()
	eqCx, eqCy := eqR.Max.X-1, (eqR.Min.Y+eqR.Max.Y)/2

	r = SetInputForTest(
		func() (int, int) { return eqCx, eqCy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Main vol should have changed (dragged to a new position).
	if dv.activeSliderKind != sliderKindMainVol {
		t.Fatalf("activeSliderKind should still be sliderKindMainVol, got %d", dv.activeSliderKind)
	}

	// EQ gain should NOT have changed.
	if dv.eqBandGainsDB[eqSliderIdx] != origGain {
		t.Fatalf("EQ gain changed from %.2f to %.2f while main vol slider was active", origGain, dv.eqBandGainsDB[eqSliderIdx])
	}

	_ = origMainVol // used for documentation clarity
}

// TestSliderCrossGroupIsolation_RowVolToMainVol verifies that while dragging a
// row volume slider, the main volume slider is not affected.
func TestSliderCrossGroupIsolation_RowVolToMainVol(t *testing.T) {
	assertDefaultParityState(t)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     0.5,
	}}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if len(dv.rowVolSliders) == 0 || dv.rowVolSliders[0].Rect().Empty() {
		t.Skip("row vol slider not visible")
	}
	if dv.mainVolSlider == nil || dv.mainVolSlider.Rect().Empty() {
		t.Skip("main vol slider not visible")
	}

	origMainVol := dv.mainVolSlider.Value

	// Frame 1: Press on row vol slider.
	rvr := dv.rowVolSliders[0].Rect()
	rvCx, rvCy := (rvr.Min.X+rvr.Max.X)/2, (rvr.Min.Y+rvr.Max.Y)/2

	r := SetInputForTest(
		func() (int, int) { return rvCx, rvCy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if dv.activeSliderKind != sliderKindRowVol {
		t.Fatalf("expected activeSliderKind=sliderKindRowVol, got %d", dv.activeSliderKind)
	}

	// Frame 2: Drag to main vol slider position.
	mvr := dv.mainVolSlider.Rect()
	mvCx, mvCy := mvr.Max.X-1, (mvr.Min.Y+mvr.Max.Y)/2

	r = SetInputForTest(
		func() (int, int) { return mvCx, mvCy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	if dv.mainVolSlider.Value != origMainVol {
		t.Fatalf("main vol changed from %.2f to %.2f while row vol slider was active", origMainVol, dv.mainVolSlider.Value)
	}
}

// TestMainVolSliderReturnsAfterHandling verifies that clicking on the main
// volume slider does not also activate EQ sliders (the old fall-through bug).
func TestMainVolSliderReturnsAfterHandling(t *testing.T) {
	assertDefaultParityState(t)
	audio.ClearAllInsertEffects()
	audio.InitInsertChains(44100)
	t.Cleanup(func() { audio.ClearAllInsertEffects() })

	const W, H = 800, 600
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	warmUp := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	warmUp()

	if dv.mainVolSlider == nil || dv.mainVolSlider.Rect().Empty() {
		t.Skip("main vol slider not visible")
	}

	// Record all EQ gains before the click.
	origGains := make([]float64, len(dv.eqBandGainsDB))
	copy(origGains, dv.eqBandGainsDB)

	// Click at main vol slider center.
	mvr := dv.mainVolSlider.Rect()
	mvCx, mvCy := (mvr.Min.X+mvr.Max.X)/2, (mvr.Min.Y+mvr.Max.Y)/2

	r := SetInputForTest(
		func() (int, int) { return mvCx, mvCy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Verify no EQ gain changed.
	for i, g := range dv.eqBandGainsDB {
		if g != origGains[i] {
			t.Fatalf("EQ band %d gain changed from %.2f to %.2f after main vol click", i, origGains[i], g)
		}
	}
}

// TestAnyDragActive_IncludesMainVolSlider verifies that anyDragActive returns
// true when the main volume slider is being dragged.
func TestAnyDragActive_IncludesMainVolSlider(t *testing.T) {
	dv := &DrumView{
		rowScroll:    NewScrollBehavior(DefaultScrollbarStyle, 24),
		activeSlider: -1,
	}
	dv.mainVolSlider = NewSlider(1.0)

	if dv.anyDragActive() {
		t.Fatal("anyDragActive should be false initially")
	}

	// Simulate main vol slider dragging.
	dv.mainVolSlider.dragging = true
	if !dv.anyDragActive() {
		t.Fatal("anyDragActive should be true when mainVolSlider is dragging")
	}

	// Also test via activeSliderKind.
	dv.mainVolSlider.dragging = false
	dv.activeSliderKind = sliderKindMainVol
	if !dv.anyDragActive() {
		t.Fatal("anyDragActive should be true when activeSliderKind is set")
	}
}
