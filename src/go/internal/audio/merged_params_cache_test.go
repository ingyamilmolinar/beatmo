//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestMergedInstrumentParamsRO_InvalidatesOnEveryMutationPath guards the
// revision plumbing behind the merged-params cache: every mutation that can
// change an instrument's flat values must be visible through the cached
// read-only accessor on the next call. A missed bump would freeze the Synth
// tab's previews/mirror at stale values (the cache backs every per-frame
// read — see merged_params_cache.go).
func TestMergedInstrumentParamsRO_InvalidatesOnEveryMutationPath(t *testing.T) {
	resetLibraryForTest(t)
	const inst = roster.DnbKick

	base := MergedInstrumentParamsRO(inst)
	if len(base) == 0 {
		t.Fatal("empty merged map for a shipped instrument")
	}
	// Same revs → the cached read repeats identically (no re-merge needed).
	if again := MergedInstrumentParamsRO(inst); len(again) != len(base) {
		t.Fatal("second read changed shape with no mutation")
	}

	// 1. SetInstrumentParam must invalidate.
	orig := base["gen1_kick_attack"]
	SetInstrumentParam(inst, "gen1_kick_attack", orig+0.25)
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig+0.25 {
		t.Errorf("after SetInstrumentParam: merged gen1_kick_attack=%v, want %v", got, orig+0.25)
	}

	// 2. ResetInstrumentParams must invalidate.
	ResetInstrumentParams(inst)
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig {
		t.Errorf("after ResetInstrumentParams: merged gen1_kick_attack=%v, want default %v", got, orig)
	}

	// 3. Replacing the whole document (import, undo) must invalidate.
	d, _ := InstrumentDoc(inst)
	d2, err := instrument.SetField(d, BuiltinModuleRegistry(), "gen1_kick_attack", orig+0.5)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetInstrumentDoc(inst, d2); err != nil {
		t.Fatal(err)
	}
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig+0.5 {
		t.Errorf("after SetInstrumentDoc: merged gen1_kick_attack=%v, want %v", got, orig+0.5)
	}

	// 4. A library Save then Reset lands on the saved entry.
	lib := d2.Clone()
	lib.ID, lib.Origin = inst, ""
	lib, _ = instrument.SetField(lib, BuiltinModuleRegistry(), "gen1_kick_attack", orig+0.75)
	if err := SaveLibraryDoc(lib); err != nil {
		t.Fatal(err)
	}
	ResetInstrumentParams(inst)
	if got := MergedInstrumentParamsRO(inst)["gen1_kick_attack"]; got != orig+0.75 {
		t.Errorf("after library Save + Reset: merged gen1_kick_attack=%v, want %v", got, orig+0.75)
	}
}
