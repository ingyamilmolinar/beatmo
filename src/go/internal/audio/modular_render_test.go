//go:build !test && !js

package audio

import (
	"math"
	"strconv"
	"strings"
	"testing"
)

// defaultModularParams returns a musically-usable default voice: a sine
// oscillator at the engine's base pitch with a short percussive amp envelope.
// Mirrors the shipped ModularSynthParamDefs defaults.
func defaultModularParams() ModularParams {
	return ModularParams{
		OscType:     0, // sine
		OscDetune:   0,
		OscOctave:   0,
		FMAlgorithm: 0,
		FMOp1Ratio:  1, FMOp2Ratio: 1, FMOp3Ratio: 1, FMOp4Ratio: 1,
		FMOp1Depth: 0, FMOp2Depth: 0, FMOp3Depth: 0, FMOp4Depth: 0,
		FMOp1Level: 1, FMOp2Level: 0, FMOp3Level: 0, FMOp4Level: 0,
		AmpAttack:       0.005,
		AmpDecay:        0.3,
		AmpSustain:      0.6,
		AmpRelease:      0.2,
		AmpCurve:        1, // exponential
		FilterType:      0, // LP
		FilterCutoff:    8000,
		FilterResonance: 0.707,
		Drive:           0,
		Pitch:           0,
		Gain:            1,
		// All stages enabled (identity) so this default voice is byte-identical
		// to the NULL/zero-struct render path. A Go ModularParams literal that
		// omits these gets 0 → the stage would be bypassed.
		OscEnabled:    1,
		FMEnabled:     1,
		EnvEnabled:    1,
		FilterEnabled: 1,
		DriveEnabled:  1,
		NoiseSeed:     0,
	}
}

func peakAbs(buf []float32) float32 {
	var pk float32
	for _, v := range buf {
		a := v
		if a < 0 {
			a = -a
		}
		if a > pk {
			pk = a
		}
	}
	return pk
}

func assertFinite(t *testing.T, buf []float32) {
	t.Helper()
	for i, v := range buf {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("non-finite sample at %d: %v", i, v)
		}
	}
}

func TestRenderModularP_DefaultsFiniteNonSilent(t *testing.T) {
	const sr = 48000
	n := sr / 2 // 0.5s
	buf := make([]float32, n)
	renderModularP(buf, sr, n, defaultModularParams())
	assertFinite(t, buf)
	pk := peakAbs(buf)
	if pk <= 0 {
		t.Fatalf("default modular voice is silent (peak=%v)", pk)
	}
	if pk > 1.5 {
		t.Fatalf("default modular voice clips hard (peak=%v)", pk)
	}
}

func TestRenderModularP_EachOscTypeFiniteNonSilent(t *testing.T) {
	const sr = 48000
	n := sr / 4
	// 0=sine 1=saw 2=square 3=triangle 4=FM
	for osc := 0; osc <= 4; osc++ {
		p := defaultModularParams()
		p.OscType = float64(osc)
		if osc == 4 {
			// FM needs a modulator with depth to be non-trivial.
			p.FMOp2Depth = 2
			p.FMOp2Level = 1
		}
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		assertFinite(t, buf)
		if peakAbs(buf) <= 0 {
			t.Fatalf("osc_type %d produced silence", osc)
		}
	}
}

func TestRenderModularP_OscShapesDiffer(t *testing.T) {
	const sr = 48000
	n := sr / 8
	sine := make([]float32, n)
	saw := make([]float32, n)
	ps := defaultModularParams()
	ps.OscType = 0
	renderModularP(sine, sr, n, ps)
	ps.OscType = 1
	renderModularP(saw, sr, n, ps)
	var diff float64
	for i := range sine {
		diff += math.Abs(float64(sine[i] - saw[i]))
	}
	if diff == 0 {
		t.Fatalf("sine and saw oscillators produced identical output")
	}
}

func TestRenderModularP_ExtremeParamsStayFinite(t *testing.T) {
	const sr = 48000
	n := sr / 8
	p := defaultModularParams()
	p.AmpAttack = 0        // zero attack must not divide-by-zero
	p.FilterResonance = 16 // max Q must stay stable
	p.FilterCutoff = 20    // near-DC cutoff
	p.Drive = 1
	p.OscType = 4
	p.FMOp2Depth = 8
	p.FMOp2Level = 1
	buf := make([]float32, n)
	renderModularP(buf, sr, n, p)
	assertFinite(t, buf)
}

