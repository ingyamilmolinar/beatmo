//go:build !test && !js

package audio

import "testing"

// Phase-4 tom-family migration.
//
// For each of the three tom recipes, two tests:
//
//   - Test<Variant>ModularMatchesOracle is the byte gate: it replays every
//     oracle fixture case (including |combo) through the PRODUCTION recipe seam
//     and asserts the pinned legacy_oracle_golden.json hash. It is a subset of
//     the legacy fixture, so it passes TRIVIALLY before the migration (the
//     regression net AFTER it). NEVER touch the fixtures.
//   - Test<Variant>RendersViaModular is the RED marker: it asserts the recipe is
//     rebound onto the modular engine (IsModularMigratedRecipe + a non-nil
//     builtinFamilyRenderers entry). It only goes green once
//     tom_modular_binding.go rebinds the recipe.

func TestTomModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-tom")
}

func TestTomRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-tom") {
		t.Fatalf("drum-tom is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-tom"] == nil {
		t.Fatalf("drum-tom migrated but not rebound onto a modular family renderer")
	}
}

func TestTomHighModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-tom-high")
}

func TestTomHighRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-tom-high") {
		t.Fatalf("drum-tom-high is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-tom-high"] == nil {
		t.Fatalf("drum-tom-high migrated but not rebound onto a modular family renderer")
	}
}

func TestTomLowModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-tom-low")
}

func TestTomLowRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-tom-low") {
		t.Fatalf("drum-tom-low is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-tom-low"] == nil {
		t.Fatalf("drum-tom-low migrated but not rebound onto a modular family renderer")
	}
}
