//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// clearClickSuppressionInst clears the global click suppression state for tests.
func clearClickSuppressionInst(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })
}

func TestInstrumentMenuComponent_OpenClose(t *testing.T) {
	comp := NewInstrumentMenuComponent()

	// Initially closed
	if comp.IsOpen() {
		t.Error("expected menu to be closed initially")
	}

	// Set props with ForceCategories to start in categories mode
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        image.Rect(0, 50, 300, 500),
		CurrentInstrument: "kick",
		Categories:        []string{"Kicks", "Snares"},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: "Kicks"},
			{ID: "snare", Label: "Snare", Category: "Snares"},
			{ID: "hihat", Label: "Hi-Hat", Category: "Cymbals"},
		},
		RowHeight:       24,
		LabelWidth:      100,
		ControlsWidth:   200,
		ForceCategories: true, // Start in categories mode
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected menu to be open after Open()")
	}

	// Should start in categories mode (since ForceCategories is set)
	if comp.Mode() != InstMenuModeCategories {
		t.Errorf("expected categories mode, got %s", comp.Mode())
	}

	// Close
	comp.Close()
	if comp.IsOpen() {
		t.Error("expected menu to be closed after Close()")
	}
}

func TestInstrumentMenuComponent_CategoriesMode(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        image.Rect(0, 50, 300, 500),
		CurrentInstrument: "kick",
		Categories:        []string{"Kicks", "Snares", "Cymbals"},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: "Kicks"},
			{ID: "snare", Label: "Snare", Category: "Snares"},
			{ID: "hihat", Label: "Hi-Hat", Category: "Cymbals"},
		},
		RowHeight:       24,
		ForceCategories: true, // Start in categories mode
	})
	comp.Open()

	// Should start in categories mode (since ForceCategories is set)
	if comp.Mode() != InstMenuModeCategories {
		t.Errorf("expected categories mode, got %s", comp.Mode())
	}

	// Should have category buttons
	if len(comp.categoryBtns) == 0 {
		t.Error("expected category buttons to be created")
	}

	// Active category should be set based on current instrument
	if comp.ActiveCategory() != "Kicks" {
		t.Errorf("expected active category 'Kicks', got '%s'", comp.ActiveCategory())
	}
}

func TestInstrumentMenuComponent_InstrumentsMode(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:        image.Rect(10, 100, 100, 124),
		VertBounds:        image.Rect(0, 50, 300, 500),
		CurrentInstrument: "kick",
		Categories:        []string{}, // No categories - go directly to instruments
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
			{ID: "snare", Label: "Snare", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// Should start in instruments mode (no categories)
	if comp.Mode() != InstMenuModeInstruments {
		t.Errorf("expected instruments mode, got %s", comp.Mode())
	}

	// Should have instrument buttons
	if len(comp.instBtns) == 0 {
		t.Error("expected instrument buttons to be created")
	}
}

func TestInstrumentMenuComponent_OnSelectCallback(t *testing.T) {
	clearClickSuppressionInst(t)

	comp := NewInstrumentMenuComponent()

	var selectedID string
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
			{ID: "snare", Label: "Snare", Category: ""},
		},
		RowHeight: 24,
		OnSelect: func(instID string) {
			selectedID = instID
		},
	})
	comp.Open()

	// Clear suppression again (Open calls SuppressClicksUntilMouseUp)
	suppressClicksUntilRelease = false

	// Find and click the first instrument button
	if len(comp.instBtns) == 0 {
		t.Fatal("no instrument buttons")
	}

	btn := comp.instBtns[0]
	btnRect := btn.Rect()

	// Press: with deferred tap, this starts touch tracking but doesn't fire yet
	result := comp.HandleInput(btnRect.Min.X+5, btnRect.Min.Y+5, true)
	if result == InputIgnored {
		t.Error("expected input to be consumed on button press")
	}

	// Release: deferred tap fires the button on release
	result = comp.HandleInput(btnRect.Min.X+5, btnRect.Min.Y+5, false)
	if result == InputIgnored {
		t.Error("expected input to be consumed on button release")
	}

	// Button callback is fired on release, so menu should be closed
	if comp.IsOpen() {
		t.Error("expected menu to be closed after selection")
	}

	// Verify callback was invoked
	if selectedID != "kick" {
		t.Errorf("expected selectedID 'kick', got '%s'", selectedID)
	}
}

