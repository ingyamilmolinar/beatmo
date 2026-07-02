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

func TestLfoTarget_RoundTripsThroughParams(t *testing.T) {
	mp := recipeParamsToModular(map[string]float64{"lfo_target": 2})
	if mp.LfoTarget != 2 {
		t.Fatalf("LfoTarget round-trip = %v, want 2", mp.LfoTarget)
	}
}

func TestLfoTarget_DefaultZeroKeepsAmpLFOByteIdentical(t *testing.T) {
	const sr, n = 48000, 24000
	// An instrument using the amp-wobble LFO at the default target (0).
	base := defaultModularParams()
	base.OscType = 1 // saw
	base.LfoEnabled = 1
	base.LfoRate = 6
	base.LfoDepth = 0.5
	// LfoTarget defaults to 0 (amp). Render it.
	withTarget := base
	withTarget.LfoTarget = 0
	a := make([]float32, n)
	b := make([]float32, n)
	renderModularP(a, sr, n, base)       // field present, zero value
	renderModularP(b, sr, n, withTarget) // explicit 0
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("amp-LFO at target 0 not byte-identical at sample %d: %v vs %v", i, a[i], b[i])
		}
	}
	// And it must actually wobble (sanity: not silent / not flat).
	var mn, mx float32 = a[0], a[0]
	for _, v := range a {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	if mx-mn < 0.01 {
		t.Fatalf("amp-LFO produced near-flat output (range %v)", mx-mn)
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
		"lfo_rate": 40, "lfo_depth": 0, "lfo_target": 1, "lfo_delay": 3,
		"burst_sharp": 200,
		"burst1_off":  0.25, "burst1_amp": 0,
		"burst2_off": 0.25, "burst2_amp": 0,
		"burst3_off": 0.25, "burst3_amp": 1,
		"burst4_off": 0.25, "burst4_amp": 1,
		"filtenv_amt": 0, "filtenv_decay": 2, "filtenv_attack": 0.8,
		"body_model": 0, "body_mix": 1.3, "bow_dynamics": 0.8,
		// Phase-8E unison: detune/mix only have authority when voices>=2.
		"unison_detune": 30, "unison_mix": 0,
		// Phase-8F drift: both params only have effect when voices>=2 and the
		// complementary param is >0 (seeded in modulatorBase below).
		"unison_drift_rate": 6, "unison_drift_depth": 15,
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
		case strings.HasPrefix(name, "filtenv_"):
			// Activate the filter envelope stage with a saw + low LP cutoff so
			// the cutoff modulation is audible, then enable filtenv.
			rp["osc_type"] = 1         // saw (rich harmonics)
			rp["env_enabled"] = 0      // steady output for clean comparison
			rp["filter_enabled"] = 1   // filter on
			rp["filter_cutoff"] = 1200 // low cutoff so amt has room to brighten
			rp["filtenv_enabled"] = 1  // stage on
			rp["filtenv_amt"] = 2      // default seed amt
			rp["filtenv_decay"] = 0.3  // default seed decay
		case strings.HasPrefix(name, "bow_"):
			rp["osc_type"] = 1 // saw (energy for the bow modulator to shape)
			rp["env_enabled"] = 0
		case strings.HasPrefix(name, "body_"):
			// Body-resonator bank: body_model and body_mix are interdependent
			// (the bank runs only when model>=1 AND mix>0). Activate both with a
			// saw so toggling/mixing the bank audibly changes the filtered output.
			rp["osc_type"] = 1    // saw (rich harmonics for the modes to shape)
			rp["env_enabled"] = 0 // steady output for clean comparison
			rp["body_model"] = 1  // violin bank on
			rp["body_mix"] = 0.6  // baseline wet
		case strings.HasPrefix(name, "unison_"):
			// Unison params only have authority when voices>=2. Activate with a
			// saw (harmonically rich) so detuning and mix changes are audible.
			rp["osc_type"] = 1       // saw
			rp["unison_voices"] = 3  // enable unison path
			rp["unison_detune"] = 15 // seed detune so mix changes are audible
			rp["unison_mix"] = 0.7   // seed mix so detune changes are audible
			// Drift params require BOTH rate>0 AND depth>0 for effect. Seed the
			// complementary param so that mutating the one under test has effect.
			if name == "unison_drift_rate" {
				rp["unison_drift_depth"] = 10 // seed depth so rate mutation registers
			}
			if name == "unison_drift_depth" {
				rp["unison_drift_rate"] = 2 // seed rate so depth mutation registers
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
		// Phase-9 structural extras seeded to the base-kick literals (a 0
		// gen_kick_sat would silence the voice — kp_get only falls back to the
		// variant literal at NaN, not at the schema-identity 0).
		rp[p("kick_attack")] = 0.3
		rp[p("kick_fade")] = 4.0
		rp[p("kick_sat")] = 0.55
		rp["kick_enabled"] = 1 // gate the source==5 voice on
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
		if name == "kick_enabled" {
			// Global KICK-stage enable: only has authority with the source==5
			// voice active. Base = on (audible kick), target = off (silence).
			rp := sineBase()
			activateSlotKick(rp, 1)
			return rp, 0, true
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
		case "ks_sustain", "ks_pluck", "ks_blow":
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
			case "ks_blow":
				rp[name] = 0         // base: plucked (one-shot decay)
				return rp, 0.6, true // target: blown (continuous-excitation tube) — audibly different
			}
		case "kick_variant", "kick_h2", "kick_h3", "kick_h4", "kick_env0",
			"kick_env1", "kick_pe_amt", "kick_pe_rate", "kick_click", "kick_noise",
			"kick_attack", "kick_fade", "kick_sat":
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
			case "kick_attack":
				rp[name] = 0.3
				return rp, 2.0, true // stronger attack-boost transient
			case "kick_fade":
				rp[name] = 4.0
				return rp, 16, true // much tighter global-fade tail
			case "kick_sat":
				rp[name] = 0.55
				return rp, 1.4, true // more saturation drive
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

func TestLfoTarget_PitchVibratoBendsAndIsIdentityAtDepthZero(t *testing.T) {
	const sr, n = 48000, 48000
	mk := func(target, depth float64) []float32 {
		p := defaultModularParams()
		p.OscType = 0 // sine — easy to measure pitch via zero crossings
		p.FilterEnabled = 0
		p.EnvEnabled = 0
		p.LfoEnabled = 1
		p.LfoRate = 6
		p.LfoDepth = depth
		p.LfoTarget = target
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}
	plain := mk(1, 0) // vibrato selected but depth 0 → must equal no-LFO baseline
	base := defaultModularParams()
	base.OscType = 0
	base.FilterEnabled = 0
	base.EnvEnabled = 0
	baseBuf := make([]float32, n)
	renderModularP(baseBuf, sr, n, base)
	for i := range plain {
		if plain[i] != baseBuf[i] {
			t.Fatalf("vibrato at depth 0 not identity at %d: %v vs %v", i, plain[i], baseBuf[i])
		}
	}
	vib := mk(1, 0.5) // ±0.5 st vibrato → output must differ from baseline
	diff := 0
	for i := range vib {
		if vib[i] != baseBuf[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("vibrato at depth 0.5 produced no change vs baseline")
	}
}

func TestModBiquadCoeffOnly_FilterStaysFinite(t *testing.T) {
	const sr, n = 48000, 24000
	p := defaultModularParams()
	p.OscType = 1 // saw
	p.FilterEnabled = 1
	p.FilterType = 0 // low-pass
	p.FilterCutoff = 2000
	p.LfoEnabled = 1
	p.LfoRate = 5
	p.LfoDepth = 0.5
	p.LfoTarget = 2 // cutoff sweep — drives the coeff-only path (Task 4)
	buf := make([]float32, n)
	renderModularP(buf, sr, n, p)
	for i, v := range buf {
		if v != v || v > 4 || v < -4 {
			t.Fatalf("non-finite/exploded filter output at %d: %v", i, v)
		}
	}
}

func TestLfoTarget_CutoffSweepChangesFilteredOutput(t *testing.T) {
	const sr, n = 48000, 48000

	// mkSweep builds a saw through an enabled LP filter with the cutoff-sweep
	// LFO (lfo_target=2). Only lfo_depth varies between calls so amplitude
	// behaviour is identical across all renders (amp-tremolo is gated to
	// target==0 and is OFF here for every depth value).
	mkSweep := func(depth float64) []float32 {
		p := defaultModularParams()
		p.OscType = 1    // saw — harmonically rich so cutoff changes are audible
		p.EnvEnabled = 0 // eliminate amp-envelope variation
		p.FilterEnabled = 1
		p.FilterType = 0 // low-pass
		p.FilterCutoff = 2000
		p.LfoEnabled = 1
		p.LfoRate = 4
		p.LfoDepth = depth
		p.LfoTarget = 2 // cutoff sweep; amp-tremolo is gated OFF at this target
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}

	// Assertion (a): depth 0.6 must differ from depth 0 — proves the sweep changes output.
	shallow := mkSweep(0)
	deep := mkSweep(0.6)
	diff := 0
	for i := range shallow {
		if deep[i] != shallow[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("cutoff sweep at depth 0.6 produced no change vs depth 0 (lfo_target=2 both)")
	}

	// Assertion (b): depth-0 with lfo_target=2 must be byte-identical to a
	// no-LFO static-filter baseline (same params, lfo_enabled=0). This proves
	// depth-0 is a true no-op and that the sweep — not an amplitude side-effect
	// — is what assertion (a) measures.
	noLFO := defaultModularParams()
	noLFO.OscType = 1
	noLFO.EnvEnabled = 0
	noLFO.FilterEnabled = 1
	noLFO.FilterType = 0
	noLFO.FilterCutoff = 2000
	noLFO.LfoEnabled = 0 // LFO completely off
	noLFOBuf := make([]float32, n)
	renderModularP(noLFOBuf, sr, n, noLFO)
	for i := range shallow {
		if shallow[i] != noLFOBuf[i] {
			t.Fatalf("assertion (b) FAILED: depth-0 cutoff-sweep (target=2) not byte-identical to no-LFO baseline at sample %d: %v vs %v", i, shallow[i], noLFOBuf[i])
		}
	}

	// Finiteness check on the swept render.
	for i, v := range deep {
		if v != v || v > 4 || v < -4 {
			t.Fatalf("cutoff sweep non-finite at %d: %v", i, v)
		}
	}
}

func TestFiltEnv_BrightenThenSettleAndIdentityWhenOff(t *testing.T) {
	const sr, n = 48000, 48000
	mk := func(enabled, amt float64) []float32 {
		p := defaultModularParams()
		p.OscType = 1 // saw (rich harmonics so cutoff motion is audible)
		p.EnvEnabled = 0
		p.FilterEnabled = 1
		p.FilterType = 0
		p.FilterCutoff = 1200
		p.FiltEnvEnabled = enabled
		p.FiltEnvAmt = amt
		p.FiltEnvDecay = 0.2
		buf := make([]float32, n)
		renderModularP(buf, sr, n, p)
		return buf
	}
	off := mk(0, 3) // disabled → identity (amt ignored)
	base := defaultModularParams()
	base.OscType = 1
	base.EnvEnabled = 0
	base.FilterEnabled = 1
	base.FilterType = 0
	base.FilterCutoff = 1200
	baseBuf := make([]float32, n)
	renderModularP(baseBuf, sr, n, base)
	for i := range off {
		if off[i] != baseBuf[i] {
			t.Fatalf("filtenv disabled not byte-identical at %d", i)
		}
	}
	on := mk(1, 3) // enabled → differs (brighter attack)
	diff := 0
	for i := range on {
		if on[i] != baseBuf[i] {
			diff++
		}
	}
	if diff == 0 {
		t.Fatal("filter envelope produced no change")
	}
	// Attack window should be brighter (more HF energy = larger sample-to-sample delta) than the tail.
	d := func(b []float32, a, z int) float64 {
		var s float64
		for i := a + 1; i < z; i++ {
			s += math.Abs(float64(b[i] - b[i-1]))
		}
		return s
	}
	if d(on, 0, 2000) <= d(on, 40000, 42000) {
		t.Fatal("filter envelope attack not brighter than tail")
	}
}

// ── Phase-8E unison tests ──────────────────────────────────────────────────

func renderSawWithUnison(voices int, detune, mix float64) []float32 {
	const sr, n = 48000, 24000
	mp := defaultModularParams()
	mp.OscType = 1 // saw
	mp.UnisonVoices = float64(voices)
	mp.UnisonDetune = detune
	mp.UnisonMix = mix
	buf := make([]float32, n)
	renderModularP(buf, sr, n, mp)
	return buf
}

// TestUnisonVoices1ByteIdentical verifies that unison_voices=1 produces
// byte-identical output to the baseline (no unison field set at all).
func TestUnisonVoices1ByteIdentical(t *testing.T) {
	const sr, n = 48000, 24000
	mp := defaultModularParams()
	mp.OscType = 1 // saw

	baseline := make([]float32, n)
	renderModularP(baseline, sr, n, mp)

	mp.UnisonVoices = 1
	mp.UnisonDetune = 20
	mp.UnisonMix = 0.5
	withUnison1 := make([]float32, n)
	renderModularP(withUnison1, sr, n, mp)

	for i, v := range withUnison1 {
		if v != baseline[i] {
			t.Fatalf("voices=1 sample[%d]: got %v, want %v (not byte-identical)", i, v, baseline[i])
		}
	}
}

// TestUnisonVoices5Differs verifies that voices=5 with detune=20 produces
// output that differs from voices=1 and remains finite and bounded.
func TestUnisonVoices5Differs(t *testing.T) {
	voices1 := renderSawWithUnison(1, 20, 0.5)
	voices5 := renderSawWithUnison(5, 20, 0.5)

	assertFinite(t, voices5)

	// Must differ from voices=1.
	same := true
	for i, v := range voices5 {
		if v != voices1[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("voices=5,detune=20 produced byte-identical output to voices=1 (no detuning applied)")
	}

	// Bounded: no sample should exceed ±2.0 (well within any sane headroom).
	pk := peakAbs(voices5)
	if pk > 2.0 {
		t.Fatalf("voices=5 peak %v exceeds ±2.0 bound", pk)
	}
	// Must have non-silent output.
	if pk <= 0 {
		t.Fatal("voices=5 output is silent")
	}
}

// TestUnisonDetune0Sanity verifies voices=3,detune=0,mix=1 stays bounded and
// is non-silent (when detune=0 voices sum in-phase → louder but not unbounded).
func TestUnisonDetune0Sanity(t *testing.T) {
	voices3det0 := renderSawWithUnison(3, 0, 1)
	assertFinite(t, voices3det0)
	pk := peakAbs(voices3det0)
	if pk <= 0 {
		t.Fatal("voices=3,detune=0 output is silent")
	}
	// 1/sqrt(3) normalization keeps things bounded even with in-phase sum.
	if pk > 2.0 {
		t.Fatalf("voices=3,detune=0 peak %v exceeds ±2.0 bound", pk)
	}
}

// TestUnisonGenBankSource1Differs verifies unison also works for gen-bank
// source==1 (wt_osc slots), producing different output when voices>1.
func TestUnisonGenBankSource1Differs(t *testing.T) {
	const sr, n = 48000, 24000

	baseMP := ModularParams{
		OscEnabled:    0, // disable legacy OSC; use gen bank only
		EnvEnabled:    1,
		FilterEnabled: 1,
		FilterCutoff:  8000, FilterResonance: 0.707,
		AmpAttack: 0.005, AmpDecay: 0.3, AmpSustain: 0.6, AmpRelease: 0.2, AmpCurve: 1,
		Gain: 1,
	}
	// slot 0: source=1 (wt_osc), freq_mode=0 (ratio), freq=1, gain=1
	baseMP.GenSource[0] = 1
	baseMP.GenFreqMode[0] = 0
	baseMP.GenFreq[0] = 1
	baseMP.GenGain[0] = 1
	baseMP.GenEnvFastMix[0] = 1
	baseMP.GenEnvFastRate[0] = 0

	baseline := make([]float32, n)
	renderModularP(baseline, sr, n, baseMP)

	withUnison := baseMP
	withUnison.UnisonVoices = 3
	withUnison.UnisonDetune = 15
	withUnison.UnisonMix = 0.8
	unisonBuf := make([]float32, n)
	renderModularP(unisonBuf, sr, n, withUnison)

	assertFinite(t, unisonBuf)

	same := true
	for i, v := range unisonBuf {
		if v != baseline[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("gen-bank source==1 voices=3 produced byte-identical output to voices=1")
	}
}

// ── Unison drift / voices=2 bug tests (Phase-8F) ───────────────────────────

// TestUnison_TwoVoicesDetune verifies that voices=2 with detune>0 actually
// detunes the side voice (the bug: ns==1 gave spread=0 → side voice at exact
// center pitch, not detuned). We verify by comparing voices=2,detune=25 against
// voices=2,detune=0: detuned should differ (frequency difference) while with
// the bug both land at center pitch so the only difference is phase (and
// phase-only 2-voice is the SAME as 2-voice detune=0).
func TestUnison_TwoVoicesDetune(t *testing.T) {
	v2det25 := renderSawWithUnison(2, 25, 1.0) // mix=1 = full ensemble
	v2det0 := renderSawWithUnison(2, 0, 1.0)   // mix=1, no detune
	assertFinite(t, v2det25)
	assertFinite(t, v2det0)
	// With proper spread math, detune=25 should differ from detune=0 because
	// the side voice frequency actually changes. With the bug (spread=0 always
	// for ns==1), both produce center+phase_offset, so output is identical.
	same := true
	for i, s := range v2det25 {
		if s != v2det0[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("unison_voices=2 detune=25 is identical to detune=0 (spread=0 bug: side voice stuck at center pitch)")
	}
}

// TestUnison_DriftIdentityAndEffect verifies:
// (a) drift_depth=0 is byte-identical to no drift fields set (identity).
// (b) drift_rate>0 && drift_depth>0 produces different output (drift has effect).
func TestUnison_DriftIdentityAndEffect(t *testing.T) {
	const sr, n = 48000, 24000

	noDrift := func() []float32 {
		mp := defaultModularParams()
		mp.OscType = 1 // saw
		mp.UnisonVoices = 5
		mp.UnisonDetune = 20
		mp.UnisonMix = 0.7
		buf := make([]float32, n)
		renderModularP(buf, sr, n, mp)
		return buf
	}
	driftOff := func() []float32 {
		mp := defaultModularParams()
		mp.OscType = 1 // saw
		mp.UnisonVoices = 5
		mp.UnisonDetune = 20
		mp.UnisonMix = 0.7
		mp.UnisonDriftRate = 0
		mp.UnisonDriftDepth = 0
		buf := make([]float32, n)
		renderModularP(buf, sr, n, mp)
		return buf
	}
	driftOn := func() []float32 {
		mp := defaultModularParams()
		mp.OscType = 1 // saw
		mp.UnisonVoices = 5
		mp.UnisonDetune = 20
		mp.UnisonMix = 0.7
		mp.UnisonDriftRate = 2
		mp.UnisonDriftDepth = 10
		buf := make([]float32, n)
		renderModularP(buf, sr, n, mp)
		return buf
	}

	base := noDrift()
	off := driftOff()
	on := driftOn()

	assertFinite(t, off)
	assertFinite(t, on)

	// (a) drift_depth=0 must be byte-identical to no drift fields.
	for i, v := range off {
		if v != base[i] {
			t.Fatalf("drift identity broken: drift_depth=0 differs from no-drift at sample %d (got %v, want %v)", i, v, base[i])
		}
	}

	// (b) drift on must differ from drift off.
	same := true
	for i, v := range on {
		if v != off[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("drift has no effect: drift_rate=2 drift_depth=10 produced byte-identical output to drift-off")
	}

	// (b) drift output must be bounded.
	for i, v := range on {
		if v > 4.0 || v < -4.0 {
			t.Fatalf("drift output unbounded at sample %d: %v", i, v)
		}
	}
}

func TestLfoDelay_IdentityAndRampsIn(t *testing.T) {
	const sr = 48000
	n := sr * 2 // 2 seconds

	// Base params: vibrato LFO enabled (target=1 = pitch vibrato for identity/early test)
	base := defaultModularParams()
	base.OscType = 1 // saw
	base.LfoEnabled = 1
	base.LfoRate = 6
	base.LfoDepth = 0.3
	base.LfoTarget = 1 // pitch vibrato

	// (a) Identity: lfo_delay=0 must be byte-identical to a render where LfoDelay is absent (zero-value).
	withZeroDelay := base
	withZeroDelay.LfoDelay = 0
	noDelay := make([]float32, n)
	withDelayZero := make([]float32, n)
	renderModularP(noDelay, sr, n, base)
	renderModularP(withDelayZero, sr, n, withZeroDelay)
	for i := range noDelay {
		if noDelay[i] != withDelayZero[i] {
			t.Fatalf("identity broken: lfo_delay=0 not byte-identical to absent field at sample %d (%v vs %v)", i, noDelay[i], withDelayZero[i])
		}
	}

	// (b) With lfo_delay=0.5: early window (first 0.25s) must differ from no-delay.
	withDelay := base
	withDelay.LfoDelay = 0.5
	delayed := make([]float32, n)
	renderModularP(delayed, sr, n, withDelay)

	earlyN := sr / 4 // first 0.25s
	var earlyDiff float64
	for i := 0; i < earlyN; i++ {
		d := float64(noDelay[i] - delayed[i])
		earlyDiff += d * d
	}
	if earlyDiff < 1e-6 {
		t.Fatalf("lfo_delay=0.5: early window identical to no-delay (earlyDiff=%v), delay ramp not applied", earlyDiff)
	}

	// (c) Late-window convergence: use amp tremolo (target=0) which is stateless
	// (per-sample multiply, no oscillator phase accumulation), so once ramp=1.0
	// the output is sample-identical to the no-delay render.
	// Render the same params with amp LFO (target=0).
	baseAmp := defaultModularParams()
	baseAmp.OscType = 1 // saw
	baseAmp.LfoEnabled = 1
	baseAmp.LfoRate = 6
	baseAmp.LfoDepth = 0.3
	baseAmp.LfoTarget = 0 // amp tremolo (stateless post-mix multiply)

	noDelayAmp := make([]float32, n)
	renderModularP(noDelayAmp, sr, n, baseAmp)

	delayedAmp := baseAmp
	delayedAmp.LfoDelay = 0.5
	delayedAmpBuf := make([]float32, n)
	renderModularP(delayedAmpBuf, sr, n, delayedAmp)

	// Late window (t>1.5s): ramp completed at 0.5s, so output must be identical.
	lateStart := int(1.5 * float64(sr))
	lateN := n - lateStart
	var lateDiff float64
	for i := lateStart; i < n; i++ {
		d := float64(noDelayAmp[i] - delayedAmpBuf[i])
		lateDiff += d * d
	}
	lateAvgDiff := lateDiff / float64(lateN)
	if lateAvgDiff > 1e-10 {
		t.Fatalf("lfo_delay=0.5 amp-tremolo: late window (t>1.5s) not identical to no-delay (avgDiff=%v), ramp did not converge to 1.0", lateAvgDiff)
	}
}

// TestVibrato_ReachesGenSlots verifies that pitch vibrato (lfo_target==1) reaches
// gen-bank wavetable-osc slots when the legacy OSC stage is disabled (osc_enabled:0).
// Additive instruments like violin/cello carry all their tone in gen-bank slots and
// had no vibrato before the fix. This test must FAIL before the C fix and PASS after.
func TestVibrato_ReachesGenSlots(t *testing.T) {
	const sr, n = 48000, 48000

	// Gen-slot-only voice: legacy OSC disabled, single wavetable-osc gen slot.
	mkGenOnly := func(lfoDepth float64) []float32 {
		mp := ModularParams{
			OscEnabled:    0, // legacy OSC stage off — tone lives in gen bank only
			EnvEnabled:    1,
			FilterEnabled: 0,
			AmpAttack:     0.005,
			AmpDecay:      2.0,
			AmpSustain:    0.8,
			AmpRelease:    0.2,
			AmpCurve:      1,
			Gain:          1,
			// LFO: pitch vibrato
			LfoEnabled: 1,
			LfoTarget:  1, // pitch vibrato
			LfoRate:    6,
			LfoDepth:   lfoDepth,
		}
		// slot 0: source=1 (wavetable osc), sine wave, ratio freq=1 (voice_freq), gain=1
		mp.GenSource[0] = 1   // wavetable osc
		mp.GenWave[0] = 0     // sine
		mp.GenFreqMode[0] = 0 // ratio (freq × voice_freq)
		mp.GenFreq[0] = 1     // ratio 1× (fundamental)
		mp.GenGain[0] = 1
		mp.GenEnvFastMix[0] = 0
		mp.GenEnvTailMix[0] = 1
		mp.GenEnvTailRate[0] = 0.5 // slow decay so there's sustained output to compare

		buf := make([]float32, n)
		renderModularP(buf, sr, n, mp)
		return buf
	}

	withVibrato := mkGenOnly(0.4)    // vibrato on: should modulate gen slot pitch
	withoutVibrato := mkGenOnly(0.0) // no vibrato: flat pitch

	assertFinite(t, withVibrato)
	assertFinite(t, withoutVibrato)

	// The gen slot must produce non-silent output.
	if peakAbs(withoutVibrato) <= 0 {
		t.Fatal("gen-slot-only voice is silent (env/params misconfigured in test)")
	}

	// With vibrato active, the pitch must wobble: the two renders must differ.
	same := true
	for i, v := range withVibrato {
		if v != withoutVibrato[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("vibrato (lfo_target=1, depth=0.4) had NO effect on gen-bank wavetable-osc slot output — vibrato is not routed to gen slots (bug)")
	}
}

// TestVibrato_GenSlotIdentityWhenOff verifies byte-identity when vibrato is
// completely disabled (lfo_enabled=0) for a gen-slot-only voice. This must pass
// both before and after the C fix (no regression on the inactive path).
func TestVibrato_GenSlotIdentityWhenOff(t *testing.T) {
	const sr, n = 48000, 48000

	mkGenNoLFO := func(lfoEnabled float64) []float32 {
		mp := ModularParams{
			OscEnabled:    0,
			EnvEnabled:    1,
			FilterEnabled: 0,
			AmpAttack:     0.005,
			AmpDecay:      2.0,
			AmpSustain:    0.8,
			AmpRelease:    0.2,
			AmpCurve:      1,
			Gain:          1,
			LfoEnabled:    lfoEnabled, // 0 = LFO completely off
			LfoTarget:     1,
			LfoRate:       6,
			LfoDepth:      0.4,
		}
		mp.GenSource[0] = 1
		mp.GenWave[0] = 0
		mp.GenFreqMode[0] = 0
		mp.GenFreq[0] = 1
		mp.GenGain[0] = 1
		mp.GenEnvFastMix[0] = 0
		mp.GenEnvTailMix[0] = 1
		mp.GenEnvTailRate[0] = 0.5

		buf := make([]float32, n)
		renderModularP(buf, sr, n, mp)
		return buf
	}

	lfoOff := mkGenNoLFO(0) // lfo_enabled=0 → no vibrato
	// lfo_enabled=0 with all LFO fields set should be byte-identical to a render
	// where LFO fields are all zero (the gate must short-circuit before any per-sample work).
	mpZeroLFO := ModularParams{
		OscEnabled:    0,
		EnvEnabled:    1,
		FilterEnabled: 0,
		AmpAttack:     0.005,
		AmpDecay:      2.0,
		AmpSustain:    0.8,
		AmpRelease:    0.2,
		AmpCurve:      1,
		Gain:          1,
		// LFO fields all zero (default)
	}
	mpZeroLFO.GenSource[0] = 1
	mpZeroLFO.GenWave[0] = 0
	mpZeroLFO.GenFreqMode[0] = 0
	mpZeroLFO.GenFreq[0] = 1
	mpZeroLFO.GenGain[0] = 1
	mpZeroLFO.GenEnvFastMix[0] = 0
	mpZeroLFO.GenEnvTailMix[0] = 1
	mpZeroLFO.GenEnvTailRate[0] = 0.5

	baseline := make([]float32, n)
	renderModularP(baseline, sr, n, mpZeroLFO)

	for i, v := range lfoOff {
		if v != baseline[i] {
			t.Fatalf("gen-slot with lfo_enabled=0 not byte-identical to zero-LFO baseline at sample %d: got %v, want %v", i, v, baseline[i])
		}
	}
}
