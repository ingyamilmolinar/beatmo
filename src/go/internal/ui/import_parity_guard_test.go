package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Guard: parity checks must be skipped while the import dialog is open so the
// sequencer cannot panic on slate/predictor mismatches that occur while the UI
// thread is blocked by the file picker.
func TestParityCheckSkippedDuringImportDialog(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	prevFatal := parityFatalEnabled.Load()
	prevWatch := parityWatchDefault

	// Simulate the import chooser being open before Game.Import is invoked.
	old := selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, error)) {}
	defer func() { selectJSONAsyncFn = old }()
	g.drum.importBtn().OnClick()
	if !g.importDialog || !g.drum.importing {
		t.Fatalf("import dialog not active after click: dialog=%v importing=%v", g.importDialog, g.drum.importing)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("parityCheck should be skipped during import dialog, got panic: %v", r)
		}
	}()

	// Force a mismatch scenario: scheduled=true while slate=false.
	info := model.BeatInfo{NodeType: model.NodeTypeRegular}
	g.parityCheck(0, 0, info, true, "test-import-guard", false)

	// Ensure dialog flag clears and parity settings are restored.
	g.endImportDialog()
	if g.importDialog {
		t.Fatalf("importDialog flag not cleared after endImportDialog")
	}
	if g.parityWatch != prevWatch {
		t.Fatalf("parity watch not restored after import dialog end: %v (want %v)", g.parityWatch, prevWatch)
	}
	if parityFatalEnabled.Load() != prevFatal {
		t.Fatalf("parity fatal not restored after import dialog end: %v (want %v)", parityFatalEnabled.Load(), prevFatal)
	}
}
