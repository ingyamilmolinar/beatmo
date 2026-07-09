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
// menu on a small screen arms file-picker native rects (via the tree-owned
// native-gesture sync, syncNativeGestures -> filePickerCandidates) for the
// Import and Upload buttons, enabling the gesture-based file picker on
// mobile. Re-pointed from the retired imperative registerFilePickerRects()
// capture mechanism to dv.lastNativeRects, the new source of truth.
func TestOverflowMenuRegistersFilePickerRects(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 844)
	dv := g.drum

	// Open the overflow menu on the File page — the tree-owned sync runs
	// during g.Update() and arms the file-picker rects.
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

// TestOverflowMenuClearsFilePickerRectsOnClose verifies that closing the
// overflow menu clears all armed file-picker native rects. Re-pointed from
// the retired imperative filePickerClearRects() capture mechanism to
// dv.lastNativeRects, the new source of truth: the tree-owned sync clears
// the channel once filePickerCandidates() stops producing candidates
// (menu closed), no explicit clear call needed.
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

// TestFilePickerRectsNotRegisteredOnDesktop verifies that file picker rects
// are NOT registered when the overflow menu opens on a large (desktop)
// screen. The file-picker rect registry is a mobile-only trusted-touch-
// gesture mechanism (desktop opens Upload/Import via normal button clicks,
// not the JS touchend rect), so filePickerCandidates() must be gated on
// Profile().IsMobile(). Checks the real source of truth, dv.lastNativeRects,
// via the tree-owned native-gesture sync (mirrors
// TestOverflowTemplatePage_NoFilePickerRectArmed's harness style).
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

	// Open the overflow menu on the File page — on mobile this arms
	// Upload+Import file-picker rects; on desktop it must not.
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
