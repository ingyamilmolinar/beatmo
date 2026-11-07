package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// THE built-in module types: a type groups the ABI fields one stage, source or voice reads. A variant is its own type
// only when it runs a different C function; one that picks constants from a table stays a value. Adding a voice:
// one declaration, its C, `make gen-synth-abi`; no document changes.

func declVals(names ...string) []instrument.ValueDecl {
	out := make([]instrument.ValueDecl, len(names))
	for i, n := range names {
		out[i] = instrument.ValueDecl{Name: n}
	}
	return out
}

func declSwitch(field string, v float64) *instrument.FieldSet {
	return &instrument.FieldSet{Field: field, Value: v}
}

func declSet(field string, v float64) []instrument.FieldSet {
	return []instrument.FieldSet{{Field: field, Value: v}}
}

func declPrefixed(prefix string, names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

// A family voice's capacity is whatever its arrays hold, never a hand-typed number.
func slotsOf(block string) int {
	for _, f := range modularFields {
		if f.Block == block {
			return f.Slots
		}
	}
	panic("slotsOf: no block " + block)
}

func builtinModuleDecls() []instrument.ModuleTypeDecl {
	const (
		st  = instrument.KindStage
		src = instrument.KindSource
		ly  = instrument.KindLayer
	)
	d := []instrument.ModuleTypeDecl{
		// ── stages ──
		{Type: "core", Kind: st, Required: true, Values: declVals("pitch", "osc_octave", "osc_detune", "voice_freq_hz", "gain", "noise_draws", "noise_prelude")},
		{Type: "amp_env", Kind: st, Enable: declSwitch("env_enabled", 1), Values: declVals("amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve")},
		{Type: "filter", Kind: st, Enable: declSwitch("filter_enabled", 1), Values: declVals("filter_type", "filter_cutoff", "filter_resonance")},
		{Type: "drive", Kind: st, Enable: declSwitch("drive_enabled", 1), Values: declVals("drive")},
		{Type: "pitch_env", Kind: st, Enable: declSwitch("pitchenv_enabled", 1), Values: declVals("pitchenv_amt", "pitchenv_decay")},
		{Type: "lfo", Kind: st, Enable: declSwitch("lfo_enabled", 1), Values: declVals("lfo_rate", "lfo_depth", "lfo_target", "lfo_delay")},
		{Type: "burst", Kind: st, Enable: declSwitch("burst_enabled", 1), Values: declVals("burst_sharp",
			"burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp")},
		{Type: "filter_env", Kind: st, Enable: declSwitch("filtenv_enabled", 1), Values: declVals("filtenv_amt", "filtenv_decay", "filtenv_attack")},
		{Type: "unison", Kind: st, Values: declVals("unison_voices", "unison_detune", "unison_mix", "unison_drift_rate", "unison_drift_depth",
			"ens_scatter", "ens_vib_rate", "ens_vib_depth", "ens_humanize", "ens_jitter")},
		{Type: "body", Kind: st, Values: declVals("body_model", "body_mix", "bow_dynamics")},
		{Type: "formant", Kind: st, Enable: declSwitch("formant_enabled", 1), Values: declVals("formant_vowel", "formant_voice_type", "formant_mix",
			"formant_shift", "formant_breath", "formant_sing", "formant_morph_rate", "formant_morph_to", "formant_dry")},
		{Type: "finish", Kind: st, Required: true, Values: declVals("post_pitch", "post_decay", "post_decay_rate", "post_body",
			"post_brightness", "post_darken", "post_drive", "out_clamp")},
		// The tables hold model parameters only.
		{Type: "modal", Kind: st, Enable: declSwitch("modal_enabled", 1),
			Values: declVals("modal_freq_mode", "modal_tune", "modal_stretch", "modal_decay_x", "modal_bright",
				"modal_keep", "modal_law_a", "modal_law_p", "modal_law_b"),
			Table: &instrument.TableDecl{Capacity: 128, CountField: "modal_count", Columns: []instrument.TableColumn{
				{Name: "freq", Field: "modal_freq", Min: 0, Max: 24000},
				{Name: "decay", Field: "modal_decay", Min: 0, Max: 10000},
				{Name: "amp", Field: "modal_amp", Min: -1000, Max: 1000},
				{Name: "phase", Field: "modal_phase", Min: -7, Max: 7},
				{Name: "attack", Field: "modal_attack", Min: 0, Max: 1},
			}}},
		{Type: "strike", Kind: st, Enable: declSwitch("strike_enabled", 1),
			Values: declVals("strike_tc", "strike_hardness", "strike_click")},
		{Type: "noiseband", Kind: st, Enable: declSwitch("noiseband_enabled", 1),
			Values: declVals("noiseband_level"),
			Table: &instrument.TableDecl{Capacity: 32, CountField: "noiseband_count", Columns: []instrument.TableColumn{
				{Name: "center", Field: "nb_center", Min: 10, Max: 24000},
				{Name: "q", Field: "nb_q", Min: 0.1, Max: 100},
				{Name: "level", Field: "nb_level", Min: -200, Max: 24},
				{Name: "attack", Field: "nb_attack", Min: 0, Max: 1},
				{Name: "fast_rate", Field: "nb_fast_rate", Min: 0, Max: 5000},
				{Name: "fast_mix", Field: "nb_fast_mix", Min: 0, Max: 1},
				{Name: "tail_rate", Field: "nb_tail_rate", Min: 0, Max: 5000},
			}}},

		// ── sources (osc_type picks the DSP) ──
		{Type: "osc.wavetable", Kind: src, Enable: declSwitch("osc_enabled", 1),
			Values: []instrument.ValueDecl{{Name: "osc_type", Allowed: []float64{0, 1, 2, 3, 12}}}},
		{Type: "osc.noise", Kind: src, Enable: declSwitch("osc_enabled", 1),
			Values: []instrument.ValueDecl{{Name: "osc_type", Allowed: []float64{5, 6}}}},
		{Type: "osc.fm", Kind: src, Enable: declSwitch("osc_enabled", 1), Set: declSet("osc_type", 4), Values: declVals("fm_enabled", "fm_algorithm",
			"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
			"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
			"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level")},
		{Type: "osc.bowed", Kind: src, Enable: declSwitch("osc_enabled", 1), Set: declSet("osc_type", 7), Values: declVals("osc_bow_pos", "osc_bow_slope", "osc_bow_vel", "osc_bow_loss")},
		{Type: "osc.brass", Kind: src, Enable: declSwitch("osc_enabled", 1), Set: declSet("osc_type", 8)},
		{Type: "osc.reed", Kind: src, Enable: declSwitch("osc_enabled", 1), Set: declSet("osc_type", 9)},
		{Type: "osc.flute", Kind: src, Enable: declSwitch("osc_enabled", 1), Set: declSet("osc_type", 10)},
		{Type: "osc.sax", Kind: src, Enable: declSwitch("osc_enabled", 1), Set: declSet("osc_type", 11), Values: declVals("osc_sax_blow", "osc_sax_reed_off",
			"osc_sax_reed_slope", "osc_sax_reflect", "osc_sax_breath", "osc_sax_loss")},

		// ── layers (gen_source picks the voice) ──
		{Type: "gen.wavetable", Kind: ly, Enable: declSwitch("source", 1), Values: declVals("wave", "freq_mode", "freq", "gain",
			"env_fast_rate", "env_tail_rate", "env_fast_mix", "env_tail_mix", "filt_type", "filt_alpha", "filt_freq", "filt_q", "phase_mode", "phase")},
		{Type: "gen.noise", Kind: ly, Enable: declSwitch("source", 2), Values: declVals("gain",
			"env_fast_rate", "env_tail_rate", "env_fast_mix", "env_tail_mix", "filt_type", "filt_alpha", "filt_freq", "filt_q", "noise_offset")},
		{Type: "ks", Kind: ly, Enable: declSwitch("source", 3), Capacity: slotsOf("ks"), Values: declVals("freq_mode", "freq", "gain", "env_fast_rate",
			"atk_amt", "atk_rate", "out_scale", "ks_sustain", "ks_pluck", "ks_blow")},
		{Type: "bass.analytic", Kind: ly, Enable: declSwitch("source", 4), Capacity: slotsOf("bass"), Values: declVals("freq_mode", "freq", "gain", "wave", "phase",
			"env_fast_rate", "pitch_env_amt", "pitch_env_rate", "harm_mix", "atk_amt", "atk_rate", "sat_k", "out_scale")},
		{Type: "tom", Kind: ly, Enable: declSwitch("source", 6), Capacity: slotsOf("tom"), Values: declVals("freq_mode", "freq", "wave", "gain",
			"tom_variant", "tom_sweep", "tom_ring", "tom_o1", "tom_o2", "tom_stick", "tom_room")},
		{Type: "clap", Kind: ly, Enable: declSwitch("source", 8), Capacity: slotsOf("snare"), Values: declVals("gain",
			"snare_tune", "snare_noise_d", "snare_tail_d", "snare_noise_m", "snare_attack")},
		{Type: "fm", Kind: ly, Enable: declSwitch("source", 10), Capacity: slotsOf("fm"), Values: declVals("wave", "fm_variant", "fm_base", "fm_pe_amt", "fm_pe_decay",
			"fm_r1", "fm_r2", "fm_r3", "fm_r4", "fm_d1", "fm_d2", "fm_d3", "fm_d4", "fm_dec1", "fm_dec2", "fm_dec3", "fm_dec4")},
	}

	// One type per C voice function. The physical kick discards the generator wave in C, so it does not declare it.
	kick := func(typ string, variant float64, own ...string) instrument.ModuleTypeDecl {
		common := []string{"freq_mode", "freq", "wave"}
		if typ == "kick.physical" {
			common = common[:2]
		}
		return instrument.ModuleTypeDecl{Type: typ, Kind: ly, Enable: declSwitch("source", 5), Capacity: slotsOf("kick"),
			Set: declSet("kick_variant", variant), Global: declSet("kick_enabled", 1),
			Values: declVals(append(common, declPrefixed("kick_", own...)...)...)}
	}
	kickCore := []string{"attack", "env0", "env1", "fade", "h2", "pe_amt", "pe_rate", "sat"}
	with := func(extra ...string) []string { return append(append([]string{}, kickCore...), extra...) }
	d = append(d,
		kick("kick.base", 0, with("click", "h3", "h4", "noise")...),
		kick("kick.deep", 1, with("click", "h3", "noise")...),
		kick("kick.punchy", 2, with("click")...),
		kick("kick.lofi", 3, with("h3", "noise")...),
		kick("kick.tight", 4, with("click", "h3", "noise")...),
		kick("kick.hybrid", 5, with("click", "h3", "noise")...),
		kick("kick.modal", 6, with("click", "h3", "noise", "mode_detune", "mode_gain", "mode_decay")...),
		kick("kick.acoustic", 7, with("click", "h3", "h4", "noise", "mode_detune", "mode_gain", "mode_decay", "reverb")...),
		kick("kick.physical", 8, with("click", "h3", "h4", "noise", "mode_detune", "mode_gain", "mode_decay", "reverb", "hold", "hiss")...),
	)

	// Snare: one type per C voice function (gen_snare_<variant>).
	snare := func(typ string, variant float64, own ...string) instrument.ModuleTypeDecl {
		return instrument.ModuleTypeDecl{Type: typ, Kind: ly, Enable: declSwitch("source", 7), Capacity: slotsOf("snare"),
			Set: declSet("snare_variant", variant), Global: declSet("snare_enabled", 1),
			Values: declVals(append([]string{"freq_mode", "freq", "wave"}, declPrefixed("snare_", own...)...)...)}
	}
	short := []string{"attack", "noise_d", "noise_m", "tone2", "tone_d", "tone_m", "tune"}
	full := append(append([]string{}, short...), "tail_d", "wire_m")
	d = append(d,
		snare("snare.main", 0, full...),
		snare("snare.rimshot", 1, short...),
		snare("snare.sidestick", 2, short...),
		snare("snare.acoustic", 3, full...),
	)

	// Hi-hat and ride always play the modal plate; open hi-hat and crash once cym_modes ≥ 1.
	cym := func(typ string, variant float64, own ...string) instrument.ModuleTypeDecl {
		return instrument.ModuleTypeDecl{Type: typ, Kind: ly, Enable: declSwitch("source", 9), Capacity: slotsOf("cym"),
			Set: declSet("cym_variant", variant), Values: declVals(own...)}
	}
	partial := declPrefixed("cym_", "tune", "env_fast", "env_tail", "tone_m", "noise_m", "noise_d")
	plate := append(append([]string{}, partial...), declPrefixed("cym_", "modes", "mode_lo", "mode_hi", "decay_tilt", "hf_corner", "beat",
		"tone_f", "tone_w", "body_f", "body_w", "body", "spark_f", "spark_w", "spark", "wash", "attack")...)
	// Hi-hats only: the C forces the rattle off for one suspended cymbal, which has nothing to rattle against.
	hat := append(append([]string{}, plate...), declPrefixed("cym_", "rattle", "rattle_rate", "rattle_decay", "drive_delay", "rattle_hold")...)
	d = append(d,
		cym("cym.hihat", 0, hat...),
		cym("cym.open", 1, append([]string{"wave"}, hat...)...),
		cym("cym.cowbell", 2, append([]string{"wave"}, partial...)...),
		cym("cym.shaker", 3, declPrefixed("cym_", "tune", "env_fast", "env_tail", "tone_m", "noise_m")...),
		cym("cym.ride", 4, plate...),
		cym("cym.crash", 5, append([]string{"wave"}, plate...)...),
	)

	// Only the effects both platforms apply to a voice.
	for _, et := range voiceFXTypes() {
		d = append(d, instrument.ModuleTypeDecl{Type: "fx." + string(et), Kind: instrument.KindFX, FreeForm: true})
	}
	for i := range d {
		d[i].Version = 1
	}
	return d
}
