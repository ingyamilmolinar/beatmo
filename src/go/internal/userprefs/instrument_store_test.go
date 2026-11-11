//go:build test

package userprefs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestInstrumentStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewInstrumentStore(Options{Path: dir})

	if err := s.SaveLibrary([]byte(`{"objects":{},"histories":{}}`)); err != nil {
		t.Fatalf("save library: %v", err)
	}
	if err := s.SaveSampleEdit(roster.Kick, map[string]float64{"gain_db": -3}); err != nil {
		t.Fatalf("save sample edit: %v", err)
	}
	if err := s.SaveWavRegistry([]byte(`{"kick.wav":[{"version":1,"sha256":"h"}]}`)); err != nil {
		t.Fatalf("save wav registry: %v", err)
	}

	// Reload from disk in a fresh instance.
	s2 := NewInstrumentStore(Options{Path: dir})
	lib, _ := s2.LoadLibrary()
	// Content round-trips (the store may re-indent the opaque JSON).
	var got map[string]json.RawMessage
	if err := json.Unmarshal(lib, &got); err != nil || got["histories"] == nil {
		t.Errorf("library not persisted: %q err=%v", lib, err)
	}
	se, _ := s2.LoadSampleEdits()
	if se[roster.Kick]["gain_db"] != -3 {
		t.Errorf("sample edit not persisted: %v", se)
	}
	reg, _ := s2.LoadWavRegistry()
	if len(reg) == 0 {
		t.Errorf("wav registry not persisted")
	}

	// Decoupling: the instrument store writes its OWN file, never prefs.json.
	if _, err := os.Stat(filepath.Join(dir, InstrumentsFileName)); err != nil {
		t.Errorf("instruments file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Errorf("instrument store wrote the prefs file %q (coupling!)", FileName)
	}
}

// TestInstrumentStoreWarnsOnLegacyRecipeSections: a file from the recipe era
// loads (its documents and sample edits survive) with ONE warning naming the
// ignored sections, and the next write drops them.
func TestInstrumentStoreWarnsOnLegacyRecipeSections(t *testing.T) {
	dir := t.TempDir()
	legacy := `{"schema_version":"0.0.1","user_recipes":{"user.kick.abc":{"id":"user.kick.abc"}},` +
		`"recipe_overrides":{"synth-modular-snare":{"tune":0.4}},"sample_edits":{"kick":{"gain_db":-3}}}`
	if err := os.WriteFile(filepath.Join(dir, InstrumentsFileName), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	var warns []string
	s := NewInstrumentStore(Options{Path: dir, Logger: func(f string, a ...any) { warns = append(warns, fmt.Sprintf(f, a...)) }})
	se, _ := s.LoadSampleEdits()
	if se[roster.Kick]["gain_db"] != -3 {
		t.Errorf("sample edits lost next to legacy sections: %v", se)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "user_recipes") {
		t.Fatalf("want one warning naming the legacy sections, got %q", warns)
	}
	if err := s.SaveLibrary([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, InstrumentsFileName))
	if strings.Contains(string(raw), "user_recipes") || strings.Contains(string(raw), "recipe_overrides") {
		t.Errorf("the next write kept the legacy sections:\n%s", raw)
	}
}

// TestInstrumentStoreKeepsDocumentsSavedBeforeVersioning: the documents saved
// before the versioned library are read into it once and never deleted — a
// library write keeps them as they were.
func TestInstrumentStoreKeepsDocumentsSavedBeforeVersioning(t *testing.T) {
	dir := t.TempDir()
	old := `{"schema_version":"0.0.1","instruments":{"user.snap.abc123":{"id":"user.snap.abc123"}}}`
	if err := os.WriteFile(filepath.Join(dir, InstrumentsFileName), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := NewInstrumentStore(Options{Path: dir}).SaveLibrary([]byte(`{"histories":{}}`)); err != nil {
		t.Fatal(err)
	}
	s := NewInstrumentStore(Options{Path: dir})
	docs, err := s.LoadInstrumentDocs()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(docs["user.snap.abc123"], &got); err != nil || got.ID != "user.snap.abc123" || len(docs) != 1 {
		t.Fatalf("a library write lost the documents saved before it: %q (err %v)", docs, err)
	}
	if lib, _ := s.LoadLibrary(); len(lib) == 0 {
		t.Fatal("the library was not persisted")
	}
}

type failingBackend struct{}

func (failingBackend) read() ([]byte, error) { return nil, nil }
func (failingBackend) write([]byte) error  { return fmt.Errorf("disk full") }

// A save that cannot be stored reports it and leaves the store as it was, so
// the UI's KeySaveFailed fires instead of a success the next launch disproves.
func TestSavesThatCannotBeStoredReturnTheirErrorAndChangeNothing(t *testing.T) {
	s := &instrumentStore{backend: failingBackend{}}
	if err := s.SaveSampleEdit(roster.Kick, map[string]float64{"gain_db": -3}); err == nil {
		t.Error("SaveSampleEdit reported success for a write that failed")
	}
	if se, _ := s.LoadSampleEdits(); len(se) != 0 {
		t.Errorf("a failed SaveSampleEdit still changed the store: %v", se)
	}
	if err := s.SaveWavRegistry([]byte(`{"kick.wav":[]}`)); err == nil {
		t.Error("SaveWavRegistry reported success for a write that failed")
	}
	if raw, _ := s.LoadWavRegistry(); raw != nil {
		t.Errorf("a failed SaveWavRegistry still changed the store: %s", raw)
	}
}
