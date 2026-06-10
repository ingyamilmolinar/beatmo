//go:build test

package ui

import (
	"bytes"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// These tests close the render-level gap in the import/export round-trip
// coverage: previous tests asserted JSON fields and audio-manager state, but
// never the tone the next trigger actually renders. The dispatch hot path
// (tryRecipeVoiceOpts, synth_recipe_dispatch.go — native-only) renders with
// exactly MergeRecipeDefaults(recipeID, GetInstrumentParams(id)); on the
// legacy fast path (empty overlay + uncustomized recipe) the C engine renders
// the shipped defaults, which equal that same merge. So
// effectiveRenderTone(id) below IS the rendered tone on every path, and is
// available under -tags test where the C renderers are stubbed.
func effectiveRenderTone(inst string) audio.RecipeParams {
	return audio.MergeRecipeDefaults(audio.RecipeForInstrument(inst), audio.GetInstrumentParams(inst))
}

// withCleanInstrumentParams makes the test hermetic against the process-global
// params manager (withDefaultAudio's ResetInstruments does not touch it):
// clears the instrument's overlay now and again at cleanup so neighbouring
// tests can't leak overlays into each other's exports.
func withCleanInstrumentParams(t *testing.T, inst string) {
	t.Helper()
	audio.ResetInstrumentParams(inst)
	t.Cleanup(func() { audio.ResetInstrumentParams(inst) })
}

// customizeRecipeForTest simulates a session where the recipe's registered
// defaults were customized away from shipped (Synth-tab Save in a previous
// edit, or a userprefs RecipeOverrides reload at startup). Restores shipped
// defaults on cleanup so the global recipe registry doesn't leak.
func customizeRecipeForTest(t *testing.T, recipe string, overrides map[string]float64) {
	t.Helper()
	if !audio.UpdateRecipeDefaultsAndInvalidate(recipe, overrides) {
		t.Fatalf("precondition: customizing recipe %q changed nothing (overrides=%v)", recipe, overrides)
	}
	t.Cleanup(func() { audio.ResetRecipeToShipped(recipe) })
}

// TestImportRender_OverlayEdit_IntoCustomizedSession is the primary
// "import doesn't restore the synth sound" repro. The export elides params
// equal to SHIPPED defaults (export.go writes synth_params as an
// effective-minus-shipped delta), so the import must resolve those elided
// params back to SHIPPED — not to whatever the live session's (possibly
// Save-customized) registered defaults happen to be.
func TestImportRender_OverlayEdit_IntoCustomizedSession(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	withCleanInstrumentParams(t, inst)
	recipe := audio.RecipeForInstrument(inst)
	if recipe == "" {
		t.Fatalf("row0 instrument %q has no recipe binding", inst)
	}
	shipped := audio.RecipeShippedDefaults(recipe)
	for _, k := range []string{"drive", "tone"} {
		if _, ok := shipped[k]; !ok {
			t.Fatalf("recipe %q lacks generic param %q", recipe, k)
		}
	}

	// User edits ONE knob; "tone" stays at shipped and is therefore elided
	// from the exported synth_params delta.
	audio.SetInstrumentParam(inst, "drive", 0.66)
	wantTone := effectiveRenderTone(inst)

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Different session: the same recipe's defaults are customized (drive AND
	// tone moved). The project file must still reproduce the exported tone.
	audio.ResetInstrumentParams(inst)
	audio.BindInstrumentToRecipe(inst, "")
	customizeRecipeForTest(t, recipe, map[string]float64{
		"drive": shipped["drive"] + 0.3,
		"tone":  shipped["tone"] + 0.25,
	})

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	got := effectiveRenderTone(inst)
	if math.Abs(got["drive"]-0.66) > 1e-6 {
		t.Errorf("rendered drive after import: got %v want 0.66 (the exported edit)", got["drive"])
	}
	if math.Abs(got["tone"]-wantTone["tone"]) > 1e-6 {
		t.Errorf("rendered tone after import: got %v want %v (shipped — elided at export must NOT resolve to the session's customized default)",
			got["tone"], wantTone["tone"])
	}

	// Re-export from the customized session must be byte-identical to the
	// original file: the delta-vs-shipped recomputation has to be stable.
	bytes2, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if !bytes.Equal(bytes1, bytes2) {
		t.Errorf("re-export not byte-identical after import into customized session\n--- export 1:\n%s\n--- export 2:\n%s", bytes1, bytes2)
	}
}

// TestImportRender_ShippedProject_IntoCustomizedSession: a project exported
// at shipped defaults carries NO synth_params at all. Importing it into a
// session whose recipe defaults are Save-customized must render SHIPPED — the
// file describes the shipped tone — not the session's saved tone.
func TestImportRender_ShippedProject_IntoCustomizedSession(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	withCleanInstrumentParams(t, inst)
	recipe := audio.RecipeForInstrument(inst)
	if recipe == "" {
		t.Fatalf("row0 instrument %q has no recipe binding", inst)
	}
	shipped := audio.RecipeShippedDefaults(recipe)

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	customizeRecipeForTest(t, recipe, map[string]float64{
		"drive": shipped["drive"] + 0.3,
	})

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	got := effectiveRenderTone(inst)
	for k, want := range shipped {
		if math.Abs(got[k]-want) > 1e-6 {
			t.Errorf("rendered %q after import: got %v want shipped %v (customized session default leaked into the imported project's tone)",
				k, got[k], want)
		}
	}
}

// TestImportRender_SampleEditPlusParams: synth_params delta + a
// non-destructive Sampler edit travel together; both must survive an import
// into a customized session (the edit applies to the recipe render the
// dispatch builds from the SAME merged params the pin reconstructs).
func TestImportRender_SampleEditPlusParams(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	withCleanInstrumentParams(t, inst)
	recipe := audio.RecipeForInstrument(inst)
	if recipe == "" {
		t.Fatalf("row0 instrument %q has no recipe binding", inst)
	}
	shipped := audio.RecipeShippedDefaults(recipe)

	audio.SetInstrumentParam(inst, "drive", 0.42)
	edit := audio.SampleEdit{StartFrac: 0.1, EndFrac: 0.9, GainDB: -3, Reverse: true}
	audio.SetSampleEdit(inst, edit)
	t.Cleanup(func() { audio.ClearSampleEdit(inst) })
	want := effectiveRenderTone(inst)

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	audio.ResetInstrumentParams(inst)
	audio.ClearSampleEdit(inst)
	customizeRecipeForTest(t, recipe, map[string]float64{
		"drive": shipped["drive"] + 0.3,
		"tone":  shipped["tone"] + 0.25,
	})

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	got := effectiveRenderTone(inst)
	if math.Abs(got["drive"]-0.42) > 1e-6 {
		t.Errorf("rendered drive after import: got %v want 0.42", got["drive"])
	}
	if math.Abs(got["tone"]-want["tone"]) > 1e-6 {
		t.Errorf("rendered tone after import: got %v want %v (shipped)", got["tone"], want["tone"])
	}
	gotEdit, ok := audio.SampleEditFor(inst)
	if !ok {
		t.Fatal("sample edit descriptor not restored by import")
	}
	if gotEdit != edit {
		t.Errorf("sample edit after import = %+v, want %+v", gotEdit, edit)
	}
}

// TestImportRender_Modular_StageEnablesEnumSeed: the modular schema's enum
// (osc_type), per-stage bypass (filter_enabled) and hidden (noise_seed) keys
// follow the same delta-vs-shipped contract. Importing a project that pins a
// few of them into a session whose synth-modular defaults are customized must
// resolve every elided key to SHIPPED and every delta key to the file value.
func TestImportRender_Modular_StageEnablesEnumSeed(t *testing.T) {
	withDefaultAudio(t)
	const inst = "modular"
	const recipe = "synth-modular"
	withCleanInstrumentParams(t, inst)
	shipped := audio.RecipeShippedDefaults(recipe)
	for _, k := range []string{"osc_type", "filter_enabled", "noise_seed", "amp_decay"} {
		if _, ok := shipped[k]; !ok {
			t.Fatalf("recipe %q lacks param %q", recipe, k)
		}
	}

	// The modular instrument is not a default row: craft the project JSON the
	// way export emits it (recipe + delta-vs-shipped synth_params).
	project := []byte(`{
		"version": 1,
		"subdiv": 8,
		"bpm": 120,
		"instruments": [{
			"name": "Modular", "id": "modular", "kind": "builtin",
			"volume": 1, "origin": 0, "color": "#44A8FFFF",
			"recipe": "synth-modular",
			"synth_params": {"osc_type": 2, "filter_enabled": 0, "noise_seed": 7}
		}],
		"nodes": [{"id": 0, "i": 0, "j": 0, "type": "regular"}]
	}`)

	// Customized session: a key NOT in the delta (amp_decay) moved by a Save.
	customizeRecipeForTest(t, recipe, map[string]float64{
		"amp_decay": shipped["amp_decay"] + 0.5,
	})

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	if err := g.Import(project); err != nil {
		t.Fatalf("import: %v", err)
	}

	got := effectiveRenderTone(inst)
	for k, want := range map[string]float64{"osc_type": 2, "filter_enabled": 0, "noise_seed": 7} {
		if math.Abs(got[k]-want) > 1e-6 {
			t.Errorf("rendered %q after import: got %v want %v (the file's delta)", k, got[k], want)
		}
	}
	if math.Abs(got["amp_decay"]-shipped["amp_decay"]) > 1e-6 {
		t.Errorf("rendered amp_decay after import: got %v want shipped %v (elided key must not pick up the Save)",
			got["amp_decay"], shipped["amp_decay"])
	}
}

// TestImportRender_PlainOverlay_CleanSession is the regression guard for the
// already-working path: overlay edit → export → import into an uncustomized
// session reproduces the exported tone and re-exports byte-identically.
func TestImportRender_PlainOverlay_CleanSession(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	withCleanInstrumentParams(t, inst)
	recipe := audio.RecipeForInstrument(inst)
	if recipe == "" {
		t.Fatalf("row0 instrument %q has no recipe binding", inst)
	}

	audio.SetInstrumentParam(inst, "drive", 0.66)
	audio.SetInstrumentParam(inst, "pitch", 4)
	want := effectiveRenderTone(inst)

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	audio.ResetInstrumentParams(inst)
	audio.BindInstrumentToRecipe(inst, "")

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	got := effectiveRenderTone(inst)
	for k, w := range want {
		if math.Abs(got[k]-w) > 1e-6 {
			t.Errorf("rendered %q after import: got %v want %v", k, got[k], w)
		}
	}
	bytes2, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	if !bytes.Equal(bytes1, bytes2) {
		t.Errorf("re-export not byte-identical in clean session")
	}
}
