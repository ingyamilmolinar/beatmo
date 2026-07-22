package audio

// instrument_seeds_showcase.go — the "balanced & wild" showcase batch (2026-07-20).
//
// 24 new modular-engine instruments, designed from the synthesis principles in
// AGENTS.synth*.md and the existing seed corpus rather than reference-WAV A/B
// tuned. Each is pure config: a RecipeParams seed consumed by one row in
// modularInstrumentDefs (modular_instruments.go) + a melodicRecipeIDs entry.
// No C changes.
//
// Conventions mirrored from instrument_seeds.go:
//   - Additive struck/bell voices: osc_type 0 (sine) as H1 (osc_enabled 1),
//     inharmonic upper partials as tone gens (source 1, wave 0) with per-partial
//     env_fast_rate for differential decay; amp_sustain 0 + amp_curve 1 (exp)
//     so the amp ADSR owns the ring (the pianoGrandSeed pattern).
//   - Sustained additive winds: osc_enabled 0, all partials in the gen bank,
//     amp_sustain high (the flute/oboe/horn pattern).
//   - Plucked: gen1 source 3 (Karplus-Strong) + ks_sustain/ks_pluck + a body
//     model (the guitar pattern).
//   - FM keys/bells: osc_type 4 + fm_* operators (the bassFMSeed pattern).
//   - NEVER seed gain:0.5 (TestPhase2Native mutates gain->0.5 and needs a delta).
//
// gen enums: source 1=tone-osc 2=noise 3=Karplus-Strong; wave 0=sine 1=saw
// 2=square 3=tri; filt_type 3=LP 4=HP 5=BP (RBJ). lfo_target 0=Amp 1=Pitch
// 2=Cutoff. body_model 1=Violin 2=Guitar 3=Cello 4=Steel.

// ─────────────────────────────────────────────────────────────────────────────
// A · Mallets & Metals — additive struck, inharmonic partials shaped by the amp ADSR
// ─────────────────────────────────────────────────────────────────────────────

// marimbaSeed: warm wooden bars. A rosewood marimba bar's first strong overtone
// sits near 4:1 (the maker tunes it there); the tone is fundamental-dominant with
// a fast, warm decay and a soft mallet chiff. LP keeps it woody, not glassy.
var marimbaSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"filter_type": 0, "filter_cutoff": 3400, "filter_resonance": 0.8,
	"amp_attack": 0.002, "amp_decay": 0.5, "amp_sustain": 0.0, "amp_release": 0.15, "amp_curve": 1,
	// tuned bar overtone ~3.9x + a faint 2nd bar mode
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 3.9, "gen1_gain": 0.30, "gen1_env_fast_rate": 9,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 9.2, "gen2_gain": 0.07, "gen2_env_fast_rate": 16,
	// soft mallet chiff (band-passed noise, gone in ~15 ms)
	"gen3_source": 2, "gen3_gain": 0.06, "gen3_filt_type": 5, "gen3_filt_freq": 2000, "gen3_filt_q": 0.5,
	"gen3_env_fast_mix": 1.0, "gen3_env_fast_rate": 60, "gen3_env_tail_mix": 0.0, "gen3_env_tail_rate": 0.0,
	"gain": 0.9,
}

// vibraphoneSeed: aluminum bars with a LONG bright ring and the signature motor
// tremolo (an LFO on amplitude). Metal bar 2nd mode ~4:1 like the marimba but it
// rings far longer and the top is brighter.
var vibraphoneSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"filter_type": 0, "filter_cutoff": 5200, "filter_resonance": 0.8,
	"amp_attack": 0.002, "amp_decay": 2.8, "amp_sustain": 0.0, "amp_release": 0.4, "amp_curve": 1,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 4.0, "gen1_gain": 0.22, "gen1_env_fast_rate": 1.4,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 10.7, "gen2_gain": 0.05, "gen2_env_fast_rate": 3,
	// the motor: amplitude tremolo ~5 Hz
	"lfo_enabled": 1, "lfo_target": 0, "lfo_rate": 5.0, "lfo_depth": 0.35, "lfo_delay": 0.0,
	"gain": 0.9,
}