func TestInstrumentMenuComponent_HoldCapture(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// Close and verify hold state
	comp.Close()

	// Should be capturing after close (hold is true)
	if !comp.Capturing() {
		t.Error("expected Capturing() to be true after closing")
	}

	// HandleInput should capture while holding
	result := comp.HandleInput(50, 50, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured while holding, got %v", result)
	}

	// Release should clear hold
	result = comp.HandleInput(50, 50, false)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on release, got %v", result)
	}

	// After release, should no longer be capturing
	if comp.Capturing() {
		t.Error("expected Capturing() to be false after mouse release")
	}
}

func TestInstrumentMenuComponent_ClickOutsideCloses(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()

	var closeCalled bool
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
		},
		RowHeight: 24,
		OnClose: func() {
			closeCalled = true
		},
	})
	comp.Open()

	// Clear suppression state (simulates user releasing mouse after opening)
	suppressClicksUntilRelease = false

	// Click far outside the menu bounds
	result := comp.HandleInput(500, 500, true)
	if result != InputConsumed {
		t.Errorf("expected click outside to be consumed, got %v", result)
	}

	if !closeCalled {
		t.Error("expected OnClose callback to be called")
	}
	if comp.IsOpen() {
		t.Error("expected menu to be closed after click outside")
	}
}

func TestInstrumentMenuComponent_SearchFiltering(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick Drum", Category: ""},
			{ID: "snare", Label: "Snare Drum", Category: ""},
			{ID: "hihat", Label: "Hi-Hat", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// Verify all instruments shown initially
	initialCount := len(comp.instBtns)
	if initialCount == 0 {
		t.Fatal("no instrument buttons initially")
	}

	// Simulate search by directly setting state and rebuilding
	comp.state.searchText = "kick"
	comp.rebuildMenu()

	// Should show fewer results
	if len(comp.state.filteredInsts) != 1 {
		t.Errorf("expected 1 filtered instrument, got %d", len(comp.state.filteredInsts))
	}
	if comp.state.filteredInsts[0] != "kick" {
		t.Errorf("expected 'kick' to match search, got '%s'", comp.state.filteredInsts[0])
	}
}

func TestInstrumentMenuComponent_CategoryFiltering(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{"Kicks", "Snares"},
		Instruments: []InstrumentOption{
			{ID: "kick1", Label: "Kick 1", Category: "Kicks"},
			{ID: "kick2", Label: "Kick 2", Category: "Kicks"},
			{ID: "snare1", Label: "Snare 1", Category: "Snares"},
		},
		RowHeight: 24,
	})
	comp.Open()

	// Switch to instruments mode with active category
	comp.state.activeCat = "Kicks"
	comp.state.mode = InstMenuModeInstruments
	comp.rebuildMenu()

	// Should only show kicks
	if len(comp.state.filteredInsts) != 2 {
		t.Errorf("expected 2 filtered instruments (kicks only), got %d", len(comp.state.filteredInsts))
	}
	for _, id := range comp.state.filteredInsts {
		if comp.state.categoryByID[id] != "Kicks" {
			t.Errorf("expected category 'Kicks', got '%s' for instrument '%s'",
				comp.state.categoryByID[id], id)
		}
	}
}

func TestInstrumentMenuComponent_HandleInputWhenClosed(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
		},
		RowHeight: 24,
	})

	// Don't open the menu, input should be ignored
	result := comp.HandleInput(50, 100, true)
	if result != InputIgnored {
		t.Error("expected input to be ignored when menu is closed")
	}
}

func TestInstrumentMenuComponent_BoundsSet(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()

	// After open, bounds should be non-empty
	bounds := comp.Bounds()
	if bounds.Empty() {
		t.Error("expected non-empty bounds after opening")
	}
}

func TestInstrumentMenuComponent_EmptyInstruments(t *testing.T) {
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:  image.Rect(10, 100, 100, 124),
		VertBounds:  image.Rect(0, 50, 300, 500),
		Categories:  []string{},
		Instruments: []InstrumentOption{},
		RowHeight:   24,
	})
	comp.Open()

	// Should handle empty instruments gracefully
	// In instruments mode, should show "No matches" placeholder
	if comp.Mode() != InstMenuModeInstruments {
		t.Errorf("expected instruments mode, got %s", comp.Mode())
	}
	// Should have one placeholder button
	if len(comp.instBtns) != 1 {
		t.Errorf("expected 1 placeholder button, got %d", len(comp.instBtns))
	}
}

