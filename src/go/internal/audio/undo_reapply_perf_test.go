//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestSetInstrumentParamsSkipsRedundantReapply proves the bulk param setter (the
// JSON import path that every undo/redo restore rides) does NOT re-render an
// instrument when the params are unchanged. Without the no-op skip, each
// undo/redo re-applied EVERY instrument's full param set, firing the WASM
// re-render bridge + voice-cache invalidation for every instrument — so undoing
// a single synth-control change re-rendered the whole kit (the "applying actions is
// slow" lag).
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

// TestSetSampleEditSkipsRedundantReapply proves re-applying an identical sampler
// edit is a no-op (no re-render) so sampler undo/redo doesn't re-render unchanged
// instruments.
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
