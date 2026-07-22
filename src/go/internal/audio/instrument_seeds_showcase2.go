package audio

// instrument_seeds_showcase2.go — showcase batch ROUND 2 (2026-07-20).
//
// 18 more config-only modular instruments, same approach as round 1
// (instrument_seeds_showcase.go): pure RecipeParams seeds, no C/DSP. Every
// instrument here defaults to the SAME reference pitch — node pitch 0 = A3
// (220 Hz) — so NONE carry an osc_octave/base-freq offset (register comes from
// timbre, and the player pitches nodes as needed). Designed from synthesis
// principles, ear-gate pending.
//
// Never seed gain:0.5 (TestPhase2Native mutates gain->0.5 and needs a delta).

// ─────────────────────────────────────────────────────────────────────────────
// A2 · Strings & bowed — bowed physical model (osc_type 7) + body resonators
// ─────────────────────────────────────────────────────────────────────────────

// violaSeed: the alto of the string family — a bowed physical model through the
// violin body, mid-dark (between violin and cello): a touch warmer/rounder than
// the violin, a gentle bowed onset and vibrato.
var violaSeed = RecipeParams{
	"osc_type": 7, "osc_enabled": 1, // bowed-string physical model
	"unison_voices": 1,
	"filter_type":   0, "filter_cutoff": 2500, "filter_resonance": 0.7,
	"amp_attack": 0.14, "amp_decay": 0.1, "amp_sustain": 0.9, "amp_release": 0.25,
	"lfo_enabled": 1, "lfo_rate": 4.4, "lfo_depth": 0.03, "lfo_target": 1, "lfo_delay": 0.15,
	"body_model": 1, "body_mix": 1.1, // violin body, a hair warmer mix than the solo violin
	"bow_dynamics": 0.28,
	"gen1_source":  2, "gen1_gain": 0.014, "gen1_filt_type": 4, "gen1_filt_freq": 2000, "gen1_filt_q": 0.7,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 10, "gen1_env_tail_mix": 0.8, "gen1_env_tail_rate": 0.0,
	"gain": 0.55,
}

// doubleBassSeed: the dark bowed bass of the string section — bowed physical model
// through the cello body, a slow heavy onset, more bow friction, dark roll-off.
// Defaults to A3 like everything else; a rich low timbre — pitch nodes down for a
// walking bass.
var doubleBassSeed = RecipeParams{
	"osc_type": 7, "osc_enabled": 1,
	"unison_voices": 1,
	"filter_type":   0, "filter_cutoff": 1600, "filter_resonance": 0.6,
	"amp_attack": 0.12, "amp_decay": 0.12, "amp_sustain": 0.9, "amp_release": 0.3,
	"lfo_enabled": 1, "lfo_rate": 3.8, "lfo_depth": 0.025, "lfo_target": 1, "lfo_delay": 0.2,
	"body_model": 3, "body_mix": 0.6, // cello body — big, dark
	"bow_dynamics": 0.32,
	"gen1_source":  2, "gen1_gain": 0.03, "gen1_filt_type": 4, "gen1_filt_freq": 1100, "gen1_filt_q": 0.7,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 9, "gen1_env_tail_mix": 0.7, "gen1_env_tail_rate": 0.0,
	"gain": 0.55,
}

// pizzStringsSeed: pizzicato strings — a soft, warm, short plucked string (KS with
// a low pluck brightness through the violin body), a quick woody decay. The
// plucked counterpart to the bowed viola.
var pizzStringsSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.972, "gen1_ks_pluck": 0.34,
	"filter_type": 0, "filter_cutoff": 4000,
	"amp_attack": 0.001, "amp_decay": 0.8, "amp_sustain": 0.0, "amp_release": 0.2,
	"body_model": 1, "body_mix": 0.35, // violin body warmth
	"gain": 0.9,
}

// ─────────────────────────────────────────────────────────────────────────────
// B2 · World plucked — Karplus-Strong
// ─────────────────────────────────────────────────────────────────────────────

// mandolinSeed: a bright tremolo mandolin — a bright steel-string KS with the
// signature fast tremolo (an amplitude LFO ~8 Hz standing in for tremolo picking)
// through a steel body. Sits between banjo and steel guitar.
var mandolinSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.982, "gen1_ks_pluck": 0.5,
	"filter_type": 0, "filter_cutoff": 6500,
	"amp_attack": 0.001, "amp_decay": 1.3, "amp_sustain": 0.0, "amp_release": 0.2,
	"body_model": 4, "body_mix": 0.2, // bright steel body
	// tremolo picking
	"lfo_enabled": 1, "lfo_target": 0, "lfo_rate": 8.0, "lfo_depth": 0.3, "lfo_delay": 0.0,
	"gain": 0.9,
}

