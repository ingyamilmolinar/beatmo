//go:build !js

package userprefs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func poolPresent(name string) bool {
	_, ok := async.DefaultRegistry().Stats()[name]
	return ok
}

func releaseAsyncPool(name string) error {
	return async.DefaultRegistry().Release(name)
}

// Prefs-document tests that span sections: one section's save never stomps
// another, a corrupt file loads empty, the marshal is deterministic, the
// writer pool is lazy, and the retired recipe sections are dropped.

func TestPrefs_FavoritesPreservedAcrossSectionSaves(t *testing.T) {
	// Saving a sample edit must not stomp the favorites section, and
	// vice-versa — the doc carries every section in one write.
	s, se, _ := newTestSampleEditStore(t)
	if err := s.SaveFavorites(map[instrument.ID]bool{roster.Kick: true, roster.Snare: true}); err != nil {
		t.Fatalf("SaveFavorites: %v", err)
	}
	if err := se.SaveSampleEdit(roster.Kick, map[string]float64{"gain_db": -3}); err != nil {
		t.Fatalf("SaveSampleEdit: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	favs, _ := s.LoadFavorites()
	if !favs[roster.Kick] || !favs[roster.Snare] {
		t.Errorf("favorites lost across a sample-edit save: got %v", favs)
	}
	edits, _ := se.LoadSampleEdits()
	if edits[roster.Kick]["gain_db"] != -3 {
		t.Errorf("sample edit lost: %v", edits)
	}
}

func TestPrefs_LoadCorruptReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{this is not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })
	favs, _ := s.LoadFavorites()
	if len(favs) != 0 {
		t.Errorf("corrupt file should yield empty favorites: %v", favs)
	}
	edits, _ := s.(SampleEditStore).LoadSampleEdits()
	if len(edits) != 0 {
		t.Errorf("corrupt file should yield empty sample edits: %v", edits)
	}
	// File preserved for operator recovery.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("corrupt file removed: %v", err)
	}
}

func TestPrefs_MarshalIsDeterministic(t *testing.T) {
	// Sorted output is required for stable on-disk diffs. We materialise
	// the same inputs twice through the marshaler and assert byte equality.
	snap := prefsSnapshot{
		favorites: map[instrument.ID]bool{"zeta": true, "alpha": true, "mike": true},
		sampleEdits: map[instrument.ID]map[string]float64{
			roster.Kick:  {"gain_db": -3, "start": 0.25},
			roster.Snare: {"reverse": 1},
		},
	}
	a, err := marshalPrefs(snap)
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	b, err := marshalPrefs(snap)
	if err != nil {
		t.Fatalf("marshal b: %v", err)
	}
	if string(a) != string(b) {
		t.Errorf("marshalPrefs is non-deterministic:\nA=%s\nB=%s", a, b)
	}

	// Favorites must serialise sorted.
	var parsed prefsDoc
	if err := json.Unmarshal(a, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	wantFavs := []string{"alpha", "mike", "zeta"}
	for i, id := range wantFavs {
		if parsed.Favorites[i] != id {
			t.Errorf("favorites[%d] = %q want %q (full=%v)", i, parsed.Favorites[i], id, parsed.Favorites)
		}
	}
}

func TestPrefs_PoolLazyAcquired(t *testing.T) {
	// A load-only session must not start the persist worker: the pool is acquired on the first write.
	name := PoolName + ".test." + t.Name()
	t.Cleanup(func() { _ = releaseAsyncPool(name) })

	dir := t.TempDir()
	s := NewBackingStore(Options{
		Path:     filepath.Join(dir, FileName),
		PoolName: name,
	})
	t.Cleanup(func() { _ = s.Close() })

	if _, err := s.LoadFavorites(); err != nil {
		t.Fatalf("LoadFavorites: %v", err)
	}
	se := s.(SampleEditStore)
	if _, err := se.LoadSampleEdits(); err != nil {
		t.Fatalf("LoadSampleEdits: %v", err)
	}

	if poolPresent(name) {
		t.Fatalf("pool %q allocated by read-only operations; expected lazy on first write", name)
	}

	if err := se.SaveSampleEdit(roster.Snare, map[string]float64{"gain_db": 1.5}); err != nil {
		t.Fatalf("SaveSampleEdit: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	if !poolPresent(name) {
		t.Fatalf("pool %q not allocated after first write", name)
	}
}

// Other sections still load, and the next write drops the retired ones: instruments are documents in the instrument
// store, with no migration.
func TestPrefs_RetiredRecipeSectionsAreDropped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	legacy := `{"schema_version":"0.0.1","favorites":["kick"],` +
		`"recipe_overrides":{"synth-modular-snare":{"decay":1.5}},` +
		`"user_recipes":{"user.snare.fat":{"id":"user.snare.fat"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := NewBackingStore(Options{Path: path, PoolName: uniquePoolName(t), Logger: t.Logf})
	t.Cleanup(func() { _ = s.Close() })

	favs, err := s.LoadFavorites()
	if err != nil || !favs[roster.Kick] {
		t.Fatalf("favorites of a recipe-era file not loaded: %v (err %v)", favs, err)
	}
	if err := s.SaveFavorites(map[instrument.ID]bool{roster.Kick: true, roster.Snare: true}); err != nil {
		t.Fatalf("SaveFavorites: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(raw), "recipe_overrides") || strings.Contains(string(raw), "user_recipes") {
		t.Errorf("the next write kept a retired recipe section:\n%s", raw)
	}
}
