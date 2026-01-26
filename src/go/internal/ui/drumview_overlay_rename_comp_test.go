//go:build test

package ui

import (
	"image"
	"testing"
)

func TestRenameComponent_OpenClose(t *testing.T) {
	comp := NewRenameComponent("test-rename")

	// Initially closed
	if comp.IsOpen() {
		t.Error("expected rename to be closed initially")
	}

	// Set props and open
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "kick",
		MaxLen:      32,
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected rename to be open after Open()")
	}
	if comp.textBox == nil {
		t.Error("expected textBox to be created")
	}
	if comp.Value() != "kick" {
		t.Errorf("expected initial text 'kick', got '%s'", comp.Value())
	}

	// Close
	comp.Close()
	if comp.IsOpen() {
		t.Error("expected rename to be closed after Close()")
	}
	if comp.textBox != nil {
		t.Error("expected textBox to be nil after Close()")
	}
}

func TestRenameComponent_HoldCapture(t *testing.T) {
	comp := NewRenameComponent("test-rename")
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
	})
	comp.Open()

	// Initially should be capturing (hold is true)
	if !comp.Capturing() {
		t.Error("expected Capturing() to be true after opening")
	}

	// First input while holding should return InputCaptured
	result := comp.HandleInput(120, 60, true)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured while holding, got %v", result)
	}

	// Release should clear hold
	result = comp.HandleInput(120, 60, false)
	if result != InputCaptured {
		t.Errorf("expected InputCaptured on release, got %v", result)
	}

	// After release, should no longer be capturing
	if comp.Capturing() {
		t.Error("expected Capturing() to be false after mouse release")
	}
}

func TestRenameComponent_ClickOutsideCancels(t *testing.T) {
	comp := NewRenameComponent("test-rename")

	var cancelCalled bool
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
		OnCancel: func() {
			cancelCalled = true
		},
	})
	comp.Open()

	// Release hold first
	comp.HandleInput(120, 60, false)

	// Click outside the text box
	result := comp.HandleInput(10, 10, true)
	if result != InputConsumed {
		t.Errorf("expected click outside to be consumed, got %v", result)
	}

	if !cancelCalled {
		t.Error("expected OnCancel callback to be called")
	}
	if comp.IsOpen() {
		t.Error("expected rename to be closed after click outside")
	}
}

func TestRenameComponent_BoundsSet(t *testing.T) {
	comp := NewRenameComponent("test-rename")
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
	})

	// Before open, bounds should be empty
	if !comp.Bounds().Empty() {
		t.Error("expected empty bounds before opening")
	}

	comp.Open()

	// After open, bounds should match anchor rect
	bounds := comp.Bounds()
	expected := image.Rect(100, 50, 250, 75)
	if bounds != expected {
		t.Errorf("expected bounds %v, got %v", expected, bounds)
	}

	comp.Close()

	// After close, bounds should be empty
	if !comp.Bounds().Empty() {
		t.Error("expected empty bounds after closing")
	}
}

func TestRenameComponent_HandleInputWhenClosed(t *testing.T) {
	comp := NewRenameComponent("test-rename")
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
	})

	// Don't open the rename, input should be ignored
	result := comp.HandleInput(120, 60, true)
	if result != InputIgnored {
		t.Error("expected input to be ignored when rename is closed")
	}
}

func TestRenameComponent_InputBounds(t *testing.T) {
	comp := NewRenameComponent("test-rename")
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
	})

	// Before open, input bounds should be empty
	if !comp.InputBounds().Empty() {
		t.Error("expected empty input bounds before opening")
	}

	comp.Open()

	// After open, input bounds should match textBox rect
	inputBounds := comp.InputBounds()
	if inputBounds.Empty() {
		t.Error("expected non-empty input bounds after opening")
	}
	if comp.textBox != nil && inputBounds != comp.textBox.Rect {
		t.Errorf("expected input bounds to match textBox rect")
	}
}

func TestRenameComponent_DefaultMaxLen(t *testing.T) {
	comp := NewRenameComponent("test-rename")
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
		MaxLen:      0, // not set
	})
	comp.Open()

	if comp.textBox == nil {
		t.Fatal("expected textBox to be created")
	}
	if comp.textBox.MaxLen != 32 {
		t.Errorf("expected default MaxLen of 32, got %d", comp.textBox.MaxLen)
	}
}

func TestRenameComponent_EmptyAnchorRect(t *testing.T) {
	comp := NewRenameComponent("test-rename")
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rectangle{}, // empty
		InitialText: "test",
	})
	comp.Open()

	// Should not open with empty anchor
	if comp.IsOpen() {
		t.Error("expected rename to not open with empty anchor rect")
	}
}
