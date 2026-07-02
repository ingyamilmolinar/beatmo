//go:build !test && !js

package audio

import "testing"

// TestPitchAwareRecipe_SynthBassIsTrue asserts that the bass-guitar recipe
// (synth-modular-bass-guitar, renamed from synth-bass) and the synth bass family
// (acid/reese/fm/808) are correctly classified as pitch-aware. This test pins it
// independently via the public RecipeForInstrument → pitchAwareRecipe path.
//
// The broad pitch-range render coverage and the cello formant-vs-resample
// check live in all_instruments_pitch_table_test.go.
func TestPitchAwareRecipe_SynthBassIsTrue(t *testing.T) {
	for _, id := range []string{"bass-guitar", "bass-acid", "bass-reese", "bass-fm", "bass-808"} {
		rec := RecipeForInstrument(id)
		if rec == "" {
			t.Fatalf("RecipeForInstrument(%s) is empty — instrument not registered", id)
		}
		if !pitchAwareRecipe(rec) {
			t.Errorf("pitchAwareRecipe(%q) = false for %s; want true (melodic pitched instrument)", rec, id)
		}
	}
}
