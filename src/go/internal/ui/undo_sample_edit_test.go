//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSamplerSaveIsUndoableAndDoesNotDrift is the regression guard for the
// sample-edit baseline-drift bug. A sampler Save changes the exported document
// (it stores a non-destructive SampleEdit descriptor) but recorded NO undo step,
// because the descriptor's hooks event is published from the audio package — not
// a UI emit helper that taps recordUndo. The committed baseline therefore went
// stale, and undoing a LATER unrelated action (e.g. a BPM change) silently
// reverted the sample edit too.
//
// After the fix: Save records its own atomic undo step, and undoing a later
// action leaves the sample edit intact.
func TestSamplerSaveIsUndoableAndDoesNotDrift(t *testing.T) {
	g := newTestGameForUndo(t)
	inst := g.drum.Rows[0].Instrument

	// Make the row instrument a synth source the sampler can edit non-destructively.
	audio.BindInstrumentToRecipe(inst, "drum-kick")
	t.Cleanup(func() { audio.ClearSampleEdit(inst) })
	g.drum.ensureSamplerLoaded(inst)
	s := &g.drum.sampler
	if !s.hasBuffer() || s.source != samplerSourceSynth {
		t.Skipf("sampler setup unavailable: hasBuffer=%v source=%v", s.hasBuffer(), s.source)
	}
	s.captureID = inst
	s.startFrac = 0.25 // non-identity edit so the export changes

	// Baseline = the document just before Save.
	g.undoManager.OnExternalLoad()
	before := g.undoCapture()

	g.drum.samplerSave()

	if !g.undoManager.CanUndo() {
		t.Fatal("sampler Save produced no undo step (sample edit not undoable)")
	}
	afterSave := g.undoCapture()
	if string(afterSave) == string(before) {
		t.Fatal("precondition: Save did not change the exported document")
	}

	// A later, unrelated recorded action.
	g.drum.SetBPM(133)
	g.undoManager.record("change BPM")

	// Undoing the BPM change must restore the post-Save document, NOT wipe the
	// sample edit (the drift symptom).
	g.undoManager.Undo()
	if got := g.undoCapture(); string(got) != string(afterSave) {
		t.Fatal("undoing the BPM change also reverted the sample edit (baseline drift)")
	}
	if !audio.HasSampleEdit(inst) {
		t.Fatal("sample edit lost after undoing an unrelated action (baseline drift)")
	}

	// And undoing once more must revert the sample edit itself (it is its own step).
	g.undoManager.Undo()
	if got := g.undoCapture(); string(got) != string(before) {
		t.Fatal("second Undo did not revert the sample edit to the pre-Save document")
	}
}
