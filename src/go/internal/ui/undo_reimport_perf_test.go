//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestReimportSameDocSkipsInstrumentRerender proves that re-importing an
// identical document — exactly what an undo/redo restore does — does NOT
// re-render any instrument. Before the audio-layer no-op skip, every undo/redo
// re-applied (and on WASM re-rendered) every instrument's params + recipe, so
// undoing a single synth-knob change re-rendered the whole kit: the
// "applying actions is slow" lag.
func TestReimportSameDocSkipsInstrumentRerender(t *testing.T) {
	g := newTestGameForUndo(t)
	id := g.drum.Rows[0].Instrument
	audio.BindInstrumentToRecipe(id, "synth-modular")
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
	restoreP := audio.SwapPlatformInstrumentParamsChangedForTest(func(string, audio.RecipeParams) { params++ })
	defer audio.SwapPlatformInstrumentParamsChangedForTest(restoreP)
	recipes := 0
	restoreR := audio.SwapPlatformInstrumentRecipeChangedForTest(func(string, string, string) { recipes++ })
	defer audio.SwapPlatformInstrumentRecipeChangedForTest(restoreR)

	g.undoManager.restoring = true
	if err := g.Import(snap); err != nil {
		t.Fatalf("second import: %v", err)
	}
	g.undoManager.restoring = false

	if params != 0 || recipes != 0 {
		t.Fatalf("re-importing the identical document re-rendered instruments "+
			"(param dispatches=%d, recipe rebinds=%d, want 0/0) — undo/redo re-renders unchanged instruments",
			params, recipes)
	}
}
