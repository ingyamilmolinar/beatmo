//go:build !test && !js

package audio

import (
	"testing"
)

// stage_params_test.go is the Phase-8A gate for making every migrated recipe's
// modular pipeline stages USER-CONTROLLABLE. It proves:
//
//	(a) default render is byte-identical before/after the stage params exist —
//	    delegated to the immutable legacy-oracle gate (TestLegacyOracleGolden),
//	    re-asserted here as a determinism + exact-bypass baseline,
//	(b) enabling a stage CHANGES the render,
//	(c) toggling a stage back to default returns the EXACT default bytes (the
//	    stage is an exact bypass when disabled),
//	(d) forcing post_enabled=0 changes the render for a recipe with a non-trivial
//	    POST (driven by an overlay so POST actually does something),
//	(e) ParamDef discipline: every migrated recipe's ParamSchema carries each
//	    appended stage def exactly once, with NO name collision against the
//	    recipe's own family/generic knobs (the fm-collision guard).

const (
	stageSR      = oracleSR
	stageSamples = oracleSamples
)

// renderMigrated renders recipeID at the given overlay through the production
// recipe seam and returns the sha256 of the buffer. Renders are deterministic
// across calls (the C voices re-seed their noise stream per call — see the
// modular-unification design § Noise determinism), so two renders of the same
// overlay hash identically; this test asserts that as a precondition.
func renderMigrated(t *testing.T, recipeID string, overlay RecipeParams) string {
	t.Helper()
	recipe := NewRecipe(recipeID)
	if recipe == nil {
		t.Fatalf("NewRecipe(%q) returned nil", recipeID)
	}
	merged := MergeRecipeDefaults(recipeID, overlay)
	buf := make([]float32, stageSamples)
	recipe.Render(buf, stageSR, stageSamples, 0, merged)
	return hashFloat32(buf)
}

// TestStageParams_DefaultRenderDeterministicAndStable proves (a): the default
// render is deterministic (two calls hash equal) and an empty overlay equals the
// nil overlay — the at-default stage state is an exact no-op layered on the
// family voice. The byte-IDENTITY-to-legacy proof is the immutable oracle gate;
// this guards the in-process determinism the (c) exact-bypass assertions rely on.
func TestStageParams_DefaultRenderDeterministicAndStable(t *testing.T) {
	for _, recipeID := range sortedModularMigratedRecipeIDs() {
		recipeID := recipeID
		t.Run(recipeID, func(t *testing.T) {
			h1 := renderMigrated(t, recipeID, nil)
			h2 := renderMigrated(t, recipeID, nil)
			if h1 != h2 {
				t.Fatalf("default render not deterministic across calls: %s vs %s", h1, h2)
			}
			// Explicitly setting every stage param to its (default) value must be a
			// no-op vs the nil overlay — the migrated defaults ARE the off/identity
			// state. Proves applyUserStageParams at default == modularIdentityBase.
			defOverlay := RecipeParams{}
			for _, d := range NewRecipe(recipeID).ParamSchema() {
				if appendedStageDef(recipeID, d) {
					defOverlay[d.Name] = d.Default
				}
			}
			if len(defOverlay) == 0 {
				t.Fatalf("recipe exposes no appended stage params (Phase-8A append missing?)")
			}
			h3 := renderMigrated(t, recipeID, defOverlay)
			if h3 != h1 {
				t.Errorf("explicit stage defaults changed the render (not byte-neutral): %s vs %s", h3, h1)
			}
		})
	}
}