// TestRenderModularP_EveryParamMutatesInContext is the modular analogue of
// TestRecipeRender_EveryWiredParamMutatesOutput: every exposed ParamDef must
// audibly change the output when set away from its value IN A CONTEXT WHERE THE
// STAGE IS ACTIVE. FM-stage params are tested in an active 4-op-stack context
// (osc_type=FM, all operators routed with non-zero depth + level); all other
// params are tested from the default sine voice. This guards against a declared
// param that the C engine silently ignores.
func TestRenderModularP_EveryParamMutatesInContext(t *testing.T) {
	const sr = 48000
	n := sr / 8

	render := func(rp RecipeParams) []float32 {
		buf := make([]float32, n)
		renderModularP(buf, sr, n, recipeParamsToModular(rp))
		return buf
	}
	differs := func(a, b []float32) bool {
		var diffSq float64
		for i := range a {
			d := float64(a[i] - b[i])
			diffSq += d * d
		}
		return diffSq >= 1e-9
	}

	sineBase := func() RecipeParams {
		rp := RecipeParams{}
		for _, d := range ModularSynthParamDefs() {
			rp[d.Name] = d.Default
		}
		return rp
	}
	// Active 4-op-stack FM context: every operator is routed (depth>0) and
	// audible (level>0) so each FM knob has authority.
	fmBase := func() RecipeParams {
		rp := sineBase()
		rp["osc_type"] = 4
		rp["fm_algorithm"] = 3
		for _, op := range []string{"1", "2", "3", "4"} {
			rp["fm_op"+op+"_ratio"] = 1.5
			rp["fm_op"+op+"_depth"] = 3
			rp["fm_op"+op+"_level"] = 0.8
		}
		return rp
	}

	isFM := func(name string) bool { return len(name) >= 3 && name[:3] == "fm_" }

	// Contrasting mutation target per FM param family (distinct from fmBase).
	fmTarget := map[string]float64{
		"fm_algorithm": 0,
		"fm_op1_ratio": 3, "fm_op2_ratio": 3, "fm_op3_ratio": 3, "fm_op4_ratio": 3,
		"fm_op1_depth": 7, "fm_op2_depth": 7, "fm_op3_depth": 7, "fm_op4_depth": 7,
		"fm_op1_level": 0.2, "fm_op2_level": 0.2, "fm_op3_level": 0.2, "fm_op4_level": 0.2,
	}

	// drive_enabled only matters when there is drive to bypass; noise_seed only
	// matters for a noise generator. Give those two a context where the stage
	// they gate is actually active.
	driveBase := func() RecipeParams {
		rp := sineBase()
		rp["drive"] = 0.8
		return rp
	}
	noiseBase := func() RecipeParams {
		rp := sineBase()
		rp["osc_type"] = 5 // white noise
		return rp
	}
	// postBase: an audible sine with the Phase-2 POST stage enabled and every
	// per-substage gate on, so each post_* param (and the post_*_on gates) has
	// authority. post_enabled itself is tested by enabling a post substage and
	// toggling enabled off.
	postBase := func() RecipeParams {
		rp := sineBase()
		rp["post_enabled"] = 1
		rp["post_decay_rate"] = 6
		rp["post_pitch"] = 5
		rp["post_decay"] = 0.5
		rp["post_drive"] = 0.8
		rp["post_body"] = 0.7
		rp["post_brightness"] = 0.7
		rp["post_tone"] = -0.7
		return rp
	}
	// post_* substage targets that contrast with postBase to move the output.
	postTarget := map[string]float64{
		"post_enabled":       0, // disable the whole stage
		"post_pitch":         -7,
		"post_decay":         2,
		"post_decay_rate":    20,
		"post_body":          0,
		"post_brightness":    0,
		"post_tone":          0,
		"post_drive":         0,
		"post_pitch_on":      0,
		"post_decay_on":      0,
		"post_body_on":       0,
		"post_brightness_on": 0,
		"post_tone_on":       0,
		"post_drive_on":      0,
		// post_order flips the body↔drive op SEQUENCE. postBase has both post_body
		// (0.7) and post_drive (0.8) non-default with their gates on, so order
		// 0 (body→drive) and order 1 (drive→body) produce different output.
		"post_order": 1,
	}
	isPost := func(name string) bool { return strings.HasPrefix(name, "post_") }

	// ── Phase-8C modulator stages (PITCH ENV / LFO / BURST): their NUMERICS
	// only have authority when the stage is ENABLED (the stages default OFF —
	// that off-state being an exact bypass is pinned by
	// TestPitchEnvDisabledIsExactBypass et al). Give each numeric a base with
	// its stage on; burst4_off additionally needs an audible 4th burst
	// (burst4_amp defaults 0, so the onset knob has no authority without it).
	// The TOGGLES need no context: flipping 0→1 over the audible knob defaults
	// changes output through the default branch. ──
	modulatorTarget := map[string]float64{
		"pitchenv_amt": -24, "pitchenv_decay": 2,
		"lfo_rate": 40, "lfo_depth": 0,
		"burst_sharp": 200,
		"burst1_off":  0.25, "burst1_amp": 0,
		"burst2_off": 0.25, "burst2_amp": 0,
		"burst3_off": 0.25, "burst3_amp": 1,
		"burst4_off": 0.25, "burst4_amp": 1,
	}
	isModulatorNumeric := func(name string) bool { _, ok := modulatorTarget[name]; return ok }
	modulatorBase := func(name string) RecipeParams {
		rp := sineBase()
		switch {
		case strings.HasPrefix(name, "pitchenv_"):
			rp["pitchenv_enabled"] = 1
		case strings.HasPrefix(name, "lfo_"):
			rp["lfo_enabled"] = 1
		case strings.HasPrefix(name, "burst"):
			rp["burst_enabled"] = 1
			if name == "burst4_off" {
				rp["burst4_amp"] = 1
			}
		}
		return rp
	}

	// ── Gen-bank contexts (Phase-1 modular unification). A gen param only has
	// authority when its slot is active; build a base that activates the right
	// slot (osc for tone/env/filter/phase fields, noise-tap with draws>1 for
	// noise fields) so each field's mutation moves the output. The bus globals
	// (noise_draws/noise_prelude) only matter when a noise tap is reading. ──
	oscGenFields := map[string]bool{
		"wave": true, "freq_mode": true, "freq": true, "gain": true,
		"env_fast_rate": true, "env_tail_rate": true, "env_fast_mix": true, "env_tail_mix": true,
		"filt_type": true, "filt_alpha": true, "filt_freq": true, "filt_q": true,
		"phase_mode": true, "phase": true,
	}
	// activateSlotOsc turns slot k (1-based) into an audible saw osc with a
	// non-trivial env so env/filter mutations register.
	activateSlotOsc := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 1 // osc
		rp[p("wave")] = 1   // saw (harmonically rich → filter has authority)
		rp[p("freq")] = 1
		rp[p("gain")] = 1
		// Give BOTH envelope terms a non-trivial rate + weight so every env
		// field (fast/tail rate + mix) has authority on its own mutation.
		rp[p("env_fast_rate")] = 8
		rp[p("env_tail_rate")] = 2
		rp[p("env_fast_mix")] = 0.7
		rp[p("env_tail_mix")] = 0.3
	}
	// activateSlotAnalytic turns slot k into an audible Phase-2 analytic dual-osc
	// voice (source==4) with an absolute fundamental + base envelope, so the
	// per-slot analytic fields (pitch env, harm mix, attack, sat, out scale) each
	// have authority on their own mutation.
	activateSlotAnalytic := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 4 // analytic dual-osc voice
		rp[p("freq_mode")] = 1
		rp[p("freq")] = 110 // absolute Hz (audible)
		rp[p("env_fast_rate")] = 2
		rp[p("sat_k")] = 1.2
		rp[p("out_scale")] = 0.95
		rp["voice_freq_hz"] = 0
	}
	// activateSlotKS turns slot k into an audible Phase-2 Karplus-Strong string
	// voice (source==3) with an absolute fundamental, so the per-slot KS fields
	// (sustain, pluck — plus the reused attack/env/out-scale fields) each have
	// authority on their own mutation.
	activateSlotKS := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 3 // Karplus-Strong string voice
		rp[p("freq_mode")] = 1
		rp[p("freq")] = 110 // absolute Hz (audible)
		rp[p("ks_sustain")] = 0.996
		rp[p("ks_pluck")] = 0.35
		rp[p("env_fast_rate")] = 1.8
		rp[p("out_scale")] = 0.85
		rp["voice_freq_hz"] = 0
	}
	// activateSlotKick turns slot k into an audible Phase-3 harmonic-bank kick
	// voice (source==5, variant 0 = base — the only variant that reads all nine
	// curated knobs, incl. h4/noise/click), so every kick field has authority on
	// its own mutation. The curated knobs are seeded to the base-kick literals so
	// a mutation off them registers.
	activateSlotKick := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 5 // harmonic-bank kick voice
		rp[p("freq_mode")] = 1
		rp[p("freq")] = 55 // absolute Hz (audible kick fundamental)
		rp[p("kick_variant")] = 0
		rp[p("kick_h2")] = 0.40
		rp[p("kick_h3")] = 0.20
		rp[p("kick_h4")] = 0.12
		rp[p("kick_env0")] = 5.5
		rp[p("kick_env1")] = 9.0
		rp[p("kick_pe_amt")] = 0.10
		rp[p("kick_pe_rate")] = 30
		rp[p("kick_click")] = 0.35
		rp[p("kick_noise")] = 0.18
		rp["voice_freq_hz"] = 0
	}
	// activateSlotTom turns slot k into an audible Phase-4 808-style tom voice
	// (source==6, variant 0 = base tom), so every tom curated knob has authority
	// on its own mutation. The curated knobs are seeded to the base-tom literals
	// so a mutation off them registers.
	activateSlotTom := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 6 // 808-style tom voice
		rp[p("freq_mode")] = 1
		rp[p("freq")] = 150 // absolute Hz (audible tom fundamental)
		rp[p("tom_variant")] = 0
		rp[p("tom_sweep")] = 18.0
		rp[p("tom_ring")] = 2.8
		rp[p("tom_o1")] = 0.5
		rp[p("tom_o2")] = 0.25
		rp[p("tom_stick")] = 0.35
		rp[p("tom_room")] = 0.08
		rp["voice_freq_hz"] = 0
	}
	// activateSlotSnare turns slot k into an audible Phase-5 snare voice
	// (source==7, variant 0 = base snare — the only variant that reads all the
	// curated knobs incl. tail_d/wire_m), so every snare curated knob has
	// authority on its own mutation. The curated knobs are seeded to the
	// base-snare literals so a mutation off them registers.
	activateSlotSnare := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 7 // snare-ish voice
		rp[p("freq_mode")] = 1
		rp[p("freq")] = 200 // absolute Hz (audible snare fundamental)
		rp[p("snare_variant")] = 0
		rp[p("snare_tone2")] = 330.0
		rp[p("snare_tune")] = 1.0
		rp[p("snare_tone_d")] = 28.0
		rp[p("snare_noise_d")] = 12.0
		rp[p("snare_tail_d")] = 18.0
		rp[p("snare_tone_m")] = 0.40
		rp[p("snare_noise_m")] = 1.1
		rp[p("snare_wire_m")] = 0.9
		rp[p("snare_attack")] = 0.3
		rp["voice_freq_hz"] = 0
	}
	// activateSlotCymbal turns slot k into an audible Phase-6 metallic cymbal voice
	// (source==9, variant 0 = hihat — the variant that reads all six curated knobs
	// incl. cym_noise_d, plus cym_wave), so every cymbal curated knob has authority
	// on its own mutation. The curated knobs are seeded to the hihat literals so a
	// mutation off them registers.
	activateSlotCymbal := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 9 // metallic cymbal voice
		rp[p("cym_variant")] = 0
		rp[p("wave")] = 2 // hihat cluster wave
		rp[p("cym_tune")] = 1.0
		rp[p("cym_env_fast")] = 180.0
		rp[p("cym_env_tail")] = 35.0
		rp[p("cym_tone_m")] = 0.85
		rp[p("cym_noise_m")] = 0.45
		rp[p("cym_noise_d")] = 100.0
		rp["voice_freq_hz"] = 0
	}
	// activateSlotFM turns slot k into an audible Phase-7 FM-family voice
	// (source==10, variant 2 = lead — the 3-op preset that reads op1/op2/op3
	// ratio/depth/decay, plus base_freq, pitch-env amount/decay, wave), so every FM
	// curated knob that any shipped preset uses has authority on its own mutation.
	// The op4 fields (r4/d4/dec4) are structurally inert — no shipped preset has a
	// 4th operator (max num_ops == 3) — so the genContext case seeds base==target
	// for them and the harness skips them ("no distinct target in context"). The
	// curated knobs are seeded to the lead literals so a mutation off them
	// registers.
	activateSlotFM := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 10    // 4-op FM preset voice
		rp[p("fm_variant")] = 2 // lead (3 ops; reads op1/op2/op3)
		rp[p("wave")] = 0       // sine (preset default)
		rp[p("fm_base")] = 220.0
		rp[p("fm_pe_amt")] = 1.5
		rp[p("fm_pe_decay")] = 0.04
		rp[p("fm_r1")] = 1.0
		rp[p("fm_r2")] = 2.0
		rp[p("fm_r3")] = 3.0
		rp[p("fm_d2")] = 3.5
		rp[p("fm_d3")] = 2.0
		rp[p("fm_dec1")] = 0.2
		rp[p("fm_dec2")] = 0.12
		rp[p("fm_dec3")] = 0.08
	}
	// activateSlotNoiseTap turns slot k into a noise tap with a multi-draw bus
	// so noise_draws / noise_prelude / noise_offset all have authority.
	activateSlotNoiseTap := func(rp RecipeParams, k int) {
		p := func(f string) string { return "gen" + strconv.Itoa(k) + "_" + f }
		rp[p("source")] = 2 // noise tap
		rp[p("gain")] = 1
		rp[p("env_fast_mix")] = 1
		rp["noise_draws"] = 2
		rp["noise_prelude"] = 4
	}

	// genContext returns (base, target) for a gen-bank param name, or ok=false
	// if name is not a gen-bank param.
	genContext := func(d ParamDef) (RecipeParams, float64, bool) {
		name := d.Name
		if name == "noise_draws" || name == "noise_prelude" {
			rp := sineBase()
			activateSlotNoiseTap(rp, 1)
			// A second target distinct from the activate-context value.
			target := 1.0
			if name == "noise_draws" {
				target = 4
			} else {
				target = 0
			}
			return rp, target, true
		}
		if !strings.HasPrefix(name, "gen") {
			return nil, 0, false
		}
		// name == gen<k>_<field>
		rest := strings.TrimPrefix(name, "gen")
		us := strings.IndexByte(rest, '_')
		if us < 0 {
			return nil, 0, false
		}
		k, err := strconv.Atoi(rest[:us])
		if err != nil {
			return nil, 0, false
		}
		field := rest[us+1:]
		rp := sineBase()
		switch field {
		case "source":
			// Slot off in base; activating it (→1 osc) must change output.
			rp[name] = 0
			return rp, 1, true
		case "noise_offset":
			activateSlotNoiseTap(rp, k)
			rp[name] = 0
			return rp, 1, true // read a different per-sample draw
		case "phase":
			// phase only has authority when phase_mode selects it. Use mode 1
			// (fixed radians); phase 0 vs a non-zero start phase shifts the osc.
			activateSlotOsc(rp, k)
			rp["gen"+strconv.Itoa(k)+"_phase_mode"] = 1
			rp[name] = 0
			return rp, 3.0, true // ~half-cycle radians
		case "phase_mode":
			// phase_mode only matters when phase is non-zero: with a fixed
			// start phase set, switching mode 0 (ignored) → 1 (apply radians)
			// shifts the oscillator start.
			activateSlotOsc(rp, k)
			rp["gen"+strconv.Itoa(k)+"_phase"] = 3 // ~half-cycle radians
			rp[name] = 0
			return rp, 1, true // 0 (zero start) -> 1 (fixed radians)
		case "filt_freq", "filt_q":
			// RBJ filter coefficients only matter when an RBJ filter type is
			// selected (filt_type 3=LP 4=HP 5=BP). Activate an RBJ LP slot.
			activateSlotOsc(rp, k)
			rp["gen"+strconv.Itoa(k)+"_filt_type"] = 3 // RBJ LP
			rp["gen"+strconv.Itoa(k)+"_filt_freq"] = 1000
			rp["gen"+strconv.Itoa(k)+"_filt_q"] = 0.707
			if field == "filt_freq" {
				return rp, 6000, true
			}
			return rp, 8, true // resonant Q
		case "filt_alpha":
			// 1-pole input coeff only matters when a 1-pole filter is selected.
			activateSlotOsc(rp, k)
			rp["gen"+strconv.Itoa(k)+"_filt_type"] = 1 // 1-pole LP
			rp[name] = 0
			return rp, 0.5, true
		case "pitch_env_amt", "pitch_env_rate", "harm_mix",
			"atk_amt", "atk_rate", "sat_k", "out_scale":
			// Phase-2 analytic dual-osc voice fields: only have authority when
			// the slot's source is 4. Build an audible analytic slot, then pick a
			// per-field target that moves the output.
			activateSlotAnalytic(rp, k)
			switch field {
			case "pitch_env_amt":
				rp[name] = 0
				return rp, 0.6, true // add a pitch punch
			case "pitch_env_rate":
				rp["gen"+strconv.Itoa(k)+"_pitch_env_amt"] = 0.5 // give the env authority
				rp[name] = 40
				return rp, 400, true
			case "harm_mix":
				rp[name] = 0
				return rp, 1, true // add the 2nd-harmonic companion
			case "atk_amt":
				rp[name] = 0
				return rp, 1, true // add the attack boost
			case "atk_rate":
				rp["gen"+strconv.Itoa(k)+"_atk_amt"] = 1 // give the attack boost authority
				rp[name] = 60
				return rp, 600, true
			case "sat_k":
				rp[name] = 1
				return rp, 6, true // harder tanh drive
			case "out_scale":
				rp[name] = 1
				return rp, 0.5, true // trim the output
			}
		case "ks_sustain", "ks_pluck":
			// Phase-2 Karplus-Strong voice fields: only have authority when the
			// slot's source is 3. Build an audible KS slot, then pick a per-field
			// target that moves the output.
			activateSlotKS(rp, k)
			switch field {
			case "ks_sustain":
				rp[name] = 0.996
				return rp, 0.5, true // much shorter string decay
			case "ks_pluck":
				rp[name] = 0.35
				return rp, 0.9, true // brighter pluck LP (different delay-line init)
			}
		case "kick_variant", "kick_h2", "kick_h3", "kick_h4", "kick_env0",
			"kick_env1", "kick_pe_amt", "kick_pe_rate", "kick_click", "kick_noise":
			// Phase-3 harmonic-bank kick voice fields: only have authority when the
			// slot's source is 5. Build an audible base-kick slot (variant 0 reads
			// every curated knob), then pick a per-field target that moves output.
			activateSlotKick(rp, k)
			switch field {
			case "kick_variant":
				rp[name] = 0
				return rp, 4, true // base → tight (different structure entirely)
			case "kick_h2":
				rp[name] = 0.40
				return rp, 1.0, true // louder 2nd harmonic
			case "kick_h3":
				rp[name] = 0.20
				return rp, 1.0, true // louder 3rd harmonic
			case "kick_h4":
				rp[name] = 0.12
				return rp, 1.0, true // louder 4th harmonic
			case "kick_env0":
				rp[name] = 5.5
				return rp, 20, true // much faster fundamental decay
			case "kick_env1":
				rp[name] = 9.0
				return rp, 30, true // much faster body decay
			case "kick_pe_amt":
				rp[name] = 0.10
				return rp, 0.5, true // wider pitch sweep
			case "kick_pe_rate":
				rp[name] = 30
				return rp, 100, true // faster sweep decay
			case "kick_click":
				rp[name] = 0.35
				return rp, 1.0, true // louder beater click
			case "kick_noise":
				rp[name] = 0.18
				return rp, 1.0, true // louder noise thud
			}
		case "tom_variant", "tom_sweep", "tom_ring", "tom_o1", "tom_o2",
			"tom_stick", "tom_room":
			// Phase-4 808-style tom voice fields: only have authority when the
			// slot's source is 6. Build an audible base-tom slot (variant 0), then
			// pick a per-field target that moves output.
			activateSlotTom(rp, k)
			switch field {
			case "tom_variant":
				rp[name] = 0
				return rp, 2, true // tom → low (different literal set entirely)
			case "tom_sweep":
				rp[name] = 18.0
				return rp, 80, true // much faster pitch sweep
			case "tom_ring":
				rp[name] = 2.8
				return rp, 12, true // much faster ring decay
			case "tom_o1":
				rp[name] = 0.5
				return rp, 1.0, true // louder 1st overtone
			case "tom_o2":
				rp[name] = 0.25
				return rp, 1.0, true // louder 2nd overtone
			case "tom_stick":
				rp[name] = 0.35
				return rp, 1.0, true // louder stick attack
			case "tom_room":
				rp[name] = 0.08
				return rp, 1.0, true // louder room ambience
			}
		case "snare_variant", "snare_tone2", "snare_tune", "snare_tone_d",
			"snare_noise_d", "snare_tail_d", "snare_tone_m", "snare_noise_m",
			"snare_wire_m", "snare_attack":
			// Phase-5 snare-ish voice fields: only have authority when the slot's
			// source is 7. Build an audible base-snare slot (variant 0 reads every
			// curated knob), then pick a per-field target that moves output.
			activateSlotSnare(rp, k)
			switch field {
			case "snare_variant":
				rp[name] = 0
				return rp, 1, true // snare → rimshot (different structure entirely)
			case "snare_tone2":
				rp[name] = 330.0
				return rp, 1200, true // higher 2nd body tone
			case "snare_tune":
				rp[name] = 1.0
				return rp, 1.5, true // shift the noise band centers up
			case "snare_tone_d":
				rp[name] = 28.0
				return rp, 80, true // much faster body decay
			case "snare_noise_d":
				rp[name] = 12.0
				return rp, 40, true // much faster head-noise decay
			case "snare_tail_d":
				rp[name] = 18.0
				return rp, 50, true // much faster wire-tail decay
			case "snare_tone_m":
				rp[name] = 0.40
				return rp, 1.0, true // louder body
			case "snare_noise_m":
				rp[name] = 1.1
				return rp, 0.2, true // quieter head noise
			case "snare_wire_m":
				rp[name] = 0.9
				return rp, 0.2, true // quieter wires
			case "snare_attack":
				rp[name] = 0.3
				return rp, 1.0, true // sharper attack boost
			}
		case "cym_variant", "cym_tune", "cym_env_fast", "cym_env_tail",
			"cym_tone_m", "cym_noise_m", "cym_noise_d":
			// Phase-6 metallic cymbal voice fields: only have authority when the
			// slot's source is 9. Build an audible hihat slot (variant 0 reads every
			// curated knob), then pick a per-field target that moves output.
			activateSlotCymbal(rp, k)
			switch field {
			case "cym_variant":
				rp[name] = 0
				return rp, 4, true // hihat → ride (different partial table entirely)
			case "cym_tune":
				rp[name] = 1.0
				return rp, 1.5, true // shift the partial-frequency tables up
			case "cym_env_fast":
				rp[name] = 180.0
				return rp, 60, true // much slower transient decay
			case "cym_env_tail":
				rp[name] = 35.0
				return rp, 10, true // much slower ring decay
			case "cym_tone_m":
				rp[name] = 0.85
				return rp, 0.2, true // quieter metallic cluster
			case "cym_noise_m":
				rp[name] = 0.45
				return rp, 1.2, true // louder sizzle
			case "cym_noise_d":
				rp[name] = 100.0
				return rp, 20, true // much slower sizzle decay
			}
		case "fm_variant", "fm_base", "fm_pe_amt", "fm_pe_decay",
			"fm_r1", "fm_r2", "fm_r3", "fm_r4",
			"fm_d1", "fm_d2", "fm_d3", "fm_d4",
			"fm_dec1", "fm_dec2", "fm_dec3", "fm_dec4":
			// Phase-7 FM-family voice fields: only have authority when the slot's
			// source is 10. Build an audible lead slot (variant 2 = 3-op preset that
			// reads op1/op2/op3 ratio/depth/decay), then pick a per-field target that
			// moves output. The op4 columns (r4/d4/dec4) and fm_d1 (op0 has no
			// outgoing edge) are structurally inert in every shipped preset, so they
			// seed base==target → the harness skips them ("no distinct target").
			activateSlotFM(rp, k)
			switch field {
			case "fm_variant":
				rp[name] = 2
				return rp, 0, true // lead (3-op) → bass (2-op, different base/routing)
			case "fm_base":
				rp[name] = 220.0
				return rp, 110.0, true // drop an octave
			case "fm_pe_amt":
				rp[name] = 1.5
				return rp, 12.0, true // much deeper pitch sweep
			case "fm_pe_decay":
				rp[name] = 0.04
				return rp, 0.3, true // much slower pitch-env decay
			case "fm_r1":
				rp[name] = 1.0
				return rp, 2.0, true // carrier an octave up
			case "fm_r2":
				rp[name] = 2.0
				return rp, 3.5, true // inharmonic modulator ratio
			case "fm_r3":
				rp[name] = 3.0
				return rp, 5.0, true // higher modulator-of-modulator ratio
			case "fm_d2":
				rp[name] = 3.5
				return rp, 6.0, true // deeper op2→op1 modulation index
			case "fm_d3":
				rp[name] = 2.0
				return rp, 4.0, true // deeper op3→op2 modulation index
			case "fm_dec1":
				rp[name] = 0.2
				return rp, 0.8, true // much slower carrier decay
			case "fm_dec2":
				rp[name] = 0.12
				return rp, 0.5, true // much slower op2 decay
			case "fm_dec3":
				rp[name] = 0.08
				return rp, 0.4, true // much slower op3 decay
			case "fm_r4", "fm_d1", "fm_d4", "fm_dec4":
				// Structurally inert in every shipped preset (no 4th operator; op0
				// has no outgoing edge). Seed base==target so the harness skips.
				return rp, rp[name], true
			}
		default:
			if oscGenFields[field] {
				activateSlotOsc(rp, k)
				return rp, pickNonDefaultValue(d), true
			}
		}
		return rp, pickNonDefaultValue(d), true
	}

	for _, d := range ModularSynthParamDefs() {
		if d.Name == "osc_type" {
			continue // exercised by TestRenderModularP_OscShapesDiffer
		}
		t.Run(d.Name, func(t *testing.T) {
			var base RecipeParams
			var target float64
			if gb, gt, ok := genContext(d); ok {
				// Gen-bank param: rendered in a slot-active context above.
				base = gb
				target = gt
				if target == base[d.Name] {
					t.Skipf("gen param %q has no distinct target in context", d.Name)
				}
				baseBuf := render(base)
				mut := cloneRecipeParams(base)
				mut[d.Name] = target
				if !differs(baseBuf, render(mut)) {
					t.Errorf("modular param %q: %v -> %v did not change output", d.Name, base[d.Name], target)
				}
				return
			}
			switch {
			case d.Name == "drive_enabled":
				base = driveBase()
				target = 0 // bypass the (active) drive stage
			case d.Name == "noise_seed":
				base = noiseBase()
				target = 1 // a different deterministic noise stream
			case isModulatorNumeric(d.Name):
				base = modulatorBase(d.Name)
				target = modulatorTarget[d.Name]
			case isPost(d.Name):
				base = postBase()
				target = postTarget[d.Name]
			case isFM(d.Name):
				base = fmBase()
				target = fmTarget[d.Name]
			default:
				base = sineBase()
				target = pickNonDefaultValue(d)
				if target == d.Default {
					t.Skipf("param %q has Min==Max==Default", d.Name)
				}
			}
			baseBuf := render(base)
			mut := cloneRecipeParams(base)
			mut[d.Name] = target
			if !differs(baseBuf, render(mut)) {
				t.Errorf("modular param %q: %v -> %v did not change output", d.Name, base[d.Name], target)
			}
		})
	}
}

