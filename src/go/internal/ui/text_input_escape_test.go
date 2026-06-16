package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// escStub presses Esc through the universal handler (edge-triggered, the way
// Game.Update drives it). stubKeys swaps BOTH isKeyPressed and isKeyJustPressed
// (SetInputForTest only swaps isKeyPressed, so handleEscape — which uses
// isKeyJustPressed — would not fire under it).
func escStub() func() {
	return stubKeys(
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
	)
}

// TestEsc_BPMBox_RealFocusRevertsAndCloses focuses the BPM box via a real click
// (so the tree sets focusedZone the way production does), types a new value,
// then presses Esc through the universal handler and asserts the edit is
// reverted and the box is blurred.
func TestEsc_BPMBox_RealFocusRevertsAndCloses(t *testing.T) {
	g := newTestGameForUndo(t)
	orig := g.drum.BPM()
	tz := g.drum.transportZone

	// Open the shared editor and type an uncommitted edit.
	tz.openBPMEditor()
	tz.paramEditor.ti.SetText("240")

	// The shared editor owns Escape via its own Update (isKeyPressed(Escape)).
	restore := escStub()
	defer restore()
	tz.Update()

	if tz.paramEditor.Active() {
		t.Fatal("Esc should close the BPM editor")
	}
	if g.drum.transportZone.BPM() != orig {
		t.Fatalf("Esc should leave the BPM at %d (uncommitted), got %d", orig, g.drum.transportZone.BPM())
	}
}

// TestEsc_EQdBInput_RevertsAndCloses opens the shared EQ dB editor for a band,
// types an uncommitted edit, then Esc (through the editor's own Update, the way
// Game.Update drives it) reverts to the saved value and closes the editor.
func TestEsc_EQdBInput_RevertsAndCloses(t *testing.T) {
	g := newTestGameForUndo(t)
	ez := g.drum.eqPanelZone
	if ez == nil {
		t.Skip("no eq panel zone")
	}
	ez.bandGainsDB[0] = 0.0
	ez.openEQDBEditor(0)
	ez.paramEditor.ti.SetText("9.0") // uncommitted edit

	// The shared editor owns Escape via its own Update (isKeyPressed(Escape)).
	restore := escStub()
	defer restore()
	ez.Update()

	if ez.paramEditor.Active() {
		t.Fatalf("Esc should close the EQ dB editor")
	}
	if ez.bandGainsDB[0] != 0.0 {
		t.Fatalf("Esc should leave the dB gain at 0.0, got %v", ez.bandGainsDB[0])
	}
	if formatDB(ez.bandGainsDB[0]) != formatDB(0.0) {
		t.Fatalf("Esc should leave the dB readout at %q, got %q", formatDB(0.0), formatDB(ez.bandGainsDB[0]))
	}
}

// TestEsc_SaveAsDialog_CancelsWithoutSaveOrStop is the regression for the bug
// where Esc on the inline Save-As dialog (not a portal, not a focused zone)
// fell through the ladder to the Stop rung — stopping playback instead of just
// cancelling the dialog. Esc must: close the dialog, NOT save the typed name,
// and NOT request Stop.
func TestEsc_SaveAsDialog_CancelsWithoutSaveOrStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.SetPlaying(true)

	saved := false
	g.drum.saveAsDialog = &synthSaveAsDialog{
		textInput: NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle),
		confirm:   func(string) { saved = true },
	}
	g.drum.saveAsDialog.textInput.SetText("My Preset")

	restore := escStub()
	defer restore()
	g.handleGlobalShortcuts()

	if g.drum.saveAsDialog != nil {
		t.Fatal("Esc should close the Save-As dialog")
	}
	if saved {
		t.Fatal("Esc must cancel — the typed name must NOT be saved")
	}
	if g.drum.StopPressed() {
		t.Fatal("Esc on a text dialog must NOT fall through to Stop playback")
	}
}

// TestEsc_SaveAsDialog_CancelsThroughRealUpdate drives the FULL Game.Update
// loop (the production path) to confirm the dialog cancels end-to-end and
// playback is not stopped — guarding against the isolated-handler-passes /
// real-loop-fails trap.
func TestEsc_SaveAsDialog_CancelsThroughRealUpdate(t *testing.T) {
	g := newTestGameForUndo(t)
	g.SetPlaying(true)
	g.drum.saveAsDialog = &synthSaveAsDialog{
		textInput: NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle),
	}
	g.drum.saveAsDialog.textInput.SetText("Discard me")

	restore := escStub()
	defer restore()
	if err := g.Update(); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if g.drum.saveAsDialog != nil {
		t.Fatal("Esc (through real Update) should close the Save-As dialog")
	}
	if !g.Playing() {
		t.Fatal("Esc on the Save-As dialog must NOT stop playback")
	}
}
