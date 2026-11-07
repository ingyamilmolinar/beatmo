package audio

import (
	"errors"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

type memLibrary struct {
	docs  map[instrument.ID][]byte
	blob  []byte
	saved bool
}

func (m *memLibrary) LoadLibrary() ([]byte, error)                          { return m.blob, nil }
func (m *memLibrary) LoadInstrumentDocs() (map[instrument.ID][]byte, error) { return m.docs, nil }
func (m *memLibrary) SaveLibrary(b []byte) error {
	m.blob, m.saved = append([]byte(nil), b...), true
	return nil
}

func resetLibraryForTest(t *testing.T) *memLibrary {
	t.Helper()
	lib := &memLibrary{}
	SetInstrumentLibrarySink(lib)
	if err := LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	ResetInstrumentDocs()
	t.Cleanup(func() {
		SetInstrumentLibrarySink(nil)
		_ = LoadUserInstrumentLibrary(&memLibrary{}, nil)
		ResetInstrumentDocs()
	})
	return lib
}

func TestStoreStartsWithLibraryCopies(t *testing.T) {
	resetLibraryForTest(t)
	d, ok := InstrumentDoc(roster.KickSnap)
	if !ok || d.Origin != roster.KickSnap {
		t.Fatalf("store copy: %+v %v", d.Origin, ok)
	}
	inst, ok := EnsureInstrumentDoc("voice-ahh--2")
	if !ok || inst.ID != "voice-ahh--2" || inst.Origin != roster.VoiceAhh {
		t.Fatalf("instance copy: id=%s origin=%s ok=%v", inst.ID, inst.Origin, ok)
	}
}

func TestSetInstrumentDocValidatesAndBumpsRev(t *testing.T) {
	resetLibraryForTest(t)
	d, _ := InstrumentDoc(roster.KickSnap)
	rev := InstrumentDocRev(roster.KickSnap)
	bad := d.Clone()
	delete(bad.Stages, "core")
	if err := SetInstrumentDoc(roster.KickSnap, bad); err == nil {
		t.Fatal("an invalid document must be refused")
	}
	d2, _ := instrument.SetField(d, BuiltinModuleRegistry(), "gen1_kick_click", 0.9)
	if err := SetInstrumentDoc(roster.KickSnap, d2); err != nil || InstrumentDocRev(roster.KickSnap) == rev {
		t.Fatalf("set: %v", err)
	}
}

// Review Focus 4 (boot store half).
func TestUserLibrarySkipsInvalidDocument(t *testing.T) {
	good, _ := ShippedDoc(roster.KickSnap)
	good.ID, good.Name = "user.snap.abc123", "My Snap"
	gb, _ := instrument.MarshalFile(good)
	lib := &memLibrary{docs: map[instrument.ID][]byte{
		"user.snap.abc123": gb,
		"user.bad.000000":  []byte(`{"schema_version":"0.0.1","id":"user.bad.000000","stages":{}}`),
	}}
	var warned []string
	restore := setLibraryWarnfForTest(func(f string, a ...any) { warned = append(warned, f) })
	defer restore()
	if err := LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = LoadUserInstrumentLibrary(&memLibrary{}, nil) }()
	if _, ok := UserLibraryDoc("user.snap.abc123"); !ok {
		t.Fatal("the valid entry must load")
	}
	if _, ok := UserLibraryDoc("user.bad.000000"); ok || len(warned) != 1 || !strings.Contains(warned[0], "skipped") {
		t.Fatalf("the invalid entry must be skipped with one WARN: %v", warned)
	}
}

