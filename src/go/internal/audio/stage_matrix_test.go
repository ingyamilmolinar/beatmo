//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"testing"
)

// stageMatrixChecksum is the same order-sensitive bit checksum the golden net
// uses (synth_golden_identity_test.go) — a single changed sample flips it.
func stageMatrixChecksum(buf []float32) uint64 {
	var bits uint64
	for i, v := range buf {
		bits ^= (uint64(i)*2654435761 + 1) * uint64(math.Float32bits(v))
	}
	return bits
}

// stageMatrixNeutralFlips lists (instrumentID, toggle) pairs where flipping
// the toggle is legitimately inaudible for reasons stageEffectivelyActive
// cannot express from the merged params alone (would require replicating
// per-sample DSP, e.g. a stateful biquad's transfer function at a specific
// (type,cutoff,Q)). Every entry needs a non-empty, verified rationale;
// enforced by TestStageMatrixResidualExemptionsHaveRationale below. Kept
// deliberately as small as possible — prefer teaching stageEffectivelyActive
// a new rule over adding an entry here.
var stageMatrixNeutralFlips = map[[2]string]string{}

// stageEffectivelyActive answers "given this instrument's MERGED params
// (defaults overlaid with its recipe seed), does the named *_enabled stage
// actually do something to the signal right now?" — a static readable
// predicate over config, never a DSP simulation. It mirrors the C gating
// conditions in src/c/modular.c (line citations in each case) so the test
// can tell "stage is wired but inert" (needs a stageMatrixNeutralFlips entry
// with a rationale) from "stage genuinely has nothing to act on" (silently
// OK — flipping it is expected to be a no-op).
//
// Two documented pipeline gaps live here, not in a hand-table:
//   - pitchenv_enabled and the lfo_enabled pitch-vibrato target (lfo_target
//     1) are structurally unreachable for osc_type 5/6 (noise) and 7-11
//     (physical-model waveguides: bowed string/brass/reed/flute/sax) — those
//     types have dedicated render branches (modular.c ~512-586) that run
//     BEFORE the generic pitch-env/vibrato wavetable branch (modular.c:587)
//     in the if/else chain, so the modulation code is simply never reached.
//     See project memory config_declarative_pipeline for the wider context
//     (physical models still take hardcoded synthesis args).
//   - lfo_enabled's OTHER two targets are NOT subject to that gap: target 0
//     (post-mix amplitude wobble, modular.c:690-699) runs unconditionally
//     after the oscillator/gen-bank stage regardless of osc_type; target 2
//     (filter cutoff sweep, modular.c:752,754-771) runs whenever the filter
//     stage itself is enabled, also regardless of osc_type.
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
		// modular.c:443,509-676 — osc_on gates the ENTIRE oscillator/generator
		// if/else chain, including the physical-model branches (osc_type
		// 7-11 run INSIDE this gate — the physical model IS the osc, so
		// disabling it silences that voice too, same as any other osc_type).
		return get("osc_enabled", 1) >= 0.5

	case "fm_enabled":
		// modular.c:444,521-528 gates whether build_fm_preset's mod_matrix
		// (modular.c:76-125) survives (fm_on false ⇒ memset to zero ⇒
		// carrier-only). Only reachable when osc_enabled and osc_type==4.
		// A mod_matrix edge only contributes when BOTH the source op's own
		// level (its output amplitude, fmsynth.c op_out=amplitude*env*wave)
		// AND that edge's depth are non-zero — matches fm_render's
		// mod_input[dst] += op_out[src]*depth (fmsynth.c:137-139). op1's
		// self-feedback (mod_matrix[0][0]=depth[0]) is wired for every
		// algorithm; the chain edges depend on fm_algorithm (modular.c:106-124).
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
		// modular.c:454-456 (pe_on/pe_amt/pe_decay), consumed at 529-537
		// (FM branch; the actual gate is fmsynth.c:127 pitch_env_amount!=0 &&
		// pitch_env_decay>0) and 587,597 (wavetable branch, same condition).
		// Structurally unreached for noise/physical-model osc_types (the
		// documented pipeline gap — see doc comment above).
		if !oscOn || isNoiseOrPhysical {
			return false
		}
		peOn := get("pitchenv_enabled", 0) >= 0.5
		return peOn && get("pitchenv_amt", 0) != 0 && get("pitchenv_decay", 0) > 0

	case "lfo_enabled":
		// modular.c:457-461 (lfo_on/rate/depth/target/delay). Three
		// independent consumers keyed by lfo_target:
		//   0 — post-mix amp wobble (690-699), unconditional on osc_type.
		//   1 — pitch vibrato (vib_on, line 506), consumed only inside the
		//       same wavetable branch as pitchenv (587) — same physical-
		//       model/noise pipeline gap as pitchenv_enabled.
		//   2 — filter cutoff sweep (752,754-771), gated by filt_on only.
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
		// modular.c:463-466 (filtenv_on/amt/decay/attack), consumed at
		// 750-804 INSIDE `if (filt_on)`. The cut_sweep branch (752,
		// lfo_target==2, 754-771) takes priority over filt_env_on in the
		// if/else-if chain (772), so a simultaneous LFO cutoff-sweep would
		// starve this stage — checked here even though no shipped instrument
		// currently configures both simultaneously.
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
		// modular.c:446,750-804: filt_on gates the entire biquad stage. No
		// shipped (filter_type,cutoff,Q) combination reduces to a true
		// identity transform, so this reduces to the raw gate. If a genuine
		// fully-open-and-inert filter config is ever found, exempt THAT
		// (instrument,toggle) pair in stageMatrixNeutralFlips rather than
		// complicating this predicate.
		return get("filter_enabled", 1) >= 0.5

	case "drive_enabled":
		// modular.c:447,935-941: drive_on && drive>0 gates the tanh
		// waveshaper — drive<=0 leaves the stage entirely unexecuted.
		return get("drive_enabled", 1) >= 0.5 && get("drive", 0) > 0

	case "burst_enabled":
		// modular.c:462,707-727: burst_on multiplies the FULL mix by a
		// summed multi-burst envelope: env(t) = Σ amp_j·e^(-sharp·(t-off_j))
		// for t>=off_j, accumulated ONLY when amp_j!=0. With all four amps
		// at 0 the envelope never accumulates anything and stays exactly
		// 0.0 for the whole buffer, so an ENABLED burst stage silences a
		// real signal outright — there is no configuration where burst_on
		// is a true no-op against a non-silent voice. Unconditionally active.
		return true

	case "env_enabled":
		// modular.c:445,730-746: env_on applies the amp ADSR. The only
		// configuration where toggling it is inaudible is a fully-unity
		// envelope for the whole buffer: zero attack, zero decay, full
		// sustain, zero release (adsr_tick never leaves 1.0 and the release
		// trigger — rel_start = samples-rel_samples — never fires inside the
		// loop when rel_samples==0).
		a := get("amp_attack", 0.005)
		d := get("amp_decay", 0.300)
		sus := get("amp_sustain", 0.600)
		rel := get("amp_release", 0.200)
		return a > 0 || d > 0 || sus < 1 || rel > 0

	case "kick_enabled":
		// modular.h:443 + modular_stages.c:390,2354: kick_enabled<0.5
		// silences every gen-slot voice configured with source==5 (the
		// harmonic-bank kick voice); the toggle is a no-op unless at least
		// one of the (up to modularGenSlots) gen slots selects it.
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_source", k)
			if int(math.Round(get(name, 0))) == 5 {
				return true
			}
		}
		return false

	case "formant_enabled":
		// modular.c (Phase-15): formant_on gates two consumers — the breath
		// noise injection (runs only when formant_breath > 0) and the vowel-
		// bank stage (modular_formant_process returns immediately when
		// formant_mix <= 0, an exact bypass). With both mix and breath at
		// their identity 0 the toggle is a structural no-op in either
		// direction.
		return get("formant_mix", 0) > 0 || get("formant_breath", 0) > 0

	case "post_enabled":
		// modular.c:954-971 assembles the shared POST stage's synth_params +
		// gate config; drums.c:182-281 (apply_post_params) applies each of
		// the 6 transforms only when BOTH its own post_*_on gate is true AND
		// its value differs from that transform's OWN identity:
		//   pitch !=0, decay(decayMul) !=1 (NOT 0 — a multiplier, drums.c:218),
		//   body >0, brightness >0, tone <0 (ONLY negative tone acts,
		//   drums.c:261), drive(post_drive) >0. Matches
		//   ModularParamSchemaIdentity (synth_param_schema.go:131-134).
		if get("post_enabled", 0) < 0.5 {
			return false
		}
		if get("post_pitch_on", 1) >= 0.5 && get("post_pitch", 0) != 0 {
			return true
		}
		if get("post_decay_on", 1) >= 0.5 && get("post_decay", 1) != 1 {
			return true
		}
		if get("post_body_on", 1) >= 0.5 && get("post_body", 0) > 0 {
			return true
		}
		if get("post_brightness_on", 1) >= 0.5 && get("post_brightness", 0) > 0 {
			return true
		}
		if get("post_tone_on", 1) >= 0.5 && get("post_tone", 0) < 0 {
			return true
		}
		if get("post_drive_on", 1) >= 0.5 && get("post_drive", 0) > 0 {
			return true
		}
		return false

	default:
		// An *_enabled toggle with no modeled predicate: fail loud (treat as
		// active) rather than silently accepting a neutral flip we can't
		// justify — forces a deliberate rule addition above instead of a
		// quiet false-negative.
		return true
	}
}

