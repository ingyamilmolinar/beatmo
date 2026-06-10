//go:build !test && !js

package audio

import "testing"

// Phase-5 snare-family migration.
//
// For each of the four snare-family recipes, two tests:
//
//   - Test<Variant>ModularMatchesOracle is the byte gate: it replays every
//     oracle fixture case (including |combo and |tonedrive) through the PRODUCTION
//     recipe seam and asserts the pinned legacy_oracle_golden.json hash. It is a
//     subset of the legacy fixture, so it passes TRIVIALLY before the migration
//     (the regression net AFTER it). The drum-snare |tonedrive case (tone=-0.5,
//     drive=0.5) is the strongest gate: it pins the legacy base-snare post-op
//     ORDER (drive BEFORE tone) that the modular POST stage must reproduce via
//     post_order=2. The |combo case CANNOT pin the order — it sets tone=+0.4,
//     which is inert in the legacy tone branch (LP-only for tone<0), so both
//     orders pass combo. |tonedrive sets tone<0 AND drive>0 simultaneously, the
//     only point where the orders diverge. The |tonedrive fixture hash was
//     captured byte-for-byte from the deleted legacy render_snare_p (verified via
//     a throwaway legacy-transplant oracle at capture time). NEVER touch the
//     fixtures.
//   - Test<Variant>RendersViaModular is the RED marker: it asserts the recipe is
//     rebound onto the modular engine (IsModularMigratedRecipe + a non-nil
//     builtinFamilyRenderers entry). It only goes green once
//     snare_modular_binding.go rebinds the recipe.

func TestSnareModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-snare")
}

func TestSnareRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-snare") {
		t.Fatalf("drum-snare is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-snare"] == nil {
		t.Fatalf("drum-snare migrated but not rebound onto a modular family renderer")
	}
}

func TestSnareRimshotModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-snare-rimshot")
}

func TestSnareRimshotRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-snare-rimshot") {
		t.Fatalf("drum-snare-rimshot is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-snare-rimshot"] == nil {
		t.Fatalf("drum-snare-rimshot migrated but not rebound onto a modular family renderer")
	}
}

func TestSnareSidestickModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-snare-sidestick")
}

func TestSnareSidestickRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-snare-sidestick") {
		t.Fatalf("drum-snare-sidestick is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-snare-sidestick"] == nil {
		t.Fatalf("drum-snare-sidestick migrated but not rebound onto a modular family renderer")
	}
}

func TestClapModularMatchesOracle(t *testing.T) {
	assertRecipeMatchesOracleFixtures(t, "drum-clap")
}

func TestClapRendersViaModular(t *testing.T) {
	if !IsModularMigratedRecipe("drum-clap") {
		t.Fatalf("drum-clap is not migrated to the modular engine yet")
	}
	if builtinFamilyRenderers["drum-clap"] == nil {
		t.Fatalf("drum-clap migrated but not rebound onto a modular family renderer")
	}
}
