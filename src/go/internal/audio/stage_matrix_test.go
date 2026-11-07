//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Order-sensitive: a single changed sample flips it.
func stageMatrixChecksum(buf []float32) uint64 {
	var bits uint64
	for i, v := range buf {
		bits ^= (uint64(i)*2654435761 + 1) * uint64(math.Float32bits(v))
	}
	return bits
}

// Flips that are legitimately inaudible for a reason stageEffectivelyActive cannot express without simulating DSP; each
// needs a verified rationale. Prefer teaching the predicate a new rule over adding one.
var stageMatrixNeutralFlips = map[[2]string]string{}

// A static predicate over the params mirroring the C gates, never a DSP simulation, so the test tells "wired but inert"
// from "nothing to act on". Pitch env and vibrato are unreachable for the noise and physical-model osc types, whose
// render branches run first; the LFO's amp and cutoff targets are not.
func stageEffectivelyActive(toggle string, merged RecipeParams) bool {
	get := func(name string, def float64) float64 {
		if v, ok := merged[name]; ok {
			return v
		}
		return def
	}
	oscType := int(math.Round(get("osc_type", 0)))
	oscOn := get("osc_enabled", 1) >= 0.5
	isNoiseOrPhysical := oscType == 5 || oscType == 6 || (oscType >= 7 && oscType <= 11)

	switch toggle {
	case "osc_enabled":
		// Gates the whole generator chain, physical models included: the model IS the osc.
		return get("osc_enabled", 1) >= 0.5

	case "fm_enabled":
		// Only with osc_type 4. An edge contributes only when the source op's level AND the edge's depth are non-zero; op1's
		// self-feedback exists in every algorithm.
		if !oscOn || oscType != 4 {
			return false
		}
		alg := int(math.Round(get("fm_algorithm", 0)))
		level := [4]float64{get("fm_op1_level", 1), get("fm_op2_level", 0), get("fm_op3_level", 0), get("fm_op4_level", 0)}
		depth := [4]float64{get("fm_op1_depth", 0), get("fm_op2_depth", 0), get("fm_op3_depth", 0), get("fm_op4_depth", 0)}
		pair := func(i int) bool { return level[i] != 0 && depth[i] != 0 }
		if pair(0) {
			return true // op1 self-feedback — wired regardless of algorithm
		}
		switch alg {
		case 1: // parallel: no mod_matrix chain edges beyond self-feedback
			return false
		case 2: // 3-op chain: op3->op2->op1
			return pair(1) || pair(2)
		case 3: // 4-op stack: op4->op3->op2->op1
			return pair(1) || pair(2) || pair(3)
		default: // alg 0: 2-op stack, op2->op1
			return pair(1)
		}

	case "pitchenv_enabled":
		// The engine gate is amount ≠ 0 && decay > 0, and never for noise/physical-model osc types.
		if !oscOn || isNoiseOrPhysical {
			return false
		}
		peOn := get("pitchenv_enabled", 0) >= 0.5
		return peOn && get("pitchenv_amt", 0) != 0 && get("pitchenv_decay", 0) > 0

	case "lfo_enabled":
		// Target 0 (amp) runs for every osc type; 1 (vibrato) has the same gap as pitch env; 2 (cutoff) needs the filter on.
		lfoOn := get("lfo_enabled", 0) >= 0.5
		if !lfoOn {
			return false
		}
		if get("lfo_depth", 0) <= 0 || get("lfo_rate", 0) <= 0 {
			return false
		}
		switch int(math.Round(get("lfo_target", 0))) {
		case 0:
			return true
		case 1:
			return oscOn && !isNoiseOrPhysical && oscType != 4
		case 2:
			return get("filter_enabled", 1) >= 0.5
		default:
			return false
		}

	case "filtenv_enabled":
		// Inside the filter stage, and an LFO cutoff sweep takes priority over it.
		if get("filter_enabled", 1) < 0.5 {
			return false
		}
		lfoOn := get("lfo_enabled", 0) >= 0.5
		cutSweep := lfoOn && int(math.Round(get("lfo_target", 0))) == 2 &&
			get("lfo_depth", 0) > 0 && get("lfo_rate", 0) > 0
		if cutSweep {
			return false
		}
		filtenvOn := get("filtenv_enabled", 0) >= 0.5
		return filtenvOn && get("filtenv_amt", 0) > 0 && get("filtenv_decay", 0) > 0

	case "filter_enabled":
		// No shipped filter configuration is a true identity, so this is the raw gate; exempt a genuinely inert pair instead
		// of complicating it.
		return get("filter_enabled", 1) >= 0.5

	case "drive_enabled":
		// drive ≤ 0 leaves the waveshaper unexecuted.
		return get("drive_enabled", 1) >= 0.5 && get("drive", 0) > 0

	case "burst_enabled":
		// An enabled burst multiplies the mix by Σ amp_j·e^(-sharp·(t-off_j)); with every amp at 0 that is 0, so it is never a
		// no-op against a sounding voice.
		return true

	case "env_enabled":
		// Inaudible only for a fully unity envelope: zero attack, zero decay, full sustain, zero release.
		a := get("amp_attack", 0.005)
		d := get("amp_decay", 0.300)
		sus := get("amp_sustain", 0.600)
		rel := get("amp_release", 0.200)
		return a > 0 || d > 0 || sus < 1 || rel > 0

	case "kick_enabled":
		// A no-op unless some gen slot plays the kick voice (source 5).
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_source", k)
			if int(math.Round(get(name, 0))) == 5 {
				return true
			}
		}
		return false

	case "snare_enabled":
		// A no-op unless some gen slot plays the snare voice (source 7).
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_source", k)
			if int(math.Round(get(name, 0))) == 7 {
				return true
			}
		}
		return false

	case "formant_enabled":
		// Gates the breath noise (breath > 0) and the vowel bank (mix > 0); with both 0 it is a no-op either way.
		return get("formant_mix", 0) > 0 || get("formant_breath", 0) > 0

	case "modal_enabled":
		// No rows, or keep 0, leaves nothing to ring.
		return get("modal_count", 0) > 0 && get("modal_keep", 128) > 0

	case "strike_enabled":
		// STRIKE only reweights the modal rows: inert without them, identity at tc 0 / hardness 0 / click 0.
		modal := get("modal_enabled", 0) >= 0.5 && get("modal_count", 0) > 0 && get("modal_keep", 128) > 0
		return modal && (get("strike_tc", 0) > 0 || get("strike_hardness", 0) != 0 || get("strike_click", 0) > 0)

	case "noiseband_enabled":
		// modal_bank.c: the NOISEBAND stage renders its table rows only.
		return get("noiseband_count", 0) > 0

	default:
		// An unmodelled toggle fails loud (active) rather than quietly accepting a flip nobody justified.
		return true
	}
}

