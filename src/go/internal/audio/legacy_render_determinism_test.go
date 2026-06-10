//go:build !test && !js

package audio

import "testing"

// TOMBSTONE (Phase-7, the LAST legacy-family migration).
//
// This file once pinned the determinism of the LEGACY bespoke renderers: the
// migration's byte-identity contract assumes every render call is deterministic
// (the legacy drum renderers seeded their ma_noise stream with 0 on EVERY call,
// so consecutive renders — and therefore all RoundRobinVariants entries — are
// byte-identical). That guard mattered because the oracle fixtures
// (legacy_oracle_golden_test.go) are only meaningful if the renderer is stable
// across calls.
//
// After Phase-7 there are NO legacy renderers left: EVERY family (kick Phase-3,
// tom Phase-4, snare/clap Phase-5, cymbal Phase-6, bass Phase-2, FM Phase-7 — the
// LAST) renders through the unified modular engine. Rather than delete the
// contract outright, this file is CONVERTED to a tombstone that preserves the
// contract's SPIRIT: it asserts the MODULAR render path is deterministic across
// consecutive calls for a representative set of migrated voices (a noise-driven
// drum, a noise-driven snare, and the base modular voice). The modular engine's
// own goldens (modular_golden_test.go) + the per-family migration oracle tests
// pin the per-recipe bytes; this tombstone keeps the "renders are stable across
// calls" invariant alive now that the legacy path is fully retired.
//
// The migrated voices read their deterministic noise stream from a fixed seed
// (noise_seed, default 0) exactly like the legacy renderers did, so the same
// "consecutive renders are byte-identical" property holds.
func TestModularRenderPathDeterministicAcrossCalls(t *testing.T) {
	cases := []struct {
		name   string
		render func(buf []float32, sampleRate, samples int)
	}{
		// A noise-driven kick (source==5), a noise-driven snare (source==7), and the
		// base modular voice — representative migrated voices spanning a structural
		// noise-bank voice and the plain pipeline. All must be byte-stable across
		// consecutive calls (deterministic seed).
		{"drum-kick", renderKickVoice},
		{"drum-snare", renderSnareVoice},
		{"modular", renderModular},
		// Karplus-Strong (source==3): the one voice with a private delay line +
		// its OWN noise_ma stream — both must be re-initialized per call (no
		// carried state). Gap-audit addition: the original 3 representatives
		// never exercised the KS state.
		{"drum-bass-guitar", renderBassGuitarVoice},
		// Phase-8C modulator stages active (PITCH ENV + LFO + BURST over the
		// base voice): the modulators are pure functions of t and must carry no
		// cross-call state either.
		{"modular+modulators", func(buf []float32, sampleRate, samples int) {
			p := recipeParamsToModular(RecipeParams(ModularParamSchemaIdentity()))
			p.PitchEnvEnabled, p.PitchEnvAmt, p.PitchEnvDecay = 1, 12, 0.08
			p.LfoEnabled, p.LfoRate, p.LfoDepth = 1, 8, 0.5
			p.BurstEnabled, p.BurstSharp, p.Burst1Amp, p.Burst2Off, p.Burst2Amp = 1, 40, 1, 0.03, 0.6
			renderModularP(buf, sampleRate, samples, p)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := make([]float32, 24000)
			b := make([]float32, 24000)
			tc.render(a, 48000, 24000)
			tc.render(b, 48000, 24000)
			if hashFloat32(a) != hashFloat32(b) {
				t.Fatalf("two consecutive modular renders differ — the modular render path is NOT deterministic; oracle/golden fixtures would be invalid")
			}
		})
	}
}
