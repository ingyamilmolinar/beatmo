//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// An undo/redo restore re-imports an identical document; re-rendering every instrument on it made a single synth
// edit's undo re-render the whole kit.
func TestReimportSameDocSkipsInstrumentRerender(t *testing.T) {
	g := newTestGameForUndo(t)
	id := g.drum.Rows[0].Instrument
	_ = audio.InstallInstrumentDocForTest(id, roster.Modular)
	t.Cleanup(func() { audio.ResetInstrumentParams(id) })
	audio.SetInstrumentParam(id, "drive", 0.5) // a non-default tone so it exports
	g.updateBeatInfos()

	snap := g.undoCapture()

	// First import establishes audio state.
	g.undoManager.restoring = true
	if err := g.Import(snap); err != nil {
		t.Fatalf("first import: %v", err)
	}
	g.undoManager.restoring = false

	// Count instrument param re-dispatches during a SECOND identical import.
	params := 0
	restoreP := audio.SwapPlatformInstrumentDocChangedForTest(func(instrument.ID, instrument.Doc) { params++ })
	defer audio.SwapPlatformInstrumentDocChangedForTest(restoreP)

	g.undoManager.restoring = true
	if err := g.Import(snap); err != nil {
		t.Fatalf("second import: %v", err)
	}
	g.undoManager.restoring = false

	if params != 0 {
		t.Fatalf("re-importing the identical document re-rendered instruments "+
			"(document pushes=%d, want 0) — undo/redo re-renders unchanged instruments",
			params)
	}
}
