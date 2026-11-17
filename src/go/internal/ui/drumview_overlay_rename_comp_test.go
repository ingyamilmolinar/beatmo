//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestRenameComponent_OpenClose(t *testing.T) {
	comp := NewRenameComponent()

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

func TestRenameComponent_NoHoldAfterOpen(t *testing.T) {
	comp := NewRenameComponent()
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "test",
	})
	comp.Open()

	// Open() no longer sets hold — portal tree prevents double-dispatch.
	if comp.Capturing() {
		t.Error("expected Capturing() to be false after opening — hold is not set")
	}

	// Input inside bounds should be consumed (text input), not captured by hold.
	result := comp.HandleInput(120, 60, true)
	if result == InputIgnored {
		t.Error("expected input inside bounds to be handled, got InputIgnored")
	}
}

func TestRenameComponent_ClickOutsideCancels(t *testing.T) {
	comp := NewRenameComponent()

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
	comp := NewRenameComponent()
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
	comp := NewRenameComponent()
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
	comp := NewRenameComponent()
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
	comp := NewRenameComponent()
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
	comp := NewRenameComponent()
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

func TestRenameComponent_MobileMode_Open(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	testMobileInputActive = map[string]bool{"rename-0": true}
	defer func() { testMobileInputActive = nil }()

	comp := NewRenameComponent()
	comp.SetProps(RenameProps{
		AnchorRect:    image.Rect(100, 50, 250, 75),
		InitialText:   "kick",
		MaxLen:        32,
		MobileInputID: "rename-0",
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected rename to be open in mobile mode")
	}
	if !comp.state.mobile {
		t.Error("expected state.mobile to be true")
	}
	if comp.textBox != nil {
		t.Error("expected textBox to be nil in mobile mode")
	}
}

func TestRenameComponent_MobileMode_Commit(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	testMobileInputActive = map[string]bool{"rename-1": true}
	testMobileInputResult = map[string]*struct {
		Value     string
		Committed bool
	}{}
	defer func() {
		testMobileInputActive = nil
		testMobileInputResult = nil
	}()

	var committed string
	comp := NewRenameComponent()
	comp.SetProps(RenameProps{
		AnchorRect:    image.Rect(100, 50, 250, 75),
		InitialText:   "kick",
		MaxLen:        32,
		MobileInputID: "rename-1",
		OnCommit:      func(name string) { committed = name },
	})
	comp.Open()

	// Release hold
	comp.HandleInput(0, 0, false)

	// Inject committed result
	testMobileInputResult["rename-1"] = &struct {
		Value     string
		Committed bool
	}{"snare", true}

	result := comp.HandleInput(0, 0, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed, got %v", result)
	}
	if committed != "snare" {
		t.Errorf("expected committed='snare', got '%s'", committed)
	}
	if comp.IsOpen() {
		t.Error("expected rename to be closed after commit")
	}
}

func TestRenameComponent_MobileMode_Cancel(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	testMobileInputActive = map[string]bool{"rename-2": true}
	testMobileInputResult = map[string]*struct {
		Value     string
		Committed bool
	}{}
	defer func() {
		testMobileInputActive = nil
		testMobileInputResult = nil
	}()

	var cancelCalled bool
	comp := NewRenameComponent()
	comp.SetProps(RenameProps{
		AnchorRect:    image.Rect(100, 50, 250, 75),
		InitialText:   "kick",
		MaxLen:        32,
		MobileInputID: "rename-2",
		OnCancel:      func() { cancelCalled = true },
	})
	comp.Open()

	// Release hold
	comp.HandleInput(0, 0, false)

	// Inject cancelled result (Escape)
	testMobileInputResult["rename-2"] = &struct {
		Value     string
		Committed bool
	}{"", false}

	comp.HandleInput(0, 0, false)
	if !cancelCalled {
		t.Error("expected OnCancel to be called")
	}
	if comp.IsOpen() {
		t.Error("expected rename to be closed after cancel")
	}
}

func TestRenameComponent_MobileMode_DrawNoop(t *testing.T) {
	forceSmallScreenForTest = true
	defer func() { forceSmallScreenForTest = false }()

	testMobileInputActive = map[string]bool{"rename-0": true}
	defer func() { testMobileInputActive = nil }()

	comp := NewRenameComponent()
	comp.SetProps(RenameProps{
		AnchorRect:    image.Rect(100, 50, 250, 75),
		InitialText:   "kick",
		MobileInputID: "rename-0",
	})
	comp.Open()

	// Draw should not panic with nil dst in mobile mode (skips drawing)
	comp.Draw(nil)
}

func TestRenameComponent_DesktopUnchanged(t *testing.T) {
	forceSmallScreenForTest = false
	testMobileInputActive = nil

	comp := NewRenameComponent()
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "kick",
		MaxLen:      32,
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected rename to be open on desktop")
	}
	if comp.textBox == nil {
		t.Error("expected textBox to be created on desktop")
	}
	if comp.state.mobile {
		t.Error("expected state.mobile to be false on desktop")
	}
}

func TestRename_EnterCommits(t *testing.T) {
	comp := NewRenameComponent()

	var committedName string
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "kick",
		MaxLen:      32,
		OnCommit: func(name string) {
			committedName = name
		},
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Fatal("expected rename to be open")
	}
	if comp.textBox == nil {
		t.Fatal("expected textBox to be created")
	}

	// Set text to "Snare" on the TextBox (simulates user typing)
	comp.textBox.SetText("Snare")
	if comp.Value() != "Snare" {
		t.Fatalf("expected Value() to be 'Snare', got '%s'", comp.Value())
	}

	// Override isKeyPressed to simulate Enter key press
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// HandleInput should detect Enter and commit
	result := comp.HandleInput(120, 60, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on Enter, got %v", result)
	}

	if committedName != "Snare" {
		t.Errorf("expected OnCommit('Snare'), got '%s'", committedName)
	}
	if comp.IsOpen() {
		t.Error("expected rename to be closed after Enter commit")
	}
}

func TestRename_EscapeCancels(t *testing.T) {
	comp := NewRenameComponent()

	var cancelCalled bool
	comp.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "kick",
		MaxLen:      32,
		OnCancel: func() {
			cancelCalled = true
		},
	})
	comp.Open()

	if !comp.IsOpen() {
		t.Fatal("expected rename to be open")
	}
	if comp.textBox == nil {
		t.Fatal("expected textBox to be created")
	}

	// Change text (simulates user typing something different)
	comp.textBox.SetText("SomethingElse")

	// Override isKeyPressed to simulate Escape key press
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// HandleInput should detect Escape and cancel
	result := comp.HandleInput(120, 60, false)
	if result != InputConsumed {
		t.Errorf("expected InputConsumed on Escape, got %v", result)
	}

	if !cancelCalled {
		t.Error("expected OnCancel to be called on Escape")
	}
	if comp.IsOpen() {
		t.Error("expected rename to be closed after Escape")
	}
}
