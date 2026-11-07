//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The JS overlay's change handler lands here when a file is picked through the transparent <input type=file>;
// it must close the menu and start the import exactly like the Import item, or the file is dropped.
func TestStartOverflowImportTriggersPickerAndClosesMenu(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	pickerCalls := 0
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { pickerCalls++ }

	const W, H = 390, 844
	dv := NewDrumView(image.Rect(0, 0, W, H), nil, game_log.New(nil, game_log.LevelError))
	dv.Rows = []*DrumRow{{Name: "Kick", Instrument: roster.Kick, Steps: make([]bool, 8), Volume: 1.0}}
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

// The native-gesture sync arms rects for Import and Upload, read from dv.lastNativeRects.
func TestOverflowMenuRegistersFilePickerRects(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 844)
	dv := g.drum

	// The sync runs inside g.Update.
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()

	if !dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be open")
	}

	found := map[string]NativeIntent{}
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel != NativeFilePicker {
			continue
		}
		found[r.Intent.ID] = r.Intent
		if r.Rect.Dx() <= 0 || r.Rect.Dy() <= 0 {
			t.Errorf("rect %q has non-positive dimensions: %v", r.Intent.ID, r.Rect)
		}
	}
	if len(found) < 2 {
		t.Fatalf("expected at least 2 file picker rects, got %d", len(found))
	}
	if _, ok := found["upload"]; !ok {
		t.Error("missing file picker rect with ID 'upload'")
	}
	if _, ok := found["import"]; !ok {
		t.Error("missing file picker rect with ID 'import'")
	}

	// Verify accept types.
	if got := found["upload"].Accept; got != ".wav" {
		t.Errorf("upload rect accept=%q, want '.wav'", got)
	}
	if got := found["import"].Accept; got != "application/json,.json" {
		t.Errorf("import rect accept=%q, want 'application/json,.json'", got)
	}
}

// The sync clears them once the closed menu stops producing candidates.
func TestOverflowMenuClearsFilePickerRectsOnClose(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 844)
	dv := g.drum

	// Open overflow menu — rects are armed.
	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()
	armed := 0
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			armed++
		}
	}
	if armed < 2 {
		t.Fatalf("expected rects after open, got %d", armed)
	}

	// Close overflow menu.
	dv.SetOverflowMenuOpen(false)
	g.Update()
	if dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be closed")
	}
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			t.Fatalf("expected 0 file-picker rects after close, found %q at %v", r.Intent.ID, r.Rect)
		}
	}
}

// The rect registry is a mobile trusted-touch mechanism; desktop opens Upload/Import with ordinary clicks.
func TestFilePickerRectsNotRegisteredOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	// Do NOT call SetForceMobileProfile — a plain Layout() is desktop.

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	dv := g.drum

	var captured []NativeRect
	prev := testCapturedNativeRects
	testCapturedNativeRects = &captured
	t.Cleanup(func() { testCapturedNativeRects = prev })

	dv.OpenOverflowMenu()
	dv.overflowPage = 0
	g.Update()

	if !dv.IsOverflowMenuOpen() {
		t.Fatal("overflow menu should be open")
	}

	fileArmed := 0
	for _, r := range dv.lastNativeRects {
		if r.Intent.Channel == NativeFilePicker {
			fileArmed++
		}
	}
	if fileArmed != 0 {
		t.Fatalf("expected 0 file picker rects on desktop, got %d", fileArmed)
	}
}
