//go:build test

package ui

import (
	"bytes"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// These tests close the render-level gap in the import/export round trip:
// JSON fields and store state can round-trip while the tone the next trigger
// renders does not. A synth voice renders exactly the flattened block of the
// instrument's project document (spec 2026-10-04 §8: render(Flatten(doc))), so
// renderedBlock(id) below IS the rendered tone on every path, and is available
// under -tags test where the C renderers are stubbed.
func renderedBlock(t *testing.T, inst instrument.ID) []float64 {
	t.Helper()
	d, ok := audio.InstrumentDoc(inst)
	if !ok {
		t.Fatalf("no instrument document for %q", inst)
	}
	return instrument.Flatten(d, audio.BuiltinModuleRegistry())
}

// customizeLibraryForTest simulates a session whose user library holds a Saved
// version of inst (a Synth-tab Save in a previous session): the library entry
// moves field by v, and the project copy is a fresh pick of it.
func customizeLibraryForTest(t *testing.T, inst, field string, v float64) {
	t.Helper()
	lib, ok := audio.LibraryDoc(instrument.ID(inst))
	if !ok {
		t.Fatalf("no library document for %q", inst)
	}
	saved, err := instrument.SetField(lib, audio.BuiltinModuleRegistry(), field, v)
	if err != nil {
		t.Fatalf("customize %s.%s: %v", inst, field, err)
	}
	if err := audio.SaveLibraryDoc(saved); err != nil {
		t.Fatal(err)
	}
	audio.ResetInstrumentDocs()
}

// importInto imports data into a fresh Game and returns it.
func importInto(t *testing.T, data []byte) *Game {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	return g
}

// assertSavedTrumpsThenResetRestores: after an import into a session whose
// library holds a Saved version of inst, inst renders the SAVED tone (the
// user's Save trumps the file's document for the same instrument), and Reset
// returns it to exactly what the file carried.
func assertSavedTrumpsThenResetRestores(t *testing.T, inst instrument.ID, fileTone []float64) {
	t.Helper()
	saved, ok := audio.UserLibraryDoc(inst)
	if !ok {
		t.Fatalf("precondition: no Saved %q in the library", inst)
	}
	savedTone := instrument.Flatten(saved, audio.BuiltinModuleRegistry())
	if slices.Equal(savedTone, fileTone) {
		t.Fatal("precondition: the Saved version sounds like the file's")
	}
	if got := renderedBlock(t, inst); !slices.Equal(got, savedTone) {
		t.Error("the imported instrument does not render the user's Saved version")
	}
}

// TestImportRender_OverlayEdit_IntoCustomizedSession: an edited instrument,
// exported, imported into a session whose library holds a DIFFERENT saved
// version of it, renders the session's Save (the user's version trumps any
// circuit's), and Reset brings back exactly what was exported.
func TestImportRender_OverlayEdit_IntoCustomizedSession(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.42)
	audio.SetInstrumentParam(roster.Snare, "gen1_snare_tune", 1.3)
	want := renderedBlock(t, roster.Snare)
	data := mustExport(t, g)

	customizeLibraryForTest(t, "snare", "post_drive", 0.5)
	importInto(t, data)
	assertSavedTrumpsThenResetRestores(t, roster.Snare, want)
}

// TestImportRender_ShippedProject_IntoCustomizedSession: a project exported
// with an UNEDITED instrument, imported where the library holds a Saved version
// of it, plays the Saved version; Reset returns to the file's.
func TestImportRender_ShippedProject_IntoCustomizedSession(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	want := renderedBlock(t, roster.Snare)
	data := mustExport(t, g)

	customizeLibraryForTest(t, "snare", "post_drive", 0.5)
	if slices.Equal(renderedBlock(t, roster.Snare), want) {
		t.Fatal("precondition: the customized library did not change the session's tone")
	}
	importInto(t, data)
	assertSavedTrumpsThenResetRestores(t, roster.Snare, want)
}

// TestImportRender_SampleEditPlusParams: a document edit and a non-destructive
// Sampler edit travel together; both survive an import (into a customized
// session the Saved document plays and Reset restores the file's, while the
// Sampler edit — not part of the document — is restored either way).
func TestImportRender_SampleEditPlusParams(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.42)
	edit := audio.SampleEdit{StartFrac: 0.1, EndFrac: 0.9, GainDB: -3, Reverse: true}
	audio.SetSampleEdit(roster.Snare, edit)
	t.Cleanup(func() { audio.ClearSampleEdit(roster.Snare) })
	want := renderedBlock(t, roster.Snare)
	data := mustExport(t, g)

	audio.ClearSampleEdit(roster.Snare)
	customizeLibraryForTest(t, "snare", "post_drive", 0.5)
	importInto(t, data)
	assertSavedTrumpsThenResetRestores(t, roster.Snare, want)
	gotEdit, ok := audio.SampleEditFor(roster.Snare)
	if !ok {
		t.Fatal("sample edit descriptor not restored by import")
	}
	if gotEdit != edit {
		t.Errorf("sample edit after import = %+v, want %+v", gotEdit, edit)
	}
}

// TestImportRender_Modular_SwitchesAndStages: the modular voice's oscillator
// switch, a stage turned off and an enum value survive the round trip.
func TestImportRender_Modular_SwitchesAndStages(t *testing.T) {
	g := newDocsGame(t, roster.Modular)
	audio.SetInstrumentParam(roster.Modular, "osc_type", 2)
	audio.SetInstrumentParam(roster.Modular, "filter_enabled", 0)
	audio.SetInstrumentParam(roster.Modular, "noise_seed", 7)
	want := renderedBlock(t, roster.Modular)
	data := mustExport(t, g)

	wipeToCleanLibrary(t)
	importInto(t, data)
	if got := renderedBlock(t, roster.Modular); !slices.Equal(got, want) {
		t.Error("the modular voice's switches/stages did not survive the round trip")
	}
}

// TestImportRender_PlainOverlay_CleanSession: edit → export → import on a clean
// machine reproduces the exported tone and re-exports byte-identically.
func TestImportRender_PlainOverlay_CleanSession(t *testing.T) {
	g := newDocsGame(t, roster.Snare)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.66)
	audio.SetInstrumentParam(roster.Snare, "pitch", 4)
	want := renderedBlock(t, roster.Snare)
	bytes1 := mustExport(t, g)

	wipeToCleanLibrary(t)
	g2 := importInto(t, bytes1)
	if got := renderedBlock(t, roster.Snare); !slices.Equal(got, want) {
		t.Error("the imported instrument does not render the exported tone")
	}
	if bytes2 := mustExport(t, g2); !bytes.Equal(bytes1, bytes2) {
		t.Errorf("re-export not byte-identical in a clean session")
	}
}
