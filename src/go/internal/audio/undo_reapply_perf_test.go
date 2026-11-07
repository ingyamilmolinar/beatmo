//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Every undo/redo rides the import path; re-applying unchanged params re-rendered the whole kit.
func TestSetInstrumentParamsSkipsRedundantReapply(t *testing.T) {
	resetLibraryForTest(t)
	const id = roster.Kick

	count := 0
	restore := SwapPlatformInstrumentDocChangedForTest(func(instrument.ID, instrument.Doc) { count++ })
	defer SwapPlatformInstrumentDocChangedForTest(restore)

	SetInstrumentParams(id, RecipeParams{"drive": 0.5})
	if count != 1 {
		t.Fatalf("first apply should dispatch exactly once, got %d", count)
	}
	SetInstrumentParams(id, RecipeParams{"drive": 0.5}) // identical → no-op
	if count != 1 {
		t.Fatalf("re-applying identical params re-rendered the instrument (count=%d, want 1)", count)
	}
	SetInstrumentParams(id, RecipeParams{"drive": 0.8}) // real change → dispatch
	if count != 2 {
		t.Fatalf("a genuine param change must dispatch (count=%d, want 2)", count)
	}
}

func TestSetSampleEditSkipsRedundantReapply(t *testing.T) {
	const id = "test.inst.redundant.sampleedit"
	t.Cleanup(func() { ClearSampleEdit(id) })

	count := 0
	restore := SwapPlatformSampleEditChangedForTest(func(instrument.ID, SampleEdit) { count++ })
	defer SwapPlatformSampleEditChangedForTest(restore)

	e := SampleEdit{StartFrac: 0.1, EndFrac: 0.9, GainDB: -3}
	SetSampleEdit(id, e)
	if count != 1 {
		t.Fatalf("first sample edit should dispatch once, got %d", count)
	}
	SetSampleEdit(id, SampleEdit{StartFrac: 0.1, EndFrac: 0.9, GainDB: -3}) // identical → no-op
	if count != 1 {
		t.Fatalf("re-applying identical sample edit re-rendered (count=%d, want 1)", count)
	}
}
