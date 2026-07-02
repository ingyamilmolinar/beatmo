//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestStartOverflowImportTriggersPickerAndClosesMenu locks the JS→Go entry
// point used by the mobile real-input file-picker overlay. When the user taps
// the (real, transparent) <input type=file> over the Import row and picks a
// file, the overlay's change handler calls _fpStartImport, which routes here.
// It must close the overflow menu and fire the import picker flow exactly like
// the overflow "Import" menu item — otherwise the picked file is dropped and
// "tapping Import does nothing".
func TestStartOverflowImportTriggersPickerAndClosesMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	pickerCalls := 0
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { pickerCalls++ }

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: "kick", Steps: make([]bool, 8), Volume: 1.0}}
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

	dv.OpenOverflowMenu()
	if !dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be open before StartOverflowImport")
	}

	dv.StartOverflowImport()

	if dv.IsOverflowMenuOpen() {
		t.Error("StartOverflowImport should close the overflow menu")
	}
	if pickerCalls != 1 {
		t.Errorf("StartOverflowImport should fire the import picker exactly once, got %d", pickerCalls)
	}
}

// TestOverflowMenuRegistersFilePickerRects verifies that opening the overflow
// menu on a small screen registers file picker rects for the Import and Upload
// buttons, enabling the gesture-based file picker on mobile.
func TestOverflowMenuRegistersFilePickerRects(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	// Enable test capture for file picker rects.
	var captured []capturedFilePickerRect
	testCapturedFilePickerRects = &captured
	t.Cleanup(func() { testCapturedFilePickerRects = nil })

	// Warm-up frame for layout init.
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

	if dv.overflowBtn() == nil {
		t.Fatal("overflowBtn not created after warm-up")
	}

	// Click the overflow button to open the menu.
	dv.overflowBtn().OnClick()

	if !dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be open after clicking overflow button")
	}

	if len(captured) < 2 {
		t.Fatalf("expected at least 2 file picker rects, got %d", len(captured))
	}

	// Verify the Upload rect (index 0).
	found := map[string]bool{}
	for _, r := range captured {
		found[r.ID] = true
		if r.W <= 0 || r.H <= 0 {
			t.Errorf("rect %q has non-positive dimensions: %dx%d", r.ID, r.W, r.H)
		}
	}
	if !found["upload"] {
		t.Error("missing file picker rect with ID 'upload'")
	}
	if !found["import"] {
		t.Error("missing file picker rect with ID 'import'")
	}

	// Verify accept types.
	for _, r := range captured {
		switch r.ID {
		case "upload":
			if r.Accept != ".wav" {
				t.Errorf("upload rect accept=%q, want '.wav'", r.Accept)
			}
		case "import":
			if r.Accept != "application/json,.json" {
				t.Errorf("import rect accept=%q, want 'application/json,.json'", r.Accept)
			}
		}
	}
}

// TestOverflowMenuClearsFilePickerRectsOnClose verifies that closing the
// overflow menu clears all registered file picker rects.
func TestOverflowMenuClearsFilePickerRectsOnClose(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	var captured []capturedFilePickerRect
	testCapturedFilePickerRects = &captured
	t.Cleanup(func() { testCapturedFilePickerRects = nil })

	// Warm-up frame.
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

	// Open overflow menu — rects are registered.
	dv.overflowBtn().OnClick()
	if len(captured) < 2 {
		t.Fatalf("expected rects after open, got %d", len(captured))
	}

	// Close overflow menu via the button toggle.
	dv.overflowBtn().OnClick()
	if dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be closed after second click")
	}
	if len(captured) != 0 {
		t.Fatalf("expected 0 rects after close, got %d", len(captured))
	}
}

// TestFilePickerRectsNotRegisteredOnDesktop verifies that file picker rects
// are NOT registered when the overflow menu opens on a large (desktop) screen.
func TestFilePickerRectsNotRegisteredOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	// Do NOT call withSmallScreen — use default (desktop) mode.

	const W, H = 1280, 720
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{
		Name:       "Kick",
		Instrument: "kick",
		Steps:      make([]bool, 8),
		Volume:     1.0,
	}}
	dv.Length = 8

	var captured []capturedFilePickerRect
	testCapturedFilePickerRects = &captured
	t.Cleanup(func() { testCapturedFilePickerRects = nil })

	// Warm-up frame.
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

	// Manually open the overflow menu (on desktop, the button may not be visible
	// but we can test the OnClick handler directly).
	dv.overflowBtn().OnClick()

	if len(captured) != 0 {
		t.Fatalf("expected 0 file picker rects on desktop, got %d", len(captured))
	}
}
