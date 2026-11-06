package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/objects"
)

// memInstrumentLibrary is an in-memory instrument store. docs is, before any
// save, the pre-versioning documents the library reads in; after a save, each
// instrument's CURRENT saved document as file bytes (what a test inspects).
type memInstrumentLibrary struct {
	docs map[instrument.ID][]byte
	blob []byte
	fail error // set: every save fails with it (a full browser storage)
}

func (m *memInstrumentLibrary) LoadLibrary() ([]byte, error) { return m.blob, nil }
func (m *memInstrumentLibrary) LoadInstrumentDocs() (map[instrument.ID][]byte, error) {
	return m.docs, nil
}
func (m *memInstrumentLibrary) SaveLibrary(b []byte) error {
	if m.fail != nil {
		return m.fail
	}
	m.blob = append([]byte(nil), b...)
	m.docs = currentLibraryDocs(b)
	return nil
}

// currentLibraryDocs decodes a persisted library into each identity's current
// document, as file bytes.
func currentLibraryDocs(blob []byte) map[instrument.ID][]byte {
	var s objects.Store
	if json.Unmarshal(blob, &s) != nil {
		return nil
	}
	out := map[instrument.ID][]byte{}
	for identity := range s.Histories {
		cur, ok := s.Current(identity)
		if !ok {
			continue
		}
		var v audio.InstrumentVersion
		if s.Body(cur, &v) != nil || v.Doc == nil {
			continue
		}
		d := v.Doc.Clone()
		d.ID = instrument.ID(identity)
		if b, err := instrument.MarshalFile(d); err == nil {
			out[d.ID] = b
		}
	}
	return out
}

// withPrivateInstrumentLibrary gives the test an empty, private user instrument
// library (so a Save never reaches the developer's real instruments.json) and
// restores an empty one afterwards.
func withPrivateInstrumentLibrary(t *testing.T) *memInstrumentLibrary {
	t.Helper()
	lib := &memInstrumentLibrary{}
	audio.SetInstrumentLibrarySink(lib)
	if err := audio.LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		audio.SetInstrumentLibrarySink(nil)
		_ = audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{}, nil)
		audio.ResetInstrumentDocs()
	})
	return lib
}

// docField reads one flat field of instID's project document.
func docField(t *testing.T, instID, field string) float64 {
	t.Helper()
	d, ok := audio.InstrumentDoc(instrument.ID(instID))
	if !ok {
		t.Fatalf("no instrument document for %q", instID)
	}
	return instrument.FieldValues(d, audio.BuiltinModuleRegistry())[field]
}

// wipeToCleanLibrary empties the user library and re-initialises every
// project copy from the shipped library — a brand-new install, where only the
// project file can bring a tone back.
func wipeToCleanLibrary(t *testing.T) {
	t.Helper()
	if err := audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{}, nil); err != nil {
		t.Fatal(err)
	}
	audio.ResetInstrumentDocs()
}

// newDocsGame is a Game with a private, empty user instrument library and the
// given instruments on rows 0..n-1, picked through SetInstrument (the real
// pick path). Shared by the library / project-document / template tests.
func newDocsGame(t *testing.T, instruments ...instrument.ID) *Game {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultAudio(t)
	withPrivateInstrumentLibrary(t)
	audio.ResetInstrumentDocs()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	for len(g.drum.Rows) < len(instruments) {
		g.drum.AddRow()
	}
	for i, inst := range instruments {
		g.drum.selRow = i
		g.drum.SetInstrument(inst)
	}
	return g
}

// Save writes the library entry; an existing project copy keeps its values.
func TestSaveChangesFuturePicksOnly(t *testing.T) {
	newDocsGame(t, roster.VoiceAhh)
	audio.SetInstrumentParam(roster.VoiceAhh, "amp_attack", 0.5)
	if _, ok := audio.EnsureInstrumentDoc("voice-ahh--2"); !ok {
		t.Fatal("instance copy")
	}
	saveRecipeForInstrument(roster.VoiceAhh)
	lib, _ := audio.LibraryDoc(roster.VoiceAhh)
	if lib.Stages["amp_env"].Values["amp_attack"] != 0.5 {
		t.Fatalf("Save must write the library entry: %+v", lib.Stages["amp_env"])
	}
	inst, _ := audio.InstrumentDoc("voice-ahh--2")
	if inst.Stages["amp_env"].Values["amp_attack"] == 0.5 {
		t.Fatal("an existing project copy must keep its own values")
	}
	if audio.InstrumentDocModified(roster.VoiceAhh) {
		t.Fatal("after Save the copy equals its library entry")
	}
}

