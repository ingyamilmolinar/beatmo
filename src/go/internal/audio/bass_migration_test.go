//go:build !test && !js

package audio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Phase-2 bass-family migration (Task 1: sub-bass).
//
// These tests prove that drum-sub-bass renders byte-identically through the
// modular engine. TestSubBassModularMatchesOracle is the byte gate (it is a
// subset of the legacy-oracle fixture and therefore passes trivially BEFORE
// the migration — it is the regression net AFTER it). TestSubBassRendersViaModular
// is the red: it asserts the recipe is rebound onto the modular binding, which
// only becomes true once bass_modular_binding.go rebinds it.

// loadOracleFixture reads testdata/legacy_oracle_golden.json into a name→hash map.
func loadOracleFixture(t *testing.T) map[string]string {
	t.Helper()
	blob, err := os.ReadFile(filepath.Join("testdata", "legacy_oracle_golden.json"))
	if err != nil {
		t.Fatalf("read oracle fixtures: %v", err)
	}
	want := map[string]string{}
	if err := json.Unmarshal(blob, &want); err != nil {
		t.Fatalf("unmarshal fixtures: %v", err)
	}
	return want
}

func TestSubBassModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-sub-bass")
}

// TestSubBassRendersViaModular asserts drum-sub-bass is bound to the modular
// engine (not the legacy render_sub_bass_p path). IsModularMigratedRecipe reads
// the tag-neutral modularMigrations registry (the single migration marker); the
// native-only builtinFamilyRenderers check additionally proves the rebind
// actually happened (the registry membership drives the binding's init).
func TestSubBassRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-sub-bass") {
		t.Fatalf("drum-sub-bass is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-sub-bass"] == nil {
		t.Fatalf("drum-sub-bass migrated but not rebound onto a modular family renderer")
	}
}

// Phase-2 bass-family migration (Task 2: bass-guitar / Karplus-Strong).
//
// TestBassGuitarModularMatchesOracle is the byte gate (a subset of the
// legacy-oracle fixture; passes trivially BEFORE the migration, the regression
// net AFTER it). TestBassGuitarRendersViaModular is the red: it asserts the
// recipe is rebound onto the modular binding, which only becomes true once
// bass_modular_binding.go rebinds drum-bass-guitar.

func TestBassGuitarModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-bass-guitar")
}

func TestBassGuitarRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-bass-guitar") {
		t.Fatalf("drum-bass-guitar is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-bass-guitar"] == nil {
		t.Fatalf("drum-bass-guitar migrated but not rebound onto a modular family renderer")
	}
}
