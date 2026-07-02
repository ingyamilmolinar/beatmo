//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// clearClickSuppression clears the global click suppression state for tests.
func clearClickSuppression(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })
}

// TestSubdivMenuBottomAnchorStaysOnScreen is the regression case for the
// "menu renders ~250px below its ÷n anchor over the EQ panel, clipped at the
// screen bottom" bug. With the ÷n button anchored at the very bottom edge of
// the bounds, AnchorPopupRect must flip/clamp the card so it stays fully on
// screen.
func TestSubdivMenuBottomAnchorStaysOnScreen(t *testing.T) {
	bounds := image.Rect(0, 0, 800, 600)
	// Anchor flush against the bottom edge — there is no room below it.
	anchor := image.Rect(700, 580, 740, 600)

	comp := NewSubdivMenuComponent()
	comp.SetScreenBounds(bounds)
	comp.SetProps(SubdivMenuProps{
		AnchorRect: anchor,
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	card := comp.CardRect()
	if card.Empty() {
		t.Fatal("subdiv card rect empty after open")
	}
	if !card.In(bounds) {
		t.Fatalf("subdiv card %v not contained in bounds %v", card, bounds)
	}
	for i, btn := range comp.Buttons() {
		if !btn.Rect().In(bounds) {
			t.Errorf("subdiv option %d rect %v escapes bounds %v", i, btn.Rect(), bounds)
		}
	}
}

// TestSubdivMenuOptionsNotTruncated verifies the option labels keep their full
// text ("16", "32") — the desktop card sizes its width from StyledTextWidth
// (RoleBody) so the wider labels never collapse to "···".
func TestSubdivMenuOptionsNotTruncated(t *testing.T) {
	bounds := image.Rect(0, 0, 800, 600)
	anchor := image.Rect(100, 100, 140, 124)

	comp := NewSubdivMenuComponent()
	comp.SetScreenBounds(bounds)
	comp.SetProps(SubdivMenuProps{
		AnchorRect: anchor,
		Current:    8,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	want := map[string]bool{"4": false, "8": false, "16": false, "32": false}
	for _, btn := range comp.Buttons() {
		want[btn.Text] = true
		// The button must be at least wide enough to show its RoleBody-styled label.
		if btn.Rect().Dx() < StyledTextWidth(btn.Text, RoleBody) {
			t.Errorf("option %q button width %d < label width %d (would truncate)",
				btn.Text, btn.Rect().Dx(), StyledTextWidth(btn.Text, RoleBody))
		}
	}
	for label, seen := range want {
		if !seen {
			t.Errorf("option %q missing from rendered buttons", label)
		}
	}
}

// TestSubdivMenuHighlightsCurrent verifies the active subdivision option is
// rendered with the active draw state (azure stripe + colTextAccent label).
// The highlight is now determined in drawSubdivRow by comparing the option
// value to props.Current — this test verifies the component correctly exposes
// the current value through Props() so the draw logic can distinguish it.
func TestSubdivMenuHighlightsCurrent(t *testing.T) {
	bounds := image.Rect(0, 0, 800, 600)
	comp := NewSubdivMenuComponent()
	comp.SetScreenBounds(bounds)
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 100, 140, 124),
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	// Verify props.Current is correctly stored — drawSubdivRow uses
	// comp.props.Current (not btn.Style) to decide the active state.
	if comp.Props().Current != 16 {
		t.Errorf("props.Current = %d, want 16", comp.Props().Current)
	}
	// All buttons use the same base DropdownStyle; active highlight is in Draw.
	for _, btn := range comp.Buttons() {
		if btn.Style != ButtonVisual(DropdownStyle) {
			t.Errorf("option %q: Style should be DropdownStyle; active highlight is drawn via drawMenuRow", btn.Text)
		}
	}
	// Verify every expected option label is present.
	wantLabels := map[string]bool{"4": false, "8": false, "16": false, "32": false}
	for _, btn := range comp.Buttons() {
		wantLabels[btn.Text] = true
	}
	for label, seen := range wantLabels {
		if !seen {
			t.Errorf("option %q missing from rendered buttons", label)
		}
	}
}

func TestSubdivMenuComponent_OpenClose(t *testing.T) {
	comp := NewSubdivMenuComponent()

	// Initially closed
	if comp.IsOpen() {
		t.Error("expected menu to be closed initially")
	}

	// Set props and open
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected menu to be open after Open()")
	}
	if len(comp.buttons) != 4 {
		t.Errorf("expected 4 buttons, got %d", len(comp.buttons))
	}

	// Close
	comp.Close()
	if comp.IsOpen() {
		t.Error("expected menu to be closed after Close()")
	}
	if len(comp.buttons) != 0 {
		t.Errorf("expected buttons to be cleared after Close(), got %d", len(comp.buttons))
	}
}

func TestSubdivMenuComponent_OnSelectCallback(t *testing.T) {
	clearClickSuppression(t)

	comp := NewSubdivMenuComponent()

	var selectedValue int
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
		OnSelect: func(value int) {
			selectedValue = value
		},
	})
	comp.Open()

	// Clear suppression again (Open calls SuppressClicksUntilMouseUp)
	suppressClicksUntilRelease = false

	// Simulate clicking the first button (value=4)
	btn := comp.buttons[0]
	btnRect := btn.Rect()

	// Button.Handle triggers OnClick on first press (held==1)
	result := comp.HandleInput(btnRect.Min.X+5, btnRect.Min.Y+5, true)
	if result == InputIgnored {
		t.Error("expected input to be consumed on button press")
	}

	// Check selection was made (callback is fired on press, not release)
	if selectedValue != 4 {
		t.Errorf("expected selected value 4, got %d", selectedValue)
	}

	// Menu should be closed after selection
	if comp.IsOpen() {
		t.Error("expected menu to be closed after selection")
	}
}

