package audio

// THE source of the only param block crossing the ABI (C struct, JS ABI, schema, Go block); table order IS ABI order.
// Slots N = gen<1..N>_<Name>, Rows R = a table column, Derived = a row count (no ParamDef for either). Init is a NEW
// module's value, never a fill for an absent field.

type modularField struct {
	Block   string
	Name    string
	Slots   int
	Rows    int
	Derived bool
	Init    float64
	Doc     string
}

var modularFields = []modularField{
	{Block: "core", Name: "osc_type", Init: 0, Doc: "0=sine 1=saw 2=square 3=triangle 4=FM 5=noise-white 6=noise-pink"},
	{Block: "core", Name: "osc_detune", Init: 0, Doc: "cents, -100..100"},
	{Block: "core", Name: "osc_octave", Init: 0, Doc: "integer octave shift, -2..2"},
	{Block: "core", Name: "fm_algorithm", Init: 0, Doc: "0..3 operator routing"},
	{Block: "core", Name: "fm_op1_ratio", Init: 1, Doc: "0.5..8"},
	{Block: "core", Name: "fm_op2_ratio", Init: 1, Doc: "0.5..8"},
	{Block: "core", Name: "fm_op3_ratio", Init: 1, Doc: "0.5..8"},
	{Block: "core", Name: "fm_op4_ratio", Init: 1, Doc: "0.5..8"},
	{Block: "core", Name: "fm_op1_depth", Init: 0, Doc: "0..8 mod index"},
	{Block: "core", Name: "fm_op2_depth", Init: 0, Doc: "0..8 mod index"},
	{Block: "core", Name: "fm_op3_depth", Init: 0, Doc: "0..8 mod index"},
	{Block: "core", Name: "fm_op4_depth", Init: 0, Doc: "0..8 mod index"},
	{Block: "core", Name: "fm_op1_level", Init: 1, Doc: "0..1"},
	{Block: "core", Name: "fm_op2_level", Init: 0, Doc: "0..1"},
	{Block: "core", Name: "fm_op3_level", Init: 0, Doc: "0..1"},
	{Block: "core", Name: "fm_op4_level", Init: 0, Doc: "0..1"},
	{Block: "core", Name: "amp_attack", Init: 0.005, Doc: "seconds"},
	{Block: "core", Name: "amp_decay", Init: 0.3, Doc: "seconds"},
	{Block: "core", Name: "amp_sustain", Init: 0.6, Doc: "0..1"},
	{Block: "core", Name: "amp_release", Init: 0.2, Doc: "seconds"},
	{Block: "core", Name: "amp_curve", Init: 1, Doc: "0=linear 1=exponential"},
	{Block: "core", Name: "filter_type", Init: 0, Doc: "0=LP 1=HP 2=BP"},
	{Block: "core", Name: "filter_cutoff", Init: 8000, Doc: "Hz, 20..20000"},
	{Block: "core", Name: "filter_resonance", Init: 0.707, Doc: "Q, 0.5..16"},
	{Block: "core", Name: "drive", Init: 0, Doc: "0..1 tanh saturation"},
	{Block: "core", Name: "pitch", Init: 0, Doc: "semitone offset, -24..24"},
	{Block: "core", Name: "gain", Init: 1, Doc: "output trim, 0..1.5"},
	{Block: "core", Name: "osc_enabled", Init: 1, Doc: "0 = silence the generator (it is the source)"},
	{Block: "core", Name: "fm_enabled", Init: 1, Doc: "osc_type==FM only; 0 = carrier-only (no modulation)"},
	{Block: "core", Name: "env_enabled", Init: 1, Doc: "0 = constant unity gain (skip the amp ADSR)"},
	{Block: "core", Name: "filter_enabled", Init: 1, Doc: "0 = filter bypassed (passthrough)"},
	{Block: "core", Name: "drive_enabled", Init: 1, Doc: "0 = drive bypassed (clean)"},
	{Block: "core", Name: "noise_seed", Init: 0, Doc: "deterministic seed for osc_type 5/6; default 0"},
	{Block: "core", Name: "noise_draws", Init: 1, Doc: "bus draws per sample (default 1)"},
	{Block: "core", Name: "noise_prelude", Init: 0, Doc: "bus draws consumed before sample 0"},

	{Block: "gen", Name: "source", Slots: 12, Init: 0, Doc: "0=off 1=osc 2=noise-tap"},
	{Block: "gen", Name: "wave", Slots: 12, Init: 0, Doc: "0=sine 1=saw 2=square 3=triangle"},
	{Block: "gen", Name: "freq_mode", Slots: 12, Init: 0, Doc: "0=ratio×voice-freq 1=absolute Hz"},
	{Block: "gen", Name: "freq", Slots: 12, Init: 1, Doc: ""},
	{Block: "gen", Name: "gain", Slots: 12, Init: 1, Doc: ""},
	{Block: "gen", Name: "env_fast_rate", Slots: 12, Init: 0, Doc: "s^-1; 0 => constant"},
	{Block: "gen", Name: "env_tail_rate", Slots: 12, Init: 0, Doc: ""},
	{Block: "gen", Name: "env_fast_mix", Slots: 12, Init: 1, Doc: ""},
	{Block: "gen", Name: "env_tail_mix", Slots: 12, Init: 0, Doc: ""},
	{Block: "gen", Name: "filt_type", Slots: 12, Init: 0, Doc: "0=none 1=1pLP 2=1pHP 3=rbjLP 4=rbjHP 5=rbjBP"},
	{Block: "gen", Name: "filt_alpha", Slots: 12, Init: 0, Doc: "1-pole input coeff: lp = lp*(1-a) + x*a"},
	{Block: "gen", Name: "filt_freq", Slots: 12, Init: 1000, Doc: "RBJ Hz"},
	{Block: "gen", Name: "filt_q", Slots: 12, Init: 0.707, Doc: ""},
	{Block: "gen", Name: "phase_mode", Slots: 12, Init: 0, Doc: "0=zero 1=fixed-radians 2=prelude-index"},
	{Block: "gen", Name: "phase", Slots: 12, Init: 0, Doc: ""},
	{Block: "gen", Name: "noise_offset", Slots: 12, Init: 0, Doc: "which per-sample bus draw this tap reads"},

	{Block: "bass", Name: "pitch_env_amt", Slots: 1, Init: 0, Doc: "sub-bass pitch-punch fraction"},
	{Block: "bass", Name: "pitch_env_rate", Slots: 1, Init: 0, Doc: "pitch-env decay /s"},
	{Block: "bass", Name: "harm_mix", Slots: 1, Init: 0, Doc: "2nd-harmonic companion mix"},
	{Block: "bass", Name: "atk_amt", Slots: 1, Init: 0, Doc: "attack-boost amount"},
	{Block: "bass", Name: "atk_rate", Slots: 1, Init: 0, Doc: "attack-boost decay /s"},
	{Block: "bass", Name: "sat_k", Slots: 1, Init: 0, Doc: "tanh input scale; 0 = no sat"},
	{Block: "bass", Name: "out_scale", Slots: 1, Init: 1, Doc: "trailing output scale"},

	{Block: "post", Name: "voice_freq_hz", Init: 0, Doc: ">0 overrides the derived modular voice freq (Hz)"},
	{Block: "post", Name: "post_pitch", Init: 0, Doc: "FINISH resample, semitones (0 = none)"},
	{Block: "post", Name: "post_decay", Init: 1, Doc: "FINISH fade, 0.01..1 (1 = none; lower fades sooner)"},
	{Block: "post", Name: "post_decay_rate", Init: 8, Doc: "Fade speed: post_decay's time constant (/s)"},
	{Block: "post", Name: "post_body", Init: 0, Doc: "FINISH low boost: a low shelf below 200 Hz, 0..1 = 0..+12 dB"},
	{Block: "post", Name: "post_brightness", Init: 0, Doc: "FINISH crisp: a high shelf above 3 kHz, 0..1 = 0..+12 dB"},
	{Block: "post", Name: "post_darken", Init: 0, Doc: "FINISH darken: one-pole low-pass crossfaded in by this amount, 0..1"},
	{Block: "post", Name: "post_drive", Init: 0, Doc: "FINISH crunch: tanh saturation crossfaded in by this amount, 0..1"},
	{Block: "post", Name: "out_clamp", Init: 1.5, Doc: "FINISH limit: hard clip at ±out_clamp after the other ops; 0 = no limit"},

	{Block: "ks", Name: "ks_sustain", Slots: 1, Init: 0, Doc: "KS decay factor"},
	{Block: "ks", Name: "ks_pluck", Slots: 1, Init: 0, Doc: "KS pluck-LP alpha"},
	{Block: "ks", Name: "ks_blow", Slots: 1, Init: 0, Doc: "KS continuous-noise excitation"},

	{Block: "kick", Name: "kick_variant", Slots: 1, Init: 0, Doc: "0=base 1=deep 2=punchy 3=lofi 4=tight"},
	{Block: "kick", Name: "kick_h2", Slots: 1, Init: 0, Doc: "kick_h2_gain"},
	{Block: "kick", Name: "kick_h3", Slots: 1, Init: 0, Doc: "kick_h3_gain"},
	{Block: "kick", Name: "kick_h4", Slots: 1, Init: 0, Doc: "kick_h4_gain"},
	{Block: "kick", Name: "kick_env0", Slots: 1, Init: 0, Doc: "kick_env0_rate"},
	{Block: "kick", Name: "kick_env1", Slots: 1, Init: 0, Doc: "kick_env1_rate"},
	{Block: "kick", Name: "kick_pe_amt", Slots: 1, Init: 0, Doc: "kick_pitch_env_amount"},
	{Block: "kick", Name: "kick_pe_rate", Slots: 1, Init: 0, Doc: "kick_pitch_env_rate"},
	{Block: "kick", Name: "kick_click", Slots: 1, Init: 0, Doc: "kick_click"},
	{Block: "kick", Name: "kick_noise", Slots: 1, Init: 0, Doc: "kick_noise"},

	{Block: "tom", Name: "tom_variant", Slots: 1, Init: 0, Doc: "0=tom 1=high 2=low"},
	{Block: "tom", Name: "tom_sweep", Slots: 1, Init: 0, Doc: "tom_sweep_rate"},
	{Block: "tom", Name: "tom_ring", Slots: 1, Init: 0, Doc: "tom_ring_rate"},
	{Block: "tom", Name: "tom_o1", Slots: 1, Init: 0, Doc: "tom_o1_gain"},
	{Block: "tom", Name: "tom_o2", Slots: 1, Init: 0, Doc: "tom_o2_gain"},
	{Block: "tom", Name: "tom_stick", Slots: 1, Init: 0, Doc: "tom_stick"},
	{Block: "tom", Name: "tom_room", Slots: 1, Init: 0, Doc: "tom_room"},

	{Block: "snare", Name: "snare_variant", Slots: 1, Init: 0, Doc: "0=snare 1=rimshot 2=sidestick (source==7)"},
	{Block: "snare", Name: "snare_tone2", Slots: 1, Init: 0, Doc: "snare_tone2_freq"},
	{Block: "snare", Name: "snare_tune", Slots: 1, Init: 0, Doc: "snare_noise_tune"},
	{Block: "snare", Name: "snare_tone_d", Slots: 1, Init: 0, Doc: "snare_tone_decay"},
	{Block: "snare", Name: "snare_noise_d", Slots: 1, Init: 0, Doc: "snare_noise_decay"},
	{Block: "snare", Name: "snare_tail_d", Slots: 1, Init: 0, Doc: "snare_tail_decay"},
	{Block: "snare", Name: "snare_tone_m", Slots: 1, Init: 0, Doc: "snare_tone_mix"},
	{Block: "snare", Name: "snare_noise_m", Slots: 1, Init: 0, Doc: "snare_noise_mix"},
	{Block: "snare", Name: "snare_wire_m", Slots: 1, Init: 0, Doc: "snare_wire_mix"},
	{Block: "snare", Name: "snare_attack", Slots: 1, Init: 0, Doc: "snare_attack"},

	{Block: "cym", Name: "cym_variant", Slots: 3, Init: 0, Doc: "0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash"},
	{Block: "cym", Name: "cym_tune", Slots: 3, Init: 1, Doc: "cym_tune"},
	{Block: "cym", Name: "cym_env_fast", Slots: 3, Init: 0, Doc: "cym_env_fast / transient rate"},
	{Block: "cym", Name: "cym_env_tail", Slots: 3, Init: 1.7, Doc: "cym_env_tail / ring rate"},
	{Block: "cym", Name: "cym_tone_m", Slots: 3, Init: 0, Doc: "cym_tone_mix"},
	{Block: "cym", Name: "cym_noise_m", Slots: 3, Init: 0, Doc: "cym_noise_mix"},
	{Block: "cym", Name: "cym_noise_d", Slots: 3, Init: 4, Doc: "cym_noise_decay"},

	{Block: "fm", Name: "fm_variant", Slots: 1, Init: 0, Doc: "0=bass 1=bell 2=lead 3=epiano 4=pluck"},
	{Block: "fm", Name: "fm_base", Slots: 1, Init: 0, Doc: "preset.base_freq"},
	{Block: "fm", Name: "fm_pe_amt", Slots: 1, Init: 0, Doc: "preset.pitch_env_amount"},
	{Block: "fm", Name: "fm_pe_decay", Slots: 1, Init: 0, Doc: "preset.pitch_env_decay"},
	{Block: "fm", Name: "fm_r1", Slots: 1, Init: 0, Doc: "ops[0].freq_ratio"},
	{Block: "fm", Name: "fm_r2", Slots: 1, Init: 0, Doc: "ops[1].freq_ratio"},
	{Block: "fm", Name: "fm_r3", Slots: 1, Init: 0, Doc: "ops[2].freq_ratio"},
	{Block: "fm", Name: "fm_r4", Slots: 1, Init: 0, Doc: "ops[3].freq_ratio"},
	{Block: "fm", Name: "fm_d1", Slots: 1, Init: 0, Doc: "op0 outgoing edge depth"},
	{Block: "fm", Name: "fm_d2", Slots: 1, Init: 0, Doc: "op1 outgoing edge depth"},
	{Block: "fm", Name: "fm_d3", Slots: 1, Init: 0, Doc: "op2 outgoing edge depth"},
	{Block: "fm", Name: "fm_d4", Slots: 1, Init: 0, Doc: "op3 outgoing edge depth"},
	{Block: "fm", Name: "fm_dec1", Slots: 1, Init: 0, Doc: "ops[0].decay_sec"},
	{Block: "fm", Name: "fm_dec2", Slots: 1, Init: 0, Doc: "ops[1].decay_sec"},
	{Block: "fm", Name: "fm_dec3", Slots: 1, Init: 0, Doc: "ops[2].decay_sec"},
	{Block: "fm", Name: "fm_dec4", Slots: 1, Init: 0, Doc: "ops[3].decay_sec"},

	{Block: "mod", Name: "pitchenv_enabled", Init: 0, Doc: ">=0.5 runs the OSC pitch envelope"},
	{Block: "mod", Name: "pitchenv_amt", Init: 0, Doc: "semitones of sweep (decays to 0)"},
	{Block: "mod", Name: "pitchenv_decay", Init: 0, Doc: "exp time constant, seconds (fm_preset convention)"},
	{Block: "mod", Name: "lfo_enabled", Init: 0, Doc: ">=0.5 runs the amp-wobble LFO"},
	{Block: "mod", Name: "lfo_rate", Init: 0, Doc: "Hz"},
	{Block: "mod", Name: "lfo_depth", Init: 0, Doc: "0..1 wobble depth (1 = full dips to silence)"},
	{Block: "mod", Name: "burst_enabled", Init: 0, Doc: ">=0.5 runs the burst gate"},
	{Block: "mod", Name: "burst_sharp", Init: 0, Doc: "per-burst exp decay rate, s^-1"},
	{Block: "mod", Name: "burst1_off", Init: 0, Doc: "burst onsets, seconds"},
	{Block: "mod", Name: "burst1_amp", Init: 0, Doc: "burst peak amplitudes"},
	{Block: "mod", Name: "burst2_off", Init: 0, Doc: ""},
	{Block: "mod", Name: "burst2_amp", Init: 0, Doc: ""},
	{Block: "mod", Name: "burst3_off", Init: 0, Doc: ""},
	{Block: "mod", Name: "burst3_amp", Init: 0, Doc: ""},
	{Block: "mod", Name: "burst4_off", Init: 0, Doc: ""},
	{Block: "mod", Name: "burst4_amp", Init: 0, Doc: ""},
	{Block: "mod", Name: "lfo_target", Init: 0, Doc: ""},
	{Block: "mod", Name: "filtenv_enabled", Init: 0, Doc: ""},
	{Block: "mod", Name: "filtenv_amt", Init: 0, Doc: ""},
	{Block: "mod", Name: "filtenv_decay", Init: 0, Doc: ""},
	{Block: "mod", Name: "filtenv_attack", Init: 0, Doc: ""},
	{Block: "mod", Name: "unison_voices", Init: 1, Doc: "1..7, integer-valued, default 1 (identity = single osc)"},
	{Block: "mod", Name: "unison_detune", Init: 0, Doc: "cents spread ±detune/2 across side voices, 0..50, default 0"},
	{Block: "mod", Name: "unison_mix", Init: 0.5, Doc: "0=only center, 1=full ensemble blend, default 0.5"},
	{Block: "mod", Name: "unison_drift_rate", Init: 0, Doc: "Hz, 0..8, base rate (each voice uses rate*(1+0.13*v))"},
	{Block: "mod", Name: "unison_drift_depth", Init: 0, Doc: "cents, 0..30, per-voice drift amplitude"},
	{Block: "mod", Name: "lfo_delay", Init: 0, Doc: "seconds, 0..3, default 0 (identity)"},
	{Block: "mod", Name: "body_model", Init: 0, Doc: "0=off, 1=violin (hardcoded mode table)"},
	{Block: "mod", Name: "body_mix", Init: 0, Doc: "0..~1.5, wet (resonant peaks) added to the dry signal"},
	{Block: "mod", Name: "bow_dynamics", Init: 0, Doc: "0..1, modulation depth"},

	{Block: "kickextra", Name: "kick_attack", Slots: 1, Init: 0, Doc: "attack-boost amount"},
	{Block: "kickextra", Name: "kick_fade", Slots: 1, Init: 0, Doc: "global-fade rate /tNorm — higher = shorter tail"},
	{Block: "kickextra", Name: "kick_sat", Slots: 1, Init: 0, Doc: "saturation pre-gain"},

	{Block: "kickenable", Name: "kick_enabled", Init: 0, Doc: ">=0.5 runs the source==5 kick voice; <0.5 silences it"},

	{Block: "kickmode", Name: "kick_mode_detune", Slots: 1, Init: 0, Doc: ""},
	{Block: "kickmode", Name: "kick_mode_gain", Slots: 1, Init: 0, Doc: ""},
	{Block: "kickmode", Name: "kick_mode_decay", Slots: 1, Init: 0, Doc: ""},

	{Block: "kickreverb", Name: "kick_reverb", Slots: 1, Init: 0, Doc: ""},

	{Block: "sax", Name: "osc_sax_blow", Init: 0.15, Doc: "render_sax position (default 0.15)"},
	{Block: "sax", Name: "osc_sax_reed_off", Init: 0.58, Doc: "render_sax reed_offset (default 0.58)"},
	{Block: "sax", Name: "osc_sax_reed_slope", Init: 0.28, Doc: "render_sax reed_slope (default 0.28)"},
	{Block: "sax", Name: "osc_sax_reflect", Init: -0.94, Doc: "render_sax reflect (default -0.94)"},
	{Block: "sax", Name: "osc_sax_breath", Init: 0.85, Doc: "render_sax breath_target (default 0.85)"},
	{Block: "sax", Name: "osc_sax_loss", Init: 0.7, Doc: "render_sax loss_b0 (default 0.70)"},

	{Block: "bow", Name: "osc_bow_pos", Init: 0.13, Doc: "render_bowed_string bow_pos (default 0.13)"},
	{Block: "bow", Name: "osc_bow_slope", Init: 3, Doc: "friction slope / bow pressure (default 3.0)"},
	{Block: "bow", Name: "osc_bow_vel", Init: 0.25, Doc: "bow speed (default 0.25)"},
	{Block: "bow", Name: "osc_bow_loss", Init: 0.55, Doc: "bridge LP loss = brightness (default 0.55)"},

	{Block: "voice", Name: "formant_enabled", Init: 0, Doc: ">=0.5 runs the formant stage (default 0)"},
	{Block: "voice", Name: "formant_vowel", Init: 0, Doc: "0..4 continuous: a → e → i → o → u"},
	{Block: "voice", Name: "formant_voice_type", Init: 0, Doc: "0 soprano / 1 alto / 2 tenor / 3 bass"},
	{Block: "voice", Name: "formant_mix", Init: 0, Doc: "0..1 wet blend (0 = exact bypass)"},
	{Block: "voice", Name: "formant_shift", Init: 1, Doc: "0.5..2 formant-frequency scale (vocal-tract size)"},
	{Block: "voice", Name: "formant_breath", Init: 0, Doc: "0..1 white-noise mix injected PRE-ADSR (whisper=1)"},
	{Block: "voice", Name: "formant_sing", Init: 0, Doc: "0..1 singer's-formant boost (~2.4-3.1 kHz by voice)"},
	{Block: "voice", Name: "formant_morph_rate", Init: 0, Doc: "Hz, 0..8: vowel-morph LFO toward morph_to (0 = static)"},
	{Block: "voice", Name: "formant_morph_to", Init: 0, Doc: "0..4 morph target vowel"},
	{Block: "voice", Name: "ens_scatter", Init: 0, Doc: "cents, 0..20: per-voice constant pitch offset"},
	{Block: "voice", Name: "ens_vib_rate", Init: 0, Doc: "Hz, 0..8: per-voice vibrato LFO rate"},
	{Block: "voice", Name: "ens_vib_depth", Init: 0, Doc: "cents, 0..100: per-voice vibrato depth"},
	{Block: "voice", Name: "ens_humanize", Init: 0, Doc: "0..1: randomizes per-voice vibrato phase/rate"},

	{Block: "voicereal", Name: "ens_jitter", Init: 0, Doc: "0..1: per-voice fast pitch jitter, cents amp = *35"},
	{Block: "voicereal", Name: "formant_dry", Init: 1, Doc: "0..1: scales the FORMANT stage's dry blend amount"},

	{Block: "snareenable", Name: "snare_enabled", Init: 0, Doc: ">=0.5 runs the source==7 snare voice; <0.5 silences it"},

	{Block: "cymmodal", Name: "cym_modes", Slots: 3, Init: 0, Doc: "number of plate modes; open hi-hat/crash play their partial voice below 1, a hi-hat/ride plate with 0 modes is silent"},
	{Block: "cymmodal", Name: "cym_mode_lo", Slots: 3, Init: 300, Doc: "lowest mode Hz"},
	{Block: "cymmodal", Name: "cym_mode_hi", Slots: 3, Init: 18000, Doc: "highest mode Hz (capped below Nyquist)"},
	{Block: "cymmodal", Name: "cym_decay_tilt", Slots: 3, Init: 0, Doc: "decay-rate exponent vs frequency (0: flat)"},
	{Block: "cymmodal", Name: "cym_hf_corner", Slots: 3, Init: 10000, Doc: "Hz where the extra HF damping reaches env_fast"},
	{Block: "cymmodal", Name: "cym_beat", Slots: 3, Init: 0, Doc: "doublet split as a fraction of f (0: no doublets)"},
	{Block: "cymmodal", Name: "cym_tone_f", Slots: 3, Init: 3000, Doc: "main strike hump centre Hz (0 or width 0: no hump)"},
	{Block: "cymmodal", Name: "cym_tone_w", Slots: 3, Init: 0.6, Doc: "main hump width, octaves"},
	{Block: "cymmodal", Name: "cym_body_f", Slots: 3, Init: 1500, Doc: "body hump centre Hz"},
	{Block: "cymmodal", Name: "cym_body_w", Slots: 3, Init: 0.15, Doc: "body hump width, octaves"},
	{Block: "cymmodal", Name: "cym_body", Slots: 3, Init: 0, Doc: "body hump gain relative to the main hump (0: none)"},
	{Block: "cymmodal", Name: "cym_spark_f", Slots: 3, Init: 11000, Doc: "stick-sparkle hump centre Hz"},
	{Block: "cymmodal", Name: "cym_spark_w", Slots: 3, Init: 0.3, Doc: "sparkle hump width, octaves"},
	{Block: "cymmodal", Name: "cym_spark", Slots: 3, Init: 0, Doc: "sparkle hump gain (0: none)"},
	{Block: "cymmodal", Name: "cym_wash", Slots: 3, Init: 0, Doc: "broadband mode floor gain (0: none)"},
	{Block: "cymmodal", Name: "cym_attack", Slots: 3, Init: 0.0005, Doc: "stick-contact onset ramp, seconds"},
	{Block: "cymmodal", Name: "cym_rattle", Slots: 3, Init: 1, Doc: "collision amplitude relative to the stick (0: none)"},
	{Block: "cymmodal", Name: "cym_rattle_rate", Slots: 3, Init: 2000, Doc: "collisions per second at t=0"},
	{Block: "cymmodal", Name: "cym_rattle_decay", Slots: 3, Init: 0.06, Doc: "rattle time constant, seconds (0: no rattle)"},
	{Block: "cymmodal", Name: "cym_drive_delay", Slots: 3, Init: 0, Doc: "seconds (0: struck — the stick hits the plate)"},
	{Block: "cymmodal", Name: "cym_rattle_hold", Slots: 3, Init: 0, Doc: "seconds (0: rate decays with rattle_decay)"},

	{Block: "kickphys", Name: "kick_hold", Slots: 1, Init: 0, Doc: ""},
	{Block: "kickphys", Name: "kick_hiss", Slots: 1, Init: 0, Doc: ""},

	// WAV-clone stages: Rows columns are table cells written by Flatten, Derived rows the row counts; neither is a knob.
	{Block: "modal", Name: "modal_enabled", Init: 0, Doc: "0=off (exact bypass) 1=on"},
	{Block: "modal", Name: "modal_freq_mode", Init: 0, Doc: "0=freq is a ratio to the voice frequency 1=freq is Hz"},
	{Block: "modal", Name: "modal_tune", Init: 0, Doc: "cents"},
	{Block: "modal", Name: "modal_stretch", Init: 0, Doc: "f' = f*(f/ref)^s; ref = voice freq (ratio mode) or 1 kHz"},
	{Block: "modal", Name: "modal_decay_x", Init: 1, Doc: "multiplies every mode's decay rate"},
	{Block: "modal", Name: "modal_bright", Init: 0, Doc: "dB/oct tilt re 1 kHz on the mode amplitudes"},
	{Block: "modal", Name: "modal_keep", Init: 128, Doc: "plays the first N rows"},
	{Block: "modal", Name: "modal_law_a", Init: 1, Doc: "decay law a*(f/1k)^p + b*(f/10k)^6, Np/s; a row's decay is a multiplier of it"},
	{Block: "modal", Name: "modal_law_p", Init: 0, Doc: "decay law exponent"},
	{Block: "modal", Name: "modal_law_b", Init: 0, Doc: "decay law high-frequency term, Np/s at 10 kHz"},
	{Block: "modal", Name: "modal_count", Init: 0, Derived: true, Doc: "rows in the table (written by Flatten)"},
	{Block: "modal", Name: "modal_freq", Rows: 128, Init: 0, Doc: "ratio or Hz"},
	{Block: "modal", Name: "modal_decay", Rows: 128, Init: 0, Doc: "multiplier of the decay law at the mode's frequency"},
	{Block: "modal", Name: "modal_amp", Rows: 128, Init: 0, Doc: "amplitude divided by the strike spectrum at the fitted contact"},
	{Block: "modal", Name: "modal_phase", Rows: 128, Init: 0, Doc: "radians, cosine phase at the onset"},
	{Block: "modal", Name: "modal_attack", Rows: 128, Init: 0, Doc: "seconds of raised-cosine onset"},
	{Block: "strike", Name: "strike_enabled", Init: 0, Doc: "0=off 1=on (reweights the modal amplitudes)"},
	{Block: "strike", Name: "strike_tc", Init: 0, Doc: "raised-cosine contact time, seconds (0: an ideal impulse)"},
	{Block: "strike", Name: "strike_hardness", Init: 0, Doc: "dB/oct tilt re 1 kHz"},
	{Block: "strike", Name: "strike_click", Init: 0, Doc: "contact click level relative to the bank's peak bound"},
	{Block: "noiseband", Name: "noiseband_enabled", Init: 0, Doc: "0=off (exact bypass) 1=on"},
	{Block: "noiseband", Name: "noiseband_level", Init: 0, Doc: "dB offset on every band"},
	{Block: "noiseband", Name: "noiseband_count", Init: 0, Derived: true, Doc: "rows in the table (written by Flatten)"},
	{Block: "noiseband", Name: "nb_center", Rows: 32, Init: 0, Doc: "Hz"},
	{Block: "noiseband", Name: "nb_q", Rows: 32, Init: 0, Doc: "band-pass Q"},
	{Block: "noiseband", Name: "nb_level", Rows: 32, Init: 0, Doc: "dB RMS at the end of the attack"},
	{Block: "noiseband", Name: "nb_attack", Rows: 32, Init: 0, Doc: "seconds of raised-cosine onset"},
	{Block: "noiseband", Name: "nb_fast_rate", Rows: 32, Init: 0, Doc: "Np/s"},
	{Block: "noiseband", Name: "nb_fast_mix", Rows: 32, Init: 0, Doc: "0..1 share of the fast decay"},
	{Block: "noiseband", Name: "nb_tail_rate", Rows: 32, Init: 0, Doc: "Np/s"},
}

// fieldsOfBlock returns the rows of one Block, in table order.
func fieldsOfBlock(block string) []modularField {
	var out []modularField
	for _, f := range modularFields {
		if f.Block == block {
			out = append(out, f)
		}
	}
	return out
}

// ModularFieldsForCodegen exposes the table to cmd/gen-synth-abi.
func ModularFieldsForCodegen() []struct {
	Block, Name, Doc string
	Slots, Rows      int
} {
	out := make([]struct {
		Block, Name, Doc string
		Slots, Rows      int
	}, len(modularFields))
	for i, f := range modularFields {
		out[i].Block, out[i].Name, out[i].Doc, out[i].Slots, out[i].Rows = f.Block, f.Name, f.Doc, f.Slots, f.Rows
	}
	return out
}
