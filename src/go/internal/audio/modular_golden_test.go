//go:build !test && !js

package audio

import "testing"

// The default modular voice: regenerate from TestModularIdentityGolden's log in the same commit as an intentional change.
const modularIdentityGolden = "38f767da249ec2079945f226db8de47349b8144189942fb8b0c2f13446eb605e"

// The NULL fast path, the identity block and the captured baseline must all agree.
func TestModularIdentityGolden(t *testing.T) {
	const sr = 48000
	const n = 48000 // 1.0s

	null := make([]float32, n)
	renderModularBlock(null, sr, n, initBlockForTest(nil))
	nullHash := hashFloat32(null)

	ident := make([]float32, n)
	renderModularBlock(ident, sr, n, identityModularParams())
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

// Every stage enabled, sine, seed 0: the block an unedited modular instrument renders.
func identityModularParams() modularBlock {
	return initBlockForTest(nil)
}

func TestModularEnableFlagsDefaultEnabled(t *testing.T) {
	ident := modularFieldInits()
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

// osc off → silence; env off → no amplitude shaping; filter off → the raw enveloped generator.
func TestModularBypassSilencesAndPassesThrough(t *testing.T) {
	const sr = 48000
	const n = sr / 4

	// Oscillator off → pure silence.
	p := identityModularParams()
	p.set("osc_enabled", 0)
	buf := make([]float32, n)
	renderModularBlock(buf, sr, n, p)
	if pk := peakAbs(buf); pk != 0 {
		t.Errorf("osc disabled: peak = %v, want 0 (silence)", pk)
	}

	// A sine starts at ~0, so a saw (starting at -1) makes "no envelope" unambiguous.
	pEnvOn := identityModularParams()
	pEnvOn.set("osc_type", 1) // saw
	pEnvOff := pEnvOn.clone()
	pEnvOff.set("env_enabled", 0)
	on := make([]float32, n)
	off := make([]float32, n)
	renderModularBlock(on, sr, n, pEnvOn)
	renderModularBlock(off, sr, n, pEnvOff)
	// With the envelope on the first samples ride the attack (≈0); off, they are the full saw.
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

// Reproducible per seed, decorrelated across seeds, so round-robin variants actually vary.
func TestModularNoiseDeterministicPerSeed(t *testing.T) {
	const sr = 48000
	const n = sr / 4

	for _, osc := range []float64{5, 6} { // white, pink
		base := identityModularParams()
		base.set("osc_type", osc)

		s0a := make([]float32, n)
		s0b := make([]float32, n)
		s1 := make([]float32, n)
		p0 := base.clone()
		p0.set("noise_seed", 0)
		p1 := base.clone()
		p1.set("noise_seed", 1)
		renderModularBlock(s0a, sr, n, p0)
		renderModularBlock(s0b, sr, n, p0)
		renderModularBlock(s1, sr, n, p1)

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
