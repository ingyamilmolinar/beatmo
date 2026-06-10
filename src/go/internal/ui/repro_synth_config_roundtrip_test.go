//go:build test

package ui

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestReproSynthConfigRoundtrip reproduces "synth config edits on an instrument
// don't persist/load". It edits an instrument's synth knobs the way the Synth
// tab does (audio.SetInstrumentParam → per-instrument overlay), exports, then
// imports into a brand-new instance (audio reset) and checks the params + recipe
// binding survived.
func TestReproSynthConfigRoundtrip(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	t.Logf("row0 instrument=%q recipe=%q", inst, audio.RecipeForInstrument(inst))

	// Edit synth knobs (UI path).
	audio.SetInstrumentParam(inst, "drive", 0.66)
	audio.SetInstrumentParam(inst, "pitch", 4)
	pre := audio.GetInstrumentParams(inst)
	t.Logf("after edit: params=%v recipe=%q", map[string]float64(pre), audio.RecipeForInstrument(inst))
	if math.Abs(pre["drive"]-0.66) > 1e-6 {
		t.Fatalf("precondition: drive not stored, got %v", pre["drive"])
	}

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Brand-new instance: clear this instrument's overlay + binding so only the
	// import can restore them (the params manager is a process-global that
	// ResetInstruments does not touch).
	audio.ResetInstrumentParams(inst)
	audio.BindInstrumentToRecipe(inst, "")
	if rem := audio.GetInstrumentParams(inst); len(rem) != 0 {
		t.Fatalf("precondition: params not cleared: %v", map[string]float64(rem))
	}

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	post := audio.GetInstrumentParams(inst)
	t.Logf("after import: params=%v recipe=%q", map[string]float64(post), audio.RecipeForInstrument(inst))
	if math.Abs(post["drive"]-0.66) > 1e-6 {
		t.Errorf("drive not restored: got %v want 0.66", post["drive"])
	}
	if math.Abs(post["pitch"]-4) > 1e-6 {
		t.Errorf("pitch not restored: got %v want 4", post["pitch"])
	}
}

// TestReproSynthSaveRoundtrip reproduces the Synth-tab "Save" round-trip
// faithfully: the real Save (saveRecipeForInstrument) bakes the effective tone
// into the recipe's registered defaults AND keeps the per-instrument overlay.
// On a brand-new instance the recipe registry is shipped-default and userprefs
// are empty, so the project file alone must reproduce the saved tone.
func TestReproSynthSaveRoundtrip(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	recipe := audio.RecipeForInstrument(inst)
	if recipe == "" {
		t.Skipf("row0 instrument %q has no recipe binding; nothing to Save", inst)
	}
	shipped := audio.RecipeShippedDefaults(recipe)
	if _, ok := shipped["drive"]; !ok {
		t.Skipf("recipe %q has no 'drive' param", recipe)
	}

	// User edits a knob, then presses Save (mirror saveRecipeForInstrument:
	// mutate recipe defaults to the effective params, keep the overlay).
	audio.SetInstrumentParam(inst, "drive", 0.7)
	effPre := audio.MergeRecipeDefaults(recipe, audio.GetInstrumentParams(inst))
	audio.UpdateRecipeDefaultsAndInvalidate(recipe, map[string]float64(effPre))
	wantDrive := effPre["drive"]
	t.Logf("after Save: effective drive=%v overlay=%v", wantDrive, map[string]float64(audio.GetInstrumentParams(inst)))

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Brand-new instance: recipe shipped, overlay cleared, binding gone.
	audio.ResetRecipeToShipped(recipe)
	audio.ResetInstrumentParams(inst)
	audio.BindInstrumentToRecipe(inst, "")

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	effPost := audio.MergeRecipeDefaults(recipe, audio.GetInstrumentParams(inst))
	t.Logf("after import: effective drive=%v overlay=%v recipe=%q",
		effPost["drive"], map[string]float64(audio.GetInstrumentParams(inst)), audio.RecipeForInstrument(inst))
	if math.Abs(effPost["drive"]-wantDrive) > 1e-6 {
		t.Errorf("Synth Save tone LOST on round-trip: effective drive=%v want %v", effPost["drive"], wantDrive)
	}
}

