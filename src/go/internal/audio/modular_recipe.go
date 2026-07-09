package audio

import "fmt"

// ModularSynthParamDefs returns the full editable parameter set of the unified
// modular synth voice (src/c/modular.h). Unlike the bespoke drum/FM recipes —
// which expose only the wired subset of the generic post-process knobs — the
// modular recipe surfaces its ENTIRE signal path: oscillator/generator, FM
// operators, full amp ADSR, filter, and post. Discrete stages carry Enum
// labels (the UI renders a selector); every other param is a knob.
//
// Defaults MUST equal ModularParamSchemaIdentity() (and therefore the C
// mp_get NULL-fallbacks in modular.c), so an unedited modular instrument
// renders identically whether it goes through the unparameterized
// render_modular fast path or the parameterized recipe path. The ordering
// mirrors modularParamSchema so the ABI index map and the UI param order agree.
func ModularSynthParamDefs() []ParamDef {
	defs := []ParamDef{
		// Oscillator stage.
		{Name: "osc_type", Label: "Oscillator", Group: "osc", Min: 0, Max: 12, Default: 0,
			Enum: []string{"Sine", "Saw", "Square", "Triangle", "FM", "Noise White", "Noise Pink", "Bowed String", "Brass", "Reed", "Flute", "Sax", "Voice"}},
		{Name: "osc_detune", Label: "Detune", Group: "osc", Min: -100, Max: 100, Default: 0, Unit: "cents"},
		{Name: "osc_octave", Label: "Octave", Group: "osc", Min: -2, Max: 2, Default: 0, Step: 1},

		// FM stage (used when osc_type == FM).
		{Name: "fm_algorithm", Label: "Algorithm", Group: "fm", Min: 0, Max: 3, Default: 0,
			Enum: []string{"2-op", "Parallel", "3-op chain", "4-op stack"}},
		{Name: "fm_op1_ratio", Label: "Op1 Ratio", Group: "fm", Min: 0.5, Max: 8, Default: 1},
		{Name: "fm_op2_ratio", Label: "Op2 Ratio", Group: "fm", Min: 0.5, Max: 8, Default: 1},
		{Name: "fm_op3_ratio", Label: "Op3 Ratio", Group: "fm", Min: 0.5, Max: 8, Default: 1},
		{Name: "fm_op4_ratio", Label: "Op4 Ratio", Group: "fm", Min: 0.5, Max: 8, Default: 1},
		{Name: "fm_op1_depth", Label: "Op1 Depth", Group: "fm", Min: 0, Max: 8, Default: 0},
		{Name: "fm_op2_depth", Label: "Op2 Depth", Group: "fm", Min: 0, Max: 8, Default: 0},
		{Name: "fm_op3_depth", Label: "Op3 Depth", Group: "fm", Min: 0, Max: 8, Default: 0},
		{Name: "fm_op4_depth", Label: "Op4 Depth", Group: "fm", Min: 0, Max: 8, Default: 0},
		{Name: "fm_op1_level", Label: "Op1 Level", Group: "fm", Min: 0, Max: 1, Default: 1},
		{Name: "fm_op2_level", Label: "Op2 Level", Group: "fm", Min: 0, Max: 1, Default: 0},
		{Name: "fm_op3_level", Label: "Op3 Level", Group: "fm", Min: 0, Max: 1, Default: 0},
		{Name: "fm_op4_level", Label: "Op4 Level", Group: "fm", Min: 0, Max: 1, Default: 0},

		// Amplitude ADSR.
		{Name: "amp_attack", Label: "Attack", Group: "env", Min: 0, Max: 2, Default: 0.005, Unit: "s"},
		{Name: "amp_decay", Label: "Decay", Group: "env", Min: 0, Max: 4, Default: 0.3, Unit: "s"},
		{Name: "amp_sustain", Label: "Sustain", Group: "env", Min: 0, Max: 1, Default: 0.6},
		{Name: "amp_release", Label: "Release", Group: "env", Min: 0, Max: 4, Default: 0.2, Unit: "s"},
		{Name: "amp_curve", Label: "Curve", Group: "env", Min: 0, Max: 1, Default: 1,
			Enum: []string{"Linear", "Exponential"}},

		// Filter stage.
		{Name: "filter_type", Label: "Filter", Group: "filter", Min: 0, Max: 2, Default: 0,
			Enum: []string{"Low-pass", "High-pass", "Band-pass"}},
		{Name: "filter_cutoff", Label: "Cutoff", Group: "filter", Min: 20, Max: 20000, Default: 8000, Unit: "Hz", Curve: "log"},
		{Name: "filter_resonance", Label: "Resonance", Group: "filter", Min: 0.5, Max: 16, Default: 0.707},

		// Post stage.
		{Name: "drive", Label: "Drive", Group: "post", Min: 0, Max: 1, Default: 0},
		{Name: "pitch", Label: "Pitch", Group: "post", Min: -24, Max: 24, Default: 0, Unit: "st"},
		{Name: "gain", Label: "Gain", Group: "post", Min: 0, Max: 1.5, Default: 1},

		// Per-stage bypass toggles. Default On (1) so an unedited voice is
		// byte-identical. Rendered as a per-section enable pill in the Synth
		// tab (NOT as knobs) — see enableParamForSection in synth_panel_zone.go.
		// Order MUST mirror modularParamSchema (appended after gain).
		{Name: "osc_enabled", Label: "Osc", Group: "osc", Min: 0, Max: 1, Default: 1,
			Enum: []string{"Off", "On"}},
		{Name: "fm_enabled", Label: "FM", Group: "fm", Min: 0, Max: 1, Default: 1,
			Enum: []string{"Off", "On"}},
		{Name: "env_enabled", Label: "Env", Group: "env", Min: 0, Max: 1, Default: 1,
			Enum: []string{"Off", "On"}},
		{Name: "filter_enabled", Label: "Filter", Group: "filter", Min: 0, Max: 1, Default: 1,
			Enum: []string{"Off", "On"}},
		{Name: "drive_enabled", Label: "Drive", Group: "post", Min: 0, Max: 1, Default: 1,
			Enum: []string{"Off", "On"}},

		// Engine-internal deterministic noise seed (set per round-robin variant
		// by nativeModularRecipe.Render). Hidden from the UI; present here only
		// to keep ModularSynthParamDefs 1:1 with modularParamSchema/the ABI.
		{Name: "noise_seed", Label: "Noise Seed", Group: SynthHiddenGroup, Min: 0, Max: 1024, Default: 0},
	}

	// ── Phase-1 gen bank: hidden until Phase 8's GEN section UI. Bounds are
	// engine bounds (the UI shows nothing yet); defaults are the schema
	// identities so untouched presets render byte-identically. ──
	defs = append(defs,
		ParamDef{Name: "noise_draws", Label: "Noise Draws", Group: SynthHiddenGroup, Min: 1, Max: 4, Default: 1},
		ParamDef{Name: "noise_prelude", Label: "Noise Prelude", Group: SynthHiddenGroup, Min: 0, Max: 16, Default: 0},
	)
	genBounds := map[string][2]float64{
		// source ceiling tracks the highest implemented gen_source voice (10=FM
		// family; 5=kick). A stale max here CLAMPS a kick/tom/snare/cymbal/FM
		// seed's gen_source down into the analytic-voice range and silently
		// renders the wrong (or no) voice — see the DnB kick clamp bug.
		"source": {0, 10}, "wave": {0, 3}, "freq_mode": {0, 1}, "freq": {0, 20000}, "gain": {0, 4},
		"env_fast_rate": {0, 2000}, "env_tail_rate": {0, 2000}, "env_fast_mix": {0, 4}, "env_tail_mix": {0, 4},
		"filt_type": {0, 5}, "filt_alpha": {0, 1}, "filt_freq": {20, 20000}, "filt_q": {0.1, 16},
		"phase_mode": {0, 2}, "phase": {0, 16}, "noise_offset": {0, 3},
		// Phase-2 (bass) analytic-voice per-slot fields.
		"pitch_env_amt": {0, 4}, "pitch_env_rate": {0, 2000}, "harm_mix": {0, 4},
		"atk_amt": {0, 4}, "atk_rate": {0, 2000}, "sat_k": {0, 8}, "out_scale": {0, 4},
	}
	ident := ModularParamSchemaIdentity()
	for _, f := range modularGenSlotFields {
		b := genBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			defs = append(defs, ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name],
			})
		}
	}

	// ── Phase-2 globals: voice-freq override + shared POST stage. Hidden until
	// a later UI phase; defaults are the schema identities (no-op) so untouched
	// presets render byte-identically. Order MUST mirror modularGlobalsPhase2
	// (which mirrors the C struct tail). ──
	globalBounds := map[string][2]float64{
		"voice_freq_hz": {0, 20000}, "post_enabled": {0, 1},
		"post_pitch": {-24, 24}, "post_decay": {0, 4}, "post_decay_rate": {0, 64},
		"post_body": {0, 1}, "post_brightness": {0, 1}, "post_tone": {-1, 1}, "post_drive": {0, 1},
		"post_pitch_on": {0, 1}, "post_decay_on": {0, 1}, "post_body_on": {0, 1},
		"post_brightness_on": {0, 1}, "post_tone_on": {0, 1}, "post_drive_on": {0, 1},
	}
	for _, g := range modularGlobalsPhase2 {
		b := globalBounds[g.Name]
		defs = append(defs, ParamDef{
			Name: g.Name, Label: g.Name, Group: SynthHiddenGroup,
			Min: b[0], Max: b[1], Default: ident[g.Name],
		})
	}

	// ── Phase-2 Task-2 KS per-slot fields (source==3): hidden until Phase 8.
	// Appended at the very tail to mirror modularGenSlotFieldsPhase2KS (which
	// mirrors the C struct tail). Defaults are the schema identities (no-op). ──
	ksBounds := map[string][2]float64{
		"ks_sustain": {0, 1}, "ks_pluck": {0, 1}, "ks_blow": {0, 2},
	}
	for _, f := range modularGenSlotFieldsPhase2KS {
		b := ksBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			defs = append(defs, ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name],
			})
		}
	}

	// ── Phase-3 kick per-slot fields (source==5): hidden until Phase 8.
	// Appended at the new very tail to mirror modularGenSlotFieldsPhase3Kick
	// (which mirrors the C struct tail). Defaults are the schema identities
	// (no-op). ──
	// Min 0 throughout so the schema-identity default (0) sits inside the
	// bounds (these hidden defs exist for ABI completeness only; the source==5
	// voice reads the live kick knobs, not these). Max = the engine ceiling.
	kickBounds := map[string][2]float64{
		"kick_variant": {0, 7}, // 5 = layered hybrid; 6 = modal (2-mode); 7 = acoustic (inharmonic modal bank)
		"kick_h2":      {0, 2}, "kick_h3": {0, 1}, "kick_h4": {0, 1},
		"kick_env0": {0, 40}, "kick_env1": {0, 40},
		"kick_pe_amt": {0, 3}, "kick_pe_rate": {0, 100},
		"kick_click": {0, 1}, "kick_noise": {0, 1},
	}
	// kickStageMeta = the friendly UI label (and enum for the variant selector)
	// for each slot-1 kick knob surfaced in the visible KICK stage.
	kickStageMeta := map[string]struct {
		label string
		enum  []string
	}{
		"kick_variant": {"Variant", []string{"Base", "Deep", "Punchy", "Lo-Fi", "Tight", "Hybrid", "Modal", "Acoustic"}},
		"kick_h2":      {"2nd Harmonic", nil},
		"kick_h3":      {"3rd Harmonic", nil},
		"kick_h4":      {"4th Harmonic", nil},
		"kick_env0":    {"Body Decay", nil},
		"kick_env1":    {"Harmonic Decay", nil},
		"kick_pe_amt":  {"Pitch Drop", nil},
		"kick_pe_rate": {"Drop Speed", nil},
		"kick_click":   {"Click", nil},
		"kick_noise":   {"Thud", nil},
		"kick_attack":  {"Punch", nil},
		"kick_fade":    {"Tail", nil},
		"kick_sat":     {"Drive", nil},
	}
	// kickStageMeta surfaces the SLOT-1 kick knobs as the visible KICK Synth-tab
	// stage (group "kick"); slots 2..12 stay hidden (ABI completeness only). The
	// kick voice lives in gen-slot 1 by convention for a modular kick instrument.
	for _, f := range modularGenSlotFieldsPhase3Kick {
		b := kickBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			d := ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name]}
			if k == 1 {
				if m, ok := kickStageMeta[f.Name]; ok {
					d.Group, d.Label, d.Enum = "kick", m.label, m.enum
				}
			}
			defs = append(defs, d)
		}
	}

	// ── Phase-3 globals (post_order): hidden POST-stage op-order selector.
	// Appended at the new very tail to mirror modularGlobalsPhase3 (the C struct's
	// last field). Default 0 = the shared apply_post_params order. ──
	p3Bounds := map[string][2]float64{
		"post_order": {0, 1},
	}
	for _, g := range modularGlobalsPhase3 {
		b := p3Bounds[g.Name]
		defs = append(defs, ParamDef{
			Name: g.Name, Label: g.Name, Group: SynthHiddenGroup,
			Min: b[0], Max: b[1], Default: ident[g.Name],
		})
	}

	// ── Phase-4 tom per-slot fields (source==6): hidden until Phase 8. Appended
	// at the new very tail to mirror modularGenSlotFieldsPhase4Tom (which mirrors
	// the C struct tail). Defaults are the schema identities (no-op). Min 0
	// throughout so the schema-identity default (0) sits inside the bounds (these
	// hidden defs exist for ABI completeness only; the source==6 voice reads the
	// live tom knobs via kp_get, not these). Max = the engine ceiling. ──
	tomBounds := map[string][2]float64{
		"tom_variant": {0, 2},
		"tom_sweep":   {0, 80}, "tom_ring": {0, 12},
		"tom_o1": {0, 1}, "tom_o2": {0, 1},
		"tom_stick": {0, 1}, "tom_room": {0, 1},
	}
	for _, f := range modularGenSlotFieldsPhase4Tom {
		b := tomBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			defs = append(defs, ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name],
			})
		}
	}

	// ── Phase-5 snare per-slot fields (source==7 snare/rimshot/sidestick +
	// source==8 clap): hidden until Phase 8. Appended at the new very tail to
	// mirror modularGenSlotFieldsPhase5Snare (which mirrors the C struct tail).
	// Defaults are the schema identities (no-op). Min 0 throughout so the
	// schema-identity default (0) sits inside the bounds (these hidden defs exist
	// for ABI completeness only; the source==7/8 voices read the live snare knobs
	// via kp_get, not these). Max = a representative engine ceiling. ──
	snareBounds := map[string][2]float64{
		"snare_variant": {0, 2},
		"snare_tone2":   {0, 2000}, "snare_tune": {0, 4},
		"snare_tone_d": {0, 200}, "snare_noise_d": {0, 300}, "snare_tail_d": {0, 100},
		"snare_tone_m": {0, 2}, "snare_noise_m": {0, 2}, "snare_wire_m": {0, 2},
		"snare_attack": {0, 200},
	}
	for _, f := range modularGenSlotFieldsPhase5Snare {
		b := snareBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			defs = append(defs, ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name],
			})
		}
	}

	// ── Phase-6 cymbal per-slot fields (source==9, 6 variant branches): hidden
	// until Phase 8. Appended at the new very tail to mirror
	// modularGenSlotFieldsPhase6Cymbal (which mirrors the C struct tail). Defaults
	// are the schema identities (no-op). Min 0 throughout so the schema-identity
	// default (0) sits inside the bounds (these hidden defs exist for ABI
	// completeness only; the source==9 voice reads the live cymbal knobs via
	// kp_get, not these). Max = a representative engine ceiling. ──
	cymBounds := map[string][2]float64{
		"cym_variant": {0, 5},
		"cym_tune":    {0, 2}, "cym_env_fast": {0, 400}, "cym_env_tail": {0, 120},
		"cym_tone_m": {0, 1.5}, "cym_noise_m": {0, 1.5}, "cym_noise_d": {0, 200},
	}
	for _, f := range modularGenSlotFieldsPhase6Cymbal {
		b := cymBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			defs = append(defs, ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name],
			})
		}
	}

	// ── Phase-7 FM per-slot fields (source==10, 5 variant branches): hidden until
	// Phase 8. Appended at the new very tail to mirror modularGenSlotFieldsPhase7FM
	// (which mirrors the C struct tail). Defaults are the schema identities (no-op).
	// Min 0 throughout so the schema-identity default (0) sits inside the bounds
	// (these hidden defs exist for ABI completeness only; the source==10 voice reads
	// the live FM knobs via kp_get, not these). Max = a representative engine
	// ceiling. ──
	fmBounds := map[string][2]float64{
		"fm_variant": {0, 4},
		"fm_base":    {0, 2000}, "fm_pe_amt": {0, 24}, "fm_pe_decay": {0, 0.5},
		"fm_r1": {0, 8}, "fm_r2": {0, 8}, "fm_r3": {0, 8}, "fm_r4": {0, 8},
		"fm_d1": {0, 8}, "fm_d2": {0, 8}, "fm_d3": {0, 8}, "fm_d4": {0, 8},
		"fm_dec1": {0, 3}, "fm_dec2": {0, 3}, "fm_dec3": {0, 3}, "fm_dec4": {0, 3},
	}
	for _, f := range modularGenSlotFieldsPhase7FM {
		b := fmBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			defs = append(defs, ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name],
			})
		}
	}

	// ── Phase-8C modulator stages (PITCH ENV / LFO / BURST — the spec-§1 gap
	// closure): USER-FACING, appended at the new very tail to mirror
	// modularGlobalsPhase8 (the C struct's last fields). Each stage's enable
	// defaults DISABLED (0 — matching the schema identity and the C mp_get
	// fallback, the byte-identity polarity for NEW stages). The NUMERIC defaults
	// deliberately DIFFER from the schema identities: they are the audible
	// starting points a user hears when flipping the stage on (no silent no-op
	// toggle), and they are gated by the off-by-default enable so every
	// pre-Phase-8C render stays byte-identical. The defaults-match invariant
	// carves these numerics out by the same rationale. Order MUST mirror
	// modularGlobalsPhase8. ──
	defs = append(defs,
		ParamDef{Name: "pitchenv_enabled", Label: "Pitch Env", Group: "pitchenv", Min: 0, Max: 1, Default: 0,
			Enum: []string{"Off", "On"}},
		ParamDef{Name: "pitchenv_amt", Label: "Sweep", Group: "pitchenv", Min: -24, Max: 24, Default: 12, Unit: "st"},
		ParamDef{Name: "pitchenv_decay", Label: "Sweep Time", Group: "pitchenv", Min: 0.01, Max: 2, Default: 0.08, Unit: "s"},
		ParamDef{Name: "lfo_enabled", Label: "LFO", Group: "lfo", Min: 0, Max: 1, Default: 0,
			Enum: []string{"Off", "On"}},
		ParamDef{Name: "lfo_rate", Label: "Rate", Group: "lfo", Min: 0.1, Max: 40, Default: 8, Unit: "Hz"},
		ParamDef{Name: "lfo_depth", Label: "Depth", Group: "lfo", Min: 0, Max: 1, Default: 0.5},
		ParamDef{Name: "burst_enabled", Label: "Burst", Group: "burst", Min: 0, Max: 1, Default: 0,
			Enum: []string{"Off", "On"}},
		ParamDef{Name: "burst_sharp", Label: "Sharpness", Group: "burst", Min: 1, Max: 200, Default: 40},
		ParamDef{Name: "burst1_off", Label: "Hit 1 Time", Group: "burst", Min: 0, Max: 0.25, Default: 0, Unit: "s"},
		ParamDef{Name: "burst1_amp", Label: "Hit 1 Level", Group: "burst", Min: 0, Max: 1, Default: 1},
		ParamDef{Name: "burst2_off", Label: "Hit 2 Time", Group: "burst", Min: 0, Max: 0.25, Default: 0.03, Unit: "s"},
		ParamDef{Name: "burst2_amp", Label: "Hit 2 Level", Group: "burst", Min: 0, Max: 1, Default: 0.6},
		ParamDef{Name: "burst3_off", Label: "Hit 3 Time", Group: "burst", Min: 0, Max: 0.25, Default: 0.06, Unit: "s"},
		ParamDef{Name: "burst3_amp", Label: "Hit 3 Level", Group: "burst", Min: 0, Max: 1, Default: 0.35},
		ParamDef{Name: "burst4_off", Label: "Hit 4 Time", Group: "burst", Min: 0, Max: 0.25, Default: 0, Unit: "s"},
		ParamDef{Name: "burst4_amp", Label: "Hit 4 Level", Group: "burst", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "lfo_target", Label: "LFO Target", Group: "lfo", Min: 0, Max: 2, Default: 0,
			Enum: []string{"Amp", "Pitch", "Cutoff"}},
		// Filter ENVELOPE is its OWN standardized Synth-tab stage (group "filtenv"
		// → synthSectionFilterEnv), distinct from the static FILTER stage: the
		// FILTER pill must stay filter_enabled (the rest of the UI — scene catalog,
		// concept-viz, preview — keys off it), so the env's filtenv_enabled gate
		// owns a dedicated FILTER-ENV section rather than clobbering FILTER's pill.
		ParamDef{Name: "filtenv_enabled", Label: "Filter Env", Group: "filtenv", Min: 0, Max: 1, Default: 0,
			Enum: []string{"Off", "On"}},
		ParamDef{Name: "filtenv_amt", Label: "Env Amount", Group: "filtenv", Min: 0, Max: 4, Default: 2, Unit: "oct"},
		ParamDef{Name: "filtenv_decay", Label: "Env Decay", Group: "filtenv", Min: 0.01, Max: 2, Default: 0.3, Unit: "s"},
		ParamDef{Name: "filtenv_attack", Label: "Env Rise", Group: "filtenv", Min: 0, Max: 2, Default: 0, Unit: "s"},
		/* Phase-8E unison/ensemble. Default voices=1 = identity (single osc). */
		ParamDef{Name: "unison_voices", Label: "Voices", Group: "unison", Min: 1, Max: 7, Default: 1,
			Enum: []string{"1", "2", "3", "4", "5", "6", "7"}},
		ParamDef{Name: "unison_detune", Label: "Detune", Group: "unison", Min: 0, Max: 50, Default: 0, Unit: "¢"},
		ParamDef{Name: "unison_mix", Label: "Mix", Group: "unison", Min: 0, Max: 1, Default: 0.5},
		/* Phase-8F unison drift. Default 0 = identity (no drift). */
		ParamDef{Name: "unison_drift_rate", Label: "Voice Drift", Group: "unison", Min: 0, Max: 8, Default: 0, Unit: "Hz"},
		ParamDef{Name: "unison_drift_depth", Label: "Drift Depth", Group: "unison", Min: 0, Max: 30, Default: 0, Unit: "¢"},
		/* Phase-8G LFO onset delay. Default 0 = identity (ramp factor 1.0 = byte-identical). */
		ParamDef{Name: "lfo_delay", Label: "Vibrato Delay", Group: "lfo", Min: 0, Max: 3, Default: 0, Unit: "s"},
		/* Body-resonator bank (organic string body). Default off = identity.
		   Group "resonator" → the RESONATOR Synth-tab stage (Task 8); no enable
		   toggle — body_model's "Off" enum entry (0) is the off-state. */
		ParamDef{Name: "body_model", Label: "Body Model", Group: "resonator", Min: 0, Max: 4, Default: 0, Enum: []string{"Off", "Violin", "Guitar", "Cello", "Steel Guitar"}},
		ParamDef{Name: "body_mix", Label: "Body Mix", Group: "resonator", Min: 0, Max: 1.5, Default: 0},
		ParamDef{Name: "bow_dynamics", Label: "Bow Dynamics", Group: "resonator", Min: 0, Max: 1, Default: 0},
	)

	// ── Phase-9 kick-extra per-slot fields (source==5): the kick voice's
	// structural shaping constants (attack-boost amount, global-fade rate,
	// saturation pre-gain) promoted to knobs. Hidden until the KICK Synth-tab
	// stage surfaces the slot-1 instances; appended at the new very tail to
	// mirror modularGenSlotFieldsPhase9KickExtra (the C struct's last fields).
	// Default 0 = the schema identity (read only at source==5; the C kp_get
	// supplies the per-variant literal at NaN). Max = the engine ceiling. ──
	kickExtraBounds := map[string][2]float64{
		"kick_attack": {0, 3}, "kick_fade": {0, 32}, "kick_sat": {0, 4},
	}
	for _, f := range modularGenSlotFieldsPhase9KickExtra {
		b := kickExtraBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			d := ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name]}
			if k == 1 {
				if m, ok := kickStageMeta[f.Name]; ok {
					d.Group, d.Label, d.Enum = "kick", m.label, m.enum
				}
			}
			defs = append(defs, d)
		}
	}

	// ── Phase-10 KICK-stage enable pill (USER-FACING). The kick voice is gated
	// by source==5 + this toggle; the Synth-tab KICK stage's enable pill writes
	// it. Default 0 (off, the schema identity / byte-identity polarity); a kick
	// instrument's seed sets it to 1. ──
	defs = append(defs, ParamDef{
		Name: "kick_enabled", Label: "Kick", Group: "kick", Min: 0, Max: 1, Default: 0,
		Enum: []string{"Off", "On"},
	})

	// ── Phase-11 modal-kick per-slot fields (source==5, variant 6): the coupled
	// two-mode drumhead knobs. Slot-1 instances surface in the visible KICK stage
	// (group "kick") via kickModeMeta; slots 2..12 stay hidden (ABI completeness).
	// Appended at the new very tail to mirror modularGenSlotFieldsPhase11KickMode
	// (the C struct's last fields). Default 0 = schema identity (read only at
	// source==5 variant 6; the C kp_get supplies the variant-6 literal at NaN). ──
	kickModeBounds := map[string][2]float64{
		"kick_mode_detune": {0, 0.5}, "kick_mode_gain": {0, 2}, "kick_mode_decay": {0, 40},
	}
	kickModeMeta := map[string]string{
		"kick_mode_detune": "Mode Split", "kick_mode_gain": "Mode Body", "kick_mode_decay": "Mode Ring",
	}
	for _, f := range modularGenSlotFieldsPhase11KickMode {
		b := kickModeBounds[f.Name]
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			d := ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: ident[name]}
			if k == 1 {
				if label, ok := kickModeMeta[f.Name]; ok {
					d.Group, d.Label = "kick", label
				}
			}
			defs = append(defs, d)
		}
	}

	// ── Phase-12 kick-reverb per-slot field (source==5, variant 7): the room-tail
	// amount. Slot-1 surfaces in the visible KICK stage as "Reverb"; the rest stay
	// hidden. Appended at the new very tail to mirror the C struct's last field. ──
	for _, f := range modularGenSlotFieldsPhase12KickReverb {
		for k := 1; k <= modularGenSlots; k++ {
			name := fmt.Sprintf("gen%d_%s", k, f.Name)
			d := ParamDef{
				Name: name, Label: fmt.Sprintf("G%d %s", k, f.Name), Group: SynthHiddenGroup,
				Min: 0, Max: 1, Default: ident[name]}
			if k == 1 {
				d.Group, d.Label = "kick", "Reverb"
			}
			defs = append(defs, d)
		}
	}
	// ── Phase-13 physical-model OSC params (osc_type 11 = render_sax). Declared
	// so they are settable config (and byte-identical at the schema-identity
	// default); ranges cover the shipped defaults so SetInstrumentParam doesn't
	// clamp them. Kept in the hidden group for now — the tiered, contextual UI
	// exposure (show only when osc_type == Sax, Essential/Advanced) is P4. ──
	saxBounds := map[string][2]float64{
		"osc_sax_blow": {0.02, 0.5}, "osc_sax_reed_off": {0.3, 0.8},
		"osc_sax_reed_slope": {0.1, 0.6}, "osc_sax_reflect": {-0.99, -0.6},
		"osc_sax_breath": {0.3, 1.3}, "osc_sax_loss": {0.4, 0.95},
	}
	saxLabels := map[string]string{
		"osc_sax_blow": "Sax Blow", "osc_sax_reed_off": "Sax Reed Offset",
		"osc_sax_reed_slope": "Sax Reed Stiffness", "osc_sax_reflect": "Sax Bell",
		"osc_sax_breath": "Sax Breath", "osc_sax_loss": "Sax Brightness",
	}
	for _, g := range modularGlobalsPhase13Sax {
		b := saxBounds[g.Name]
		defs = append(defs, ParamDef{
			Name: g.Name, Label: saxLabels[g.Name], Group: "osc",
			Min: b[0], Max: b[1], Default: ident[g.Name],
		})
	}
	// ── Phase-14 bowed-string physical-model params (osc_type 7 = violin/cello).
	// Same rationale as the sax block: settable config, byte-identical at default,
	// hidden until the P4 contextual UI. ──
	bowBounds := map[string][2]float64{
		"osc_bow_pos": {0.02, 0.4}, "osc_bow_slope": {1.0, 6.0},
		"osc_bow_vel": {0.05, 0.6}, "osc_bow_loss": {0.2, 0.9},
	}
	bowLabels := map[string]string{
		"osc_bow_pos": "Bow Position", "osc_bow_slope": "Bow Pressure",
		"osc_bow_vel": "Bow Speed", "osc_bow_loss": "Bow Brightness",
	}
	for _, g := range modularGlobalsPhase14Bow {
		b := bowBounds[g.Name]
		defs = append(defs, ParamDef{
			Name: g.Name, Label: bowLabels[g.Name], Group: "osc",
			Min: b[0], Max: b[1], Default: ident[g.Name],
		})
	}
	// ── Phase-15 voice/choir: the FORMANT stage + ENSEMBLE humanization.
	// Task 8 flips all 13 defs OUT of SynthHiddenGroup now that the routing
	// machinery knows them: formant_* → Group "formant" (routes to the new
	// FORMANT Synth-tab section, gated by the formant_enabled pill); ens_* →
	// Group "unison" (joins the existing unison/drift knobs in the renamed
	// ENSEMBLE section — no enable pill, voices==1 is the off-state). Both
	// groups are now registered in modularStageGroups (modular_stage_params.go)
	// so audio.IsAppendedStageName recognizes them and the UI section router
	// (modularStageGroupSection) sends them to FORMANT/ENSEMBLE instead of
	// falling through to VOICE. formant_enabled defaults 0 (off =
	// byte-identity polarity for new stages); the numeric defaults are audible
	// starting points gated behind the pill (same carve-out rationale as
	// Phase-8C). Order MUST mirror modularGlobalsPhase15Voice. ──
	defs = append(defs,
		ParamDef{Name: "formant_enabled", Label: "Formant", Group: "formant", Min: 0, Max: 1, Default: 0,
			Enum: []string{"Off", "On"}},
		ParamDef{Name: "formant_vowel", Label: "Vowel", Group: "formant", Min: 0, Max: 4, Default: 0,
			Enum: []string{"Ah", "Eh", "Ee", "Oh", "Oo"}},
		ParamDef{Name: "formant_voice_type", Label: "Voice Type", Group: "formant", Min: 0, Max: 3, Default: 0,
			Enum: []string{"Soprano", "Alto", "Tenor", "Bass"}},
		ParamDef{Name: "formant_mix", Label: "Mix", Group: "formant", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_shift", Label: "Head Size", Group: "formant", Min: 0.5, Max: 2, Default: 1},
		ParamDef{Name: "formant_breath", Label: "Breath", Group: "formant", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_sing", Label: "Shine", Group: "formant", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_morph_rate", Label: "Morph Speed", Group: "formant", Min: 0, Max: 8, Default: 0, Unit: "Hz"},
		ParamDef{Name: "formant_morph_to", Label: "Morph To", Group: "formant", Min: 0, Max: 4, Default: 0,
			Enum: []string{"Ah", "Eh", "Ee", "Oh", "Oo"}},
		ParamDef{Name: "ens_scatter", Label: "Scatter", Group: "unison", Min: 0, Max: 20, Default: 0, Unit: "¢"},
		ParamDef{Name: "ens_vib_rate", Label: "Vibrato Rate", Group: "unison", Min: 0, Max: 8, Default: 0, Unit: "Hz"},
		ParamDef{Name: "ens_vib_depth", Label: "Vibrato Depth", Group: "unison", Min: 0, Max: 100, Default: 0, Unit: "¢"},
		ParamDef{Name: "ens_humanize", Label: "Humanize", Group: "unison", Min: 0, Max: 1, Default: 0},
	)
	// ── Phase-16 voice-realism: ens_jitter (per-voice fast pitch roughness on
	// the ENSEMBLE stage) + formant_dry (the FORMANT stage's dry-blend
	// scaler). Order MUST mirror modularGlobalsPhase16VoiceRealism. ──
	defs = append(defs,
		ParamDef{Name: "ens_jitter", Label: "Roughness", Group: "unison", Min: 0, Max: 1, Default: 0},
		ParamDef{Name: "formant_dry", Label: "Dry Blend", Group: "formant", Min: 0, Max: 1, Default: 1},
	)
	// P4 tiering: mark the fine-tuning knobs Advanced so each stage shows only its
	// 3-5 character knobs by default; the rest collapse under the stage's Advanced
	// expander. Everything unlisted stays Essential.
	for i := range defs {
		if advancedTierParams[defs[i].Name] {
			defs[i].Tier = TierAdvanced
		}
	}
	return defs
}

