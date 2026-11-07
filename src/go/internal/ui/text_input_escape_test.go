package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Swaps isKeyPressed AND isKeyJustPressed: SetInputForTest swaps only the first, and handleEscape reads the second.
func escStub() func() {
	return stubKeys(
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
		map[ebiten.Key]bool{ebiten.KeyEscape: true},
	)
}

// The inline Save As dialog is neither a portal nor a focused zone, so Esc must stop at it: close, no save, no Stop.
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

// Through the full Game.Update loop, where an isolated handler passing proves nothing.
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
