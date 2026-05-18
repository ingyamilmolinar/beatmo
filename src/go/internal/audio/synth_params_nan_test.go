//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// TestSynthParams_NoNaNAtExtremes drives every shipped recipe through every
// generic-slider extreme (Min / Max / a couple of pathological combinations)
// and asserts the rendered PCM contains no NaN or ±Inf samples.
//
// Background: on WASM, rendered PCM flows directly into AudioBuffer →
// BufferSourceNode → channel BiquadFilterNode. WebAudio's biquad has no
// NaN guard at its input — a single NaN sample permanently corrupts the
// filter state and the channel goes silent. Desktop's miniaudio mixer
// hard-clamps to [-1, 1] before EQ so the symptom hides on native, but
// the underlying NaN is still a real defect.
//
// The canonical failure mode is decay=0: the envelope formula
//   exp(-t * decay_rate * (1.0/decayMul - 1.0))
// computes 1/0 = +Inf at decayMul=0, then at t=0 the product
// 0 * decay_rate * +Inf = NaN, and exp(NaN) = NaN, blanking the buffer.
// Both apply_post_params (drums.c:1709) and the six hand-written _p()
// variants clamp decayMul to 0.01 to defuse this. This test locks the
// contract in place: removing the clamp re-introduces the bug.
func TestSynthParams_NoNaNAtExtremes(t *testing.T) {
	const sr = 44100
	const samples = sr / 4 // 0.25 s

	cases := []struct {
		name   string
		mutate func(RecipeParams)
	}{
		{"decay_min", func(p RecipeParams) { p["decay"] = 0 }},
		{"decay_max", func(p RecipeParams) { p["decay"] = 4 }},
		{"attack_min", func(p RecipeParams) { p["attack"] = 0 }},
		{"attack_max", func(p RecipeParams) { p["attack"] = 4 }},
		{"pitch_min", func(p RecipeParams) { p["pitch"] = -24 }},
		{"pitch_max", func(p RecipeParams) { p["pitch"] = 24 }},
		{"tone_min", func(p RecipeParams) { p["tone"] = -1 }},
		{"tone_max", func(p RecipeParams) { p["tone"] = 1 }},
		{"drive_max", func(p RecipeParams) { p["drive"] = 1 }},
		{"body_max", func(p RecipeParams) { p["body"] = 1 }},
		{"color_min", func(p RecipeParams) { p["color"] = -1 }},
		{"color_max", func(p RecipeParams) { p["color"] = 1 }},
		{"brightness_max", func(p RecipeParams) { p["brightness"] = 1 }},
		{"all_min", func(p RecipeParams) {
			p["decay"] = 0
			p["attack"] = 0
			p["pitch"] = -24
			p["tone"] = -1
			p["drive"] = 0
			p["body"] = 0
			p["color"] = -1
			p["brightness"] = 0
		}},
		{"all_max", func(p RecipeParams) {
			p["decay"] = 4
			p["attack"] = 4
			p["pitch"] = 24
			p["tone"] = 1
			p["drive"] = 1
			p["body"] = 1
			p["color"] = 1
			p["brightness"] = 1
		}},
	}

	for _, id := range RecipeOrder() {
		recipe := NewRecipe(id)
		if recipe == nil {
			t.Fatalf("NewRecipe(%q) returned nil", id)
		}
		for _, tc := range cases {
			t.Run(id+"/"+tc.name, func(t *testing.T) {
				params := RecipeDefaultParams(id)
				tc.mutate(params)
				buf := make([]float32, samples)
				recipe.Render(buf, sr, samples, 0, params)

				var nanCount, infCount int
				var firstNaN, firstInf int = -1, -1
				for i, v := range buf {
					if math.IsNaN(float64(v)) {
						if firstNaN < 0 {
							firstNaN = i
						}
						nanCount++
					} else if math.IsInf(float64(v), 0) {
						if firstInf < 0 {
							firstInf = i
						}
						infCount++
					}
				}
				if nanCount > 0 || infCount > 0 {
					t.Fatalf("recipe %q with params %v produced %d NaN (first @%d) and %d ±Inf (first @%d) samples — would silence the WASM channel via BiquadFilterNode poisoning",
						id, params, nanCount, firstNaN, infCount, firstInf)
				}
			})
		}
	}
}
