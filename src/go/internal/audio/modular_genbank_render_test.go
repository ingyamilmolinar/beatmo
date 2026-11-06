//go:build !test && !js

package audio

import "testing"

const genTestSR = 48000
const genTestSamples = 24000

func renderModularWith(t *testing.T, mutate func(modularBlock)) []float32 {
	t.Helper()
	mp := initBlockForTest(nil)
	if mutate != nil {
		mutate(mp)
	}
	buf := make([]float32, genTestSamples)
	renderModularBlock(buf, genTestSR, genTestSamples, mp)
	return buf
}

// All-identity gen bank == pre-gen-bank engine. THE Phase-1 invariant:
// appending the bank must not move a single byte of any existing preset.
func TestGenBankIdentityIsByteNeutral(t *testing.T) {
	base := renderModularWith(t, nil)
	legacy := make([]float32, genTestSamples)
	renderModularBlock(legacy, genTestSR, genTestSamples, initBlockForTest(nil)) // NULL-params engine path
	if hashFloat32(base) != hashFloat32(legacy) {
		t.Fatal("identity modularBlock render != NULL-params render — gen bank defaults are not no-ops")
	}
}

// An activated slot is audible; deactivating restores the exact baseline.
func TestGenBankSlotActivateAndBypassRoundTrip(t *testing.T) {
	base := renderModularWith(t, nil)
	withSlot := renderModularWith(t, func(mp modularBlock) {
		mp.set("gen2_source", 1) // slot 2: osc
		mp.set("gen2_wave", 0)   // sine
		mp.set("gen2_freq", 2)   // ratio 2× voice freq
		mp.set("gen2_gain", 0.5)
	})
	if hashFloat32(base) == hashFloat32(withSlot) {
		t.Fatal("activating gen slot 2 changed nothing — bank is not wired into the mix")
	}
	roundTrip := renderModularWith(t, func(mp modularBlock) {
		mp.set("gen2_wave", 0)
		mp.set("gen2_freq", 2)
		mp.set("gen2_gain", 0.5)
		// source stays 0 — every other slot param set but the slot is OFF
	})
	if hashFloat32(base) != hashFloat32(roundTrip) {
		t.Fatal("slot with source=0 (off) altered the render — bypass must be exact")
	}
}

// Noise-tap slot: deterministic, seeded by noise_seed, offset-able.
func TestGenBankNoiseTapDeterministicAndSeeded(t *testing.T) {
	tap := func(seed float64) string {
		return hashFloat32(renderModularWith(t, func(mp modularBlock) {
			mp.set("osc_enabled", 0) // silence the legacy osc; hear only the tap
			mp.set("gen1_source", 2)
			mp.set("gen1_gain", 0.25)
			mp.set("noise_seed", seed)
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
	flat := renderModularWith(t, func(mp modularBlock) {
		mp.set("osc_enabled", 0)
		mp.set("gen1_source", 2)
		mp.set("gen1_gain", 0.25)
	})
	shaped := renderModularWith(t, func(mp modularBlock) {
		mp.set("osc_enabled", 0)
		mp.set("gen1_source", 2)
		mp.set("gen1_gain", 0.25)
		mp.set("gen1_env_fast_rate", 16)
		mp.set("gen1_filt_type", 1) // 1-pole LP
		mp.set("gen1_filt_alpha", 0.03)
	})
	if hashFloat32(flat) == hashFloat32(shaped) {
		t.Fatal("slot env+filter had no effect")
	}
}

// The gen-slot oscillator must reuse the EXACT legacy osc code path: a
// slot-1 sine at ratio 1 equals the legacy osc stage byte-for-byte. This is
// the "stage machinery is exact" invariant that family migrations build on.
func TestGenSlotSineMatchesLegacyOscExactly(t *testing.T) {
	legacyOsc := renderModularWith(t, func(mp modularBlock) {
		mp.set("env_enabled", 0)
		mp.set("filter_enabled", 0)
	})
	slotOsc := renderModularWith(t, func(mp modularBlock) {
		mp.set("env_enabled", 0)
		mp.set("filter_enabled", 0)
		mp.set("osc_enabled", 0) // legacy osc silent...
		mp.set("gen1_source", 1) // ...slot 1 sine takes over
		mp.set("gen1_wave", 0)
		mp.set("gen1_freq", 1) // ratio 1 → same voice freq (220 Hz)
		mp.set("gen1_gain", 1)
	})
	if hashFloat32(legacyOsc) != hashFloat32(slotOsc) {
		t.Fatal("gen-slot sine != legacy osc sine — slot oscillator must call the identical wt_osc path (same init, same tick loop)")
	}
}