func TestSaveLibraryDocShadowsShipped(t *testing.T) {
	lib := resetLibraryForTest(t)
	d, _ := ShippedDoc(roster.KickSnap)
	d, _ = instrument.SetField(d, BuiltinModuleRegistry(), "gen1_kick_click", 0.9)
	if err := SaveLibraryDoc(d); err != nil {
		t.Fatal(err)
	}
	got, _ := LibraryDoc(roster.KickSnap)
	if got.Layers[0].Values["kick_click"] != 0.9 || !lib.saved {
		t.Fatal("a saved document shadows the shipped one and persists")
	}
	if err := DeleteUserLibraryDoc(roster.KickSnap); err != nil {
		t.Fatal(err)
	}
	if got, _ := LibraryDoc(roster.KickSnap); got.Layers[0].Values["kick_click"] == 0.9 {
		t.Fatal("Restore shipped must bring the shipped document back")
	}
	if err := DeleteUserLibraryDoc(roster.KickSnap); !errors.Is(err, ErrNoUserLibraryDoc) {
		t.Fatalf("want ErrNoUserLibraryDoc, got %v", err)
	}
}

func TestLibraryIDsListsShippedThenUserOnly(t *testing.T) {
	resetLibraryForTest(t)
	d, _ := ShippedDoc(roster.KickSnap)
	d.ID, d.Name = "user.zzz.000001", "Mine"
	if err := SaveLibraryDoc(d); err != nil {
		t.Fatal(err)
	}
	ids := LibraryIDs()
	if ids[0] != roster.All[0] || ids[len(ids)-1] != "user.zzz.000001" || len(ids) != len(roster.All)+1 {
		t.Fatalf("LibraryIDs tail = %v (len %d)", ids[len(ids)-3:], len(ids))
	}
}

// audio.js has no render table: it renders exactly what Go pushes.
func TestEveryShippedInstrumentIsPushedToThePlatform(t *testing.T) {
	pushed := map[string]bool{}
	prev := platformInstrumentDocChanged
	platformInstrumentDocChanged = func(id instrument.ID, d instrument.Doc) { pushed[string(id)] = true }
	t.Cleanup(func() { platformInstrumentDocChanged = prev; ResetInstrumentDocs() })
	ResetInstrumentDocs()
	for _, id := range roster.All {
		if !pushed[string(id)] {
			t.Errorf("%s was never pushed to the platform — it would be silent in the browser", id)
		}
	}
}

