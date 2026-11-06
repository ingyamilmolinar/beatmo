//go:build !test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The reported path through the REAL startup load (buildDemo is a stub under
// -tags test, so only this build reaches it): the user edits the startup
// circuit's first instrument and presses Save; on the NEXT launch (a new Game
// whose startup circuit loads through buildDemo, after the bootstrap loaded the
// persisted library) that instrument plays the saved tone, not the circuit's,
// and Reset returns it to the circuit's. buildDemo raises the undo manager's
// restoring flag to keep the load out of the history; that must not make the
// open look like an undo restore. Reads whatever instrument the demo's row 0
// carries — it pins nothing about the demo's contents. The real-input Save /
// dial / Reset flow is covered by synth_save_trumps_circuit_test.go.
func TestSynthSaveTrumpsTheStartupCircuitOnNextLaunch(t *testing.T) {
	lib := withPrivateInstrumentLibrary(t)
	boot := func() *Game {
		audio.ResetInstrumentDocs()
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(1280, 720)
		g.buildDemo()
		if len(g.drum.Rows) == 0 {
			t.Fatal("the startup circuit built no rows")
		}
		return g
	}
	g := boot()
	inst := g.drum.Rows[0].Instrument
	circuitDoc, ok := audio.CircuitInstrumentDoc(inst)
	if !ok {
		t.Fatalf("the startup circuit carried no document for its row-0 %q", inst)
	}
	reg := audio.BuiltinModuleRegistry()
	gain := instrument.FieldValues(circuitDoc, reg)["gain"]
	audio.SetInstrumentParam(inst, "gain", gain*0.5+0.01)
	saved, _ := audio.InstrumentDoc(inst)
	if len(instrument.Diff(saved, circuitDoc)) == 0 {
		t.Fatal("premise: the edit changed nothing")
	}
	if got := saveRecipeForInstrument(inst); got == "" || len(lib.docs[got]) == 0 {
		t.Fatalf("Save persisted nothing for %q (saved as %q)", inst, got)
	}

	// Next launch: the bootstrap loads the persisted library, then the circuit.
	if err := audio.LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	g2 := boot()
	got, _ := audio.InstrumentDoc(inst)
	if d := instrument.Diff(got, saved); len(d) != 0 {
		t.Fatalf("on relaunch the startup circuit's %s won over the user's Save: %v", inst, d)
	}
	if audio.InstrumentDocModified(inst) {
		t.Error("on relaunch the saved instrument reads dirty (Save lit)")
	}
	if g2.undoManager.CanUndo() {
		t.Error("the startup load must not leave an undo step")
	}
}
