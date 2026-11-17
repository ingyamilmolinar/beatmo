//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestReproSynthConfigRoundtrip reproduces "synth config edits on an instrument
// don't persist/load": edit the knobs the way the Synth tab does, export, wipe
// to a clean machine, import — the edited document must come back.
func TestReproSynthConfigRoundtrip(t *testing.T) {
	g := newDocsGame(t)
	inst := g.drum.Rows[0].Instrument

	audio.SetInstrumentParam(inst, "drive", 0.66)
	audio.SetInstrumentParam(inst, "pitch", 4)
	if math.Abs(docField(t, string(inst), "drive")-0.66) > 1e-6 {
		t.Fatalf("precondition: drive not stored, got %v", docField(t, string(inst), "drive"))
	}
	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	wipeToCleanLibrary(t)
	if audio.InstrumentDocModified(inst) {
		t.Fatal("precondition: the wipe left the edit in place")
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := docField(t, string(inst), "drive"); math.Abs(got-0.66) > 1e-6 {
		t.Errorf("drive not restored: got %v want 0.66", got)
	}
	if got := docField(t, string(inst), "pitch"); math.Abs(got-4) > 1e-6 {
		t.Errorf("pitch not restored: got %v want 4", got)
	}
}

// TestReproSynthSaveRoundtrip: a Synth-tab Save moves the tone into the user
// library; the project file alone must still reproduce it on a clean machine
// (whose library has never seen the Save).
func TestReproSynthSaveRoundtrip(t *testing.T) {
	g := newDocsGame(t)
	inst := g.drum.Rows[0].Instrument

	audio.SetInstrumentParam(inst, "drive", 0.7)
	if saveRecipeForInstrument(inst) == "" {
		t.Fatalf("Save of %q saved nothing", inst)
	}
	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	wipeToCleanLibrary(t)
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := docField(t, string(inst), "drive"); math.Abs(got-0.7) > 1e-6 {
		t.Errorf("Synth Save tone LOST on round-trip: drive=%v want 0.7", got)
	}
}

// TestReproRecipeDefaultOnlyRoundtrip: the user library holds a saved version
// and the project copy is a fresh, UNEDITED pick of it (a prior Save + restart).
// The file must still carry the saved tone to a clean machine.
func TestReproRecipeDefaultOnlyRoundtrip(t *testing.T) {
	g := newDocsGame(t)
	inst := g.drum.Rows[0].Instrument
	lib, ok := audio.LibraryDoc(inst)
	if !ok {
		t.Fatalf("row0 instrument %q has no library document", inst)
	}
	want := instrument.FieldValues(lib, audio.BuiltinModuleRegistry())["drive"] + 0.4
	saved, err := instrument.SetField(lib, audio.BuiltinModuleRegistry(), "drive", want)
	if err != nil {
		t.Fatal(err)
	}
	if err := audio.SaveLibraryDoc(saved); err != nil {
		t.Fatal(err)
	}
	audio.ResetInstrumentDocs() // the project copy is a fresh pick of the saved entry
	if audio.InstrumentDocModified(inst) || math.Abs(docField(t, string(inst), "drive")-want) > 1e-6 {
		t.Fatalf("precondition: fresh copy of the saved entry, drive=%v", docField(t, string(inst), "drive"))
	}
	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	wipeToCleanLibrary(t)
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := docField(t, string(inst), "drive"); math.Abs(got-want) > 1e-6 {
		t.Errorf("Saved tone LOST on round-trip (unedited copy): drive=%v want %v", got, want)
	}
}

// TestReproSamplerSaveRoundtrip reproduces the Sampler-tab "Save" round-trip:
// save() bakes the current edit (gain/trim/normalize/etc.) into a final PCM and
// stores it via audio.SaveUserSample → userSamples, which the exporter embeds.
// A brand-new instance must reproduce the baked sample from the project bytes.
func TestReproSamplerSaveRoundtrip(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument

	// Emulate a sampler edit + Save: bake a buffer and store it for the row's
	// instrument (audio.SaveUserSample is exactly what samplerState.save calls).
	raw := make([]float32, 2000)
	for i := range raw {
		raw[i] = float32(math.Sin(float64(i) * 0.05))
	}
	baked := audio.BakeSample(raw, 48000, audio.SampleEdit{
		StartFrac: 0.1, EndFrac: 0.9, GainDB: -6, Normalize: true, Fade: true,
	})
	audio.SaveUserSample(inst, baked, 48000)
	if rec, ok := audio.UserSamplePCM(inst); !ok || len(rec.PCM) != len(baked) {
		t.Fatalf("precondition: baked sample not stored (ok=%v len=%d want %d)", ok, len(rec.PCM), len(baked))
	}

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Simulate a fresh instance: overwrite the in-memory PCM with a different
	// buffer so a passing assertion can only come from the project bytes.
	audio.PutUserSample(inst, []float32{0, 0, 0, 0}, 48000)

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	rec, ok := audio.UserSamplePCM(inst)
	t.Logf("after import: sample present=%v frames=%d (baked=%d)", ok, len(rec.PCM), len(baked))
	if !ok || len(rec.PCM) != len(baked) {
		t.Fatalf("sampler edit LOST on round-trip: present=%v frames=%d want %d", ok, len(rec.PCM), len(baked))
	}
	// Content must match the baked buffer (proves the edited audio survived).
	var maxDiff float64
	for i := range baked {
		if d := math.Abs(float64(rec.PCM[i] - baked[i])); d > maxDiff {
			maxDiff = d
		}
	}
	if maxDiff > 1e-6 {
		t.Errorf("sampler PCM content diverged after round-trip: maxDiff=%v", maxDiff)
	}
}