// glockenspielSeed: struck steel bars — very bright, glassy, long metallic ring.
// High inharmonic partials (5.4x, 10.4x, 17x) give the bell-tree sparkle; a thin
// HP strike chiff supplies the "tink".
var glockenspielSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"filter_type": 0, "filter_cutoff": 9500, "filter_resonance": 0.8,
	"amp_attack": 0.001, "amp_decay": 1.9, "amp_sustain": 0.0, "amp_release": 0.3, "amp_curve": 1,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 5.4, "gen1_gain": 0.28, "gen1_env_fast_rate": 1.8,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 10.4, "gen2_gain": 0.16, "gen2_env_fast_rate": 3,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 17.0, "gen3_gain": 0.07, "gen3_env_fast_rate": 5,
	"gen4_source": 2, "gen4_gain": 0.05, "gen4_filt_type": 4, "gen4_filt_freq": 6000, "gen4_filt_q": 0.7,
	"gen4_env_fast_mix": 1.0, "gen4_env_fast_rate": 90, "gen4_env_tail_mix": 0.0, "gen4_env_tail_rate": 0.0,
	"gain": 0.85,
}

// gamelanSeed: an Indonesian saron/bonang metallophone — the definitively
// INHARMONIC tuned metal (partials near 2.76x and 5.4x, not integer harmonics),
// with the paired-instrument "ombak" beating from a slightly detuned unison. Medium
// clangy ring. This is the deliberately-exotic one.
var gamelanSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	// the ombak shimmer: two slightly-detuned voices beating against each other
	"unison_voices": 2, "unison_detune": 12, "unison_mix": 0.5,
	"filter_type": 0, "filter_cutoff": 6000, "filter_resonance": 0.9,
	"amp_attack": 0.002, "amp_decay": 1.5, "amp_sustain": 0.0, "amp_release": 0.3, "amp_curve": 1,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2.76, "gen1_gain": 0.40, "gen1_env_fast_rate": 1.8,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 5.40, "gen2_gain": 0.22, "gen2_env_fast_rate": 3,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 8.90, "gen3_gain": 0.10, "gen3_env_fast_rate": 4,
	"gain": 0.85,
}

// ─────────────────────────────────────────────────────────────────────────────
// B · Plucked & struck strings — Karplus-Strong
// ─────────────────────────────────────────────────────────────────────────────

// kalimbaSeed: thumb-piano tines — a short, bright plucked KS with a tiny wooden
// resonator. Shorter ring than a guitar, brighter/plinkier pluck (the metal tine).
var kalimbaSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.976, "gen1_ks_pluck": 0.42,
	"filter_type": 0, "filter_cutoff": 4500,
	"amp_attack": 0.001, "amp_decay": 1.2, "amp_sustain": 0.0, "amp_release": 0.2,
	"body_model": 2, "body_mix": 0.12,
	"gain": 0.9,
}

// sitarSeed: the buzzing, droning Indian long-neck — a bright KS pluck through a
// HIGH-Q STEEL body (body_model 4, the jawari bridge's ringing overtones) at a
// strong mix, plus light drive for the sitar buzz, and a long ring.
var sitarSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.99, "gen1_ks_pluck": 0.5,
	"filter_type": 0, "filter_cutoff": 6000,
	"amp_attack": 0.001, "amp_decay": 3.0, "amp_sustain": 0.0, "amp_release": 0.4,
	// jawari buzz: strong steel body (high-Q ringing modes) + a little grit
	"body_model": 4, "body_mix": 0.5,
	"drive_enabled": 1, "drive": 0.25,
	"gain": 0.85,
}

// banjoSeed: a bright, snappy, fast-decaying twang — a very bright KS pluck with a
// short ring and a thin (drum-head) body. Brighter and shorter than any guitar.
var banjoSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.962, "gen1_ks_pluck": 0.62,
	"filter_type": 0, "filter_cutoff": 7000,
	"amp_attack": 0.001, "amp_decay": 1.0, "amp_sustain": 0.0, "amp_release": 0.2,
	"body_model": 2, "body_mix": 0.15,
	"gain": 0.9,
}