func TestInstrumentMenuComponent_ScrollState(t *testing.T) {
	// Create menu with many instruments to trigger scrolling
	insts := []InstrumentOption{}
	for i := 0; i < 20; i++ {
		insts = append(insts, InstrumentOption{
			ID:       "inst" + string(rune('a'+i)),
			Label:    "Instrument " + string(rune('A'+i)),
			Category: "",
		})
	}

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:  image.Rect(10, 100, 100, 124),
		VertBounds:  image.Rect(0, 50, 300, 400), // Limited height
		Categories:  []string{},
		Instruments: insts,
		RowHeight:   24,
	})
	comp.Open()

	scroll := comp.Scroll()
	if scroll.Total != 20 {
		t.Errorf("expected scroll.Total = 20, got %d", scroll.Total)
	}
	if !scroll.HasScroll() {
		t.Error("expected scrolling to be enabled")
	}
}

func TestInstrumentMenuComponent_AnchorRectConsumed(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()

	// Use a layout where the anchor rect does NOT overlap the scroll view
	// (desktop-like: menu opens downward, anchor is above the menu).
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 50, 100, 74),
		VertBounds: image.Rect(0, 0, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
			{ID: "snare", Label: "Snare", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()
	// Clear suppression for this test to isolate the anchor rect behavior.
	suppressClicksUntilRelease = false

	if !comp.IsOpen() {
		t.Fatal("menu should be open")
	}

	// The anchor rect should NOT overlap with the scroll view.
	ax := comp.props.AnchorRect.Min.X + 5
	ay := comp.props.AnchorRect.Min.Y + 5

	// Verify the anchor point is outside the fullRect/scrollView.
	pt := image.Pt(ax, ay)
	if pt.In(comp.fullRect) {
		t.Skipf("anchor overlaps fullRect in this layout — test not applicable")
	}

	// Press at the anchor rect. The "click outside" check at line 771
	// explicitly excludes AnchorRect. Without a consuming guard, this would
	// return InputIgnored and fall through to the row label.
	// With the suppression guard: when suppression is active, it is consumed.
	// Without suppression: it returns InputIgnored (allowing toggle on deliberate click).
	result := comp.HandleInput(ax, ay, true)
	// Without suppression, anchor rect press should be InputIgnored (allows toggle).
	if result != InputIgnored {
		t.Errorf("expected InputIgnored for anchor rect press without suppression, got %v", result)
	}

	// Now test with suppression active (simulates the post-Open window).
	suppressClicksUntilRelease = true
	result = comp.HandleInput(ax, ay, true)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed for anchor rect press during suppression, got %v", result)
	}
	if !comp.IsOpen() {
		t.Error("menu should still be open during suppression")
	}
}

func TestInstrumentMenuComponent_SuppressedPressConsumed(t *testing.T) {
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick", Category: ""},
			{ID: "snare", Label: "Snare", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()
	// Suppress is now owned by the tree, not the global flag.
	// Simulate the tree's suppress being active (as it would be after Open).
	suppressClicksUntilRelease = true

	// Press at a point outside both fullRect and anchorRect.
	// With suppression active, this must still be consumed (not ignored)
	// to prevent fall-through to other handlers during geometry transitions.
	result := comp.HandleInput(500, 500, true)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed for suppressed press outside menu, got %v", result)
	}
	if !comp.IsOpen() {
		t.Error("menu should still be open during suppressed press")
	}
}

// ---------- Search box keyboard input tests ----------

// newInstMenuInInstrumentsMode creates an InstrumentMenuComponent already open
// in instruments mode with a search box, ready for search tests.
func newInstMenuInInstrumentsMode(t *testing.T) *InstrumentMenuComponent {
	t.Helper()
	clearClickSuppressionInst(t)
	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect: image.Rect(10, 100, 100, 124),
		VertBounds: image.Rect(0, 50, 300, 500),
		Categories: []string{},
		Instruments: []InstrumentOption{
			{ID: "kick", Label: "Kick Drum", Category: ""},
			{ID: "snare", Label: "Snare Drum", Category: ""},
			{ID: "hihat", Label: "Hi-Hat", Category: ""},
			{ID: "clap", Label: "Clap", Category: ""},
		},
		RowHeight: 24,
	})
	comp.Open()
	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %s", comp.Mode())
	}
	if comp.searchBox == nil {
		t.Fatal("searchBox should exist in instruments mode")
	}
	return comp
}

