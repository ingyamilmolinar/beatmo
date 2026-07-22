//go:build test

package userprefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstrumentStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewInstrumentStore(Options{Path: dir})

	if err := s.SaveUserRecipe("user.kick.abc", []byte(`{"id":"user.kick.abc"}`)); err != nil {
		t.Fatalf("save recipe: %v", err)
	}
	if err := s.SaveRecipeOverride("drum-snare", map[string]float64{"tune": 0.4}); err != nil {
		t.Fatalf("save override: %v", err)
	}
	if err := s.SaveSampleEdit("kick", map[string]float64{"gain_db": -3}); err != nil {
		t.Fatalf("save sample edit: %v", err)
	}
	if err := s.SaveWavRegistry([]byte(`{"kick.wav":[{"version":1,"sha256":"h"}]}`)); err != nil {
		t.Fatalf("save wav registry: %v", err)
	}

	// Reload from disk in a fresh instance.
	s2 := NewInstrumentStore(Options{Path: dir})
	recipes, _ := s2.LoadUserRecipes()
	// Content round-trips (the store may re-indent the opaque JSON; recipes are
	// re-parsed on load, so compare semantically, not byte-for-byte).
	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recipes["user.kick.abc"], &got); err != nil || got.ID != "user.kick.abc" {
		t.Errorf("user recipe not persisted: %q err=%v", recipes["user.kick.abc"], err)
	}
	ov, _ := s2.LoadRecipeOverrides()
	if ov["drum-snare"]["tune"] != 0.4 {
		t.Errorf("override not persisted: %v", ov)
	}
	se, _ := s2.LoadSampleEdits()
	if se["kick"]["gain_db"] != -3 {
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

func TestInstrumentStoreDeleteAndClear(t *testing.T) {
	dir := t.TempDir()
	s := NewInstrumentStore(Options{Path: dir})
	_ = s.SaveUserRecipe("a", []byte(`{}`))
	_ = s.DeleteUserRecipe("a")
	if r, _ := s.LoadUserRecipes(); len(r) != 0 {
		t.Errorf("delete left %d recipes", len(r))
	}
	// Empty params clears an override.
	_ = s.SaveRecipeOverride("x", map[string]float64{"a": 1})
	_ = s.SaveRecipeOverride("x", nil)
	if o, _ := s.LoadRecipeOverrides(); len(o) != 0 {
		t.Errorf("empty override not cleared: %v", o)
	}
}