// musicBoxSeed: a delicate wind-up music box — a plucked FM bell (inharmonic
// modulator ratio) with a clean glassy ring, quiet and fragile.
var musicBoxSeed = RecipeParams{
	"osc_type": 4, "osc_enabled": 1,
	"fm_enabled": 1, "fm_algorithm": 0,
	"fm_op1_ratio": 1, "fm_op2_ratio": 3.0, "fm_op2_depth": 2.0, "fm_op1_level": 1, "fm_op2_level": 0,
	"filter_type": 0, "filter_cutoff": 7000,
	"amp_attack": 0.001, "amp_decay": 1.3, "amp_sustain": 0.0, "amp_release": 0.3, "amp_curve": 1,
	"gain": 0.7,
}

// ─────────────────────────────────────────────────────────────────────────────
// C · Keys — FM / KS / additive variations
// ─────────────────────────────────────────────────────────────────────────────

// pianoRhodesSeed: an electric-piano tine (Rhodes) — 2-op FM with the classic
// high modulator ratio (~14) that makes the metallic tine "bark", warm LP body,
// soft attack, medium decay with a little sustain.
var pianoRhodesSeed = RecipeParams{
	"osc_type": 4, "osc_enabled": 1,
	"fm_enabled": 1, "fm_algorithm": 0,
	"fm_op1_ratio": 1, "fm_op2_ratio": 14, "fm_op2_depth": 1.3, "fm_op1_level": 1, "fm_op2_level": 0,
	"filter_type": 0, "filter_cutoff": 2800,
	"amp_attack": 0.004, "amp_decay": 1.4, "amp_sustain": 0.12, "amp_release": 0.35, "amp_curve": 1,
	"gain": 0.85,
}

// clavinetSeed: a funky, bright, plucky clav — a medium-short KS with a hard bright
// pluck and light drive for the characteristic bite.
var clavinetSeed = RecipeParams{
	"osc_enabled": 0,
	// pluck brightness pulled back (was 0.58 — the raw render's energy sat on the
	// 7th harmonic = thin/plinky) so the fundamental carries the body; darker LP +
	// gentler drive keep it funky-bright without going nasal.
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.975, "gen1_ks_pluck": 0.4,
	"filter_type": 0, "filter_cutoff": 3800,
	"amp_attack": 0.001, "amp_decay": 0.7, "amp_sustain": 0.0, "amp_release": 0.12,
	"drive_enabled": 1, "drive": 0.2,
	"gain": 0.9,
}

// harpsichordSeed: a baroque quill-plucked string — a bright KS with a metallic
// 4' overtone course (a bright tone partial an octave up) and no dynamics (the
// harpsichord can't play soft): fixed velocity, medium bright ring.
var harpsichordSeed = RecipeParams{
	"osc_enabled": 0,
	"gen1_source": 3, "gen1_gain": 1.0, "gen1_ks_sustain": 0.984, "gen1_ks_pluck": 0.52,
	// the 4' course — an octave-up metallic shimmer
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.12, "gen2_env_fast_rate": 3,
	"filter_type": 0, "filter_cutoff": 6500,
	"amp_attack": 0.001, "amp_decay": 1.6, "amp_sustain": 0.0, "amp_release": 0.25,
	"body_model": 2, "body_mix": 0.12,
	"gain": 0.85,
}

// celestaSeed: a soft struck-metal keyboard (celesta/toy-piano) — like the
// glockenspiel but gentler and tinklier: lower partials, a softer strike, a
// slightly darker top.
var celestaSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"filter_type": 0, "filter_cutoff": 8000, "filter_resonance": 0.8,
	"amp_attack": 0.002, "amp_decay": 1.5, "amp_sustain": 0.0, "amp_release": 0.3, "amp_curve": 1,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 4.2, "gen1_gain": 0.22, "gen1_env_fast_rate": 2.5,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 6.8, "gen2_gain": 0.10, "gen2_env_fast_rate": 4,
	"gen3_source": 2, "gen3_gain": 0.04, "gen3_filt_type": 4, "gen3_filt_freq": 5000, "gen3_filt_q": 0.7,
	"gen3_env_fast_mix": 1.0, "gen3_env_fast_rate": 80, "gen3_env_tail_mix": 0.0, "gen3_env_tail_rate": 0.0,
	"gain": 0.8,
}