// advancedTierParams is the P4 Essential/Advanced classification (see the tiered
// Synth-tab exposure). Each stage keeps its 3-6 CHARACTER knobs Essential (always
// shown); the fine-tuning knobs below collapse under the stage's Advanced
// expander. Rule of thumb: Essential = "what a beginner reaches for to change the
// sound"; Advanced = "shaping you only touch when dialing in".
var advancedTierParams = map[string]bool{
	// OSC — Essential: Oscillator (shape), Octave (big pitch). Advanced: fine cents.
	"osc_detune": true,
	// OSC/Unison — Essential: Voices, Detune, Mix. Advanced: the slow wander.
	"unison_drift_rate": true, "unison_drift_depth": true,
	// FM — Essential: Algorithm + the 4 operator RATIOS (the character). Advanced:
	// the per-op modulation DEPTHS and output LEVELS (fine balance).
	"fm_op1_depth": true, "fm_op2_depth": true, "fm_op3_depth": true, "fm_op4_depth": true,
	"fm_op1_level": true, "fm_op2_level": true, "fm_op3_level": true, "fm_op4_level": true,
	// ENVELOPE — Essential: A/D/S/R. Advanced: the curve shape.
	"amp_curve": true,
	// FILTER ENV — Essential: Amount, Decay. Advanced: the rise.
	"filtenv_attack": true,
	// LFO — Essential: Rate, Depth, Target. Advanced: the onset delay.
	"lfo_delay": true,
	// BURST — Essential: Sharpness + the first hit. Advanced: extra hits 2-4.
	"burst2_off": true, "burst2_amp": true,
	"burst3_off": true, "burst3_amp": true,
	"burst4_off": true, "burst4_amp": true,
	// KICK (folds into the agnostic VOICE stage) — Essential: Variant, Punch, Body
	// Decay, Pitch Drop, Thud, Drive (the 6 that define the kick). Advanced: the
	// transient click, harmonic tuning/decay, tail, drop-speed, coupled-drumhead
	// modes, and room reverb.
	"gen1_kick_click": true, "gen1_kick_env1": true, "gen1_kick_fade": true,
	"gen1_kick_h2": true, "gen1_kick_h3": true, "gen1_kick_h4": true,
	"gen1_kick_pe_rate":     true,
	"gen1_kick_mode_detune": true, "gen1_kick_mode_gain": true, "gen1_kick_mode_decay": true,
	"gen1_kick_reverb": true,
	// Physical-model OSC params (shown in OSC only when the matching oscillator is
	// selected — see synthOscModelParamHidden): fine reed / bowed-string shaping.
	"osc_sax_blow": true, "osc_sax_reed_off": true, "osc_sax_reed_slope": true,
	"osc_sax_reflect": true, "osc_sax_breath": true, "osc_sax_loss": true,
	"osc_bow_pos": true, "osc_bow_slope": true, "osc_bow_vel": true, "osc_bow_loss": true,
	// FORMANT — Essential: Vowel, Voice Type, Mix, Breath. Advanced: the rest.
	"formant_shift": true, "formant_sing": true,
	"formant_morph_rate": true, "formant_morph_to": true,
	// FORMANT/Phase-16 — Dry Blend is a fine-tuning knob (rarely touched vs Mix).
	"formant_dry": true,
}

