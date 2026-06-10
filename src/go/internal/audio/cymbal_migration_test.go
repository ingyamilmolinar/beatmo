//go:build !test && !js

package audio

import "testing"

// Phase-6 cymbal-family migration.
//
// For each of the six cymbal-family recipes, two tests:
//
//   - Test<Variant>ModularMatchesOracle is the byte gate: it replays every
//     oracle fixture case (including |combo) through the PRODUCTION recipe seam
//     and asserts the pinned legacy_oracle_golden.json hash. It is a subset of
//     the legacy fixture, so it passes TRIVIALLY before the migration (the
//     regression net AFTER it). The cymbal recipes wire BRIGHTNESS (not tone),
//     so there is no |tonedrive case; the |combo case sets every wired knob
//     non-default at once, exercising the shared POST stage's
//     decay→brightness→drive order (PostOrder=0 — the only order any cymbal
//     wrapper uses). NEVER touch the fixtures.
//   - Test<Variant>RendersViaModular is the RED marker: it asserts the recipe is
//     rebound onto the modular engine (IsModularMigratedRecipe + a non-nil
//     builtinFamilyRenderers entry). It only goes green once
//     cymbal_modular_binding.go rebinds the recipe.

func TestHihatModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-hihat")
}

func TestHihatRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-hihat") {
		t.Fatalf("drum-hihat is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-hihat"] == nil {
		t.Fatalf("drum-hihat migrated but not rebound onto a modular family renderer")
	}
}

func TestOpenHihatModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-open-hihat")
}

func TestOpenHihatRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-open-hihat") {
		t.Fatalf("drum-open-hihat is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-open-hihat"] == nil {
		t.Fatalf("drum-open-hihat migrated but not rebound onto a modular family renderer")
	}
}

func TestCowbellModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-cowbell")
}

func TestCowbellRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-cowbell") {
		t.Fatalf("drum-cowbell is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-cowbell"] == nil {
		t.Fatalf("drum-cowbell migrated but not rebound onto a modular family renderer")
	}
}

func TestShakerModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-shaker")
}

func TestShakerRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-shaker") {
		t.Fatalf("drum-shaker is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-shaker"] == nil {
		t.Fatalf("drum-shaker migrated but not rebound onto a modular family renderer")
	}
}

func TestRideModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-ride")
}

func TestRideRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-ride") {
		t.Fatalf("drum-ride is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-ride"] == nil {
		t.Fatalf("drum-ride migrated but not rebound onto a modular family renderer")
	}
}

func TestCrashModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-crash")
}

func TestCrashRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-crash") {
		t.Fatalf("drum-crash is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-crash"] == nil {
		t.Fatalf("drum-crash migrated but not rebound onto a modular family renderer")
	}
}
