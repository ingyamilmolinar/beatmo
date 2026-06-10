package audio

import "testing"

// TestResetSampleToFactory_BuiltinRevertsToRecipe — a built-in instrument that
// was converted to a sample (Sampler Save) reverts all the way to its original
// synth: the shipped recipe is re-bound and the user sample is discarded.
func TestResetSampleToFactory_BuiltinRevertsToRecipe(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	const instID, recipeID = "kick", "drum-kick"
	orig := RecipeForInstrument(instID)
	t.Cleanup(func() { BindInstrumentToRecipe(instID, orig) })

	// Simulate the Sampler Save flow: bind recipe, record origin, save the
	// baked sample under the same id, then clear the binding (as save() does).
	BindInstrumentToRecipe(instID, recipeID)
	RecordSampleOriginRecipe(instID, recipeID)
	SaveUserSample(instID, []float32{0.3, -0.3}, 44100)
	BindInstrumentToRecipe(instID, "")

	prior, factory := ResetSampleToFactory(instID)
	if !factory || prior != recipeID {
		t.Fatalf("ResetSampleToFactory = (%q, %v), want (%q, true)", prior, factory, recipeID)
	}
	if got := RecipeForInstrument(instID); got != recipeID {
		t.Errorf("after reset RecipeForInstrument(%q) = %q, want %q", instID, got, recipeID)
	}
	if IsUserSample(instID) {
		t.Errorf("after reset %q is still a user sample, want discarded", instID)
	}
	if SampleOriginRecipe(instID) != "" {
		t.Errorf("after reset origin recipe record still set")
	}
	found := false
	for _, d := range sink.deleted {
		if d == instID {
			found = true
		}
	}
	if !found {
		t.Errorf("sink.DeleteSample(%q) not called; deleted=%v", instID, sink.deleted)
	}
}

// TestResetSampleToFactory_UserWAVRestoresOriginal — a user sample with no
// origin recipe (loaded WAV / Save-As) reverts to its first-loaded buffer.
func TestResetSampleToFactory_UserWAVRestoresOriginal(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	const id = "user.sample.wav"
	SaveUserSample(id, []float32{0.1, 0.2, 0.3}, 48000) // first load = original
	SaveUserSample(id, []float32{0.9, 0.8}, 48000)      // edited re-save

	prior, factory := ResetSampleToFactory(id)
	if factory || prior != "" {
		t.Fatalf("ResetSampleToFactory = (%q, %v), want (\"\", false)", prior, factory)
	}
	cur, ok := UserSamplePCM(id)
	if !ok || len(cur.PCM) != 3 || cur.PCM[0] != 0.1 {
		t.Errorf("after reset buffer = %+v, want pristine 3-sample original", cur)
	}
	saved, ok := sink.saved[id]
	if !ok || len(saved.PCM) != 3 || saved.PCM[0] != 0.1 {
		t.Errorf("reset did not re-persist original via sink; saved=%+v", saved)
	}
}

// TestResetSampleToFactory_RestartScenario_RevertsBuiltinViaFactoryTable —
// after an app restart the session-only origin-recipe record is gone (the
// persisted sample is reloaded via PutUserSample, which records no origin), yet
// the recipe binding is re-applied at startup. Reset must still recognise a
// built-in via the authoritative builtinInstrumentRecipeBindings table and
// revert to its synth — re-binding the recipe, discarding the user sample, and
// deleting the persisted copy so the next startup is clean. This is the
// "reverse kick-1 won't reset after restart" bug.
func TestResetSampleToFactory_RestartScenario_RevertsBuiltinViaFactoryTable(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	const instID, recipeID = "kick-1", "drum-kick-punchy"
	// Simulate startup: built-in recipe bindings re-applied…
	bindBuiltinInstrumentRecipes()
	t.Cleanup(bindBuiltinInstrumentRecipes)
	// …then ApplySavedSamples reloads the persisted reversed chop. This does
	// NOT record an origin recipe and does NOT clear the binding.
	PutUserSample(instID, []float32{0.7, -0.7, 0.7}, 44100)
	if SampleOriginRecipe(instID) != "" {
		t.Fatalf("precondition: origin recipe should be unrecorded after a restart-style load")
	}

	prior, factory := ResetSampleToFactory(instID)
	if !factory || prior != recipeID {
		t.Fatalf("ResetSampleToFactory(%q) = (%q, %v), want (%q, true) via factory table", instID, prior, factory, recipeID)
	}
	if got := RecipeForInstrument(instID); got != recipeID {
		t.Errorf("after reset RecipeForInstrument(%q) = %q, want %q", instID, got, recipeID)
	}
	if IsUserSample(instID) {
		t.Errorf("after reset %q is still a user sample, want discarded so startup is clean", instID)
	}
	found := false
	for _, d := range sink.deleted {
		if d == instID {
			found = true
		}
	}
	if !found {
		t.Errorf("sink.DeleteSample(%q) not called; persisted sample would survive restart. deleted=%v", instID, sink.deleted)
	}
}

// TestResetSampleToFactory_NoOpWhenUnknown — unknown id is a safe no-op.
func TestResetSampleToFactory_NoOpWhenUnknown(t *testing.T) {
	resetUserSamplesForTest()
	sink := &fakeSampleSink{}
	restore := SetSampleSink(sink)
	defer restore()

	prior, factory := ResetSampleToFactory("does.not.exist")
	if factory || prior != "" {
		t.Errorf("ResetSampleToFactory(unknown) = (%q, %v), want (\"\", false)", prior, factory)
	}
	if len(sink.deleted) != 0 || len(sink.saved) != 0 {
		t.Errorf("unknown reset touched sink: deleted=%v saved=%v", sink.deleted, sink.saved)
	}
}