// ukuleleSeed: a small bright nylon uke — a KS pluck warmer than the guitars but
// shorter and higher-bodied, a quick friendly decay through a small box.
var ukuleleSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.972, "gen1_ks_pluck": 0.4,
	"filter_type": 0, "filter_cutoff": 5000,
	"amp_attack": 0.001, "amp_decay": 0.9, "amp_sustain": 0.0, "amp_release": 0.2,
	"body_model": 2, "body_mix": 0.16, // small nylon box
	"gain": 0.9,
}

// kotoSeed: a Japanese koto zither — a bright, ringing plucked string with a clear
// clean attack and a medium-long shimmering ring, lightly bodied.
var kotoSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.986, "gen1_ks_pluck": 0.46,
	"filter_type": 0, "filter_cutoff": 6000,
	"amp_attack": 0.001, "amp_decay": 2.0, "amp_sustain": 0.0, "amp_release": 0.3,
	"body_model": 2, "body_mix": 0.14,
	"gain": 0.9,
}

// ─────────────────────────────────────────────────────────────────────────────
// C2 · Metals & bells — additive / FM, inharmonic
// ─────────────────────────────────────────────────────────────────────────────

// steelDrumSeed: a Caribbean steel pan — tuned metal with strong octave (2×) and
// twelfth (3×) partials plus a metallic shimmer, a bright but round tone and a
// medium ring, with a slight paired-note beating.
var steelDrumSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"unison_voices": 2, "unison_detune": 8, "unison_mix": 0.45, // shimmer beat
	"filter_type": 0, "filter_cutoff": 6000, "filter_resonance": 0.9,
	"amp_attack": 0.002, "amp_decay": 1.4, "amp_sustain": 0.0, "amp_release": 0.3, "amp_curve": 1,
	// pans are tuned to the octave + twelfth, with an inharmonic upper for zing
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2.0, "gen1_gain": 0.45, "gen1_env_fast_rate": 2.0,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 3.0, "gen2_gain": 0.28, "gen2_env_fast_rate": 2.5,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 5.2, "gen3_gain": 0.10, "gen3_env_fast_rate": 4,
	"gain": 0.85,
}

// tubularBellsSeed: orchestral tubular bells — a deep, long, clanging church bell
// via FM with an inharmonic modulator ratio (the metallic bell partial) and a very
// long ring.
var tubularBellsSeed = RecipeParams{
	"osc_type": 4, "osc_enabled": 1,
	"fm_enabled": 1, "fm_algorithm": 0,
	"fm_op1_ratio": 1, "fm_op2_ratio": 1.41, "fm_op2_depth": 1.8, "fm_op1_level": 1, "fm_op2_level": 0,
	"filter_type": 0, "filter_cutoff": 6000,
	"amp_attack": 0.001, "amp_decay": 4.0, "amp_sustain": 0.0, "amp_release": 0.6, "amp_curve": 1,
	"gain": 0.8,
}

// xylophoneSeed: a hard, bright xylophone — like the marimba but tuned to its 3rd
// partial (the wooden-bar signature), brighter and MUCH shorter (a hard mallet on
// a small bar).
var xylophoneSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"filter_type": 0, "filter_cutoff": 5200, "filter_resonance": 0.8,
	"amp_attack": 0.001, "amp_decay": 0.35, "amp_sustain": 0.0, "amp_release": 0.12, "amp_curve": 1,
	// xylophone bars are tuned to the 3rd partial (vs marimba's 4th)
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 3.0, "gen1_gain": 0.34, "gen1_env_fast_rate": 14,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 6.8, "gen2_gain": 0.09, "gen2_env_fast_rate": 22,
	// hard mallet click
	"gen3_source": 2, "gen3_gain": 0.08, "gen3_filt_type": 4, "gen3_filt_freq": 4000, "gen3_filt_q": 0.6,
	"gen3_env_fast_mix": 1.0, "gen3_env_fast_rate": 80, "gen3_env_tail_mix": 0.0, "gen3_env_tail_rate": 0.0,
	"gain": 0.9,
}