func TestSubdivMenuComponent_ClickOutsideCloses(t *testing.T) {
	comp := NewSubdivMenuComponent()

	var closeCalled bool
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
		OnClose: func() {
			closeCalled = true
		},
	})
	comp.Open()

	// Click outside the menu bounds
	result := comp.HandleInput(10, 10, true)
	if result != InputConsumed {
		t.Error("expected click outside to be consumed")
	}

	if !closeCalled {
		t.Error("expected OnClose callback to be called")
	}
	if comp.IsOpen() {
		t.Error("expected menu to be closed after click outside")
	}
}

func TestSubdivMenuComponent_CapturingFalse(t *testing.T) {
	comp := NewSubdivMenuComponent()
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	// Subdiv menu should never capture (no drag state)
	if comp.Capturing() {
		t.Error("subdiv menu should not be capturing")
	}
}

func TestSubdivMenuComponent_ButtonLayout(t *testing.T) {
	comp := NewSubdivMenuComponent()
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	// Check buttons are positioned correctly
	if len(comp.buttons) != 4 {
		t.Fatalf("expected 4 buttons, got %d", len(comp.buttons))
	}

	// First button should start at anchor's bottom
	btn0 := comp.buttons[0].Rect()
	if btn0.Min.Y < 75 {
		t.Errorf("first button Y should be >= anchor bottom (75), got %d", btn0.Min.Y)
	}

	// Each subsequent button should be below the previous
	for i := 1; i < len(comp.buttons); i++ {
		prevBtn := comp.buttons[i-1].Rect()
		currBtn := comp.buttons[i].Rect()
		if currBtn.Min.Y < prevBtn.Max.Y-SpaceXS*2 {
			t.Errorf("button %d top (%d) should be near or below button %d bottom (%d)",
				i, currBtn.Min.Y, i-1, prevBtn.Max.Y)
		}
	}
}

func TestSubdivMenuComponent_BoundsUpdated(t *testing.T) {
	comp := NewSubdivMenuComponent()
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})

	// Before open, bounds should be empty
	if !comp.Bounds().Empty() {
		t.Error("expected empty bounds before opening")
	}

	comp.Open()

	// After open, bounds should cover all buttons
	bounds := comp.Bounds()
	if bounds.Empty() {
		t.Error("expected non-empty bounds after opening")
	}

	// Bounds should start at first button and end at last button
	if len(comp.buttons) > 0 {
		first := comp.buttons[0].Rect()
		last := comp.buttons[len(comp.buttons)-1].Rect()
		if bounds.Min.Y > first.Min.Y {
			t.Errorf("bounds Min.Y (%d) should be <= first button Min.Y (%d)",
				bounds.Min.Y, first.Min.Y)
		}
		if bounds.Max.Y < last.Max.Y {
			t.Errorf("bounds Max.Y (%d) should be >= last button Max.Y (%d)",
				bounds.Max.Y, last.Max.Y)
		}
	}
}