func TestRenderModularP_FilterCutoffChangesTone(t *testing.T) {
	const sr = 48000
	n := sr / 8
	// A saw is harmonically rich; a low LP cutoff should reduce high-freq
	// energy vs a wide-open cutoff. We compare total high-frequency content
	// via a crude first-difference energy measure.
	hfEnergy := func(cutoff float64) float64 {
		p := defaultModularParams()
		p.OscType = 1 // saw
		p.FilterType = 0
		p.FilterCutoff = cutoff
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		var e float64
		for i := 1; i < len(buf); i++ {
			d := float64(buf[i] - buf[i-1])
			e += d * d
		}
		return e
	}
	dark := hfEnergy(300)
	bright := hfEnergy(18000)
	if !(bright > dark) {
		t.Fatalf("LP cutoff did not reduce HF energy: dark=%v bright=%v", dark, bright)
	}
}

// TestModularPadRendersDistinctFromBase — the two shipped modular presets
// must produce different audio through the recipe (edit) path. Rendering
// each recipe with its own shipped defaults proves the pad Seed reaches the
// engine, so an edited modular-pad stays coherent (it does not snap to the
// base preset's tone).
func TestModularPadRendersDistinctFromBase(t *testing.T) {
	const sr = 48000
	n := sr / 4

	render := func(recipeID string) []float32 {
		r := NewRecipe(recipeID)
		if r == nil {
			t.Fatalf("NewRecipe(%q) = nil", recipeID)
		}
		buf := make([]float32, n)
		r.Render(buf, sr, n, 0, RecipeDefaultParams(recipeID))
		assertFinite(t, buf)
		return buf
	}

	base := render("synth-modular")
	pad := render("synth-modular-pad")

	var diff float64
	for i := range base {
		d := float64(base[i] - pad[i])
		diff += d * d
	}
	if diff == 0 {
		t.Fatalf("synth-modular and synth-modular-pad rendered identical buffers; the pad preset is not diverging")
	}
}