// ─────────────────────────────────────────────────────────────────────────────
// D2 · Winds & brass — additive
// ─────────────────────────────────────────────────────────────────────────────

// bassoonSeed: the dark double-reed bass of the woodwinds — a reedy additive tone
// with a weak fundamental and strong H2–H4 (the bassoon's hollow buzz), a dark
// roll-off and a reedy onset.
var bassoonSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 2600, "filter_resonance": 1.0,
	"amp_attack": 0.035, "amp_decay": 0.1, "amp_sustain": 0.9, "amp_release": 0.15,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.22, // H1 weak
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.48, // H2 strong
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.42, // H3 strong
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4.0, "gen4_gain": 0.22, // H4
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5.0, "gen5_gain": 0.09, // H5
	// reedy buzz noise
	"gen6_source": 2, "gen6_gain": 0.05, "gen6_filt_type": 5, "gen6_filt_freq": 1400, "gen6_filt_q": 0.6,
	"gen6_env_fast_mix": 0.5, "gen6_env_fast_rate": 8, "gen6_env_tail_mix": 0.6, "gen6_env_tail_rate": 0.0,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.0, "lfo_depth": 0.015, "lfo_delay": 0.25,
	"gain": 0.8,
}

// piccoloSeed: the piccolo — a piercingly bright near-sine flute with lots of air
// and a fast light vibrato. Same air-reed model as the flute but thinner and much
// brighter.
var piccoloSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 9000, "filter_resonance": 1.0,
	"amp_attack": 0.03, "amp_decay": 0.05, "amp_sustain": 0.92, "amp_release": 0.1,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.55, // H1
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.14, // H2
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.05, // H3
	// bright air
	"gen4_source": 2, "gen4_gain": 0.28, "gen4_filt_type": 5, "gen4_filt_freq": 3500, "gen4_filt_q": 0.4,
	"gen4_env_fast_mix": 0.5, "gen4_env_fast_rate": 5, "gen4_env_tail_mix": 0.85, "gen4_env_tail_rate": 0.0,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 6.0, "lfo_depth": 0.04, "lfo_delay": 0.2,
	"gain": 0.85,
}

// accordionSeed: a wet musette accordion — reedy saws with a WIDE detuned unison
// (the shimmering "wet" musette beating of multiple reeds), brighter and sharper
// than the harmonium, with a steady vibrato.
var accordionSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1, // saw reed
	"filter_type": 0, "filter_cutoff": 3800, "filter_resonance": 1.0,
	"unison_voices": 3, "unison_detune": 16, "unison_mix": 0.6, "unison_drift_rate": 1.5, "unison_drift_depth": 5,
	"amp_attack": 0.02, "amp_decay": 0.08, "amp_sustain": 1.0, "amp_release": 0.12,
	// sharp reed formant
	"gen1_source": 1, "gen1_wave": 1, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.35,
	"gen1_filt_type": 5, "gen1_filt_freq": 1600, "gen1_filt_q": 1.4,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 6.5, "lfo_depth": 0.025, "lfo_delay": 0.1,
	"gain": 0.7,
}

// tromboneSeed: a mid brass trombone — between trumpet and tuba: H1–H3-dominant
// additive brass, a mellow-dark roll-off, a slower onset and a gentle brassy
// filter bloom.
var tromboneSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 2200, "filter_resonance": 1.0,
	"amp_attack": 0.07, "amp_decay": 0.1, "amp_sustain": 0.92, "amp_release": 0.18,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.42, // H1
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.5, // H2
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.34, // H3
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4.0, "gen4_gain": 0.16, // H4
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5.0, "gen5_gain": 0.07, // H5
	"gen6_source": 2, "gen6_gain": 0.04, "gen6_filt_type": 5, "gen6_filt_freq": 1100, "gen6_filt_q": 0.5,
	"gen6_env_fast_mix": 0.7, "gen6_env_fast_rate": 8, "gen6_env_tail_mix": 0.3, "gen6_env_tail_rate": 2,
	"filtenv_enabled": 1, "filtenv_amt": 0.8, "filtenv_decay": 0.14,
	"gain": 0.85,
}