// ─────────────────────────────────────────────────────────────────────────────
// D · Winds & reeds — additive / subtractive
// ─────────────────────────────────────────────────────────────────────────────

// clarinetSeed: the hollow woody clarinet — its signature is a spectrum of ODD
// harmonics only (a cylindrical bore closed at one end), which reads as hollow and
// dark. Sustained reed onset, gentle vibrato, light band-passed breath.
var clarinetSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 4000, "filter_resonance": 1.0,
	"amp_attack": 0.04, "amp_decay": 0.1, "amp_sustain": 0.9, "amp_release": 0.15,
	// ODD harmonics only (the closed-cylinder signature)
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.50, // H1
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 3.0, "gen2_gain": 0.30, // H3
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 5.0, "gen3_gain": 0.16, // H5
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 7.0, "gen4_gain": 0.08, // H7
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 9.0, "gen5_gain": 0.04, // H9
	// sustained band-passed breath
	"gen6_source": 2, "gen6_gain": 0.05, "gen6_filt_type": 5, "gen6_filt_freq": 2000, "gen6_filt_q": 0.5,
	"gen6_env_fast_mix": 0.4, "gen6_env_fast_rate": 6, "gen6_env_tail_mix": 0.7, "gen6_env_tail_rate": 0.0,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.0, "lfo_depth": 0.01, "lfo_delay": 0.3,
	"gain": 0.85,
}

// tubaSeed: deep, round, orchestral low brass — a dark, fundamental+H2-dominant
// additive brass with a slow onset and a gentle brassy filter bloom. Slight unison
// for section warmth; a low band-passed air tap.
var tubaSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 1800, "filter_resonance": 1.0,
	"unison_voices": 2, "unison_detune": 5, "unison_mix": 0.3, "unison_drift_rate": 2.0, "unison_drift_depth": 5,
	"amp_attack": 0.06, "amp_decay": 0.1, "amp_sustain": 0.92, "amp_release": 0.2,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.55, // H1
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.45, // H2
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.22, // H3
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4.0, "gen4_gain": 0.10, // H4
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5.0, "gen5_gain": 0.04, // H5
	"gen6_source": 2, "gen6_gain": 0.03, "gen6_filt_type": 5, "gen6_filt_freq": 900, "gen6_filt_q": 0.5,
	"gen6_env_fast_mix": 0.5, "gen6_env_fast_rate": 12, "gen6_env_tail_mix": 0.7, "gen6_env_tail_rate": 0.0,
	"filtenv_enabled": 1, "filtenv_amt": 0.5, "filtenv_decay": 0.1,
	"gain": 0.9,
}

// panFluteSeed: a breathy hollow pan flute — a near-sine tone (weak upper partials)
// under a PROMINENT sustained air-noise band, gentle breathy onset, light vibrato.
// The air is louder here than the concert flute — the pan pipe is all breath.
var panFluteSeed = RecipeParams{
	"osc_enabled": 0,
	"filter_type": 0, "filter_cutoff": 3500, "filter_resonance": 1.0,
	"amp_attack": 0.05, "amp_decay": 0.05, "amp_sustain": 0.9, "amp_release": 0.12,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.60, // H1
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.10, // H2
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.04, // H3
	// prominent, sustained breath air — the defining pan-pipe sound
	"gen4_source": 2, "gen4_gain": 0.35, "gen4_filt_type": 5, "gen4_filt_freq": 2200, "gen4_filt_q": 0.4,
	"gen4_env_fast_mix": 0.5, "gen4_env_fast_rate": 5, "gen4_env_tail_mix": 0.85, "gen4_env_tail_rate": 0.0,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.5, "lfo_depth": 0.04, "lfo_delay": 0.25,
	"gain": 0.9,
}