func TestInstMenuSearchBox_ClickToFocus(t *testing.T) {
	comp := newInstMenuInInstrumentsMode(t)

	// Mock cursor inside search box rect + mouse pressed → searchBox.Update() focuses it.
	sr := comp.searchBox.Rect
	cx, cy := sr.Min.X+2, sr.Min.Y+2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	comp.Update()
	if !comp.searchBox.Focused() {
		t.Error("searchBox should be focused after click via Update()")
	}
}

func TestInstMenuSearchBox_TypeCharsUpdatesFilter(t *testing.T) {
	comp := newInstMenuInInstrumentsMode(t)

	// First, focus the search box via a click.
	sr := comp.searchBox.Rect
	cx, cy := sr.Min.X+2, sr.Min.Y+2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	if !comp.searchBox.Focused() {
		t.Fatal("searchBox should be focused after click")
	}

	// Now type "kick" — consume-once pattern: return chars once, then nil.
	chars := []rune{'k', 'i', 'c', 'k'}
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	comp.Update()

	if comp.searchBox.Value() != "kick" {
		t.Errorf("expected searchBox value 'kick', got %q", comp.searchBox.Value())
	}
	if comp.state.searchText != "kick" {
		t.Errorf("expected state.searchText 'kick', got %q", comp.state.searchText)
	}
	// Only "Kick Drum" should match.
	if len(comp.state.filteredInsts) != 1 {
		t.Errorf("expected 1 filtered instrument, got %d", len(comp.state.filteredInsts))
	} else if comp.state.filteredInsts[0] != "kick" {
		t.Errorf("expected filtered inst 'kick', got %q", comp.state.filteredInsts[0])
	}
}

func TestInstMenuSearchBox_BackspaceDeletesChar(t *testing.T) {
	comp := newInstMenuInInstrumentsMode(t)

	// Focus and type "ab".
	sr := comp.searchBox.Rect
	cx, cy := sr.Min.X+2, sr.Min.Y+2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	chars := []rune{'a', 'b'}
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	if comp.searchBox.Value() != "ab" {
		t.Fatalf("expected 'ab', got %q", comp.searchBox.Value())
	}

	// Now press backspace.
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyBackspace },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	comp.Update()

	if comp.searchBox.Value() != "a" {
		t.Errorf("expected 'a' after backspace, got %q", comp.searchBox.Value())
	}
}

