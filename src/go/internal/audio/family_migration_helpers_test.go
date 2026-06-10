//go:build !test && !js

package audio

import (
	"strings"
	"testing"
)

// assertRecipeMatchesOracleFixtures is the family-generic oracle-replay gate for
// a Phase-2 legacy→modular migration. It renders every oracle case for recipeID
// through the PRODUCTION recipe seam (NewRecipe(id).Render with
// MergeRecipeDefaults) and asserts each render's sha256 matches the pinned
// legacy_oracle_golden.json hash. That fixture predates (and outlives) the
// modular rebind, so a green run is the byte-for-byte proof the migrated render
// is identical to the legacy C renderer.
//
// The expected case set is derived from oracleCases(recipe.ParamSchema()) — no
// hardcoded count — and the test also asserts that EVERY "recipeID|*" key in the
// fixture file is covered by the sweep (count derived from the fixture, not a
// magic number), so a param the schema dropped can't silently skip its golden.
func assertRecipeMatchesOracleFixtures(t *testing.T, recipeID string) {
	t.Helper()
	want := loadOracleFixture(t)

	recipe := NewRecipe(recipeID)
	if recipe == nil {
		t.Fatalf("NewRecipe(%q) returned nil", recipeID)
	}

	prefix := recipeID + "|"
	swept := map[string]bool{}
	for _, c := range oracleCases(recipeID, recipe.ParamSchema()) {
		key := prefix + c.Name
		wantHash, ok := want[key]
		if !ok {
			t.Fatalf("fixture missing key %q (param schema drifted?)", key)
		}
		merged := MergeRecipeDefaults(recipeID, c.Overlay)
		buf := make([]float32, oracleSamples)
		recipe.Render(buf, oracleSR, oracleSamples, 0, merged)
		gotHash := hashFloat32(buf)
		if gotHash != wantHash {
			t.Errorf("%s: render bytes drifted\n  got:  %s\n  want: %s", key, gotHash, wantHash)
		}
		swept[c.Name] = true
	}

	// Every fixture key for this recipe must be covered by the sweep — count
	// derived from the fixture file, never hardcoded.
	for key := range want {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		caseName := strings.TrimPrefix(key, prefix)
		if !swept[caseName] {
			t.Errorf("fixture key %q (case %q) was not exercised by the sweep", key, caseName)
		}
	}
}
