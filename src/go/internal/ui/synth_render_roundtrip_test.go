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

// A voice renders exactly the flattened block of its document, so renderedBlock is the rendered tone on every path,
// even under -tags test where the C is stubbed.
func renderedBlock(t *testing.T, inst instrument.ID) []float64 {
	t.Helper()
	d, ok := audio.InstrumentDoc(inst)
	if !ok {
		t.Fatalf("no instrument document for %q", inst)
	}
	return instrument.Flatten(d, audio.BuiltinModuleRegistry())
}

// A library holding a Saved version from a previous session; the project copy is a fresh pick of it.
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

// The user's Save trumps the file's document, and Reset returns exactly what the file carried.
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

// Into a session whose library holds a different saved version: the session's Save plays, and Reset restores the
// export.
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

// An unedited instrument into a session with a Saved version: the Saved version plays; Reset returns to the file's.
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

// A document edit and a Sampler edit travel together; the Sampler edit, not part of the document, survives either way.
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

// The oscillator switch, a stage off and an enum value survive.
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

// On a clean machine the tone is reproduced and the re-export is byte-identical.
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
