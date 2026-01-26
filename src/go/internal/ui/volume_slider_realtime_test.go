//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRowVolumeSliderInvalidatesCache verifies that dragging a row volume slider
// invalidates the row controls cache, ensuring real-time visual updates.
func TestRowVolumeSliderInvalidatesCache(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 600, 300), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders) == 0 {
		t.Fatal("expected at least one row volume slider")
	}

	// Clear cache dirty flag to simulate a fresh render state
	dv.rowControlsCacheDirty = false

	// Get the first row volume slider and simulate dragging it
	slider := dv.rowVolSliders[0]
	rect := slider.Rect()
	if rect.Dx() <= 0 {
		t.Fatalf("slider rect not initialized: %v", rect)
	}

	// Target the right side of the slider to change the value
	targetX := rect.Max.X - 1
	targetY := rect.Min.Y + rect.Dy()/2

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return targetX, targetY },
		func(btn ebiten.MouseButton) bool { return btn == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	defer restore()

	// Simulate the drag interaction
	dv.Update()

	// Verify cache was invalidated
	if !dv.rowControlsCacheDirty {
		t.Fatal("expected rowControlsCacheDirty=true after slider drag")
	}
}

// TestRowVolumeSliderContinuedDragInvalidatesCache verifies that continued
// dragging of an active slider continues to invalidate the cache.
func TestRowVolumeSliderContinuedDragInvalidatesCache(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 600, 300), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowVolSliders) == 0 {
		t.Fatal("expected at least one row volume slider")
	}

	slider := dv.rowVolSliders[0]
	rect := slider.Rect()

	// Start with a click on the slider to make it active
	targetX := rect.Min.X + rect.Dx()/2
	targetY := rect.Min.Y + rect.Dy()/2
	pressed := true

	restore := SetInputForTest(
		func() (int, int) { return targetX, targetY },
		func(btn ebiten.MouseButton) bool { return btn == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return dv.Bounds.Dx(), dv.Bounds.Dy() },
	)
	defer restore()

	// First update to activate the slider
	dv.Update()

	if dv.activeSlider < 0 {
		t.Fatal("expected activeSlider to be set after initial click")
	}

	// Clear the dirty flag to test continued drag
	dv.rowControlsCacheDirty = false

	// Move to a new position while still pressed (simulating drag)
	targetX = rect.Max.X - 1
	dv.Update()

	// Verify cache was invalidated again during continued drag
	if !dv.rowControlsCacheDirty {
		t.Fatal("expected rowControlsCacheDirty=true after continued slider drag")
	}
}
