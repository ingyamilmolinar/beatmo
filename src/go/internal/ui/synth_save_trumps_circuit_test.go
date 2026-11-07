//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// A Save is the user's own version: it persists and, on every later circuit open, trumps the circuit's document
// for the same instrument (identity = Origin). Real input throughout.

// A private library and a circuit whose violin differs from the shipped one.
func newSaveTrumpsGame(t *testing.T) (*Game, *memInstrumentLibrary, []byte, instrument.Doc) {
	t.Helper()
	lib := withPrivateInstrumentLibrary(t)
	g := newDialGame(t)
	inst := dialInst(g)
	// The circuit's OWN tone: a Length turn the shipped library does not have.
	dragFrom(g, centre(dialRect(t, g, "length", "tail")), 50)
	if !audio.InstrumentDocModified(instrument.ID(inst)) {
		t.Fatal("premise: the circuit's violin must differ from the library's")
	}
	circuit := mustExport(t, g)
	circuitDoc, _ := audio.InstrumentDoc(instrument.ID(inst))
	if err := g.Import(circuit); err != nil {
		t.Fatal(err)
	}
	openDialRoot(t, g)
	return g, lib, circuit, circuitDoc
}

// Origin and name are identity, not sound.
func soundDiff(a, b instrument.Doc) []string {
	return instrument.Diff(a, b)
}

func TestSynthSaveTrumpsTheCircuitOnTheNextOpen(t *testing.T) {
	g, lib, circuit, circuitDoc := newSaveTrumpsGame(t)
	inst := dialInst(g)

	// A high-level category dial, then the Save button.
	dragFrom(g, centre(dialRect(t, g, "brightness", "bright")), -70)
	saved, _ := audio.InstrumentDoc(instrument.ID(inst))
	if len(soundDiff(saved, circuitDoc)) == 0 {
		t.Fatal("premise: the dial turn changed nothing")
	}
	saveOverThroughDialog(t, g)

	if len(lib.docs[instrument.ID(inst)]) == 0 {
		t.Fatalf("Save persisted nothing for %q (sink holds %d docs)", inst, len(lib.docs))
	}
	if audio.InstrumentDocModified(instrument.ID(inst)) {
		t.Error("after Save the instrument still reads dirty")
	}

	// A fresh boot: the persisted library is loaded, then the circuit opens.
	if err := audio.LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	audio.ResetInstrumentDocs()
	if err := g.Import(circuit); err != nil {
		t.Fatal(err)
	}
	idleFrame(g)
	got, _ := audio.InstrumentDoc(instrument.ID(inst))
	if d := soundDiff(got, saved); len(d) != 0 {
		t.Fatalf("the circuit's violin won over the user's Save on reopen: %v", d)
	}
	if got.Origin != instrument.ID(inst) {
		t.Errorf("identity lost: origin %q, want %q", got.Origin, inst)
	}
	if audio.InstrumentDocModified(instrument.ID(inst)) {
		t.Error("the opened copy equals the user's saved entry; it must read clean")
	}

}

// The Save As button, then the dialog's "Save over …" confirm with the prefilled name.
func saveOverThroughDialog(t *testing.T, g *Game) {
	t.Helper()
	tapAtPoint(g, centre(g.drum.SynthTabHeader().saveAsRect))
	dlg := g.drum.SynthSaveAsDialog()
	if dlg == nil {
		t.Fatal("Save As did not open its dialog")
	}
	if dlg.ConfirmLabel() == "" || dlg.Refused() != "" {
		t.Fatalf("the prefilled name cannot be saved over: %q / %q", dlg.ConfirmLabel(), dlg.Refused())
	}
	tapAtPoint(g, centre(dlg.OKRect()))
	if g.drum.SynthSaveAsDialog() != nil {
		t.Fatal("confirming Save As left its dialog open")
	}
}

