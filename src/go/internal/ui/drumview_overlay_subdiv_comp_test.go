//go:build test

package ui

import (
	"image"
	"testing"
)

// clearClickSuppression clears the global click suppression state for tests.
func clearClickSuppression(t *testing.T) {
	prev := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prev })
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
		if currBtn.Min.Y < prevBtn.Max.Y-buttonPad*2 {
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