// harmoniumSeed: a sustained reed-organ / Indian harmonium — a reedy saw with a
// nasal band-passed formant, a beating detuned-reed unison, and a steady vibrato.
// Full sustain (the bellows drone).
var harmoniumSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1, // saw = reedy
	"filter_type": 0, "filter_cutoff": 3000, "filter_resonance": 1.0,
	"unison_voices": 3, "unison_detune": 10, "unison_mix": 0.5, "unison_drift_rate": 2.0, "unison_drift_depth": 6,
	"amp_attack": 0.03, "amp_decay": 0.1, "amp_sustain": 1.0, "amp_release": 0.15,
	// nasal reed formant — a band-passed saw lift in the mids
	"gen1_source": 1, "gen1_wave": 1, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.4,
	"gen1_filt_type": 5, "gen1_filt_freq": 1200, "gen1_filt_q": 1.8,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 6.0, "lfo_depth": 0.03, "lfo_delay": 0.1,
	"gain": 0.7,
}

// ─────────────────────────────────────────────────────────────────────────────
// E · Synth leads & basses — subtractive
// ─────────────────────────────────────────────────────────────────────────────

// leadSupersawSeed: the trance supersaw — 7 detuned saws, wide unison, a bright
// resonant filter with a gentle envelope bloom. Big and shimmering.
var leadSupersawSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1,
	"unison_voices": 7, "unison_detune": 25, "unison_mix": 0.85, "unison_drift_rate": 3.0, "unison_drift_depth": 10,
	"filter_type": 0, "filter_cutoff": 6500, "filter_resonance": 1.0,
	"filtenv_enabled": 1, "filtenv_amt": 1.2, "filtenv_decay": 0.25,
	"amp_attack": 0.01, "amp_decay": 0.2, "amp_sustain": 0.85, "amp_release": 0.25,
	"gain": 0.6,
}

// leadHooverSeed: the 90s rave "hoover" — heavily detuned saws with a downward
// pitch-envelope "wah" and a resonant filter sweep, plus drive. Aggressive.
var leadHooverSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1,
	"unison_voices": 5, "unison_detune": 30, "unison_mix": 0.8, "unison_drift_rate": 2.0, "unison_drift_depth": 12,
	"pitchenv_enabled": 1, "pitchenv_amt": 5, "pitchenv_decay": 0.18, // the hoover pitch-drop
	"filter_type": 0, "filter_cutoff": 4000, "filter_resonance": 2.0,
	"filtenv_enabled": 1, "filtenv_amt": 1.5, "filtenv_decay": 0.3,
	"amp_attack": 0.008, "amp_decay": 0.25, "amp_sustain": 0.8, "amp_release": 0.2,
	"drive_enabled": 1, "drive": 0.3,
	"gain": 0.6,
}

// leadChipSeed: an 8-bit chiptune square lead — a raw square wave, minimal
// filtering, a snappy envelope and a fast vibrato (the NES "trill" character).
var leadChipSeed = RecipeParams{
	"osc_type": 2, "osc_enabled": 1, // square
	"unison_voices": 1,
	"filter_type":   0, "filter_cutoff": 12000, "filter_resonance": 0.8,
	"amp_attack": 0.001, "amp_decay": 0.08, "amp_sustain": 0.7, "amp_release": 0.05,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 7.0, "lfo_depth": 0.03, "lfo_delay": 0.05,
	"gain": 0.7,
}

// bassWobbleSeed: a dubstep wobble bass — a saw an octave down through a low
// resonant low-pass whose cutoff is swept by an LFO (the wobble), plus drive.
var bassWobbleSeed = RecipeParams{
	// NO osc_octave: every melodic instrument defaults to the same pitch (A3, node
	// pitch 0). It's a bass by timbre (low resonant filter) — pitch nodes down for
	// a bassline. (Existing bass-guitar/808/etc. keep their -1 octave; unchanged.)
	"osc_type": 1, "osc_enabled": 1,
	"filter_type": 0, "filter_cutoff": 700, "filter_resonance": 4.0,
	"lfo_enabled": 1, "lfo_target": 2, "lfo_rate": 4.0, "lfo_depth": 0.9, // cutoff wobble
	"amp_attack": 0.005, "amp_decay": 0.3, "amp_sustain": 0.85, "amp_release": 0.15,
	"drive_enabled": 1, "drive": 0.35,
	"gain": 0.9,
}