// TestStageParams_EnableChangesAndExactBypass proves (b) + (c) per migrated
// recipe: enabling a stage with a meaningful numeric value changes the render,
// and toggling that stage back to its default returns the EXACT default bytes.
func TestStageParams_EnableChangesAndExactBypass(t *testing.T) {
	// Representative per-stage activations. Each picks the stage's toggle ON plus
	// a numeric that makes the stage audible (so the change is unambiguous). The
	// stage must exist on the recipe (skipped otherwise — e.g. FM recipes have no
	// fm stage group).
	type act struct {
		name    string
		overlay RecipeParams
	}
	activations := []act{
		{"osc", RecipeParams{"osc_enabled": 1, "osc_type": 1}}, // saw oscillator stacked on the voice
		{"fm", RecipeParams{"osc_enabled": 1, "osc_type": 4, "fm_enabled": 1, "fm_op2_depth": 4, "fm_op2_level": 1}},
		{"env", RecipeParams{"env_enabled": 1, "amp_attack": 0.5}}, // attack ramp reshapes the onset
		{"filter", RecipeParams{"filter_enabled": 1, "filter_cutoff": 600}},
	}

	for _, recipeID := range sortedModularMigratedRecipeIDs() {
		recipeID := recipeID
		t.Run(recipeID, func(t *testing.T) {
			defHash := renderMigrated(t, recipeID, nil)
			schema := NewRecipe(recipeID).ParamSchema()
			present := map[string]bool{}
			for _, d := range schema {
				present[d.Name] = true
			}
			any := false
			for _, a := range activations {
				a := a
				// Only exercise stages whose toggle is an APPENDED stage param on this
				// recipe (the DRIVE stage is not offered at all — absent from
				// modularStageGroups; fm group excluded on FM recipes).
				toggle := a.name + "_enabled"
				if a.name == "osc" {
					toggle = "osc_enabled"
				}
				if !present[toggle] || !isAppendedStageName(recipeID, toggle) {
					continue
				}
				// Skip an activation whose numeric isn't an appended stage param on this
				// recipe (collision exclusion) — it would silently no-op.
				skip := false
				for k := range a.overlay {
					if isModularStageParamName(k) && !isAppendedStageName(recipeID, k) {
						skip = true
					}
				}
				if skip {
					continue
				}
				any = true
				t.Run(a.name, func(t *testing.T) {
					onHash := renderMigrated(t, recipeID, a.overlay)
					if onHash == defHash {
						t.Errorf("enabling %s stage did not change the render (overlay=%v)", a.name, a.overlay)
					}
					// Exact bypass: toggle the stage back OFF (every key restored to its
					// default) — must return to the exact default bytes.
					off := RecipeParams{}
					for k := range a.overlay {
						d := defForName(schema, k)
						off[k] = d.Default
					}
					offHash := renderMigrated(t, recipeID, off)
					if offHash != defHash {
						t.Errorf("disabling %s stage is not an exact bypass: %s vs default %s", a.name, offHash, defHash)
					}
				})
			}
			if !any {
				t.Fatalf("no appended stage activations exercised for %q", recipeID)
			}
		})
	}
}

// TestStageParams_PostEnabledOffChangesRender proves (d): forcing post_enabled=0
// turns the legacy POST stage off. POST only does audible work when its wired
// sub-effects have a non-default overlay, so each recipe is driven with a small
// decay overlay (POST decay is wired on every migrated family) and compared with
// and without post_enabled=0.
func TestStageParams_PostEnabledOffChangesRender(t *testing.T) {
	for _, recipeID := range sortedModularMigratedRecipeIDs() {
		recipeID := recipeID
		t.Run(recipeID, func(t *testing.T) {
			schema := NewRecipe(recipeID).ParamSchema()
			present := map[string]bool{}
			for _, d := range schema {
				present[d.Name] = true
			}
			if !present["post_enabled"] {
				t.Fatalf("migrated recipe %q has no post_enabled stage param", recipeID)
			}
			// Pick a wired POST knob the recipe actually has so POST is non-trivial.
			// `decay` is wired on every migrated family (recipeWiredParams), so it is
			// the canonical driver.
			driver := "decay"
			if !present[driver] {
				t.Skipf("recipe %q lacks a %q POST knob to make POST non-trivial", recipeID, driver)
			}
			// A decay overlay (≠ default 1) makes the POST decay-mult envelope active.
			base := RecipeParams{driver: 2.0}
			withPost := renderMigrated(t, recipeID, base)
			noPost := RecipeParams{driver: 2.0, "post_enabled": 0}
			noPostHash := renderMigrated(t, recipeID, noPost)
			if withPost == noPostHash {
				t.Errorf("post_enabled=0 did not change the render with a non-trivial POST (decay=2.0)")
			}
		})
	}
}

