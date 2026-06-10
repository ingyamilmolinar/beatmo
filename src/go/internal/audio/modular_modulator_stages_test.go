//go:build !test && !js

package audio

import "testing"

// modular_modulator_stages_test.go covers the Phase-8C modulator stages
// (PITCH ENV / LFO / BURST) promised by the unification spec §1 and delivered
// after the gap audit. Contract per stage:
//
//   - *_enabled defaults DISABLED (schema identity 0, mp_get fallback 0) so
//     every pre-Phase-8C render — all 648 oracle fixtures, the modular goldens,
//     the NULL fast path — is byte-identical with the stage code present.
//   - enabled=0 is an EXACT bypass: setting the stage's numeric knobs while the
//     toggle is off must not change a single byte.
//   - enabled=1 with the canonical UI defaults audibly changes the render
//     (no silent no-op toggle).
//
// PITCH ENV scope: the legacy OSC stage oscillator (wavetable types 0-3) and
// the FM core (osc_type 4, via fm_preset.pitch_env_*). Like the FM stage on
// migrated drums, it is context-dependent — it does nothing until the OSC
// stage is enabled and uses a pitched osc type (the documented fm_enabled
// precedent in modular_stage_params.go). LFO and BURST are post-mix
// multiplicative stages and shape EVERY voice, including migrated family
// gen-slot voices.

const modStageSR = 48000
const modStageSamples = modStageSR / 2 // 0.5 s

// modIdentityVoice returns the identity modular voice params (the same values
// the NULL fast path uses): sine osc, default ADSR/filter, all stages at their
// engine defaults.
func modIdentityVoice() ModularParams {
	return recipeParamsToModular(RecipeParams(ModularParamSchemaIdentity()))
}

func renderModularFor(t *testing.T, p ModularParams) []float32 {
	t.Helper()
	buf := make([]float32, modStageSamples)
	renderModularP(buf, modStageSR, modStageSamples, p)
	return buf
}

func assertSameBytes(t *testing.T, name string, a, b []float32) {
	t.Helper()
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("%s: sample %d differs: %v vs %v (must be byte-identical)", name, i, a[i], b[i])
		}
	}
}

func assertDiffers(t *testing.T, name string, a, b []float32) {
	t.Helper()
	for i := range a {
		if a[i] != b[i] {
			return
		}
	}
	t.Fatalf("%s: renders are byte-identical — the enabled stage is a silent no-op", name)
}

// ── PITCH ENV ──────────────────────────────────────────────────────────────

func TestPitchEnvDisabledIsExactBypass(t *testing.T) {
	base := modIdentityVoice()
	got := renderModularFor(t, base)

	withKnobs := base
	withKnobs.PitchEnvAmt = 12
	withKnobs.PitchEnvDecay = 0.08
	// PitchEnvEnabled stays 0 (the identity): knobs set, stage off.
	assertSameBytes(t, "pitchenv knobs with stage off", got, renderModularFor(t, withKnobs))
}

func TestPitchEnvEnabledChangesOscRender(t *testing.T) {
	base := modIdentityVoice()
	on := base
	on.PitchEnvEnabled = 1
	on.PitchEnvAmt = 12
	on.PitchEnvDecay = 0.08
	assertDiffers(t, "pitchenv on (wavetable osc)", renderModularFor(t, base), renderModularFor(t, on))
}

func TestPitchEnvEnabledChangesFMRender(t *testing.T) {
	base := modIdentityVoice()
	base.OscType = 4 // FM core
	base.FMOp1Level = 1
	on := base
	on.PitchEnvEnabled = 1
	on.PitchEnvAmt = 12
	on.PitchEnvDecay = 0.08
	assertDiffers(t, "pitchenv on (FM core)", renderModularFor(t, base), renderModularFor(t, on))
}

// ── LFO ────────────────────────────────────────────────────────────────────

func TestLFODisabledIsExactBypass(t *testing.T) {
	base := modIdentityVoice()
	got := renderModularFor(t, base)

	withKnobs := base
	withKnobs.LfoRate = 8
	withKnobs.LfoDepth = 0.5
	assertSameBytes(t, "lfo knobs with stage off", got, renderModularFor(t, withKnobs))
}

func TestLFOEnabledChangesRender(t *testing.T) {
	base := modIdentityVoice()
	on := base
	on.LfoEnabled = 1
	on.LfoRate = 8
	on.LfoDepth = 0.5
	assertDiffers(t, "lfo on", renderModularFor(t, base), renderModularFor(t, on))
}

// TestLFOShapesMigratedFamilyVoice proves the LFO is a real post-mix stage on a
// migrated drum (the gen-slot kick voice), not just the legacy osc path.
func TestLFOShapesMigratedFamilyVoice(t *testing.T) {
	r := NewRecipe("drum-kick")
	if r == nil {
		t.Fatal("NewRecipe(drum-kick) returned nil")
	}
	base := make([]float32, modStageSamples)
	r.Render(base, modStageSR, modStageSamples, 0, nil)
	on := make([]float32, modStageSamples)
	r.Render(on, modStageSR, modStageSamples, 0, RecipeParams{
		"lfo_enabled": 1, "lfo_rate": 8, "lfo_depth": 0.5,
	})
	assertDiffers(t, "lfo on migrated kick", base, on)
}

// ── BURST ──────────────────────────────────────────────────────────────────

func TestBurstDisabledIsExactBypass(t *testing.T) {
	base := modIdentityVoice()
	got := renderModularFor(t, base)

	withKnobs := base
	withKnobs.BurstSharp = 40
	withKnobs.Burst1Amp = 1
	withKnobs.Burst2Off = 0.03
	withKnobs.Burst2Amp = 0.6
	assertSameBytes(t, "burst knobs with stage off", got, renderModularFor(t, withKnobs))
}

func TestBurstEnabledChangesRender(t *testing.T) {
	base := modIdentityVoice()
	on := base
	on.BurstEnabled = 1
	on.BurstSharp = 40
	on.Burst1Amp = 1
	on.Burst2Off = 0.03
	on.Burst2Amp = 0.6
	assertDiffers(t, "burst on", renderModularFor(t, base), renderModularFor(t, on))
}

func TestBurstShapesMigratedFamilyVoice(t *testing.T) {
	r := NewRecipe("drum-snare")
	if r == nil {
		t.Fatal("NewRecipe(drum-snare) returned nil")
	}
	base := make([]float32, modStageSamples)
	r.Render(base, modStageSR, modStageSamples, 0, nil)
	on := make([]float32, modStageSamples)
	r.Render(on, modStageSR, modStageSamples, 0, RecipeParams{
		"burst_enabled": 1, "burst_sharp": 40, "burst1_amp": 1,
		"burst2_off": 0.03, "burst2_amp": 0.6,
	})
	assertDiffers(t, "burst on migrated snare", base, on)
}

// ── Identity / golden neutrality ───────────────────────────────────────────

// TestModulatorStagesIdentityIsByteNeutral renders the identity voice and the
// NULL fast path and asserts they still agree — the appended modulator fields
// must not disturb the pre-Phase-8C engine at identity. (The committed golden
// hash in modular_golden_test.go pins the same property against history.)
func TestModulatorStagesIdentityIsByteNeutral(t *testing.T) {
	viaIdentity := renderModularFor(t, modIdentityVoice())
	null := make([]float32, modStageSamples)
	renderModular(null, modStageSR, modStageSamples)
	assertSameBytes(t, "identity vs NULL fast path", viaIdentity, null)
}
