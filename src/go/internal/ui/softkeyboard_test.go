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
	// This test verifies the inputChars() path (used by both soft keyboard and
	// hardware keyboard). When softKeyboardActive() returns false (stub mode),
	// the TextInput uses inputChars() which we can mock.
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

// TestFocusRectCoordinatesMatchGameCoords verifies that softKeyboardRegisterRect
// receives game coordinates (CSS pixels), not DPR-scaled coordinates.
func TestFocusRectCoordinatesMatchGameCoords(t *testing.T) {
	assertDefaultParityState(t)

	// Capture rects during layout.
	var rects []capturedRect
	testCapturedRects = &rects
	defer func() { testCapturedRects = nil }()

	// iPhone 12 CSS dimensions (390×844). Layout returns the same values
	// since Game.Layout echoes its inputs; these are game coordinates.
	dv := newTestDrumView(t, 390, 844)
	dv.calcLayout()

	// Find the "bpm" rect.
	var found *capturedRect
	for i := range rects {
		if rects[i].ID == "bpm" {
			found = &rects[i]
			break
		}
	}
	if found == nil {
		t.Fatal("softKeyboardRegisterRect was never called with id='bpm'")
	}

	// The registered rect must match bpmBox.Rect exactly (game coordinates).
	box := dv.bpmBox.Rect
	if found.X != box.Min.X || found.Y != box.Min.Y {
		t.Fatalf("bpm rect origin mismatch: registered=(%d,%d) bpmBox=(%d,%d)",
			found.X, found.Y, box.Min.X, box.Min.Y)
	}
	if found.W != box.Dx() || found.H != box.Dy() {
		t.Fatalf("bpm rect size mismatch: registered=(%dx%d) bpmBox=(%dx%d)",
			found.W, found.H, box.Dx(), box.Dy())
	}
	if found.InputMode != "numeric" {
		t.Fatalf("expected inputmode='numeric', got %q", found.InputMode)
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
