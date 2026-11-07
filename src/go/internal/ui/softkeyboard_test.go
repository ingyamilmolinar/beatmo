//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestTextInputFocusCallbacksFired verifies OnFocusGained/OnFocusLost callbacks fire.
func TestTextInputFocusCallbacksFired(t *testing.T) {
	gained := 0
	lost := 0

	ti := NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)
	ti.OnFocusGained = func() { gained++ }
	ti.OnFocusLost = func() { lost++ }

	// Mock input: click inside the rect to focus
	restore := SetInputForTest(
		func() (int, int) { return 50, 20 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore()

	ti.Update()
	if gained != 1 {
		t.Fatalf("expected OnFocusGained called once, got %d", gained)
	}
	if lost != 0 {
		t.Fatalf("expected OnFocusLost not called, got %d", lost)
	}

	// Click outside to blur
	restore2 := SetInputForTest(
		func() (int, int) { return 500, 500 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore2()

	ti.Update()
	if lost != 1 {
		t.Fatalf("expected OnFocusLost called once, got %d", lost)
	}
}

// TestTextInputSoftKBCharsAppear verifies that soft keyboard chars update Text.
func TestTextInputSoftKBCharsAppear(t *testing.T) {
	// With no soft keyboard (stub), TextInput reads inputChars(), which the test controls.
	ti := NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)
	ti.focused = true
	ti.prevFocused = true

	charCalls := 0
	chars := []rune{'H', 'i'}

	restore := SetInputForTest(
		func() (int, int) { return 50, 20 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune {
			charCalls++
			if charCalls == 1 {
				return chars
			}
			return nil
		},
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore()

	ti.Update()
	if ti.Text != "Hi" {
		t.Fatalf("expected Text='Hi', got %q", ti.Text)
	}
}

// TestTextInputSoftKBBackspace verifies backspace through inputChars path.
func TestTextInputSoftKBBackspace(t *testing.T) {
	ti := NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)
	ti.SetText("ABC")
	ti.focused = true
	ti.prevFocused = true

	// Simulate backspace via key press
	bsPressed := true
	restore := SetInputForTest(
		func() (int, int) { return 50, 20 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool {
			if k == ebiten.KeyBackspace && bsPressed {
				return true
			}
			return false
		},
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore()

	ti.Update()
	if ti.Text != "AB" {
		t.Fatalf("expected Text='AB' after backspace, got %q", ti.Text)
	}

	bsPressed = false
	ti.Update() // release
	bsPressed = true
	ti.Update()
	if ti.Text != "A" {
		t.Fatalf("expected Text='A' after second backspace, got %q", ti.Text)
	}
}

// TestTextInputDesktopRegression verifies that TextInput works without callbacks set.
func TestTextInputDesktopRegression(t *testing.T) {
	ti := NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)
	// No OnFocusGained/OnFocusLost set

	// Click to focus
	restore := SetInputForTest(
		func() (int, int) { return 50, 20 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore()

	ti.Update()
	if !ti.Focused() {
		t.Fatal("expected focused after click")
	}

	// Type chars
	restore2 := SetInputForTest(
		func() (int, int) { return 50, 20 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return []rune{'X'} },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore2()

	ti.Update()
	if ti.Text != "X" {
		t.Fatalf("expected Text='X', got %q", ti.Text)
	}
}

// CSS pixels, never DPR-scaled.
func TestFocusRectCoordinatesMatchGameCoords(t *testing.T) {
	assertDefaultParityState(t)

	// Game.Layout echoes its inputs, so these are game coordinates.
	dv := newTestDrumView(t, 390, 844)
	dv.calcLayout()

	// A live TextField's candidate rect is exactly its rect in game coordinates.
	box := NewTextField(dv.textFields, TextFieldSpec{ID: "probe-coords", Owner: "probe"}, 32)
	box.Rect = image.Rect(120, 700, 300, 736) // a field near the bottom of a 390x844 view
	box.Update()                              // live this frame
	var found *NativeRectCandidate
	for _, c := range dv.softKeyboardCandidates() {
		if c.Intent.ID == "probe-coords" {
			cc := c
			found = &cc
			break
		}
	}
	if found == nil {
		t.Fatal("softKeyboardCandidates never produced the live text field")
	}
	if found.Rect != box.Rect {
		t.Fatalf("candidate rect %v != field rect %v (game coordinates)", found.Rect, box.Rect)
	}
	if found.Intent.InputMode != "text" {
		t.Fatalf("expected inputmode='text', got %q", found.Intent.InputMode)
	}
}

// TestTextInputInputModePropagated verifies InputMode is passed to the callback.
func TestTextInputInputModePropagated(t *testing.T) {
	var receivedMode string
	ti := NewTextInput(image.Rect(10, 10, 200, 40), BPMBoxStyle)
	ti.InputMode = "numeric"
	ti.OnFocusGained = func() { receivedMode = ti.InputMode }

	// Click to focus
	restore := SetInputForTest(
		func() (int, int) { return 50, 20 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1920, 1080 },
	)
	defer restore()

	ti.Update()
	if receivedMode != "numeric" {
		t.Fatalf("expected InputMode='numeric' in callback, got %q", receivedMode)
	}
}
