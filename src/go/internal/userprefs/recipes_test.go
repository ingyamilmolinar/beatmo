//go:build !js

package userprefs

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

func poolPresent(name string) bool {
	_, ok := async.DefaultRegistry().Stats()[name]
	return ok
}

func releaseAsyncPool(name string) error {
	return async.DefaultRegistry().Release(name)
}

// Recipe-store tests: cover the RecipeStore surface (override + user-recipe
// persistence), NaN/Inf sanitization at the persistence boundary, and the
// deterministic marshal contract.

func newTestRecipeStore(t *testing.T) (Store, RecipeStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })
	rs, ok := s.(RecipeStore)
	if !ok {
		t.Fatalf("backing store does not implement RecipeStore")
	}
	return s, rs, path
}

func TestPrefs_RoundTripOverridesAndUserRecipes(t *testing.T) {
	s, rs, path := newTestRecipeStore(t)

	if err := rs.SaveRecipeOverride("drum-snare", map[string]float64{"decay": 1.7, "tone": -0.4}); err != nil {
		t.Fatalf("SaveRecipeOverride(drum-snare): %v", err)
	}
	if err := rs.SaveRecipeOverride("fm-bass", map[string]float64{"body": 0.8}); err != nil {
		t.Fatalf("SaveRecipeOverride(fm-bass): %v", err)
	}
	if err := rs.SaveUserRecipe("user.snare.fat", []byte(`{"id":"user.snare.fat","display_name":"Fat Snare","base_recipe":"drum-snare","param_defs":[],"origin":"user"}`)); err != nil {
		t.Fatalf("SaveUserRecipe: %v", err)
	}
	if err := s.SaveFavorites(map[string]bool{"kick": true}); err != nil {
		t.Fatalf("SaveFavorites: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("prefs.json not written at %s: %v", path, err)
	}

	// Read back through a fresh store so we exercise the disk → cache load.
	_, rs2, _ := newTestRecipeStore(t)
	rs2.(*fileStore).path = path

	overrides, err := rs2.LoadRecipeOverrides()
	if err != nil {
		t.Fatalf("LoadRecipeOverrides: %v", err)
	}
	if len(overrides) != 2 {
		t.Fatalf("overrides len = %d, want 2 (got %v)", len(overrides), overrides)
	}
	if overrides["drum-snare"]["decay"] != 1.7 || overrides["drum-snare"]["tone"] != -0.4 {
		t.Errorf("drum-snare overrides mismatch: got %v", overrides["drum-snare"])
	}
	if overrides["fm-bass"]["body"] != 0.8 {
		t.Errorf("fm-bass overrides mismatch: got %v", overrides["fm-bass"])
	}

	recipes, err := rs2.LoadUserRecipes()
	if err != nil {
		t.Fatalf("LoadUserRecipes: %v", err)
	}
	if len(recipes) != 1 {
		t.Fatalf("user recipes len = %d, want 1", len(recipes))
	}
	if _, ok := recipes["user.snare.fat"]; !ok {
		t.Errorf("user.snare.fat not in loaded user recipes (got %v)", recipes)
	}
}

func TestPrefs_RejectsNonFiniteOverrideValues(t *testing.T) {
	// NaN/Inf in override values must be silently dropped — they would
	// poison the audio param manager (sanitizeParamValue applies the same
	// rule at the SetInstrumentParam boundary). The persistence layer is
	// a second line of defence.
	_, rs, _ := newTestRecipeStore(t)
	if err := rs.SaveRecipeOverride("drum-snare", map[string]float64{
		"decay": math.NaN(),
		"tone":  math.Inf(1),
		"drive": 0.5,
	}); err != nil {
		t.Fatalf("SaveRecipeOverride: %v", err)
	}
	got, err := rs.LoadRecipeOverrides()
	if err != nil {
		t.Fatalf("LoadRecipeOverrides: %v", err)
	}
	params := got["drum-snare"]
	if _, present := params["decay"]; present {
		t.Errorf("NaN decay survived persistence: %v", params)
	}
	if _, present := params["tone"]; present {
		t.Errorf("Inf tone survived persistence: %v", params)
	}
	if params["drive"] != 0.5 {
		t.Errorf("finite drive value lost: got %v", params)
	}
}

func TestPrefs_DeleteOverrideAndUserRecipe(t *testing.T) {
	_, rs, _ := newTestRecipeStore(t)
	_ = rs.SaveRecipeOverride("drum-snare", map[string]float64{"decay": 1.5})
	_ = rs.SaveUserRecipe("user.test", []byte(`{"id":"user.test"}`))

	if err := rs.DeleteRecipeOverride("drum-snare"); err != nil {
		t.Fatalf("DeleteRecipeOverride: %v", err)
	}
	overrides, _ := rs.LoadRecipeOverrides()
	if _, present := overrides["drum-snare"]; present {
		t.Errorf("drum-snare override survived delete: %v", overrides)
	}

	if err := rs.DeleteUserRecipe("user.test"); err != nil {
		t.Fatalf("DeleteUserRecipe: %v", err)
	}
	recipes, _ := rs.LoadUserRecipes()
	if _, present := recipes["user.test"]; present {
		t.Errorf("user.test recipe survived delete: %v", recipes)
	}

	// Idempotent delete of a missing id is a no-op (no error).
	if err := rs.DeleteRecipeOverride("does-not-exist"); err != nil {
		t.Errorf("DeleteRecipeOverride(missing): %v", err)
	}
	if err := rs.DeleteUserRecipe("does-not-exist"); err != nil {
		t.Errorf("DeleteUserRecipe(missing): %v", err)
	}
}

func TestPrefs_FavoritesPreservedAcrossRecipeSaves(t *testing.T) {
	// Saving an override must not stomp the favorites section, and
	// vice-versa — the doc carries all three sections in one write.
	s, rs, _ := newTestRecipeStore(t)
	if err := s.SaveFavorites(map[string]bool{"kick": true, "snare": true}); err != nil {
		t.Fatalf("SaveFavorites: %v", err)
	}
	if err := rs.SaveRecipeOverride("drum-kick", map[string]float64{"drive": 0.7}); err != nil {
		t.Fatalf("SaveRecipeOverride: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	favs, _ := s.LoadFavorites()
	if !favs["kick"] || !favs["snare"] {
		t.Errorf("favorites lost across recipe save: got %v", favs)
	}
	overrides, _ := rs.LoadRecipeOverrides()
	if overrides["drum-kick"]["drive"] != 0.7 {
		t.Errorf("override lost: %v", overrides)
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
	rs := s.(RecipeStore)
	favs, _ := s.LoadFavorites()
	if len(favs) != 0 {
		t.Errorf("corrupt file should yield empty favorites: %v", favs)
	}
	overrides, _ := rs.LoadRecipeOverrides()
	if len(overrides) != 0 {
		t.Errorf("corrupt file should yield empty overrides: %v", overrides)
	}
	recipes, _ := rs.LoadUserRecipes()
	if len(recipes) != 0 {
		t.Errorf("corrupt file should yield empty user recipes: %v", recipes)
	}
	// File preserved for operator recovery.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("corrupt file removed: %v", err)
	}
}

func TestPrefs_MarshalIsDeterministic(t *testing.T) {
	// Sorted output is required for stable on-disk diffs. We materialise
	// the same inputs twice through the marshaler and assert byte equality.
	favs := map[string]bool{"zeta": true, "alpha": true, "mike": true}
	overrides := map[string]map[string]float64{
		"drum-kick":  {"drive": 0.5, "body": 0.6},
		"drum-snare": {"decay": 1.5},
	}
	userRecipes := map[string][]byte{
		"user.snare.fat": []byte(`{"id":"user.snare.fat"}`),
		"user.kick.big":  []byte(`{"id":"user.kick.big"}`),
	}
	a, err := marshalPrefs(favs, overrides, userRecipes)
	if err != nil {
		t.Fatalf("marshal a: %v", err)
	}
	b, err := marshalPrefs(favs, overrides, userRecipes)
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
	// A session that only loads must not allocate the userprefs.persist
	// worker — the pool is acquired on the first write.
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
	rs := s.(RecipeStore)
	if _, err := rs.LoadRecipeOverrides(); err != nil {
		t.Fatalf("LoadRecipeOverrides: %v", err)
	}
	if _, err := rs.LoadUserRecipes(); err != nil {
		t.Fatalf("LoadUserRecipes: %v", err)
	}

	if poolPresent(name) {
		t.Fatalf("pool %q allocated by read-only operations; expected lazy on first write", name)
	}

	if err := rs.SaveRecipeOverride("drum-snare", map[string]float64{"decay": 1.5}); err != nil {
		t.Fatalf("SaveRecipeOverride: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	if !poolPresent(name) {
		t.Fatalf("pool %q not allocated after first write", name)
	}
}
