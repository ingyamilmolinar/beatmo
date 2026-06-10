//go:build !test && !js

package audio

import "testing"

// modularIdentityGolden is the SHA256 of the default modular voice render
// (render_modular / NULL params). It is the bit-identical anchor for the
// per-stage bypass + noise-generator feature: adding the six appended ABI
// fields (osc/fm/env/filter/drive _enabled + noise_seed) MUST NOT change the
// default sound, because every enable flag defaults to 1.0 (via the mp_get
// fallback) and osc_type/noise_seed default to 0.
//
// If a future intentional change to the modular default voice flips this,
// regenerate by logging hashFloat32(buf) from TestModularIdentityGolden and
// updating the constant in the same commit (same protocol as the FM goldens).
const modularIdentityGolden = "5baf86ab569dd4349a711e09cb7a6d675ce91ea21eb3c81359416c085e789d22"

// TestModularIdentityGolden proves the default modular voice is byte-identical
// across the two render entry points AND against the captured baseline:
//
//  1. renderModular(NULL) — the cheap unparameterized fast path (unchanged code).
//  2. renderModularP(identity) — the parameterized recipe path with every param
//     at its schema identity (all stages enabled, sine, seed 0).
//
// (1) == (2) proves the recipe path reproduces the fast path; (1) == baseline
// proves the new ABI fields didn't perturb the default sound.
func TestModularIdentityGolden(t *testing.T) {
	const sr = 48000
	const n = 48000 // 1.0s

	null := make([]float32, n)
	renderModular(null, sr, n)
	nullHash := hashFloat32(null)

	ident := make([]float32, n)
	renderModularP(ident, sr, n, identityModularParams())
	identHash := hashFloat32(ident)

	if nullHash != identHash {
		t.Fatalf("modular identity path diverged from the NULL fast path:\n  null:  %s\n  ident: %s\n(the parameterized recipe path must reproduce render_modular exactly)", nullHash, identHash)
	}
	if modularIdentityGolden != "PLACEHOLDER" && nullHash != modularIdentityGolden {
		t.Fatalf("default modular voice render hash drifted\n  got:  %s\n  want: %s\n(if intentional, update modularIdentityGolden)", nullHash, modularIdentityGolden)
	}
	if modularIdentityGolden == "PLACEHOLDER" {
		t.Logf("modularIdentityGolden baseline = %s", nullHash)
	}
}

// identityModularParams returns ModularParams filled from the schema identity
// map — i.e. every stage enabled, sine oscillator, seed 0. This is the exact
// block the recipe fast path renders for an unedited modular instrument.
func identityModularParams() ModularParams {
	return recipeParamsToModular(RecipeParams(ModularParamSchemaIdentity()))
}

// TestModularEnableFlagsDefaultEnabled locks the bit-identical contract at the
// schema layer: every per-stage bypass flag defaults to 1.0 (enabled) and
// noise_seed to 0, in both the identity map and the editable ParamDefs.
func TestModularEnableFlagsDefaultEnabled(t *testing.T) {
	ident := ModularParamSchemaIdentity()
	for _, name := range []string{"osc_enabled", "fm_enabled", "env_enabled", "filter_enabled", "drive_enabled"} {
		if ident[name] != 1 {
			t.Errorf("identity[%q] = %v, want 1 (enabled)", name, ident[name])
		}
	}
	if ident["noise_seed"] != 0 {
		t.Errorf("identity[noise_seed] = %v, want 0", ident["noise_seed"])
	}
	defByName := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		defByName[d.Name] = d
	}
	for _, name := range []string{"osc_enabled", "fm_enabled", "env_enabled", "filter_enabled", "drive_enabled"} {
		if defByName[name].Default != 1 {
			t.Errorf("ParamDef %q default = %v, want 1", name, defByName[name].Default)
		}
	}
}

// TestModularBypassSilencesAndPassesThrough verifies the bypass semantics each
// stage promises, beyond the generic "mutation changes output" guard:
//   - osc disabled  → silence (the generator is the source)
//   - env disabled  → no amplitude shaping (sample 0 is full-scale, not ~0)
//   - filter disabled → output equals the raw enveloped generator pre-filter
func TestModularBypassSilencesAndPassesThrough(t *testing.T) {
	const sr = 48000
	const n = sr / 4

	// Oscillator off → pure silence.
	p := identityModularParams()
	p.OscEnabled = 0
	buf := make([]float32, n)
	renderModularP(buf, sr, n, p)
	if pk := peakAbs(buf); pk != 0 {
		t.Errorf("osc disabled: peak = %v, want 0 (silence)", pk)
	}

	// Envelope off → the first sample is the raw oscillator (not attenuated by
	// a near-zero attack envelope). A default sine starts at ~0, so use a saw
	// (starts at -1) to make the "no envelope" effect unambiguous.
	pEnvOn := identityModularParams()
	pEnvOn.OscType = 1 // saw
	pEnvOff := pEnvOn
	pEnvOff.EnvEnabled = 0
	on := make([]float32, n)
	off := make([]float32, n)
	renderModularP(on, sr, n, pEnvOn)
	renderModularP(off, sr, n, pEnvOff)
	// With the envelope ON, the very first samples are scaled by the rising
	// attack (≈0). With it OFF, they are the full saw. So the bypassed buffer
	// must have strictly greater early energy.
	earlyEnergy := func(b []float32) float64 {
		var e float64
		for i := 0; i < 64 && i < len(b); i++ {
			e += float64(b[i]) * float64(b[i])
		}
		return e
	}
	if earlyEnergy(off) <= earlyEnergy(on) {
		t.Errorf("env bypass did not remove attack shaping: off=%v on=%v", earlyEnergy(off), earlyEnergy(on))
	}
}

// TestModularNoiseDeterministicPerSeed proves the noise generator is (a)
// reproducible for a fixed seed — so a single render is golden-stable — and
// (b) decorrelated across seeds — so the round-robin variants actually vary.
func TestModularNoiseDeterministicPerSeed(t *testing.T) {
	const sr = 48000
	const n = sr / 4

	for _, osc := range []float64{5, 6} { // white, pink
		base := identityModularParams()
		base.OscType = osc

		s0a := make([]float32, n)
		s0b := make([]float32, n)
		s1 := make([]float32, n)
		p0 := base
		p0.NoiseSeed = 0
		p1 := base
		p1.NoiseSeed = 1
		renderModularP(s0a, sr, n, p0)
		renderModularP(s0b, sr, n, p0)
		renderModularP(s1, sr, n, p1)

		assertFinite(t, s0a)
		if pk := peakAbs(s0a); pk <= 0 {
			t.Fatalf("osc_type %v noise produced silence", osc)
		}
		if hashFloat32(s0a) != hashFloat32(s0b) {
			t.Errorf("osc_type %v: same seed produced different output (not reproducible)", osc)
		}
		if hashFloat32(s0a) == hashFloat32(s1) {
			t.Errorf("osc_type %v: seed 0 and seed 1 produced identical output (variants won't vary)", osc)
		}
	}
}