// mutedTrumpetSeed: a harmon-muted trumpet — a pinched, nasal, buzzy brass: a
// band-passed formant lift gives the muted "wah" while the fundamental is held
// back, bright and thin with a fast onset.
var mutedTrumpetSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 5000, "filter_resonance": 1.0,
	"amp_attack": 0.02, "amp_decay": 0.08, "amp_sustain": 0.9, "amp_release": 0.12,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.18, // H1 held back (pinched)
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.32, // H2
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.5, // H3 peak (nasal)
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4.0, "gen4_gain": 0.34, // H4
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5.0, "gen5_gain": 0.2, // H5
	// the harmon "wah" formant — a band-passed saw lift in the pinched mid
	"gen6_source": 1, "gen6_wave": 1, "gen6_freq_mode": 0, "gen6_freq": 1.0, "gen6_gain": 0.35,
	"gen6_filt_type": 5, "gen6_filt_freq": 1800, "gen6_filt_q": 2.2,
	"filtenv_enabled": 1, "filtenv_amt": 1.0, "filtenv_decay": 0.12,
	"gain": 0.8,
}

// brassStabSeed: a punchy synth-brass section stab — additive brass with a fast
// attack, a bright bloom and a detuned unison for section width; the classic
// analog "brass hit".
var brassStabSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 3800, "filter_resonance": 1.0,
	"unison_voices": 3, "unison_detune": 8, "unison_mix": 0.4, "unison_drift_rate": 2.5, "unison_drift_depth": 6,
	"amp_attack": 0.02, "amp_decay": 0.12, "amp_sustain": 0.75, "amp_release": 0.15,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.36, // H1
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.5, // H2
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.44, // H3
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4.0, "gen4_gain": 0.26, // H4
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5.0, "gen5_gain": 0.14, // H5
	"filtenv_enabled": 1, "filtenv_amt": 1.4, "filtenv_attack": 0.02, "filtenv_decay": 0.18, // the stab bloom
	"gain": 0.7,
}

// ─────────────────────────────────────────────────────────────────────────────
// E2 · Synth & textural
// ─────────────────────────────────────────────────────────────────────────────

// moogBassSeed: a fat mono analog bass — a saw through a resonant low-pass swept
// by a filter envelope, with a sine sub-partial for weight. Round, punchy, ladder-
// filter growl. Defaults to A3 (pitch nodes down for a bassline).
var moogBassSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1, // saw
	"filter_type": 0, "filter_cutoff": 800, "filter_resonance": 3.0,
	"filtenv_enabled": 1, "filtenv_amt": 2.0, "filtenv_decay": 0.3, "filtenv_attack": 0.0, // the moog "wow"
	"amp_attack": 0.004, "amp_decay": 0.25, "amp_sustain": 0.7, "amp_release": 0.15,
	// sine sub for body weight
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.4,
	"drive_enabled": 1, "drive": 0.2,
	"gain": 0.9,
}

// pwmPadSeed: a warm chorused poly pad — a detuned square-wave ensemble (the
// Juno/PWM shimmer) under a slow filter LFO for gentle movement, with a soft pad
// attack and a long release.
var pwmPadSeed = RecipeParams{
	"osc_type": 2, "osc_enabled": 1, // square
	"unison_voices": 5, "unison_detune": 12, "unison_mix": 0.75, "unison_drift_rate": 0.5, "unison_drift_depth": 6,
	"filter_type": 0, "filter_cutoff": 4000, "filter_resonance": 1.0,
	"amp_attack": 0.3, "amp_decay": 0.4, "amp_sustain": 0.85, "amp_release": 1.1,
	// slow filter movement (PWM-ish shimmer)
	"lfo_enabled": 1, "lfo_target": 2, "lfo_rate": 0.5, "lfo_depth": 0.35, "lfo_delay": 0.0,
	"gain": 0.6,
}

// glassArmonicaSeed: a glass armonica (Franklin's spinning wine-glasses) — an
// almost-pure sine ensemble with a slow ethereal swell, a slight detuned shimmer,
// and a long sustain. Delicate, otherworldly, glassy.
var glassArmonicaSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1, // pure sine
	"unison_voices": 3, "unison_detune": 5, "unison_mix": 0.5, "unison_drift_rate": 0.3, "unison_drift_depth": 4,
	"filter_type": 0, "filter_cutoff": 6000, "filter_resonance": 0.8,
	"amp_attack": 0.8, "amp_decay": 0.3, "amp_sustain": 0.92, "amp_release": 0.9, // slow ethereal swell
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 4.0, "lfo_depth": 0.015, "lfo_delay": 0.4, // faint shimmer
	// a whisper of a bright partial for glass edge
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 3.0, "gen1_gain": 0.06,
	"gain": 0.6,
}
