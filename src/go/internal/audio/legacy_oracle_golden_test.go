//go:build !test && !js

package audio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// Legacy-oracle golden fixtures for the modular-unification migration
// (docs/superpowers/specs/2026-06-05-modular-synth-unification-design.md).
//
// Every drum/FM recipe is rendered through the PRODUCTION recipe seam
// (NewRecipe(id).Render with MergeRecipeDefaults), at defaults and at each
// editable param's Min and Max. The sha256 of each render is pinned in
// testdata/legacy_oracle_golden.json. The fixture file is the migration
// contract: when a family's renderer is replaced by a modular-preset config
// (Phases 2-7), this test must keep passing UNCHANGED — that is the
// byte-for-byte proof. The fixtures deliberately outlive the legacy C code.
//
// Regeneration (Phase-0 capture only; NEVER during a family migration):
//
//	cd src/go && BEATMO_UPDATE_ORACLE=1 xvfb-run -a ../../.tools/go/bin/go \
//	  test ./internal/audio/ -run TestLegacyOracleGolden -v
const (
	oracleSR      = 48000
	oracleSamples = 48000 // 1.0s, matches nativeGoldenSamples
)

type oracleCase struct {
	Name    string
	Overlay RecipeParams
}

// oracleRecipeIDs returns the sorted set of drum + fm recipe IDs — the legacy
// families the migration replaces. Modular recipes (Category "modular") are
// deliberately excluded; they are the migration TARGET, not a legacy oracle.
func oracleRecipeIDs(t *testing.T) []string {
	t.Helper()
	regs := RecipeRegistrations()
	var ids []string
	for _, id := range RecipeOrder() {
		reg := regs[id]
		if reg == nil {
			continue
		}
		if reg.Category == "drum" || reg.Category == "fm" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) != 24 {
		t.Fatalf("expected 24 drum+fm oracle recipes, got %d: %v", len(ids), ids)
	}
	return ids
}

// oracleCases builds the per-recipe sweep: defaults plus each editable param's
// Min and Max. Hidden params (Group == SynthHiddenGroup) exist only for ABI
// completeness and are skipped so the sweep matches what a user can actually
// edit.
// oracleCases builds the legacy-oracle sweep for recipeID. The Phase-8A modular
// stage params (osc/env/filter/drive + per-stage toggles) APPENDED to a migrated
// recipe are skipped: the deleted C renderers had no such capability, so there
// is nothing to pin a stage min/max sweep against, and a new case would trip the
// immutable-fixture count check. A stage NAME that is actually a family/generic
// knob on the recipe (the generic `drive`; the FM voice's `fm_op*` on FM
// recipes) is a COLLISION exclusion — it is NOT an appended stage param, so it
// stays in the sweep. recipeID drives that distinction via the collision set.
func oracleCases(recipeID string, defs []ParamDef) []oracleCase {
	collision := recipeExistingStageCollisionSet(recipeID)
	isAppendedStage := func(name string) bool {
		return isModularStageParamName(name) && !collision[name]
	}
	cases := []oracleCase{{Name: "default", Overlay: nil}}
	combo := RecipeParams{}
	for _, d := range defs {
		// Skip is purely NAME-based for stage params: SynthHiddenGroup covers the
		// genuinely-hidden ABI params (noise_seed / gen banks); isAppendedStage
		// covers the Phase-8B stage params now in their canonical osc/fm/env/
		// filter/post groups. Un-hiding the stage params (Phase 8B) therefore did
		// NOT change which cases are generated — both predicates exclude them.
		if d.Group == SynthHiddenGroup {
			continue
		}
		if isAppendedStage(d.Name) {
			continue
		}
		cases = append(cases,
			oracleCase{Name: d.Name + "=min", Overlay: RecipeParams{d.Name: d.Min}},
			oracleCase{Name: d.Name + "=max", Overlay: RecipeParams{d.Name: d.Max}},
		)
		// COMBO: every non-hidden knob set simultaneously to a deterministic
		// off-default value (min + 0.7*(max-min)). Per-param sweeps never set two
		// post knobs at once, so they miss inter-param interactions — most
		// importantly the post-op ORDER (e.g. the legacy base kick applies
		// drive-before-body while the shared apply_post_params applies
		// body-before-drive). The combo case forces every post knob non-default at
		// once, so a migration that uses the wrong op order diverges here even
		// though it passes every per-param case. For enum/wave params the 0.7
		// fraction lands mid-range; that is fine — determinism is the contract, not
		// musical meaning.
		combo[d.Name] = d.Min + 0.7*(d.Max-d.Min)
	}
	if len(combo) > 0 {
		cases = append(cases, oracleCase{Name: "combo", Overlay: combo})
	}
	// TONEDRIVE: the post-op ORDER trap the per-param and combo cases both miss.
	// The legacy base-snare post applies drive BEFORE tone while the shared
	// apply_post_params applies tone BEFORE drive; the two only diverge when BOTH
	// drive>0 AND the tone one-pole LP is active. The legacy snare tone branch is
	// LP-ONLY for tone<0 (positive tone is inert), and the |combo case sets
	// tone = min + 0.7*range = +0.4 (positive ⇒ inert), so combo can't reach the
	// divergent point. The per-param tone=min case sets tone<0 but leaves drive at
	// its default (0 ⇒ no saturation), so it can't reach it either. This case sets
	// tone=-0.5 (LP active) AND drive=0.5 (saturation active) at once, the exact
	// divergent point, so a migration with the wrong post-op order goes RED here.
	// Added for every recipe whose schema exposes both tone and drive (harmless for
	// the rest, which never reach this append). For migrated recipes (snare trio,
	// bass) it self-pins via the live seam; for not-yet-migrated recipes (fm-*) it
	// captures the legacy bytes via oracleLegacyComboRenderers — see the |tonedrive
	// route in TestLegacyOracleGolden's capture branch.
	hasTone, hasDrive := false, false
	for _, d := range defs {
		if d.Group == SynthHiddenGroup {
			continue
		}
		if d.Name == "tone" {
			hasTone = true
		}
		if d.Name == "drive" {
			hasDrive = true
		}
	}
	if hasTone && hasDrive {
		cases = append(cases, oracleCase{Name: "tonedrive", Overlay: RecipeParams{"tone": -0.5, "drive": 0.5}})
	}
	return cases
}

