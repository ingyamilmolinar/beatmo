package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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

// TestEsc_SaveAsDialog_CancelsWithoutSaveOrStop is the regression for the bug
// where Esc on the inline Save-As dialog (not a portal, not a focused zone)
// fell through the ladder to the Stop rung — stopping playback instead of just
// cancelling the dialog. Esc must: close the dialog, NOT save the typed name,
// and NOT request Stop.
func TestEsc_SaveAsDialog_CancelsWithoutSaveOrStop(t *testing.T) {
	g := newTestGameForUndo(t)
	g.SetPlaying(true)

	withPrivateInstrumentLibrary(t)
	g.drum.saveAsDialog = &synthSaveAsDialog{
		textInput: NewTextInput(image.Rect(0, 0, 120, 24), BPMBoxStyle),
		instID:    g.drum.Rows[0].Instrument,
	}
	g.drum.saveAsDialog.textInput.SetText("My Preset")

	restore := escStub()
	defer restore()
	g.handleGlobalShortcuts()

	if g.drum.saveAsDialog != nil {
		t.Fatal("Esc should close the Save-As dialog")
	}
	if len(audio.LibraryIDs()) != len(roster.All) {
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
