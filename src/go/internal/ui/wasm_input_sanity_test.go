//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestWASMInputNotSkippedInFastPath verifies that input polling happens
// even when fastPath is enabled (the default on WASM). This is a regression
// test for a bug where fastPath=true caused input to be skipped entirely,
// breaking all click and drag interactions on WASM builds.
func TestWASMInputNotSkippedInFastPath(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// Enable fast path (simulates WASM default)
	g.perfMode.SetFastPath(true)

	// Simulate mouse press at the play button
	btn := g.drum.playBtn()
	r := btn.Rect()
	x, y := r.Min.X+1, r.Min.Y+1

	clicked := false
	btn.OnClick = func() { clicked = true }

	// Press
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	// Release
	restore = SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	if !clicked {
		t.Fatal("button click not detected with fastPath enabled - WASM input bug")
	}
}

// TestWASMDrumViewUpdateCalledInFastPath ensures that drum.Update() is called
// even when fastPath is enabled, so drum view buttons process input correctly.
func TestWASMDrumViewUpdateCalledInFastPath(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// Enable fast path
	g.perfMode.SetFastPath(true)

	// Test that BPM increment button works
	btn := g.drum.bpmIncBtn()
	r := btn.Rect()
	x, y := r.Min.X+1, r.Min.Y+1

	initialBPM := g.drum.BPM()

	// Click BPM increment
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	// Release
	restore = SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	newBPM := g.drum.BPM()
	if newBPM <= initialBPM {
		t.Fatalf("BPM did not increment with fastPath enabled: initial=%d, after=%d", initialBPM, newBPM)
	}
}

// TestWASMInputStateNotCorrupted verifies that leftPrev is properly maintained
// even when fastPath is enabled, preventing the state corruption that caused
// clicks to never register (because left && !leftPrev was always false).
func TestWASMInputStateNotCorrupted(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Enable fast path
	g.perfMode.SetFastPath(true)

	// Run a few updates with no mouse pressed
	restore := SetInputForTest(
		func() (int, int) { return 100, 100 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	for i := 0; i < 5; i++ {
		g.Update()
	}
	restore()

	// Now leftPrev should be false
	if g.leftPrev {
		t.Fatal("leftPrev should be false when mouse not pressed")
	}

	// Press mouse
	restore = SetInputForTest(
		func() (int, int) { return 100, 100 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	// After processing a frame with mouse pressed, leftPrev should be true
	if !g.leftPrev {
		t.Fatal("leftPrev should be true after mouse press with fastPath enabled")
	}
}

// TestWASMCameraPanningWorksInFastPath verifies that camera panning works
// when fastPath is enabled (the default on WASM). This is a regression test
// for a bug where fastPath=true caused the camera handling code to be skipped,
// freezing the grid on WASM builds.
func TestWASMCameraPanningWorksInFastPath(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Enable fast path (WASM default)
	g.perfMode.SetFastPath(true)

	// Record initial camera position
	initialX, initialY := g.cam.OffsetX, g.cam.OffsetY

	// Simulate mouse press in the grid area (above drum view)
	startX, startY := 100, 50

	// Press mouse to start drag
	restore := SetInputForTest(
		func() (int, int) { return startX, startY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	// Move mouse while holding button (simulate drag)
	endX, endY := startX+50, startY+30
	restore = SetInputForTest(
		func() (int, int) { return endX, endY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	// Verify camDragged was set during the drag (before release clears it)
	if !g.camDragged {
		t.Fatal("camDragged was not set during drag with fastPath enabled")
	}

	// Release mouse
	restore = SetInputForTest(
		func() (int, int) { return endX, endY },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.Update()
	restore()

	// Check that camera position changed (panning worked)
	if g.cam.OffsetX == initialX && g.cam.OffsetY == initialY {
		t.Fatal("camera did not move with fastPath enabled - WASM grid panning bug")
	}
}

// TestAllControlButtonsClickableInFastPath ensures all main control buttons
// respond to clicks when fastPath is enabled.
func TestAllControlButtonsClickableInFastPath(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	// Enable fast path (WASM default)
	g.perfMode.SetFastPath(true)

	buttons := []*Button{
		g.drum.playBtn(),
		g.drum.stopBtn(),
		g.drum.bpmDecBtn(),
		g.drum.bpmIncBtn(),
		g.drum.lenDecBtn,
		g.drum.lenIncBtn,
	}

	for i, btn := range buttons {
		if btn == nil {
			continue
		}
		called := false
		original := btn.OnClick
		btn.OnClick = func() { called = true }

		r := btn.Rect()
		x, y := r.Min.X+1, r.Min.Y+1

		// Press
		restore := SetInputForTest(
			func() (int, int) { return x, y },
			func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 0, 0 },
		)
		g.Update()
		restore()

		// Release
		restore = SetInputForTest(
			func() (int, int) { return x, y },
			func(ebiten.MouseButton) bool { return false },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 0, 0 },
		)
		g.Update()
		restore()

		if !called {
			t.Fatalf("button %d (%s) not clickable with fastPath enabled", i, btn.Text)
		}

		btn.OnClick = original
	}
}
