//go:build !test && !js

package audio

import "testing"

// Phase-7 FM-family migration (the LAST legacy family).
//
// For each of the five FM-family recipes, two tests:
//
//   - Test<Variant>ModularMatchesOracle is the byte gate: it replays every
//     oracle fixture case (including |combo and, for the four tone-wiring
//     presets, |tonedrive) through the PRODUCTION recipe seam and asserts the
//     pinned legacy_oracle_golden.json hash. It is a subset of the legacy
//     fixture, so it passes TRIVIALLY before the migration (the regression net
//     AFTER it). Every FM _p() wrapper called the SHARED apply_post_params, so
//     every FM recipe uses PostOrder=0 (the shared op order) — there is no
//     drive-before-tone trap like the base snare. fm-bell wires brightness (not
//     tone) so it has no |tonedrive case. NEVER touch the fixtures.
//   - Test<Variant>RendersViaModular is the RED marker: it asserts the recipe is
//     rebound onto the modular engine (IsModularMigratedRecipe + a non-nil
//     builtinFamilyRenderers entry). It only goes green once
//     fm_modular_binding.go rebinds the recipe.

func TestFMBassModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "fm-bass")
}

func TestFMBassRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("fm-bass") {
		t.Fatalf("fm-bass is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["fm-bass"] == nil {
		t.Fatalf("fm-bass migrated but not rebound onto a modular family renderer")
	}
}

func TestFMBellModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "fm-bell")
}

func TestFMBellRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("fm-bell") {
		t.Fatalf("fm-bell is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["fm-bell"] == nil {
		t.Fatalf("fm-bell migrated but not rebound onto a modular family renderer")
	}
}

func TestFMLeadModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "fm-lead")
}

func TestFMLeadRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("fm-lead") {
		t.Fatalf("fm-lead is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["fm-lead"] == nil {
		t.Fatalf("fm-lead migrated but not rebound onto a modular family renderer")
	}
}

func TestFMEPianoModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "fm-epiano")
}

func TestFMEPianoRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("fm-epiano") {
		t.Fatalf("fm-epiano is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["fm-epiano"] == nil {
		t.Fatalf("fm-epiano migrated but not rebound onto a modular family renderer")
	}
}

func TestFMPluckModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "fm-pluck")
}

func TestFMPluckRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("fm-pluck") {
		t.Fatalf("fm-pluck is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["fm-pluck"] == nil {
		t.Fatalf("fm-pluck migrated but not rebound onto a modular family renderer")
	}
}