func TestSubdivMenuComponent_HandleInputWhenClosed(t *testing.T) {
	comp := NewSubdivMenuComponent()
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})

	// Don't open the menu, input should be ignored
	result := comp.HandleInput(120, 85, true)
	if result != InputIgnored {
		t.Error("expected input to be ignored when menu is closed")
	}
}

func TestSubdivMenuComponent_EmptyOptions(t *testing.T) {
	comp := NewSubdivMenuComponent()
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Options:    []int{},
		RowHeight:  24,
	})
	comp.Open()

	// Should handle empty options gracefully
	if len(comp.buttons) != 0 {
		t.Errorf("expected 0 buttons for empty options, got %d", len(comp.buttons))
	}
}

func TestSubdiv_SetPropsRebuildWhileOpen(t *testing.T) {
	comp := NewSubdivMenuComponent()

	// Open with initial anchor rect
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(100, 50, 150, 75),
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})
	comp.Open()

	if len(comp.buttons) != 4 {
		t.Fatalf("expected 4 buttons, got %d", len(comp.buttons))
	}

	// Record original button positions
	origBtn0Y := comp.buttons[0].Rect().Min.Y
	origBounds := comp.Bounds()

	// Change AnchorRect while open → should trigger rebuildButtons
	newAnchor := image.Rect(200, 300, 250, 325)
	comp.SetProps(SubdivMenuProps{
		AnchorRect: newAnchor,
		Current:    16,
		Options:    []int{4, 8, 16, 32},
		RowHeight:  24,
	})

	// Menu should still be open
	if !comp.IsOpen() {
		t.Error("expected menu to remain open after SetProps")
	}

	// Buttons should still exist
	if len(comp.buttons) != 4 {
		t.Fatalf("expected 4 buttons after SetProps, got %d", len(comp.buttons))
	}

	// First button should now start at the new anchor's bottom (325), not old (75)
	newBtn0Y := comp.buttons[0].Rect().Min.Y
	if newBtn0Y == origBtn0Y {
		t.Errorf("expected button Y to change after anchor rect update, still %d", newBtn0Y)
	}
	if newBtn0Y < newAnchor.Max.Y {
		t.Errorf("first button Y (%d) should be >= new anchor bottom (%d)", newBtn0Y, newAnchor.Max.Y)
	}

	// Bounds should have updated
	newBounds := comp.Bounds()
	if newBounds == origBounds {
		t.Errorf("expected bounds to change after SetProps, still %v", newBounds)
	}
}

// TestSubdivRowAdvancesPressAnimAfterRefactor verifies that after routing
// drawSubdivRow through drawMenuRow, successive Draw calls advance the button's
// press animation (drawMenuRow calls btn.Draw → AdvancePressAnim). Before the
// refactor the old flat path never called btn.Draw, so pressDepth never moved.
func TestSubdivRowAdvancesPressAnimAfterRefactor(t *testing.T) {
	comp := NewSubdivMenuComponent()
	comp.SetScreenBounds(image.Rect(0, 0, 200, 400))
	comp.SetProps(SubdivMenuProps{
		AnchorRect: image.Rect(50, 50, 150, 75),
		Current:    8,
		Options:    []int{4, 8, 16},
		RowHeight:  24,
	})
	comp.Open()

	dst := ebiten.NewImage(200, 400)
	if len(comp.buttons) == 0 {
		t.Fatal("no subdiv buttons built")
	}
	b := comp.buttons[0]
	b.pressTarget = 1
	start := b.pressDepth
	for i := 0; i < 5; i++ {
		comp.Draw(dst)
	}
	// Persistence: the same *Button must survive across frames (animation state
	// lives on it). A separate assertion from pressDepth so a failing run
	// distinguishes "button recreated" from "animation didn't advance".
	if comp.buttons[0] != b {
		t.Fatal("subdiv button was recreated across frames; press-animation state would be lost")
	}
	if b.pressDepth <= start {
		t.Fatalf("subdiv row did not animate after drawMenuRow refactor: %v <= %v", b.pressDepth, start)
	}
}
