//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	gamelog "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestRenameLabelStyleNotMissing verifies that after renaming an instrument,
// the row label does NOT get MissingInstStyle (red). Rename is now
// metadata-only (the instrument ID is unchanged), so the row's instrument stays
// available and the label keeps its normal style.
func TestRenameLabelStyleNotMissing(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 600), nil, logger)

	// Initial Update to build layout.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	dv.Update()
	restore()

	if len(dv.rowLabels()) == 0 || len(dv.rowEditBtns()) == 0 {
		t.Fatal("no row labels or edit buttons after initial Update")
	}

	// Rename is metadata-only: clear the process-global display override it leaves.
	renamedID := dv.Rows[0].Instrument
	t.Cleanup(func() { audio.ClearInstrumentDisplayName(renamedID) })

	// Precondition: label is NOT MissingInstStyle before rename.
	if dv.rowLabels()[0].Style == MissingInstStyle {
		t.Fatal("label already has MissingInstStyle before rename")
	}

	// Open rename via edit button (wires up the production OnCommit callback).
	dv.rowEditBtns()[0].OnClick()

	// Invoke OnCommit directly with a new name.
	dv.renameComp.Props().OnCommit("RenamedSnare")

	// Run a few Update frames to trigger calcLayout (bgDirty → label rebuild).
	for i := 0; i < 3; i++ {
		restore = SetInputForTest(
			func() (int, int) { return 0, 0 },
			func(b ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 800, 600 },
		)
		dv.Update()
		restore()
	}

	// The row should have the renamed instrument.
	if dv.Rows[0].Name != "RenamedSnare" {
		t.Fatalf("Rows[0].Name = %q, want %q", dv.Rows[0].Name, "RenamedSnare")
	}

	// The label must NOT have MissingInstStyle — the instrument is valid.
	if dv.rowLabels()[0].Style == MissingInstStyle {
		t.Fatalf("row label has MissingInstStyle after rename — instrument %q not recognized as available", dv.Rows[0].Instrument)
	}
}

// TestMobileSoftKeyboardRenameCommits verifies that pressing Enter/Done on a
// soft keyboard (which sends '\n' via inputChars rather than isKeyPressed(Enter))
// correctly commits the rename. This was broken because TextInput.Update() saw
// '\n', defocused, and returned true — but OnCommit was never called.
func TestMobileSoftKeyboardRenameCommits(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withSmallScreen(t, true)

	logger := gamelog.New(testLogOutput(), gamelog.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 600), nil, logger)

	if len(dv.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	origName := dv.Rows[0].Name
	// Rename is metadata-only: clear the process-global display override it leaves.
	softID := dv.Rows[0].Instrument
	t.Cleanup(func() { audio.ClearInstrumentDisplayName(softID) })

	// Initial Update to build layout and buttons.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 600 },
	)
	dv.Update()
	restore()

	if len(dv.rowEditBtns()) == 0 {
		t.Fatal("no edit buttons after initial Update")
	}

	// Click the edit button to open rename in desktop-fallback mode
	// (mobileInputActive is false, so Open() creates a TextInput).
	dv.rowEditBtns()[0].OnClick()

	if dv.renameComp == nil || !dv.renameComp.IsOpen() {
		t.Fatal("renameComp not open after edit click")
	}

	// 1 idle frame to clear the tree's button capture from the opening click.
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 600 },
	)
	dv.Update()
	restore()

	// Directly set the text box contents (simulates the user clearing and typing).
	tb := dv.renameComp.TextBox()
	if tb == nil {
		t.Fatal("renameComp textBox is nil")
	}
	tb.SetText("NewKick")

	// Inject '\n' via inputChars with isKeyPressed(Enter) = false.
	// This simulates the soft keyboard Done/Enter button.
	restore = SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return []rune{'\n'} },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 400, 600 },
	)
	dv.Update()
	restore()

	// 2 idle frames to settle.
	for i := 0; i < 2; i++ {
		restore = SetInputForTest(
			func() (int, int) { return 0, 0 },
			func(b ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return 400, 600 },
		)
		dv.Update()
		restore()
	}

	// Assert: rename committed.
	if dv.Rows[0].Name != "NewKick" {
		t.Errorf("Rows[0].Name = %q, want %q (was %q)", dv.Rows[0].Name, "NewKick", origName)
	}
	if len(dv.rowLabels()) > 0 && dv.rowLabels()[0].Text != "NewKick" {
		t.Errorf("rowLabels[0].Text = %q, want %q", dv.rowLabels()[0].Text, "NewKick")
	}
	if dv.renameComp.IsOpen() {
		t.Error("renameComp still open after soft keyboard Enter")
	}

	// Verify notification was shown.
	found := false
	for _, n := range dv.notifStore.History() {
		if strings.Contains(n.display(), "NewKick") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected info notification containing 'NewKick', got %v", dv.notifStore.History())
	}
}
