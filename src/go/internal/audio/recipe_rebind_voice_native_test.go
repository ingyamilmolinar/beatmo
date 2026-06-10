//go:build !test && !js

package audio

import "testing"

// Mid-playback recipe rebinding: when an instrument's recipe binding changes
// (MigrateGenType at import, user-sample origin rebinding, Save-As flows),
// the NEXT trigger must render through the new recipe. tryRecipeVoice
// resolves the binding live, but the voice-cache key is
// (instrument, bpm, sampleRate, paramsHash) — and two same-schema recipes
// (e.g. drum-snare and drum-hihat, both generic-8-knob) produce IDENTICAL
// merged-param hashes, so without rebind invalidation the cache serves the
// OLD recipe's buffer forever.
func TestRebindInstrumentRendersNewRecipeOnNextTrigger(t *testing.T) {
	const inst = "snare"
	const bpm, sr = 120, 44100

	origBinding := RecipeForInstrument(inst)
	t.Cleanup(func() {
		ResetInstrumentParams(inst)
		BindInstrumentToRecipe(inst, origBinding)
		globalVoiceCache.Clear()
	})

	// User params engage the recipe path; identical merged hash across
	// generic-only drum recipes is the trap this test pins.
	ResetInstrumentParams(inst)
	SetInstrumentParam(inst, "decay", 0.5)

	v1 := newRecipeAwareVoice(inst, bpm, sr)
	if v1 == nil {
		t.Fatal("baseline voice is nil")
	}
	snareBuf := variantSamples(v1, sr)

	// Rebind to a sonically different generic-schema recipe.
	BindInstrumentToRecipe(inst, "drum-hihat")
	v2 := newRecipeAwareVoice(inst, bpm, sr)
	if v2 == nil {
		t.Fatal("rebound voice is nil")
	}
	hihatBuf := variantSamples(v2, sr)

	if len(snareBuf) == 0 || len(hihatBuf) == 0 {
		t.Fatalf("voices empty: %d / %d", len(snareBuf), len(hihatBuf))
	}
	if buffersEqualF32(snareBuf, hihatBuf) {
		t.Fatal("rebound instrument still renders the OLD recipe's cached buffer — rebind must invalidate the voice cache")
	}

	// Rebinding back must also re-render (not replay the hi-hat entry).
	BindInstrumentToRecipe(inst, origBinding)
	v3 := newRecipeAwareVoice(inst, bpm, sr)
	backBuf := variantSamples(v3, sr)
	if buffersEqualF32(backBuf, hihatBuf) {
		t.Fatal("rebinding back still renders the hi-hat buffer")
	}
}