// A push re-renders that voice in the browser; re-pushing all of them made one undo re-render the roster.
func TestReplaceInstrumentDocsTouchesOnlyWhatChanged(t *testing.T) {
	resetLibraryForTest(t)
	edited, _ := InstrumentDoc(roster.Kick)
	edited, err := instrument.SetField(edited, BuiltinModuleRegistry(), "post_decay", 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if err := SetInstrumentDoc(roster.Kick, edited); err != nil {
		t.Fatal(err)
	}
	SetInstrumentParam(roster.Hihat, "post_decay", 0.3) // a stale edit the file does not carry

	pushed := map[string]int{}
	prev := platformInstrumentDocChanged
	platformInstrumentDocChanged = func(id instrument.ID, d instrument.Doc) { pushed[string(id)]++ }
	t.Cleanup(func() { platformInstrumentDocChanged = prev })

	// The file carries kick (as it already is) and snare (edited).
	snare, _ := InstrumentDoc(roster.Snare)
	snare, _ = instrument.SetField(snare, BuiltinModuleRegistry(), "post_decay", 0.7)
	if err := ReplaceInstrumentDocs(map[instrument.ID]instrument.Doc{roster.Kick: edited, roster.Snare: snare}); err != nil {
		t.Fatal(err)
	}
	if len(pushed) != 2 || pushed["snare"] != 1 || pushed["hihat"] != 1 {
		t.Fatalf("pushed %v, want exactly snare (file edit) and hihat (back to its library entry)", pushed)
	}
	if InstrumentDocModified(roster.Hihat) {
		t.Error("hihat kept the previous session's edit")
	}
	if d, _ := InstrumentDoc(roster.Kick); instrument.FieldValues(d, BuiltinModuleRegistry())["post_decay"] != 0.9 {
		t.Error("kick lost the file's value")
	}

	// An unknown id (a Save-As instrument from another library) becomes playable.
	alien := snare.Clone()
	alien.ID, alien.Origin = "user.alien.abc123", "user.alien.abc123"
	if err := ReplaceInstrumentDocs(map[instrument.ID]instrument.Doc{"user.alien.abc123": alien}); err != nil {
		t.Fatal(err)
	}
	if !HasInstrumentDoc("user.alien.abc123") || !instanceAlreadyRegistered("user.alien.abc123") {
		t.Fatal("a project-only document must be stored and playable")
	}
}

func TestIsMelodicInstrumentReadsTheDocument(t *testing.T) {
	resetLibraryForTest(t)
	for _, id := range roster.All {
		d, _ := ShippedDoc(id)
		if got := IsMelodicInstrument(id); got != d.Render.Melodic {
			t.Errorf("IsMelodicInstrument(%q) = %v, document says %v", id, got, d.Render.Melodic)
		}
	}
	if !IsMelodicInstrument("piano-grand--3") {
		t.Error("an instance id follows its base (piano-grand is melodic)")
	}
	if IsMelodicInstrument(roster.Kick) || IsMelodicInstrument("no-such-instrument") {
		t.Error("kick / an unknown id are not melodic")
	}
}

func TestFlatFieldMapCoversEveryABIField(t *testing.T) {
	resetLibraryForTest(t)
	d, _ := InstrumentDoc(roster.Kick)
	m := FlatFieldMap(d)
	b := instrument.Flatten(d, BuiltinModuleRegistry())
	if len(m) != len(b) || len(m) != len(modularParamSchema) {
		t.Fatalf("map has %d fields, block %d, schema %d", len(m), len(b), len(modularParamSchema))
	}
	for i, name := range modularParamSchema {
		if m[name] != b[i] {
			t.Fatalf("%s = %v, block says %v", name, m[name], b[i])
		}
	}
}

// The layer's fields that sort before gen9_source are inactive until the source creates the layer; a single sorted
// pass dropped them.
func TestInstrumentDocWithFieldsAddsALayerWithAllItsFields(t *testing.T) {
	resetLibraryForTest(t)
	fields := map[string]float64{
		"gen9_source": 1, "gen9_freq": 3.4527, "gen9_gain": 0.14,
		"gen9_env_fast_mix": 1, "gen9_env_fast_rate": 0.34,
	}
	d, err := InstrumentDocWithFields(roster.TubularBells, fields)
	if err != nil {
		t.Fatal(err)
	}
	got := instrument.FieldValues(d, BuiltinModuleRegistry())
	for k, want := range fields {
		if got[k] != want {
			t.Errorf("%s = %v, want %v (the layer field was dropped)", k, got[k], want)
		}
	}
	d2, skipped, err := ApplyInstrumentFields(d, map[string]float64{"gen9_env_fast_rate": 0.5})
	if err != nil || len(skipped) != 0 {
		t.Fatalf("re-apply on the extended doc: skipped=%v err=%v", skipped, err)
	}
	if v := instrument.FieldValues(d2, BuiltinModuleRegistry())["gen9_env_fast_rate"]; v != 0.5 {
		t.Errorf("gen9_env_fast_rate = %v, want 0.5", v)
	}
}

// The bulk writer (JS bridge, offline renders) has the same layer-adding case.
func TestSetInstrumentParamsAddsALayerWithAllItsFields(t *testing.T) {
	resetLibraryForTest(t)
	fields := RecipeParams{"gen9_source": 1, "gen9_freq": 3.4527, "gen9_gain": 0.14}
	SetInstrumentParams(roster.TubularBells, fields)
	got := GetInstrumentParams(roster.TubularBells)
	for k, want := range fields {
		if got[k] != want {
			t.Errorf("%s = %v, want %v (the layer field was dropped)", k, got[k], want)
		}
	}
}