// oracleLegacyComboRenderers maps recipe id → the PRE-MIGRATION legacy family
// renderer (the bespoke render_<family>_p path in drums.c / fmsynth.c). The
// combo case renders through this when a recipe has one, so the fixture pins the
// TRUE legacy contract — including the legacy post-op ORDER (the base kick's
// drive-before-body) that the migrated modular POST stage must reproduce. The
// per-param min/max cases already pin byte-identity through the live recipe
// seam; the combo case additionally pins inter-param interactions against the
// legacy bytes, so a migration with the wrong op order goes RED here.
//
// Bass (drum-sub-bass) has NO legacy renderer — its C path
// was deleted in the Phase-2 cutover — so those recipes are absent from this map
// and their combo renders via the live (modular) recipe seam, self-pinning the
// new contract. The kick family (drum-kick + deep/punchy/lofi/tight) followed in
// Phase-3: its capture-era renderers (kickFamilyRenderer over renderKick*Fam)
// are GONE, so the kick recipes are likewise absent here and their combo renders
// via the live (modular) seam. The combo fixtures captured BEFORE deletion stay
// IMMUTABLE — they pin the legacy base-kick drive-before-body op order that the
// modular POST stage (post_order=1) must reproduce; a wrong order goes RED in
// VERIFY mode against the pinned bytes. The literal mirrors what
// builtinFamilyRenderers held for the still-legacy families.
var oracleLegacyComboRenderers = map[string]cRecipeRenderer{
	// fm-bass / fm-bell / fm-lead / fm-epiano / fm-pluck removed — capture-era FM
	// renderers (renderFM*Fam over recipeParamsToFM) deleted in the Phase-7 cutover
	// (the LAST family). Combo + tonedrive render via the live modular seam
	// (fixtures immutable). EVERY FM _p() wrapper used the SHARED apply_post_params
	// (PostOrder=0 — no drive-before-tone/body trap), so the |tonedrive fixtures
	// (captured from the legacy render_fm_*_p before deletion for the four
	// tone-wiring presets; fm-bell wires brightness so has none) pin the shared
	// post order the modular POST stage reproduces; a wrong order goes RED.
	// drum-kick* removed — capture-era kick renderers deleted in the Phase-3
	// cutover; combo renders via the live modular seam (fixture immutable).
	// drum-tom* removed — capture-era tom renderers (tomFamilyRenderer over
	// renderTom*Fam) deleted in the Phase-4 cutover; combo renders via the live
	// modular seam (fixture immutable, pins the legacy pitch→decay→drive post order
	// the modular POST stage reproduces).
	// drum-snare* / drum-clap removed — capture-era snare renderers
	// (snareFamilyRenderer over renderSnare*Fam / renderClapFam) deleted in the
	// Phase-5 cutover; combo + tonedrive render via the live modular seam (fixtures
	// immutable). The legacy base-snare drive-before-tone post order is pinned by
	// the drum-snare |tonedrive fixture (tone=-0.5,drive=0.5 — the only case where
	// the orders diverge; |combo's tone=+0.4 is inert in the legacy LP). The
	// |tonedrive hash was captured byte-for-byte from the deleted legacy
	// render_snare_p via a throwaway transplant oracle — see TestSnarePostOrderTrap
	// for the observability proof and the Phase-5 review notes for the capture.
	// drum-hihat / drum-open-hihat / drum-cowbell / drum-shaker / drum-ride /
	// drum-crash removed — capture-era cymbal renderers (cymbalFamilyRenderer over
	// renderHiHatFam etc.) deleted in the Phase-6 cutover; combo renders via the
	// live modular seam (fixtures immutable, pin the legacy
	// decay→brightness→drive post order — PostOrder=0 — the modular POST stage
	// reproduces). The cymbals wire brightness (not tone), so no |tonedrive trap.
}

