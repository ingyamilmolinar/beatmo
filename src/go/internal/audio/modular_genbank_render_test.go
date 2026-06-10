//go:build !test && !js

package audio

import "testing"

const genTestSR = 48000
const genTestSamples = 24000

func renderModularWith(t *testing.T, mutate func(*ModularParams)) []float32 {
	t.Helper()
	mp := recipeParamsToModular(RecipeParams(ModularParamSchemaIdentity()))
	if mutate != nil {
		mutate(&mp)
	}
	buf := make([]float32, genTestSamples)
	renderModularP(buf, genTestSR, genTestSamples, mp)
	return buf
}

// All-identity gen bank == pre-gen-bank engine. THE Phase-1 invariant:
// appending the bank must not move a single byte of any existing preset.
func TestGenBankIdentityIsByteNeutral(t *testing.T) {
	base := renderModularWith(t, nil)
	legacy := make([]float32, genTestSamples)
	renderModular(legacy, genTestSR, genTestSamples) // NULL-params engine path
	if hashFloat32(base) != hashFloat32(legacy) {
		t.Fatal("identity ModularParams render != NULL-params render — gen bank defaults are not no-ops")
	}
}

// An activated slot is audible; deactivating restores the exact baseline.
func TestGenBankSlotActivateAndBypassRoundTrip(t *testing.T) {
	base := renderModularWith(t, nil)
	withSlot := renderModularWith(t, func(mp *ModularParams) {
		mp.GenSource[1] = 1 // slot 2: osc
		mp.GenWave[1] = 0   // sine
		mp.GenFreq[1] = 2   // ratio 2× voice freq
		mp.GenGain[1] = 0.5
	})
	if hashFloat32(base) == hashFloat32(withSlot) {
		t.Fatal("activating gen slot 2 changed nothing — bank is not wired into the mix")
	}
	roundTrip := renderModularWith(t, func(mp *ModularParams) {
		mp.GenWave[1] = 0
		mp.GenFreq[1] = 2
		mp.GenGain[1] = 0.5
		// source stays 0 — every other slot param set but the slot is OFF
	})
	if hashFloat32(base) != hashFloat32(roundTrip) {
		t.Fatal("slot with source=0 (off) altered the render — bypass must be exact")
	}
}

// Noise-tap slot: deterministic, seeded by noise_seed, offset-able.
func TestGenBankNoiseTapDeterministicAndSeeded(t *testing.T) {
	tap := func(seed float64) string {
		return hashFloat32(renderModularWith(t, func(mp *ModularParams) {
			mp.OscEnabled = 0 // silence the legacy osc; hear only the tap
			mp.GenSource[0] = 2
			mp.GenGain[0] = 0.25
			mp.NoiseSeed = seed
		}))
	}
	a, b := tap(0), tap(0)
	if a != b {
		t.Fatal("noise tap render is nondeterministic")
	}
	if tap(0) != tap(4321) {
		t.Fatal("noise bus seed 0 must equal seed 4321 (ma_noise_config_init rule)")
	}
	if tap(0) == tap(7) {
		t.Fatal("noise bus ignores the seed")
	}
}

// Slot env + 1-pole filter shape the tap (golden-style: just prove effect +
// determinism; family phases prove byte-exactness against legacy oracles).
func TestGenBankSlotEnvAndFilterHaveEffect(t *testing.T) {
	flat := renderModularWith(t, func(mp *ModularParams) {
		mp.OscEnabled = 0
		mp.GenSource[0] = 2
		mp.GenGain[0] = 0.25
	})
	shaped := renderModularWith(t, func(mp *ModularParams) {
		mp.OscEnabled = 0
		mp.GenSource[0] = 2
		mp.GenGain[0] = 0.25
		mp.GenEnvFastRate[0] = 16
		mp.GenFiltType[0] = 1 // 1-pole LP
		mp.GenFiltAlpha[0] = 0.03
	})
	if hashFloat32(flat) == hashFloat32(shaped) {
		t.Fatal("slot env+filter had no effect")
	}
}

// The gen-slot oscillator must reuse the EXACT legacy osc code path: a
// slot-1 sine at ratio 1 equals the legacy osc stage byte-for-byte. This is
// the "stage machinery is exact" invariant that family migrations build on.
func TestGenSlotSineMatchesLegacyOscExactly(t *testing.T) {
	legacyOsc := renderModularWith(t, func(mp *ModularParams) {
		mp.EnvEnabled = 0
		mp.FilterEnabled = 0
	})
	slotOsc := renderModularWith(t, func(mp *ModularParams) {
		mp.EnvEnabled = 0
		mp.FilterEnabled = 0
		mp.OscEnabled = 0   // legacy osc silent...
		mp.GenSource[0] = 1 // ...slot 1 sine takes over
		mp.GenWave[0] = 0
		mp.GenFreq[0] = 1 // ratio 1 → same voice freq (220 Hz)
		mp.GenGain[0] = 1
	})
	if hashFloat32(legacyOsc) != hashFloat32(slotOsc) {
		t.Fatal("gen-slot sine != legacy osc sine — slot oscillator must call the identical wt_osc path (same init, same tick loop)")
	}
}