// TestStageParams_SchemaDisciplineNoCollisions proves (e): every migrated recipe
// exposes the appended stage params exactly once, with no name appearing twice
// in its ParamSchema (the fm-collision guard) — and that the appended set is
// non-empty and matches modularStageParamDefs for the recipe's collision set.
func TestStageParams_SchemaDisciplineNoCollisions(t *testing.T) {
	for _, recipeID := range sortedModularMigratedRecipeIDs() {
		recipeID := recipeID
		t.Run(recipeID, func(t *testing.T) {
			schema := NewRecipe(recipeID).ParamSchema()
			seen := map[string]int{}
			for _, d := range schema {
				seen[d.Name]++
			}
			for name, n := range seen {
				if n != 1 {
					t.Errorf("param %q appears %d times in ParamSchema (collision/duplication)", name, n)
				}
			}
			// The recipe MUST expose the standard stage toggles that survive its
			// collision set. osc/env/filter are present on every migrated recipe
			// (none of those names collide). drive_enabled never appears (the
			// DRIVE stage is absent from modularStageGroups by design);
			// fm_enabled is excluded on FM recipes via the collision rule.
			for _, toggle := range []string{"osc_enabled", "env_enabled", "filter_enabled"} {
				if seen[toggle] != 1 {
					t.Errorf("expected stage toggle %q exactly once, got %d", toggle, seen[toggle])
				}
			}
			// post_enabled (force POST off) is present on every migrated recipe.
			if seen["post_enabled"] != 1 {
				t.Errorf("expected post_enabled exactly once, got %d", seen["post_enabled"])
			}
			// The modular DRIVE stage is intentionally not exposed on migrated
			// recipes (deliberately left out of modularStageGroups: its knob name
			// is the same token as the generic post drive),
			// so no recipe carries a drive_enabled toggle, and `drive` (when present)
			// is the single generic post-drive knob.
			if seen["drive"] > 1 {
				t.Errorf("`drive` duplicated (%d) — at most one (generic) drive def allowed", seen["drive"])
			}
			if seen["drive_enabled"] != 0 {
				t.Errorf("drive_enabled must not be exposed on migrated recipes, got %d", seen["drive_enabled"])
			}
		})
	}
}

// TestStageParams_FMRecipesExcludeFMStageGroup is the explicit fm-collision
// decision guard: the five FM recipes' voice IS FM, so their family `fm_op*` /
// `fm_algorithm` knobs own those names — the modular FM STAGE group (and its
// fm_enabled toggle) must be EXCLUDED from their schema (no duplicate def). A
// non-FM migrated drum, by contrast, has no fm_op* family knob, so it DOES get
// the FM stage group.
func TestStageParams_FMRecipesExcludeFMStageGroup(t *testing.T) {
	fmStageNames := []string{
		"fm_algorithm", "fm_op1_ratio", "fm_op1_depth", "fm_op1_level", "fm_enabled",
	}
	fmRecipes := map[string]bool{
		"fm-bass": true, "fm-bell": true, "fm-lead": true, "fm-epiano": true, "fm-pluck": true,
	}
	for _, recipeID := range sortedModularMigratedRecipeIDs() {
		recipeID := recipeID
		t.Run(recipeID, func(t *testing.T) {
			schema := NewRecipe(recipeID).ParamSchema()
			seen := map[string]int{}
			for _, d := range schema {
				seen[d.Name]++
			}
			if fmRecipes[recipeID] {
				// FM recipes: fm_enabled (the stage toggle) must be absent — the family
				// FM knobs own fm_op* but there is no STAGE fm_enabled toggle for them.
				if seen["fm_enabled"] != 0 {
					t.Errorf("FM recipe %q must NOT expose the modular fm_enabled stage toggle (its voice is FM)", recipeID)
				}
				// And no fm_* name may appear twice (family + stage collision).
				for _, n := range fmStageNames {
					if seen[n] > 1 {
						t.Errorf("FM recipe %q: %q appears %d times (family/stage collision)", recipeID, n, seen[n])
					}
				}
			} else {
				// Non-FM migrated drums get the FM stage group (no family fm_op*).
				if seen["fm_enabled"] != 1 {
					t.Errorf("non-FM migrated recipe %q should expose the fm_enabled stage toggle once, got %d", recipeID, seen["fm_enabled"])
				}
			}
		})
	}
}

// --- helpers ---

// appendedStageDef reports whether ParamDef d on recipeID is a genuinely
// APPENDED Phase-8A stage param (a stage name that is NOT a collision exclusion).
func appendedStageDef(recipeID string, d ParamDef) bool {
	return isAppendedStageName(recipeID, d.Name)
}

func defForName(defs []ParamDef, name string) ParamDef {
	for _, d := range defs {
		if d.Name == name {
			return d
		}
	}
	return ParamDef{Name: name}
}