// Every *_enabled toggle on every built-in renders finite and audibly changes the render, unless the predicate says
// the stage has nothing to act on. A neutral flip of an active stage fails unless exempted; an audible flip of an
// exempted pair fails as a stale exemption.
func TestP5_EveryStageToggleablePerInstrument(t *testing.T) {
	Reset()
	ResetInstruments()

	assertFinite := func(t *testing.T, id, toggle string, buf []float32) {
		t.Helper()
		for i, v := range buf {
			f := float64(v)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				t.Fatalf("%s with %s flipped: non-finite sample at %d", id, toggle, i)
			}
		}
	}

	for _, id := range roster.All {
		defs := InstrumentParamDefs(id)
		if len(defs) == 0 {
			continue
		}

		base, _ := RenderInstrumentOneShotRaw(id)
		baseSum := stageMatrixChecksum(base)

		merged := MergedInstrumentParamsRO(id)
		for _, def := range defs {
			name := def.Name
			if len(name) < 9 || name[len(name)-8:] != "_enabled" {
				continue
			}
			// Constants of the voice's module type, not switches: the voice IS the instrument.
			if IsFamilyEnableParam(name) {
				continue
			}
			cur, ok := merged[name]
			if !ok {
				cur = def.Default
			}
			flipped := 1.0
			if cur >= 0.5 {
				flipped = 0.0
			}

			SetInstrumentParam(id, name, flipped)
			buf, _ := RenderInstrumentOneShotRaw(id)
			ResetInstrumentParams(id)

			if len(buf) == 0 {
				t.Errorf("%s: flipping %s rendered empty", id, name)
				continue
			}
			assertFinite(t, string(id), name, buf)

			key := [2]string{string(id), name}
			why, exempt := stageMatrixNeutralFlips[key]

			if stageMatrixChecksum(buf) != baseSum {
				// An exempted pair that is now audible is a stale exemption.
				if exempt {
					t.Errorf("%s: flipping %s (%v -> %v) DID change the render — stale stageMatrixNeutralFlips entry (rationale: %q); remove the stale entry", id, name, cur, flipped, why)
				}
				continue
			}

			// No audible change.
			if !stageEffectivelyActive(name, merged) {
				continue
			}
			// An inert gate, or a residual case the predicate cannot express.
			if !exempt {
				t.Errorf("%s: flipping %s (%v -> %v) did not change the render — stage gate inert per stageEffectivelyActive (add a stageMatrixNeutralFlips entry ONLY with a verified rationale, or teach stageEffectivelyActive the missing rule)", id, name, cur, flipped)
			}
		}
	}
}

// An empty rationale would be a silent, unexplained exemption.
func TestStageMatrixResidualExemptionsHaveRationale(t *testing.T) {
	for key, why := range stageMatrixNeutralFlips {
		if why == "" {
			t.Errorf("stageMatrixNeutralFlips[%v] has an empty rationale", key)
		}
	}
}