// TestP5_EveryStageToggleablePerInstrument is the composability audit (spec P5:
// "every stage toggleable"): for every builtin instrument and every *_enabled
// stage toggle its recipe schema exposes, flipping the toggle must produce a
// finite render, and must audibly change the render (checksum) unless
// stageEffectivelyActive says the stage has nothing to act on given this
// instrument's merged params. Two possible outcomes on a checksum match:
//   - stage was NOT effectively active → OK, expected no-op.
//   - stage WAS effectively active → FAIL (inert gate) unless the pair is a
//     residual stageMatrixNeutralFlips entry with a verified rationale for why
//     the rule can't express this case.
//
// A checksum MISMATCH (audible change) always passes UNLESS the pair sits in
// stageMatrixNeutralFlips — that's a stale exemption (the gate is no longer
// inert) and must be removed from the table.
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

	for _, id := range BuiltinInstrumentIDs {
		recipeID := RecipeForInstrument(id)
		if recipeID == "" {
			continue
		}
		reg := RecipeRegistrations()[recipeID]
		if reg == nil {
			continue
		}

		base, _ := RenderInstrumentOneShotRaw(id)
		baseSum := stageMatrixChecksum(base)

		merged := MergeRecipeDefaults(recipeID, GetInstrumentParams(id))
		for _, def := range reg.Params {
			name := def.Name
			if len(name) < 9 || name[len(name)-8:] != "_enabled" {
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
			assertFinite(t, id, name, buf)

			key := [2]string{id, name}
			why, exempt := stageMatrixNeutralFlips[key]

			if stageMatrixChecksum(buf) != baseSum {
				// Audible change. Good — unless this pair was exempted as a
				// documented neutral flip, in which case the exemption is
				// now stale (the gate became audible, e.g. after a seed or
				// engine change) and must be removed.
				if exempt {
					t.Errorf("%s: flipping %s (%v -> %v) DID change the render — stale stageMatrixNeutralFlips entry (rationale: %q); remove the stale entry", id, name, cur, flipped, why)
				}
				continue
			}

			// No audible change.
			if !stageEffectivelyActive(name, merged) {
				// Expected: the stage had nothing to act on given this
				// instrument's merged params. No exemption needed.
				continue
			}
			// The rule says this stage SHOULD have been audible but wasn't —
			// either a genuine inert gate (regression) or a documented
			// residual case the rule can't express.
			if !exempt {
				t.Errorf("%s: flipping %s (%v -> %v) did not change the render — stage gate inert per stageEffectivelyActive (add a stageMatrixNeutralFlips entry ONLY with a verified rationale, or teach stageEffectivelyActive the missing rule)", id, name, cur, flipped)
			}
		}
	}
}

// TestStageMatrixResidualExemptionsHaveRationale enforces Finding 3: every
// surviving entry in stageMatrixNeutralFlips (ideally none) must carry a
// non-empty rationale string — an empty string would defeat the point of the
// residual table (a silent, unexplained exemption).
func TestStageMatrixResidualExemptionsHaveRationale(t *testing.T) {
	for key, why := range stageMatrixNeutralFlips {
		if why == "" {
			t.Errorf("stageMatrixNeutralFlips[%v] has an empty rationale", key)
		}
	}
}