// ─────────────────────────────────────────────────────────────────────────────
// F · Experimental / textural
// ─────────────────────────────────────────────────────────────────────────────

// thereminSeed: the eerie theremin — a pure sine with a heavy expressive vibrato
// and a slow swell onset (the hands never touch the instrument). Sci-fi séance.
var thereminSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"unison_voices": 1,
	"filter_type":   0, "filter_cutoff": 5000, "filter_resonance": 0.8,
	"amp_attack": 0.3, "amp_decay": 0.1, "amp_sustain": 0.95, "amp_release": 0.4,
	// expressive vibrato, but tamed from 0.35 so the CENTER pitch reads cleanly as
	// A3 (a ±56¢ swing smeared the perceived default pitch flat).
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.5, "lfo_depth": 0.22, "lfo_delay": 0.15,
	"gain": 0.75,
}

// singingBowlSeed: a Tibetan singing bowl — inharmonic struck metal (bowl modes
// near 2.34x and 4.1x) with a VERY long ring, a slow amplitude undulation (the
// bowl's breathing shimmer) and a slow beat from a detuned unison. Meditative.
var singingBowlSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"unison_voices": 2, "unison_detune": 6, "unison_mix": 0.5, // the slow bowl beat
	"filter_type": 0, "filter_cutoff": 5000, "filter_resonance": 0.8,
	"amp_attack": 0.02, "amp_decay": 4.0, "amp_sustain": 0.0, "amp_release": 0.5, "amp_curve": 1,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2.34, "gen1_gain": 0.35, "gen1_env_fast_rate": 0.6,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 4.10, "gen2_gain": 0.18, "gen2_env_fast_rate": 1.0,
	"lfo_enabled": 1, "lfo_target": 0, "lfo_rate": 1.2, "lfo_depth": 0.15, "lfo_delay": 0.0, // slow shimmer
	"gain": 0.85,
}

// glassWashSeed: an ambient glass/waterphone wash — inharmonic partials in a slow
// swell-and-fade pad, a wide drifting detuned unison for shimmering beats, and a
// slow filter LFO that opens and closes the wash. Horror-film glass texture.
var glassWashSeed = RecipeParams{
	"osc_type": 0, "osc_enabled": 1,
	"unison_voices": 3, "unison_detune": 14, "unison_mix": 0.6, "unison_drift_rate": 0.4, "unison_drift_depth": 8,
	"filter_type": 0, "filter_cutoff": 6000, "filter_resonance": 1.0,
	"amp_attack": 0.6, "amp_decay": 0.5, "amp_sustain": 0.85, "amp_release": 1.2,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2.40, "gen1_gain": 0.30,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 3.80, "gen2_gain": 0.20,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 6.20, "gen3_gain": 0.12,
	"lfo_enabled": 1, "lfo_target": 2, "lfo_rate": 0.3, "lfo_depth": 0.4, "lfo_delay": 0.0, // slow wash motion
	"gain": 0.6,
}

// voiceAlienSeed: a morphing alien choir — a glottal-voice source through the
// formant vowel bank with a FAST vowel morph (Ah->Ee) so the pad seems to "speak",
// a humanized detuned ensemble and vibrato. Uncanny, otherworldly.
var voiceAlienSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1, // glottal voice
	"unison_voices": 4, "unison_detune": 10, "unison_mix": 0.7, "unison_drift_rate": 0.4, "unison_drift_depth": 5,
	"ens_scatter": 9, "ens_vib_rate": 5.5, "ens_vib_depth": 15, "ens_humanize": 0.7,
	"filter_type": 0, "filter_cutoff": 5000, "filter_resonance": 0.8,
	"amp_attack": 0.15, "amp_decay": 0.4, "amp_sustain": 0.9, "amp_release": 0.6,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 1, "formant_mix": 0.9,
	"formant_morph_rate": 2.5, "formant_morph_to": 2, // Ah -> Ee morph, fast
	"formant_breath": 0.1, "formant_sing": 0.3,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 5.5, "lfo_depth": 0.2, "lfo_delay": 0.2,
	"gain": 0.6,
}