// Pointer parked off the panel.
func typeFrame(g *Game, s string) {
	restore := SetInputForTest(
		func() (int, int) { return 1, 1 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return []rune(s) },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	_ = g.Update()
	restore()
}

// Prefilled with the current name; the typed name becomes the new instrument's, it persists, and the row keeps its id
// while its copy points at the new entry.
func TestSynthSaveAsPromptsPrefilledWithTheCurrentName(t *testing.T) {
	g, lib, _, _ := newSaveTrumpsGame(t)
	inst := dialInst(g)
	dragFrom(g, centre(dialRect(t, g, "brightness", "bright")), -70)
	edited, _ := audio.InstrumentDoc(instrument.ID(inst))

	tapAtPoint(g, centre(g.drum.SynthTabHeader().saveAsRect))
	dlg := g.drum.SynthSaveAsDialog()
	if dlg == nil {
		t.Fatal("Save As did not open the name prompt")
	}
	want := audio.InstrumentDisplayName(instrument.ID(inst))
	if dlg.Value() != want {
		t.Fatalf("prompt prefilled %q, want the current name %q", dlg.Value(), want)
	}
	if !dlg.Focused() {
		t.Fatal("the name field must be focused so the user can type at once")
	}
	typeFrame(g, " Bright")
	tapAtPoint(g, centre(dlg.OKRect()))
	if g.drum.SynthSaveAsDialog() != nil {
		t.Fatal("OK did not close the prompt")
	}

	d, _ := audio.InstrumentDoc(instrument.ID(inst))
	if d.ID != instrument.ID(inst) {
		t.Fatalf("the row's instrument id changed to %q", d.ID)
	}
	saved, ok := audio.UserLibraryDoc(d.Origin)
	if !ok || saved.Name != want+" Bright" {
		t.Fatalf("library entry %q: ok=%v name=%q, want %q", d.Origin, ok, saved.Name, want+" Bright")
	}
	if len(lib.docs[d.Origin]) == 0 {
		t.Fatalf("Save As persisted nothing for %q", d.Origin)
	}
	if diff := instrument.Diff(saved, edited); len(diff) != 0 && !onlyIdentity(diff) {
		t.Fatalf("Save As stored a different sound: %v", diff)
	}
	if audio.InstrumentDocModified(instrument.ID(inst)) {
		t.Error("after Save As the copy must read clean")
	}

	// Header, row label and the next prompt show it.
	idleFrame(g)
	if got := audioTabHeaderName(g.drum.SynthTabHeader().instLabel); got != want+" Bright" {
		t.Errorf("Synth header reads %q after Save As, want the new name", got)
	}
	if g.drum.Rows[0].Name != want+" Bright" {
		t.Errorf("row label %q after Save As, want %q", g.drum.Rows[0].Name, want+" Bright")
	}
	tapAtPoint(g, centre(g.drum.SynthTabHeader().saveAsRect))
	if dlg := g.drum.SynthSaveAsDialog(); dlg == nil || dlg.Value() != want+" Bright" {
		t.Fatalf("second Save As prefilled %q, want the current name %q", dlg.Value(), want+" Bright")
	}
	keyTap(g, ebiten.KeyEscape)
	if g.drum.SynthSaveAsDialog() != nil {
		t.Fatal("Esc did not cancel the prompt")
	}
}

// onlyIdentity reports whether every diff path is identity (id/origin/name).
func onlyIdentity(diff []string) bool {
	for _, p := range diff {
		if p != "id" && p != "origin" && p != "name" {
			return false
		}
	}
	return true
}

// The saved version trumps only on OPEN, never on an undo, or undoing an unsaved edit would jump to the saved tone.
func TestUndoNeverReappliesTheSavedVersion(t *testing.T) {
	g, _, _, _ := newSaveTrumpsGame(t)
	inst := dialInst(g)
	g.undoManager.OnExternalLoad()

	saveOverThroughDialog(t, g) // saved = the circuit's tone
	dragFrom(g, centre(dialRect(t, g, "brightness", "bright")), -60)
	unsaved, _ := audio.InstrumentDoc(instrument.ID(inst))
	dragFrom(g, centre(dialRect(t, g, "length", "tail")), -40)

	g.performUndo()
	idleFrame(g)
	got, _ := audio.InstrumentDoc(instrument.ID(inst))
	if d := soundDiff(got, unsaved); len(d) != 0 {
		t.Fatalf("undo did not restore the previous (unsaved) step: %v", d)
	}
}

// The exported circuit names the new entry as origin, so after a reboot the saved-as tone still plays.
func TestSaveAsIdentityTrumpsAfterReopen(t *testing.T) {
	g, lib, _, _ := newSaveTrumpsGame(t)
	inst := dialInst(g)
	g.drum.SaveActiveRecipeAs("Violin Mine")
	origin := func() string { d, _ := audio.InstrumentDoc(instrument.ID(inst)); return string(d.Origin) }()
	circuit := mustExport(t, g) // carries origin = the new entry

	// Later the user tweaks it again and Saves over the saved-as entry.
	dragFrom(g, centre(dialRect(t, g, "brightness", "bright")), -60)
	saveOverThroughDialog(t, g)
	saved, _ := audio.InstrumentDoc(instrument.ID(inst))
	if o := func() string { d, _ := audio.InstrumentDoc(instrument.ID(inst)); return string(d.Origin) }(); o != origin {
		t.Fatalf("Save after Save As moved the origin %q → %q", origin, o)
	}

	if err := audio.LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	audio.ResetInstrumentDocs()
	if err := g.Import(circuit); err != nil {
		t.Fatal(err)
	}
	got, _ := audio.InstrumentDoc(instrument.ID(inst))
	if got.Origin != instrument.ID(origin) {
		t.Fatalf("identity lost on reopen: origin %q, want %q", got.Origin, origin)
	}
	if d := soundDiff(got, saved); len(d) != 0 {
		t.Fatalf("the reopened circuit did not play the user's latest Save: %v", d)
	}
}