func TestSaveAsRepointsOriginAndRegisters(t *testing.T) {
	g := newDocsGame(t, roster.KickSnap)
	audio.SetInstrumentParam(roster.KickSnap, "gen1_kick_click", 0.9)
	g.drum.SaveActiveRecipeAsFor(roster.KickSnap, "My Snap")
	d, _ := audio.InstrumentDoc(roster.KickSnap)
	lib, ok := audio.LibraryDoc(d.Origin)
	if !ok || lib.Name != "My Snap" || len(d.Origin) < 5 || d.Origin[:5] != "user." || audio.InstrumentDocModified(roster.KickSnap) {
		t.Fatalf("origin=%s lib=%q ok=%v", d.Origin, lib.Name, ok)
	}
	if _, ok := audio.InstrumentDoc(d.Origin); !ok {
		t.Fatal("the new library instrument must be pickable (store copy)")
	}
}

// TestRepickingAnUnusedInstrumentGetsTheLibraryVersion: edits made to an
// instrument stay with the rows using it; once no row uses it, picking it again
// starts from the library entry (spec §6: a pick = a copy of the library).
func TestRepickingAnUnusedInstrumentGetsTheLibraryVersion(t *testing.T) {
	g := newDocsGame(t, roster.KickSnap, roster.Snare)
	audio.SetInstrumentParam(roster.KickSnap, "gen1_kick_click", 0.9)

	// Another row picking the same instrument shares the edited copy.
	g.drum.selRow = 1
	g.drum.SetInstrument(roster.KickSnap)
	if !audio.InstrumentDocModified(roster.KickSnap) {
		t.Fatal("a second row picking an instrument in use must keep the edits")
	}

	// Move both rows off it, then pick it again: the library version.
	g.drum.selRow = 0
	g.drum.SetInstrument(roster.Snare)
	g.drum.selRow = 1
	g.drum.SetInstrument(roster.Snare)
	g.drum.SetInstrument(roster.KickSnap)
	if audio.InstrumentDocModified(roster.KickSnap) {
		t.Fatal("re-picking an instrument no row used must start from its library entry")
	}
}

// instSchemaForTest is what the Synth tab feeds buildMacroGroups for a library
// instrument id: its ParamDefs and a private copy of its flat values.
func instSchemaForTest(t testing.TB, id instrument.ID) ([]audio.ParamDef, audio.RecipeParams) {
	t.Helper()
	defs := audio.InstrumentParamDefs(id)
	if len(defs) == 0 {
		t.Fatalf("instrument %q has no document", id)
	}
	merged := audio.RecipeParams{}
	for k, v := range audio.MergedInstrumentParamsRO(id) {
		merged[k] = v
	}
	return defs, merged
}

// keepInstrumentDoc snapshots id's project document and restores it (or its
// absence) on cleanup.
func keepInstrumentDoc(t testing.TB, id instrument.ID) {
	t.Helper()
	d, had := audio.InstrumentDoc(id)
	t.Cleanup(func() {
		if had {
			_ = audio.SetInstrumentDoc(id, d)
		} else {
			audio.RemoveInstrumentDocForTest(id)
		}
	})
}

// assertSynthDocKept fails unless id still plays a synth document copied from
// library entry origin — the synth stays the instrument's source of truth.
func assertSynthDocKept(t testing.TB, id, origin, when string) {
	t.Helper()
	d, ok := audio.InstrumentDoc(instrument.ID(id))
	if !ok || d.Origin != instrument.ID(origin) {
		t.Errorf("%s: %s document present=%v origin=%q, want the synth document from %q", when, id, ok, d.Origin, origin)
	}
}
