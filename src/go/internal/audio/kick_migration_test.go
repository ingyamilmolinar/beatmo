//go:build !test && !js

package audio

import "testing"

// Phase-3 kick-family migration.
//
// For each of the five kick recipes, two tests:
//
//   - Test<Variant>ModularMatchesOracle is the byte gate: it replays every
//     oracle fixture case through the PRODUCTION recipe seam and asserts the
//     pinned legacy_oracle_golden.json hash. It is a subset of the legacy
//     fixture, so it passes TRIVIALLY before the migration (the regression net
//     AFTER it). NEVER touch the fixtures.
//   - Test<Variant>RendersViaModular is the RED marker: it asserts the recipe is
//     rebound onto the modular engine (IsModularMigratedRecipe + a non-nil
//     builtinFamilyRenderers entry). It only goes green once
//     kick_modular_binding.go rebinds the recipe.

func TestKickModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-kick")
}

func TestKickRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-kick") {
		t.Fatalf("drum-kick is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-kick"] == nil {
		t.Fatalf("drum-kick migrated but not rebound onto a modular family renderer")
	}
}

func TestKickDeepModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-kick-deep")
}

func TestKickDeepRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-kick-deep") {
		t.Fatalf("drum-kick-deep is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-kick-deep"] == nil {
		t.Fatalf("drum-kick-deep migrated but not rebound onto a modular family renderer")
	}
}

func TestKickPunchyModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-kick-punchy")
}

func TestKickPunchyRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-kick-punchy") {
		t.Fatalf("drum-kick-punchy is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-kick-punchy"] == nil {
		t.Fatalf("drum-kick-punchy migrated but not rebound onto a modular family renderer")
	}
}

func TestKickLofiModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-kick-lofi")
}

func TestKickLofiRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-kick-lofi") {
		t.Fatalf("drum-kick-lofi is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-kick-lofi"] == nil {
		t.Fatalf("drum-kick-lofi migrated but not rebound onto a modular family renderer")
	}
}

func TestKickTightModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-kick-tight")
}

func TestKickTightRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-kick-tight") {
		t.Fatalf("drum-kick-tight is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-kick-tight"] == nil {
		t.Fatalf("drum-kick-tight migrated but not rebound onto a modular family renderer")
	}
}