func TestInstMenuSearchBox_UnfocusOnClickOutside(t *testing.T) {
	comp := newInstMenuInInstrumentsMode(t)

	// Focus the search box.
	sr := comp.searchBox.Rect
	cx, cy := sr.Min.X+2, sr.Min.Y+2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	if !comp.searchBox.Focused() {
		t.Fatal("searchBox should be focused")
	}

	// Click outside search rect but inside menu bounds.
	bounds := comp.Bounds()
	ox := bounds.Min.X + 5
	oy := bounds.Max.Y - 5
	restore = SetInputForTest(
		func() (int, int) { return ox, oy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	comp.Update()

	if comp.searchBox.Focused() {
		t.Error("searchBox should be unfocused after clicking outside its rect")
	}
}

func TestInstMenuSearchBox_ClearedOnReopen(t *testing.T) {
	comp := newInstMenuInInstrumentsMode(t)

	// Set search text directly.
	comp.SetSearchText("test")
	if comp.state.searchText != "test" {
		t.Fatal("searchText should be 'test'")
	}

	// Close and reopen.
	comp.Close()
	comp.Open()

	if comp.state.searchText != "" {
		t.Errorf("expected searchText cleared on reopen, got %q", comp.state.searchText)
	}
	if comp.searchBox != nil && comp.searchBox.Value() != "" {
		t.Errorf("expected searchBox text cleared on reopen, got %q", comp.searchBox.Value())
	}
}

func TestInstMenuSearchBox_EnterCommitsSearch(t *testing.T) {
	comp := newInstMenuInInstrumentsMode(t)

	// Focus the search box.
	sr := comp.searchBox.Rect
	cx, cy := sr.Min.X+2, sr.Min.Y+2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	comp.Update()
	restore()

	if !comp.searchBox.Focused() {
		t.Fatal("searchBox should be focused")
	}

	// Type some chars then press Enter.
	chars := []rune{'a'}
	enter := false
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return enter && k == ebiten.KeyEnter },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	defer restore()

	// Frame 1: type 'a'
	comp.Update()
	if comp.searchBox.Value() != "a" {
		t.Fatalf("expected 'a', got %q", comp.searchBox.Value())
	}

	// Frame 2: press Enter
	enter = true
	comp.Update()

	if comp.searchBox.Focused() {
		t.Error("searchBox should be unfocused after Enter")
	}
}

func TestInstMenuSearchBox_FullIntegration_TypeViaPortal(t *testing.T) {
	assertDefaultParityState(t)

	const W, H = 800, 600
	dv, cx, cy := newCategoryDrumView(t, W, H)

	// Open instrument menu by clicking row label.
	clickDrumViewAt(t, dv, cx, cy, W, H)
	if !dv.IsInstMenuOpen() {
		t.Fatal("menu should be open")
	}

	comp := dv.instMenuComp
	if comp == nil || !comp.IsOpen() {
		t.Fatal("instMenuComp should be open")
	}

	// Ensure we're in instruments mode (may need to select a category first).
	if comp.Mode() == InstMenuModeCategories {
		// Click the first category button.
		if len(comp.categoryBtns) == 0 {
			t.Fatal("no category buttons")
		}
		cr := comp.categoryBtns[0].Rect()
		ccx := cr.Min.X + cr.Dx()/2
		ccy := cr.Min.Y + cr.Dy()/2
		clickDrumViewAt(t, dv, ccx, ccy, W, H)
		ri := idleFrames(dv, 2, W, H)
		ri()
	}

	if comp.Mode() != InstMenuModeInstruments {
		t.Fatalf("expected instruments mode, got %s", comp.Mode())
	}

	sb := comp.SearchBox()
	if sb == nil {
		t.Fatal("searchBox should exist")
	}

	// Click the search box to focus it.
	sr := sb.Rect
	scx := sr.Min.X + sr.Dx()/2
	scy := sr.Min.Y + sr.Dy()/2

	r := pressDrumView(dv, scx, scy, W, H)
	dv.Update()
	r()
	r = releaseDrumView(dv, scx, scy, W, H)
	dv.Update()
	r()

	if !sb.Focused() {
		t.Fatal("searchBox should be focused after click")
	}

	initialCount := len(comp.state.filteredInsts)

	// Type chars on idle frames (no mouse input, just chars via inputChars).
	chars := []rune{'k', 'i', 'c', 'k'}
	r = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return W, H },
	)
	dv.Update()
	r()

	// Run a few idle frames to let portal updateFn propagate.
	ri := idleFrames(dv, 2, W, H)
	ri()

	if sb.Value() != "kick" {
		t.Errorf("expected searchBox value 'kick', got %q", sb.Value())
	}

	// Verify filtering reduced the list.
	if len(comp.state.filteredInsts) >= initialCount {
		t.Errorf("expected filtering to reduce instruments: was %d, now %d",
			initialCount, len(comp.state.filteredInsts))
	}
}

func TestInstrumentMenuComponent_WheelScroll(t *testing.T) {
	// Create menu with many instruments
	insts := []InstrumentOption{}
	for i := 0; i < 20; i++ {
		insts = append(insts, InstrumentOption{
			ID:       "inst" + string(rune('a'+i)),
			Label:    "Instrument " + string(rune('A'+i)),
			Category: "",
		})
	}

	comp := NewInstrumentMenuComponent()
	comp.SetProps(InstrumentMenuProps{
		AnchorRect:  image.Rect(10, 100, 100, 124),
		VertBounds:  image.Rect(0, 50, 300, 400),
		Categories:  []string{},
		Instruments: insts,
		RowHeight:   24,
	})
	comp.Open()

	initialFirst := comp.Scroll().First

	// Scroll down
	bounds := comp.Bounds()
	result := comp.HandleWheel(bounds.Min.X+10, bounds.Min.Y+10, -1)
	if result == InputIgnored {
		t.Error("expected wheel scroll to be handled")
	}

	// First should have changed
	if comp.Scroll().First == initialFirst {
		t.Error("expected scroll.First to change after wheel scroll")
	}
}