// TestReproRecipeDefaultOnlyRoundtrip reproduces the realistic broken flow:
// a Synth-tab "Save" persists the customization to the recipe DEFAULTS (and
// userprefs), keeping a per-instrument overlay only for the rest of the session.
// After an app restart, ApplyUserRecipeOverrides restores the recipe default but
// the overlay is EMPTY. Exporting then must still capture the customized tone so
// it loads on a brand-new instance (different machine / cleared storage). The
// fix: export the effective params as a delta from the recipe's shipped defaults.
func TestReproRecipeDefaultOnlyRoundtrip(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument
	recipe := audio.RecipeForInstrument(inst)
	if recipe == "" {
		t.Skipf("row0 instrument %q has no recipe binding", inst)
	}
	shipped := audio.RecipeShippedDefaults(recipe)
	if _, ok := shipped["drive"]; !ok {
		t.Skipf("recipe %q has no 'drive' param", recipe)
	}

	// Customized recipe default with NO per-instrument overlay (the
	// startup-from-userprefs state after a prior Save + restart).
	wantDrive := shipped["drive"] + 0.4
	audio.UpdateRecipeDefaultsAndInvalidate(recipe, map[string]float64{"drive": wantDrive})
	audio.ResetInstrumentParams(inst) // overlay empty
	t.Logf("recipe-default drive=%v overlay=%v", audio.RecipeDefaultParams(recipe)["drive"], map[string]float64(audio.GetInstrumentParams(inst)))

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Brand-new instance: recipe shipped, overlay empty, binding gone.
	audio.ResetRecipeToShipped(recipe)
	audio.ResetInstrumentParams(inst)
	audio.BindInstrumentToRecipe(inst, "")

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	effPost := audio.MergeRecipeDefaults(recipe, audio.GetInstrumentParams(inst))
	t.Logf("after import: effective drive=%v overlay=%v", effPost["drive"], map[string]float64(audio.GetInstrumentParams(inst)))
	if math.Abs(effPost["drive"]-wantDrive) > 1e-6 {
		t.Errorf("Saved recipe tone LOST on round-trip (no overlay): effective drive=%v want %v", effPost["drive"], wantDrive)
	}
}

// TestReproSamplerSaveRoundtrip reproduces the Sampler-tab "Save" round-trip:
// save() bakes the current edit (gain/trim/normalize/etc.) into a final PCM and
// stores it via audio.SaveUserSample → userSamples, which the exporter embeds.
// A brand-new instance must reproduce the baked sample from the project bytes.
func TestReproSamplerSaveRoundtrip(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	inst := g.drum.Rows[0].Instrument

	// Emulate a sampler edit + Save: bake a buffer and store it for the row's
	// instrument (audio.SaveUserSample is exactly what samplerState.save calls).
	raw := make([]float32, 2000)
	for i := range raw {
		raw[i] = float32(math.Sin(float64(i) * 0.05))
	}
	baked := audio.BakeSample(raw, 48000, audio.SampleEdit{
		StartFrac: 0.1, EndFrac: 0.9, GainDB: -6, Normalize: true, FadeInMs: 5, FadeOutMs: 5,
	})
	audio.SaveUserSample(inst, baked, 48000)
	if rec, ok := audio.UserSamplePCM(inst); !ok || len(rec.PCM) != len(baked) {
		t.Fatalf("precondition: baked sample not stored (ok=%v len=%d want %d)", ok, len(rec.PCM), len(baked))
	}

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Simulate a fresh instance: overwrite the in-memory PCM with a different
	// buffer so a passing assertion can only come from the project bytes.
	audio.PutUserSample(inst, []float32{0, 0, 0, 0}, 48000)

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import: %v", err)
	}

	rec, ok := audio.UserSamplePCM(inst)
	t.Logf("after import: sample present=%v frames=%d (baked=%d)", ok, len(rec.PCM), len(baked))
	if !ok || len(rec.PCM) != len(baked) {
		t.Fatalf("sampler edit LOST on round-trip: present=%v frames=%d want %d", ok, len(rec.PCM), len(baked))
	}
	// Content must match the baked buffer (proves the edited audio survived).
	var maxDiff float64
	for i := range baked {
		if d := math.Abs(float64(rec.PCM[i] - baked[i])); d > maxDiff {
			maxDiff = d
		}
	}
	if maxDiff > 1e-6 {
		t.Errorf("sampler PCM content diverged after round-trip: maxDiff=%v", maxDiff)
	}
}