// SynthHiddenGroup marks a ParamDef that exists for ABI completeness but must
// not be rendered as a knob/pill in the Synth tab. The UI build loop and
// sectionForParam skip any def in this group.
const SynthHiddenGroup = "hidden"

// modularRecipeCategory is the Category string for modular-voice recipes. The
// registry, the BuildBuiltinRecipeDocs param-source branch, the native renderer
// factory, and the wired-params discipline exemptions all key off it.
const modularRecipeCategory = "modular"

// modularPadSeed diverges the second shipped modular preset (synth-modular-pad
// / the "modular-pad" instrument) from the base voice: a soft, dark, slow pad —
// low filter cutoff, long attack + release, high sustain. Every value sits
// inside the corresponding ParamDef [Min,Max]; ToRegistration overlays it onto
// ModularSynthParamDefs so the registered defaults (and RecipeDefaultParams)
// reflect the pad without forking the schema. Because these defaults differ
// from the engine's plain render_modular built-ins, the browser must be seeded
// with them at bootstrap (SeedInstrumentDefaultsToPlatform) — see
// modular_defaults_seed.go.
var modularPadSeed = RecipeParams{
	"filter_cutoff":    1200,
	"filter_resonance": 1.4,
	"amp_attack":       0.12,
	"amp_decay":        0.8,
	"amp_sustain":      0.85,
	"amp_release":      1.4,
	"gain":             0.9,
}