func TestLegacyOracleGolden(t *testing.T) {
	update := os.Getenv("BEATMO_UPDATE_ORACLE") == "1"
	fixturePath := filepath.Join("testdata", "legacy_oracle_golden.json")

	got := map[string]string{} // "recipeID|case" -> sha256
	for _, id := range oracleRecipeIDs(t) {
		recipe := NewRecipe(id)
		if recipe == nil {
			t.Fatalf("NewRecipe(%q) returned nil", id)
		}
		for _, c := range oracleCases(id, recipe.ParamSchema()) {
			merged := MergeRecipeDefaults(id, c.Overlay)
			buf := make([]float32, oracleSamples)
			// CAPTURE (update mode): the combo case is pinned to the LEGACY contract
			// — rendered through the pre-migration legacy renderer when one exists, so
			// the fixture records the legacy post-op ORDER (the base kick's
			// drive-before-body). Recipes whose legacy C was deleted (bass) fall back
			// to the live seam and self-pin the new modular contract.
			//
			// VERIFY (default mode): EVERY case — including combo — is rendered through
			// the LIVE recipe seam (the migrated path). So a migration that produces
			// the wrong post-op order matches the legacy combo fixture only if the
			// migrated POST stage reproduces the legacy op sequence. This is what turns
			// the combo case into a red→green gate for the kick post-order fix.
			if update && (c.Name == "combo" || c.Name == "tonedrive") {
				if legacy := oracleLegacyComboRenderers[id]; legacy != nil {
					legacy(buf, oracleSR, oracleSamples, merged)
					got[id+"|"+c.Name] = hashFloat32(buf)
					continue
				}
			}
			recipe.Render(buf, oracleSR, oracleSamples, 0, merged)
			got[id+"|"+c.Name] = hashFloat32(buf)
		}
	}

	if update {
		blob, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatalf("marshal fixtures: %v", err)
		}
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatalf("mkdir testdata: %v", err)
		}
		if err := os.WriteFile(fixturePath, append(blob, '\n'), 0o644); err != nil {
			t.Fatalf("write fixtures: %v", err)
		}
		t.Logf("wrote %d oracle fixtures to %s", len(got), fixturePath)
		return
	}

	blob, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read oracle fixtures (run the BEATMO_UPDATE_ORACLE=1 capture first): %v", err)
	}
	want := map[string]string{}
	if err := json.Unmarshal(blob, &want); err != nil {
		t.Fatalf("unmarshal fixtures: %v", err)
	}
	if len(want) != len(got) {
		t.Errorf("fixture count drift: fixture has %d cases, current render set has %d (param schema changed without the re-capture protocol?)", len(want), len(got))
	}
	var failed int
	for key, wantHash := range want {
		gotHash, ok := got[key]
		if !ok {
			t.Errorf("%s: case missing from current render set", key)
			continue
		}
		if gotHash != wantHash {
			failed++
			if failed <= 10 {
				t.Errorf("%s: render bytes drifted\n  got:  %s\n  want: %s", key, gotHash, wantHash)
			}
		}
	}
	if failed > 10 {
		t.Errorf("... and %d more byte-drift failures (total %d)", failed-10, failed)
	}
}
