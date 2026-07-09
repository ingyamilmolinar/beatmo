package audio

// violinSeed: additive synthesis — OSC disabled, gen slots define the harmonic spectrum.
// Real violin G3 (195Hz): P2-P3 dominate (10-12x fundamental), slow attack 0.20s.
// Additive partials at harmonic ratios; formant-bank approach doesn't work well for violin
// because body formants (462, 551Hz) don't align with harmonics of G3 (195Hz).
var violinSeed = RecipeParams{
	// SOLO violin (bowed saw through the BODY-RESONATOR BANK), tuned to mtg violin-d5 (Good-Sounds, D5≈590Hz,
	// A=442). A bowed string is a SAWTOOTH (Helmholtz); a violin's character is its BODY. The pure-saw version
	// sounded like an "organ"; the body bank (`body_model:1`, 22 narrow violin signature modes incl. the
	// ~2.6kHz bridge hill, in modular.c) drives the bowed saw through the violin body → woody body coloration
	// (`body_mix` suppresses the dry as it rises so the body SHAPES the tone). A GENTLE note-SAFE high-pass
	// (130Hz, sub-bass only — a higher HP killed low notes) + the body's modal de-emphasis tame the boomy
	// fundamental the user flagged. Vibrato sweeps harmonics across the sharp modes; airy bow noise; 1 voice.
	// NOTE: the real violin's dramatic shimmer (centroid-wobble 0.29) is mostly the PLAYER's expressive bowing
	// (amplitude/brightness over the note) = performance, not static timbre — the body bank gives the body
	// coloration; full expressiveness would need bowing-dynamics modelling.
	"osc_type":         7, // BOWED-STRING PHYSICAL MODEL (digital waveguide)
	"osc_enabled":      1,
	"unison_voices":    1,    // SOLO violin (one string), not a section
	"filter_type":      0,    // LOW-PASS: the mtg D5 ref is fundamental-dominated (Tristimulus T1=0.88,
	"filter_cutoff":    3100, // centroid 2716Hz) — roll off the buzzy saw upper harmonics; LP cutoff just above
	"filter_resonance": 0.9,  // the bridge hill (~2.5kHz) keeps the body color while taming H4+ richness.
	"amp_attack":       0.30, // slower bowed onset to match ref attack ~0.21s (rendered onset was ~0.07s)
	"amp_decay":        0.10,
	"amp_sustain":      0.9,
	"amp_release":      0.2,
	// vibrato — matched to the mtg D5 ref (a fairly STRAIGHT good-sounds note): ~3.6Hz, shallow ~±2c
	"lfo_enabled": 1,
	"lfo_rate":    3.6,
	"lfo_depth":   0.025,
	"lfo_target":  1,
	"lfo_delay":   0.08,
	// THE VIOLIN BODY — modal resonator bank = the organic body coloration
	"body_model": 1,
	"body_mix":   1.0,
	// BOWING DYNAMICS — slow random loudness+brightness motion (the bow pressure/speed) = the organic life vs a static "organ"
	"bow_dynamics": 0.25,
	// continuous airy bow-hair noise (high-passed so no low rumble)
	"gen1_source": 2, "gen1_gain": 0.012, "gen1_filt_type": 4, "gen1_filt_freq": 2200, "gen1_filt_q": 0.7,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 10, "gen1_env_tail_mix": 0.85, "gen1_env_tail_rate": 0.0,
	"gain": 0.45,
}

// violinEnsembleSeed diverges the violin ensemble preset from the base modular voice:
// additive core + detuned ensemble saw + bow-noise tap + filter envelope.
var violinEnsembleSeed = RecipeParams{
	"osc_type":           1,
	"osc_enabled":        0,
	"filter_type":        0,
	"filter_cutoff":      3500,
	"filter_resonance":   1.0,
	"unison_voices":      5,
	"unison_detune":      10,
	"unison_mix":         0.42,
	"unison_drift_rate":  3.0,
	"unison_drift_depth": 8.0,
	"amp_attack":         0.14,
	"amp_decay":          0.09,
	"amp_sustain":        0.85,
	"amp_release":        0.30,
	"lfo_enabled":        1,
	"lfo_rate":           5.5,
	"lfo_depth":          0.12,
	"lfo_target":         1,
	"lfo_delay":          0.3,
	// P1 fundamental — weak
	"gen1_source":    1,
	"gen1_wave":      0,
	"gen1_freq_mode": 0,
	"gen1_freq":      1.0,
	"gen1_gain":      0.014,
	// P2 (10.57x real)
	"gen2_source":    1,
	"gen2_wave":      0,
	"gen2_freq_mode": 0,
	"gen2_freq":      2.0,
	"gen2_gain":      0.148,
	// P3 (12.20x real)
	"gen3_source":    1,
	"gen3_wave":      0,
	"gen3_freq_mode": 0,
	"gen3_freq":      3.0,
	"gen3_gain":      0.168,
	// P4 (5.60x real)
	"gen4_source":    1,
	"gen4_wave":      0,
	"gen4_freq_mode": 0,
	"gen4_freq":      4.0,
	"gen4_gain":      0.074,
	// P5 (1.71x real)
	"gen5_source":    1,
	"gen5_wave":      0,
	"gen5_freq_mode": 0,
	"gen5_freq":      5.0,
	"gen5_gain":      0.022,
	// P6 (5.68x real)
	"gen6_source":    1,
	"gen6_wave":      0,
	"gen6_freq_mode": 0,
	"gen6_freq":      6.0,
	"gen6_gain":      0.076,
	// ensemble detuned saw +7¢ (1.00404) for section width
	"gen7_source":    1,
	"gen7_wave":      1,
	"gen7_freq_mode": 0,
	"gen7_freq":      1.00404,
	"gen7_gain":      0.25,
	// bow noise tap — narrowed
	"gen8_source":        2,
	"gen8_gain":          0.06,
	"gen8_filt_type":     5,
	"gen8_filt_freq":     2000,
	"gen8_filt_q":        1.5,
	"gen8_env_fast_rate": 6,
	"gen8_env_tail_mix":  0.2,
	"gen8_env_tail_rate": 2,
	// filter envelope: gentler than solo violin
	"filtenv_enabled": 1,
	"filtenv_amt":     1.4,
	"filtenv_decay":   0.18,
	"gain":            0.9,
}

// celloSeed: SOLO cello — a BOWED-STRING PHYSICAL MODEL (osc_type:7, the STK digital-
// waveguide render_bowed_string() in modular.c) through the cello body resonator
// (body_model:3), tuned toward the real cello D2 (257996__xserra__cello-d2.wav).
// The journey: ADDITIVE sines → church organ (saved as organChurchSeed); subtractive
// saw → sci-fi lead (sciFiLeadSeed); saw + body on a low note → "radio hum". Only the
// physical model self-oscillates into a real bowed tone. osc_octave:-1 = pitch mapping.
// NOTE: the unison_*/lfo_* params below are VESTIGIAL — osc_type:7 bypasses the
// wavetable path; the body, amp, filter and bow_dynamics stages still apply.
var celloSeed = RecipeParams{
	"osc_type":    7, // BOWED-STRING PHYSICAL MODEL (digital waveguide) — render_bowed_string
	"osc_enabled": 1, // SUBTRACTIVE — the fix (was additive: osc_enabled 0)
	"osc_octave":  -1,
	// A single periodic saw reads as a steady machine/radio hum. Detuned, DRIFTING
	// voices give the non-periodic beating/shimmer of a real vibrating string (the
	// project's documented "organic leap" that kills electronic regularity).
	"unison_voices":      3,
	"unison_detune":      11,
	"unison_mix":         0.5,
	"unison_drift_rate":  2.5,
	"unison_drift_depth": 10,
	"filter_type":        0,    // low-pass
	"filter_cutoff":      2800, // body model now active → reopen for the real cello centroid ~1177
	"filter_resonance":   0.5,  // less peaky/buzzy
	"amp_attack":         0.08, // bowed onset (ref attack ~0.06s)
	"amp_decay":          0.12,
	"amp_sustain":        0.9, // sustained bow (ref DecaySlope ~-1.2 = holds, not plucky)
	"amp_release":        0.3,
	"lfo_enabled":        1,
	"lfo_rate":           4.0,   // cello vibrato ~4-6Hz
	"lfo_depth":          0.025, // subtle vibrato — a clean LFO warble on a saw reads electronic, keep light
	"lfo_target":         1,
	"lfo_delay":          0.25, // vibrato comes in sooner
	// cello body coloration (body_model 1 = violin body; its ~2.6kHz bridge hill sits
	// close to the real cello's 2470Hz hill, giving woody resonance).
	"body_model":   3,    // CELLO body resonator bank (added in modular.c)
	"body_mix":     0.55, // colour only — at high mix the fixed body formants vocode the saw into a "radio" buzz
	"bow_dynamics": 0.3,  // bow pressure/speed irregularity = organic friction (research: bowing-force noise → natural feel)
	// SUSTAINED bow-friction noise — the gritty bow-on-string texture. Research:
	// this is what separates a real bowed string from an electronic saw (ref NoiseRatio ~0.34).
	"gen1_source": 2, "gen1_gain": 0.022, "gen1_filt_type": 4, "gen1_filt_freq": 1400, "gen1_filt_q": 0.7,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 10, "gen1_env_tail_mix": 0.65, "gen1_env_tail_rate": 0.0,
	// filter envelope — cutoff opens on the bow attack then settles, so the spectrum
	// EVOLVES (bow articulation) instead of sitting as a static buzz.
	"filtenv_enabled": 1,
	"filtenv_amt":     1.0,
	"filtenv_decay":   0.3,
	"gain":            0.55, // not 0.5: TestPhase2Native mutates gain->0.5 and needs a delta
}

// sciFiLeadSeed: another happy accident — the cello's bowed-saw-through-body tuning,
// before the body resonator was dialed in, read as a bright sci-fi / 8-bit synth lead
// (a raw sawtooth through a resonant low-pass = the classic analog-synth-lead timbre).
// Kept verbatim as its own instrument. osc_octave:-1 retained.
var sciFiLeadSeed = RecipeParams{
	"osc_type":         1,
	"osc_enabled":      1,
	"osc_octave":       -1,
	"unison_voices":    1,
	"filter_type":      0,
	"filter_cutoff":    2300,
	"filter_resonance": 0.8,
	"amp_attack":       0.08,
	"amp_decay":        0.12,
	"amp_sustain":      0.9,
	"amp_release":      0.3,
	"lfo_enabled":      1,
	"lfo_rate":         4.0,
	"lfo_depth":        0.03,
	"lfo_target":       1,
	"lfo_delay":        0.3,
	"body_model":       1,
	"body_mix":         0.7,
	"bow_dynamics":     0.2,
	"gen1_source":      2, "gen1_gain": 0.007, "gen1_filt_type": 4, "gen1_filt_freq": 1800, "gen1_filt_q": 0.7,
	"gen1_env_fast_mix": 0.4, "gen1_env_fast_rate": 10, "gen1_env_tail_mix": 0.35, "gen1_env_tail_rate": 0.0,
	"gain": 0.55,
}

// organChurchSeed: a happy accident — an early additive-cello tuning (dominant 2nd
// harmonic + extended upper partials + heavy unison/drift, OSC disabled) that read
// as a sustained pipe/church organ rather than a bowed cello. Kept verbatim as its
// own instrument. osc_octave:-1 retained (plays an octave below the requested pitch).
var organChurchSeed = RecipeParams{
	"osc_type":           1,
	"osc_enabled":        0,
	"osc_octave":         -1,
	"filter_type":        0,
	"filter_cutoff":      5616.350246241032,
	"filter_resonance":   3,
	"unison_voices":      7,
	"unison_detune":      40,
	"unison_mix":         0.65,
	"unison_drift_rate":  3.0,
	"unison_drift_depth": 8.0,
	"amp_attack":         0.05179144526863056,
	"amp_decay":          0.85,
	"amp_sustain":        0.62,
	"amp_release":        0.34967507497621253,
	"lfo_enabled":        1,
	"lfo_rate":           3.684847594572423,
	"lfo_depth":          0.04018237211705741,
	"lfo_target":         1,
	"lfo_delay":          0.3,
	"gen1_source":        1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.12,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 2.0, "gen2_gain": 0.3,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 3.0, "gen3_gain": 0.1,
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4.0, "gen4_gain": 0.16,
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5.0, "gen5_gain": 0.08,
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 6.0, "gen6_gain": 0.15,
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 7.0, "gen7_gain": 0.07,
	"gen8_source": 1, "gen8_wave": 0, "gen8_freq_mode": 0, "gen8_freq": 8.0, "gen8_gain": 0.26,
	"gen9_source": 1, "gen9_wave": 0, "gen9_freq_mode": 0, "gen9_freq": 10.0, "gen9_gain": 0.22,
	"gen10_source": 1, "gen10_wave": 0, "gen10_freq_mode": 0, "gen10_freq": 13.0, "gen10_gain": 0.22,
	"gen11_source": 1, "gen11_wave": 0, "gen11_freq_mode": 0, "gen11_freq": 16.0, "gen11_gain": 0.2,
	"gen12_source": 2, "gen12_gain": 0.035, "gen12_filt_type": 5, "gen12_filt_freq": 600, "gen12_filt_q": 0.8,
	"gen12_env_fast_rate": 6, "gen12_env_tail_mix": 0.05, "gen12_env_tail_rate": 2,
	"filtenv_enabled": 1,
	"filtenv_amt":     1.593822115304857,
	"filtenv_decay":   0.19947731679846067,
	"gain":            0.88,
	"gen1_filt_freq":  3329.1038595643486,
}

// celloWarmSeed: warm cello — same additive approach but emphasis on P2-P4 warmth,
// slower attack, deeper vibrato. osc_octave:-1 retained.
var celloWarmSeed = RecipeParams{
	"osc_type":           1,
	"osc_enabled":        0,
	"osc_octave":         -1,
	"filter_type":        0,
	"filter_cutoff":      1800,
	"filter_resonance":   1.0,
	"unison_voices":      7,
	"unison_detune":      40,
	"unison_mix":         0.65,
	"unison_drift_rate":  3.0,
	"unison_drift_depth": 8.0,
	"amp_attack":         0.16,
	"amp_decay":          0.12,
	"amp_sustain":        0.92,
	"amp_release":        0.50,
	"lfo_enabled":        1,
	"lfo_rate":           4.6,
	"lfo_depth":          0.11,
	"lfo_target":         1,
	"lfo_delay":          0.3,
	// P1 fundamental — very weak
	"gen1_source":    1,
	"gen1_wave":      0,
	"gen1_freq_mode": 0,
	"gen1_freq":      1.0,
	"gen1_gain":      0.025,
	// P2 (3.41x) — slightly emphasized for warmth
	"gen2_source":    1,
	"gen2_wave":      0,
	"gen2_freq_mode": 0,
	"gen2_freq":      2.0,
	"gen2_gain":      0.095,
	// P3 (11.10x — dominant)
	"gen3_source":    1,
	"gen3_wave":      0,
	"gen3_freq_mode": 0,
	"gen3_freq":      3.0,
	"gen3_gain":      0.278,
	// P4 (3.32x)
	"gen4_source":    1,
	"gen4_wave":      0,
	"gen4_freq_mode": 0,
	"gen4_freq":      4.0,
	"gen4_gain":      0.083,
	// P5 (3.33x)
	"gen5_source":    1,
	"gen5_wave":      0,
	"gen5_freq_mode": 0,
	"gen5_freq":      5.0,
	"gen5_gain":      0.083,
	// P6 (2.08x) — slightly lower for warmth
	"gen6_source":    1,
	"gen6_wave":      0,
	"gen6_freq_mode": 0,
	"gen6_freq":      6.0,
	"gen6_gain":      0.045,
	// bow noise tap — lower BP for warm cello
	"gen7_source":        2,
	"gen7_gain":          0.015,
	"gen7_filt_type":     5,
	"gen7_filt_freq":     800,
	"gen7_filt_q":        2.0,
	"gen7_env_fast_rate": 5,
	"gen7_env_tail_mix":  0.18,
	"gen7_env_tail_rate": 2,
	// filter envelope: gentle warm bow attack
	"filtenv_enabled": 1,
	"filtenv_amt":     1.3,
	"filtenv_decay":   0.25,
	"gain":            0.90,
}

// ---------------------------------------------------------------------------
// Task 2: Plucked Strings (guitars — Karplus-Strong)
// ---------------------------------------------------------------------------

// guitarNylonSeed: KS gen-bank slot; osc silent; warm nylon guitar body filter.
// Real nylon guitar: warm rolloff ~1500Hz, slow decay, plucked (near-instant attack),
// fundamental-dominant (P2 falls off naturally with KS). Less sustain than steel.
// Tuned to a real nylon-strung guitar (kyster 2-oct-c, CC0, C5≈525Hz). The pluck is MELLOW and
// FUNDAMENTAL-DOMINANT: just after the pluck H2 −10, H3 −38 (H3+ nearly absent — a mandolin/banjo has
// strong bright upper harmonics, which is why the old seed read as a mandolin). It RINGS ~1.1s (decay
// to −20dB) with the upper harmonics decaying faster than the fundamental (the natural KS loop-damping),
// and a soft ~40ms attack. So: high ks_sustain (long ring at this pitch), low ks_pluck (dark/soft pluck →
// kill the mid twang), a warm body LP, and a soft amp attack.
var guitarNylonSeed = RecipeParams{
	"osc_enabled":      0,
	"gen1_source":      3,
	"gen1_gain":        1.0,
	"gen1_ks_sustain":  0.997, // long ring — ~1.1s decay at C5 (old 0.972 died in ~160ms = plinky mandolin)
	"gen1_ks_pluck":    0.09,  // softer/darker pluck (was 0.13) — less harpy high-harmonic click
	"filter_type":      0,
	"filter_resonance": 0.8,   // FLAT (was 2.6): the Q-peak sat on H2 (~1048Hz=H2 of C5) boosting the 2nd
	"filter_cutoff":    3600,  // harmonic into a "harp/organ" formant. WARM nylon roll-off (ear: 1700 was too
	"amp_attack":       0.009, // bright/harpy); a touch more attack softens the razor pluck-click without organ.
	"amp_decay":        2.6,   // (was 45ms soft onset = organ-like swell; a finger pluck is ~0ms transient).
	"amp_sustain":      0.0,
	"amp_release":      0.3,
	// CLASSICAL-GUITAR BODY (body_model:2, modular.c) — low-mid box resonances (Helmholtz ~100Hz, top
	// ~195Hz, back ~270Hz...) give the "box"/warmth that separates a guitar from a bare plucked string (harp).
	"body_model": 2,
	"body_mix":   0.30,
	"gain":       0.95,
}

// guitarNylonBrightSeed: brighter nylon with higher pluck brightness.
var guitarNylonBrightSeed = RecipeParams{
	"osc_enabled":     0,
	"gen1_source":     3,
	"gen1_gain":       1.0,
	"gen1_ks_sustain": 0.982,
	"gen1_ks_pluck":   0.27,
	"filter_cutoff":   5000,
	"amp_attack":      0.002,
	"amp_decay":       1.4,
	"amp_sustain":     0.0,
	"amp_release":     0.2,
	"gain":            0.95,
}

// guitarSteelSeed: STEEL-string acoustic — bright, ringing, metallic. The steel
// character vs nylon: a brighter/sharper pluck (ks_pluck 0.6 vs nylon's 0.09), a
// long bright sustain (the highs ring — that's the metallic zing), an open filter,
// AND the high-Q steel body (body_model:4) whose ringing upper modes give the
// bright acoustic "presence" a bare string lacks. Contrast guitarNylonSeed (warm
// body_model:2, soft dark pluck).
var guitarSteelSeed = RecipeParams{
	"osc_enabled":     0,
	"gen1_source":     3,
	"gen1_gain":       1.0,
	"gen1_ks_sustain": 0.988, // long bright ring (the metallic sustain)
	"gen1_ks_pluck":   0.6,   // bright/sharp pluck (vs nylon 0.09)
	"filter_cutoff":   7000,
	"amp_attack":      0.001,
	"amp_decay":       2.5,
	"amp_sustain":     0.0,
	"amp_release":     0.3,
	"body_model":      4, // STEEL body: high-Q ringing upper modes = the metallic zing
	"body_mix":        0.30,
	"gain":            0.9,
}

// guitarSteelWarmSeed: warmer steel (e.g. a mellower dreadnought) — steel body but
// a softer pluck + darker filter than the bright steel above.
var guitarSteelWarmSeed = RecipeParams{
	"osc_enabled":     0,
	"gen1_source":     3,
	"gen1_gain":       1.0,
	"gen1_ks_sustain": 0.982,
	"gen1_ks_pluck":   0.28,
	"filter_cutoff":   3400,
	"amp_attack":      0.001,
	"amp_decay":       2.2,
	"amp_sustain":     0.0,
	"amp_release":     0.3,
	"body_model":      4,
	"body_mix":        0.18,
	"gain":            0.9,
}

// harpSeed: CELTIC HARP — a bright, clean, long-RINGING plucked string (Karplus-
// Strong, like the guitars but more OPEN/shimmering): a brighter clear pluck, a
// much longer free sustain (the strings ring out), and a lighter soundboard than
// a guitar's box. Reference: celtic harp G2 (daphne_in_wonderland, CC0).
var harpSeed = RecipeParams{
	"osc_enabled":     0,
	"gen1_source":     3,
	"gen1_gain":       1.0,
	"gen1_ks_sustain": 0.993, // very long ring — harp strings sustain freely
	"gen1_ks_pluck":   0.25,  // bright, clear, shimmering pluck
	"filter_cutoff":   3300,  // open/bright
	"amp_attack":      0.001, // instant pluck transient
	"amp_decay":       3.5,   // long ring-out
	"amp_sustain":     0.0,
	"amp_release":     0.5,
	"body_model":      2, // light soundboard coloration (harps are more open than a guitar box)
	"body_mix":        0.18,
	"gain":            0.9,
}

// guitarElectricSeed: electric guitar with very long sustain + mild resonance.
var guitarElectricSeed = RecipeParams{
	"osc_enabled":      0,
	"gen1_source":      3,
	"gen1_gain":        1.0,
	"gen1_ks_sustain":  0.99,
	"gen1_ks_pluck":    0.5,
	"filter_cutoff":    5000,
	"filter_resonance": 1.2,
	"amp_attack":       0.001,
	"amp_decay":        4.0,
	"amp_sustain":      0.0,
	"amp_release":      0.3,
	"gain":             0.9,
}

// guitarElectricNeckSeed: neck pickup — warmer, longer sustain.
var guitarElectricNeckSeed = RecipeParams{
	"osc_enabled":     0,
	"gen1_source":     3,
	"gen1_gain":       1.0,
	"gen1_ks_sustain": 0.992,
	"gen1_ks_pluck":   0.4,
	"filter_cutoff":   3800,
	"amp_attack":      0.001,
	"amp_decay":       4.0,
	"amp_sustain":     0.0,
	"amp_release":     0.3,
	"gain":            0.9,
}

// ---------------------------------------------------------------------------
// Task 3: Keys (acoustic piano — additive)
// ---------------------------------------------------------------------------

// pianoGrandSeed: additive sine partials at ratios 1/2/3/4 + hammer noise tap.
var pianoGrandSeed = RecipeParams{
	// ACOUSTIC-PIANO note (struck string). The previous version sounded ELECTRONIC
	// because its partials were EXACT integer harmonics (freq 2,3,4). A real piano
	// string is STIFF, so its partials are STRETCHED progressively SHARP:
	// f_n = n·f0·√(1+B·n²). Measured on the ref (pinkyfinger piano-g, G5≈792Hz):
	// H3 +5, H4 +15, H5 +28, H6 +42 cents (B≈0.0013) — the inharmonic shimmer that
	// reads as a real struck string, NOT a synth/organ. Also: more upper partials +
	// a brighter filter (the ref centroid is ~1686 Hz, not the old dark 1021), a
	// slightly-detuned unison for beating, and a SHORT (~20 ms) percussive HAMMER
	// transient (the felt thunk of the strike — NOT sustained hiss).
	"osc_type":      0, // sine fundamental (exact; the stiffness stretch is in the upper partials)
	"osc_enabled":   1,
	"osc_detune":    1.5,   // slight unison detune = the piano's mistuned string pairs (beating/warmth)
	"amp_attack":    0.006, // sharp hammer strike
	"amp_decay":     0.9,   // struck decay — longer ring (the ref sustains, less plinky)
	"amp_curve":     1,
	"amp_sustain":   0.0,
	"amp_release":   0.3,
	"filter_type":   0,
	"filter_cutoff": 6000, // let H5/H6 through — the ref is brighter (centroid ~1686 Hz)
	// STRETCHED upper partials (stiff-string inharmonicity) — the anti-electronic fix.
	// Frequencies = n·2^(measured cents/1200); upper partials boosted + slower decay
	// so the sustained tone keeps the ref's brightness (not just the strike).
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2.0000, "gen1_gain": 0.48, "gen1_env_fast_rate": 3.0, // H2
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 3.0087, "gen2_gain": 0.26, "gen2_env_fast_rate": 4.0, // H3 +5c
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 4.0347, "gen3_gain": 0.26, "gen3_env_fast_rate": 5.0, // H4 +15c
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 5.0813, "gen4_gain": 0.20, "gen4_env_fast_rate": 6.0, // H5 +28c
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 6.1466, "gen5_gain": 0.13, "gen5_env_fast_rate": 7.0, // H6 +42c
	// percussive hammer strike — brief broadband thunk, gone in ~20 ms (no sustained hiss)
	"gen6_source": 2, "gen6_gain": 0.22, "gen6_filt_type": 5, "gen6_filt_freq": 2600, "gen6_filt_q": 0.4,
	"gen6_env_fast_mix": 1.0, "gen6_env_fast_rate": 45, "gen6_env_tail_mix": 0.0, "gen6_env_tail_rate": 0.0,
	"gain": 0.9,
}

// pianoFeltSeed: felt-damped piano — darker filter, softer attack, fewer partials.
var pianoFeltSeed = RecipeParams{
	// CLEAN felt-damped piano — a darker, softer sibling of pianoGrandSeed: the same
	// clean struck-string approach (sine + gentle decaying partials + slight string
	// detune; NO hammer-noise gen, NO phantom sub-octave) but a duller filter, a
	// softer strike, and weaker upper partials (the felt over the hammers mutes the
	// highs). Pure note, no background hiss.
	"osc_type":      0,
	"osc_enabled":   1,
	"osc_detune":    1.5,
	"amp_attack":    0.008, // softer felt strike
	"amp_curve":     1,     // exponential struck decay
	"amp_decay":     0.9,
	"amp_sustain":   0.0,
	"amp_release":   0.3,
	"filter_type":   0,
	"filter_cutoff": 4000, // duller, felt-damped tone (darker than the grand)
	// STRETCHED partials (stiff-string inharmonicity — the anti-electronic fix),
	// but weaker/darker upper partials than the grand (felt mutes the highs).
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2.0000, "gen1_gain": 0.38, "gen1_env_fast_rate": 4.0, // H2
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 3.0087, "gen2_gain": 0.16, "gen2_env_fast_rate": 6.0, // H3 +5c
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 4.0347, "gen3_gain": 0.10, "gen3_env_fast_rate": 8.0, // H4 +15c
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 5.0813, "gen4_gain": 0.05, "gen4_env_fast_rate": 10.0, // H5 +28c
	// soft felt-hammer thunk — quieter + darker than the grand
	"gen5_source": 2, "gen5_gain": 0.14, "gen5_filt_type": 5, "gen5_filt_freq": 1800, "gen5_filt_q": 0.4,
	"gen5_env_fast_mix": 1.0, "gen5_env_fast_rate": 55, "gen5_env_tail_mix": 0.0, "gen5_env_tail_rate": 0.0,
	"gain": 0.9,
}

// ---------------------------------------------------------------------------
// Task 4: Winds — Woodwind (flute, oboe)
// ---------------------------------------------------------------------------

// fluteSeed: additive partials matching real flute profile + breath noise + vibrato.
// Real flute D4: strong 2nd harmonic (p2=0.83, near-equal), scattered even/odd upper
// partials (p4=0.24, p6=0.17), breath noise, big centroid motion, cMean=1812Hz.
// fluteSeed — BLOWN WAVEGUIDE (Surge XT "Constant" / STK / RipplerX open-tube). A real flute is a
// resonant air column driven by continuous breath, NOT a sum of static sines (which read as an organ).
// gen_source:3 (Karplus-Strong delay line) + gen_ks_blow>0 runs the blown branch in modular_gen_slot_ks:
// continuous filtered breath noise into a tuned delay with a tanh-bounded feedback loop (self-limiting
// limit cycle → pitched, breathy, never-exactly-repeating). Verified by TestBlownKS_* (bounded + sustains
// + pitched). gen_ks_sustain → blown loop gain (×1.05 inside; ~0.96-0.99 sustains), gen_ks_blow = breath
// drive, gen_ks_pluck = reflection/air-color lowpass alpha.
var fluteSeed = RecipeParams{
	// FLUTE = ADDITIVE near-sine tone + CONTINUOUS BREATH AIR (osc_enabled:0). The
	// old Karplus-Strong blown tube (gen1_source:3) read HARPY: at low blow the KS
	// excitation decays like a PLUCKED STRING (that's what KS is) rather than a
	// continuously BLOWN pipe — and raising blow to sustain it made it 5x too airy
	// and warbly (the KS delay pitch-mod is nonlinear). Additive is the right model
	// (same as the oboe fix): a sustained sparse harmonic series (real flute E5 is
	// near a sine — P1 1.0, P2 .27, P3 .10) + a PROMINENT, SUSTAINED band-passed
	// breath hiss = the defining "blowing" air, present the whole note, not a
	// decaying pluck. Additive pitch vibrato is stable, so the flute can actually
	// waver (~8 cents) without the KS wander.
	"osc_enabled":      0,
	"filter_type":      0,
	"filter_cutoff":    2900, // the flute centroid is only ~2 kHz — roll off the top for a rounder, less whistly tone
	"filter_resonance": 1.0,
	"amp_attack":       0.04, // soft breathy chiff onset
	"amp_decay":        0.05,
	"amp_sustain":      0.94,
	"amp_release":      0.12,
	// PITCH vibrato — the air-jet wavers (~8 cents, 5.6 Hz). Stable in additive.
	"lfo_enabled": 1,
	"lfo_rate":    5.6,
	"lfo_depth":   0.05,
	"lfo_target":  1,
	"lfo_delay":   0.25,
	// sparse near-sine harmonic series (a real flute is close to a sine + air)
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.55, // H1
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.15, // H2 (ref .27)
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.05, // H3 (ref .10)
	// CONTINUOUS breath air — the flute's defining sound. Band-passed around the
	// blowing frequencies, SUSTAINED for the whole note (tail_mix high, no decay),
	// prominent (this is what makes it read as BLOWN, not plucked/organ).
	"gen1_source": 2, "gen1_gain": 0.42, "gen1_filt_type": 5, "gen1_filt_freq": 2100, "gen1_filt_q": 0.45,
	"gen1_env_fast_mix": 0.4, "gen1_env_fast_rate": 4, "gen1_env_tail_mix": 0.9, "gen1_env_tail_rate": 0.0,
	"gain": 0.9,
}

// fluteBreathySeed: breathier flute — more noise + same harmonic structure.
var fluteBreathySeed = RecipeParams{
	"osc_type":      0,
	"osc_enabled":   0,
	"filter_type":   0,
	"filter_cutoff": 7000,
	"amp_attack":    0.09,
	"amp_sustain":   0.80,
	"amp_release":   0.14,
	"lfo_enabled":   1,
	"lfo_rate":      5.5,
	"lfo_depth":     0.13,
	"lfo_target":    1,
	"lfo_delay":     0.20,
	// P1 fundamental
	"gen1_source":    1,
	"gen1_wave":      0,
	"gen1_freq_mode": 0,
	"gen1_freq":      1.0,
	"gen1_gain":      0.120,
	// P2 (real p2=0.83)
	"gen2_source":    1,
	"gen2_wave":      0,
	"gen2_freq_mode": 0,
	"gen2_freq":      2.0,
	"gen2_gain":      0.100,
	// P4 (real p4=0.24)
	"gen3_source":    1,
	"gen3_wave":      0,
	"gen3_freq_mode": 0,
	"gen3_freq":      4.0,
	"gen3_gain":      0.029,
	// P6 (real p6=0.17)
	"gen4_source":    1,
	"gen4_wave":      0,
	"gen4_freq_mode": 0,
	"gen4_freq":      6.0,
	"gen4_gain":      0.020,
	// Breath noise — more prominent than standard flute
	"gen5_source":        2,
	"gen5_gain":          0.22,
	"gen5_filt_type":     5,
	"gen5_filt_freq":     2800,
	"gen5_filt_q":        0.7,
	"gen5_env_fast_rate": 0.5,
	// filter envelope
	"filtenv_enabled": 1,
	"filtenv_amt":     1.2,
	"filtenv_decay":   0.18,
	"gain":            0.88,
}

// oboeSeed: SUBTRACTIVE reed synthesis (mirrors trumpetSeed's saw→band-pass→filtenv
// architecture). The previous additive sine-bank version had a clean ~11ms onset because
// the partial gen-slots fired at full gain on frame 1 — amp_attack could not slow the
// MEASURED time-to-80%-peak. A SAW source routed through the amp ADSR fixes this: the saw
// is the tone source so amp_attack produces a genuine slow reed onset (real oboe ≈0.08s).
// Architecture:
//   - osc_enabled=1, osc_type=1 (SAW) — full harmonic series, shaped by the filter below.
//   - global BAND-PASS @1450Hz (filter_type=2) — the reed FORMANT: real oboe D4 (F0≈294Hz)
//     peaks at P4-P5 ≈ 1.2-1.5kHz, NOT at the fundamental. BP suppresses the strong saw
//     fundamental so the mid-harmonic reed "vowel" dominates (bright, nasal, reedy).
//   - filtenv +2.6 oct / 0.20s decay — BP center BLOOMS up on attack then settles, so the
//     centroid MOVES (evolving brightness, not a static organ).
//   - gen1 = upper-formant lift (band-pass saw @2400Hz) — keeps the bright nasal top the
//     main BP rolls off, pushing the centroid toward the real oboe's high Cmean.
//   - gen2 = reed CHIFF — light high-passed noise burst, fast decay for the breathy onset.
//   - amp_attack=0.05 — reed speaks fast but not instant; the saw routing makes this measure.
//   - 6Hz vibrato (lfo_target=1, pitch) for the characteristic oboe wobble.
var oboeSeed = RecipeParams{
	// OBOE = ADDITIVE reed formant (osc_enabled:0, sine bank). The old reed
	// WAVEGUIDE (osc_type:9 render_reed) was a CLARINET — inverting single-delay
	// bell → odd harmonics only (measured even/odd 0.03 vs the real oboe's 0.53).
	// The conical two-delay model gives even+odd but its limit cycle wanders
	// 20–140 cents at the oboe's short bore (unstable). Additive is rock-stable
	// and places the real oboe's H3-DOMINANT reedy spectrum directly: measured
	// (acclivity A4) H1 1.0, H2 .26, H3 1.39 (peak), H4 .63, H5 .60, H6 .18 — a
	// reed formant around H3–H5 that reads bright/nasal. A band-passed reed-buzz
	// noise supplies the ~0.5 broadband content; a fast reedy attack + subtle
	// vibrato the movement.
	"osc_enabled":      0,
	"filter_type":      0,
	"filter_cutoff":    7000, // bright/nasal — the oboe centroid is ~3 kHz
	"filter_resonance": 1.0,
	"amp_attack":       0.012, // fast reedy speech
	"amp_decay":        0.05,
	"amp_sustain":      0.92,
	"amp_release":      0.10,
	// subtle vibrato (real oboe is nearly straight, ~2 cents)
	"lfo_enabled": 1,
	"lfo_rate":    5.6,
	"lfo_depth":   0.015,
	"lfo_target":  1,
	"lfo_delay":   0.2,
	// measured H3-dominant reed formant (gains boosted ~2x for gen-slot attenuation)
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.42, // H1
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.12, // H2 (ref weak)
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.58, // H3 PEAK
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 4.0, "gen5_gain": 0.27, // H4
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 5.0, "gen6_gain": 0.25, // H5
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.08, // H6
	"gen8_source": 1, "gen8_wave": 0, "gen8_freq_mode": 0, "gen8_freq": 7.0, "gen8_gain": 0.05, // H7
	// band-passed reed-buzz noise, SUSTAINED (real oboe ~0.54 broadband); centred
	// near the reed formant so it adds buzz without over-brightening the centroid.
	"gen1_source": 2, "gen1_gain": 0.7, "gen1_filt_type": 5, "gen1_filt_freq": 1900, "gen1_filt_q": 0.35,
	"gen1_env_fast_mix": 0.6, "gen1_env_fast_rate": 10, "gen1_env_tail_mix": 0.6, "gen1_env_tail_rate": 0.0,
	// slight reedy attack bloom
	"filtenv_enabled": 1, "filtenv_amt": 0.4, "filtenv_decay": 0.1,
	"gain": 0.75,
}

// oboeFullSeed: fuller oboe — same SUBTRACTIVE saw→band-pass→filtenv architecture as
// oboeSeed but BRIGHTER and louder (higher BP center, stronger upper-formant lift, bigger
// filtenv bloom) for a fatter reed tone. Distinct from oboeSeed (seed-distinct guard).
// FULLER oboe — the additive oboe with a stronger fundamental + upper body and
// higher gain for a fatter reed tone (no longer the clarinet reed waveguide).
var oboeFullSeed = cloneSeed(oboeSeed, RecipeParams{
	"gen2_gain":   0.58, // fuller fundamental
	"gen5_gain":   0.32, // more H4 body
	"gen6_gain":   0.30, // more H5 body
	"amp_sustain": 0.95,
	"amp_release": 0.12,
	"gain":        0.9,
})

// ---------------------------------------------------------------------------
// Task 5: Winds — Brass (trumpet, french horn)
// ---------------------------------------------------------------------------

// trumpetSeed: SUBTRACTIVE brass synthesis — the additive sine-bank version
// (12 osc_enabled=0 gen slots) "sounded like an organ": a static spectrum with a
// clean ~37ms onset and no evolving brightness. Real brass is a SAW (all 16+
// harmonics) shaped by an opening filter — the centroid BLOOMS on the air-onset
// then settles, and the attack is slow (real trumpet E3 onset ≈ 0.21s).
//
// Architecture (signal flows osc → gen bank → amp ADSR → global filter+filtenv):
//   - osc_enabled=1, osc_type=1 (SAW) — supplies the full harmonic series incl.
//     P13-P16 the 12-slot additive bank could never reach (the missing upper
//     partials were ~14 of the old 21 partial-distance and the 290Hz cMean gap).
//   - global LP filter @1450Hz + filtenv +3.4 oct / 0.26s decay — the cutoff
//     opens bright on attack then settles, so the centroid MOVES (brass bite),
//     not a flat organ. This whole-mix stage is what produces CentroidRange.
//   - gen1 = brass FORMANT: a band-pass saw @1200Hz (filt_type=5 RBJ-BP, Q≈2.5)
//     lifting the P6-P9 "vowel" peak the LP alone would roll off.
//   - gen2 = attack CHIFF: a high-passed noise tap, very fast decay
//     (env_fast_rate≈150) for the air-onset transient.
//   - amp_attack=0.12 — slow brass onset (no per-slot gen spike to shortcut it).
//   - unison 3 + drift + lfo_target=0 amplitude shimmer for ensemble life
//     (vibrato/pitch-LFO would disable the unison branch in the engine, so the
//     shimmer rides amplitude instead — keeps the ensemble width).
var trumpetSeed = RecipeParams{
	// TRUMPET — ADDITIVE brass formant. The real ref (sgossner SHTrumpet, CC0, G4≈392)
	// is H3-DOMINANT (H2=1.88, H3=2.33 peak, H4=1.14) — a bright brass formant across
	// H2–H4, NOT the fundamental-dominant saw the old seed produced (which read "very
	// bad"). Same additive approach that fixed the french horn. Keeps the trumpet
	// DYNAMICS: a slow crescendo + a filter-env bloom that brightens as it swells, a
	// noisy tonguing attack, and amplitude tremolo (NO pitch vibrato — straight tone).
	"osc_enabled":      0,
	"filter_type":      0,
	"filter_cutoff":    4000, // gentle roll-off; the brass formant is set by the gen gains
	"filter_resonance": 1.0,
	"amp_attack":       0.45, // crescendo swell
	"amp_decay":        0.10,
	"amp_sustain":      0.95,
	"amp_release":      0.15,
	"filtenv_enabled":  1,
	"filtenv_amt":      1.6, // brass bloom: cutoff rises over the crescendo (dark->bright)
	"filtenv_attack":   0.4,
	"filtenv_decay":    2.0, // honest ParamDef max (was 4.0, silently clamped to 2.0 in the recipe path)
	"lfo_enabled":      1,
	"lfo_rate":         3.2,
	"lfo_depth":        0.06, // amplitude tremolo only (real trumpet is a straight tone)
	"lfo_target":       0,
	"lfo_delay":        0.3,
	// brass formant harmonics (measured ratios, boosted ~3.5x for gen-slot attenuation):
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.30, // H1
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.55, // H2 (ref 1.88)
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.68, // H3 PEAK (ref 2.33)
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 4.0, "gen5_gain": 0.34, // H4 (ref 1.14)
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 5.0, "gen6_gain": 0.20, // H5
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.13, // H6
	"gen8_source": 1, "gen8_wave": 0, "gen8_freq_mode": 0, "gen8_freq": 7.0, "gen8_gain": 0.16, // H7
	// noisy tonguing/breath attack (real 78%->12% noise)
	"gen1_source": 2, "gen1_gain": 0.6, "gen1_filt_type": 5, "gen1_filt_freq": 1500, "gen1_filt_q": 0.3,
	"gen1_env_fast_mix": 1.0, "gen1_env_fast_rate": 6, "gen1_env_tail_mix": 0.08, "gen1_env_tail_rate": 2.5,
	"gain": 0.7,
}

// trumpetMellowSeed: mellow/muted trumpet — the SAME subtractive brass
// architecture as trumpetSeed (saw → band-pass formant → opening filtenv) but
// DARKER: a lower band-pass center, a gentler/slower filtenv bloom, a slightly
// slower attack and tighter unison. The lower BP center keeps the upper
// partials softer (muted-trumpet timbre) while the evolving filter still moves
// the centroid (not a static organ).
var trumpetMellowSeed = RecipeParams{
	// MELLOW trumpet — the SAME additive brass formant as the base trumpet but
	// softer/darker: more fundamental + gentler upper harmonics (less H3/H4
	// brilliance), darker roll-off, a smaller brass bloom, slower softer onset.
	// Was an old saw; re-synced to the additive-formant model.
	"osc_enabled":      0,
	"filter_type":      0,
	"filter_cutoff":    2800,
	"filter_resonance": 1.0,
	"amp_attack":       0.5,
	"amp_decay":        0.10,
	"amp_sustain":      0.92,
	"amp_release":      0.18,
	"filtenv_enabled":  1,
	"filtenv_amt":      1.0,
	"filtenv_attack":   0.45,
	"filtenv_decay":    2.0, // honest ParamDef max (was 4.0, silently clamped to 2.0 in the recipe path)
	"lfo_enabled":      1,
	"lfo_rate":         3.0,
	"lfo_depth":        0.05,
	"lfo_target":       0,
	"lfo_delay":        0.3,
	"gen2_source":      1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.40, // H1 (mellow = more fundamental)
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.50, // H2
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.42, // H3 (less peak than bright)
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 4.0, "gen5_gain": 0.18, // H4
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 5.0, "gen6_gain": 0.09, // H5
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.05, // H6
	"gen1_source": 2, "gen1_gain": 0.4, "gen1_filt_type": 5, "gen1_filt_freq": 1200, "gen1_filt_q": 0.3,
	"gen1_env_fast_mix": 1.0, "gen1_env_fast_rate": 6, "gen1_env_tail_mix": 0.06, "gen1_env_tail_rate": 2.5,
	"gain": 0.72,
}

// frenchHornSeed: SUBTRACTIVE horn synthesis (mirrors trumpetSeed's saw→filter→filtenv
// architecture, but DARKER and SLOWER). The previous additive sine-bank had a ~37ms onset
// (gen-slots fired instantly); a SAW source routed through the amp ADSR gives a genuine slow
// mellow horn onset (real horn ≈0.43s). Architecture:
//   - osc_enabled=1, osc_type=1 (SAW) — full odd+even harmonic series, the horn's mellow body.
//   - global LOW-PASS @550Hz (filter_type=0) — horn is DARK and dominated by P1-P2, so a LP
//     (not a band-pass) keeps the fundamental dominant and rolls off the bright upper partials.
//   - filtenv +2.6 oct / 0.55s decay — a GENTLE slow bloom (gentler than trumpet): the cutoff
//     opens from dark→mellow over the long onset, so the centroid EVOLVES without getting bright.
//   - gen1 = a soft mid lift (low-passed saw) for the P2-P3 "horn" body the main LP rolls off.
//   - amp_attack=0.30 — slow mellow horn onset; the saw routing makes this measure.
//   - light 4.8Hz vibrato + lfo_delay for breath; unison 3 for ensemble warmth.
//
// frenchHornSeed — ADDITIVE, matched to the real horn envelope (henkonen fhorn-18, CC0, C5 ≈525Hz).
// A saw+LP gave "harp+organ"/wrong spectrum: the real horn has a BRASS FORMANT — a plateau/bump at H4
// (~2100Hz, H4≈H3) that a monotonic LP cannot make (at the cutoff that gets H2 right, H4 is always too
// weak). So PLACE the partials directly at the measured ratios (H2 −18, H3 −29, H4 −31 plateau, then
// gentle rolloff). Unison drift + a continuous breath-noise tap give movement (anti-organ); a gentle
// filter-env bloom gives the brass attack. (gen-slot path renders upper partials ~10 dB under their gain,
// so H4+ gains are boosted above the bare measured ratios — verified by the A/B harmonic table.)
var frenchHornSeed = RecipeParams{
	"osc_enabled":      0,
	"filter_type":      0,
	"filter_cutoff":    2600, // gentle top roll-off only; the SPECTRUM is set by the gen gains below
	"filter_resonance": 1.0,
	// unison + drift = warmth + movement (kills the static "organ" purity)
	"unison_voices":      3,
	"unison_detune":      6.0,
	"unison_mix":         0.35,
	"unison_drift_rate":  2.5,
	"unison_drift_depth": 8.0,
	"amp_attack":         0.02, // fast horn onset — the ref speaks in ~12ms; the old 80ms + drift beating pushed the measured attack to ~0.7s
	"amp_decay":          0.06,
	"amp_sustain":        0.9,
	"amp_release":        0.15,
	// gentle vibrato — horn is steady (real ~3 Hz / 5 cents); short delay so it doesn't delay the onset peak
	"lfo_enabled": 1,
	"lfo_rate":    4.0,
	"lfo_depth":   0.02,
	"lfo_target":  1,
	"lfo_delay":   0.05,
	// measured harmonic ladder (rel f0): H2 −18, H3 −29, H4 −31 (formant plateau), H5 −41, H6 −49.
	// Upper gens are boosted above the bare ratio to offset the gen-slot ~10 dB attenuation.
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.32, // H1 (ref: H2 is LOUDER)
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.65, // H2 DOMINANT (ref 2.08)
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.34, // H3 strong (ref 1.13)
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 4.0, "gen5_gain": 0.12, // H4 (ref 0.32)
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 5.0, "gen6_gain": 0.05, // H5
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.016, // H6
	"gen8_source": 1, "gen8_wave": 0, "gen8_freq_mode": 0, "gen8_freq": 7.0, "gen8_gain": 0.008, // H7
	// gentle attack-brightness bloom (the brassy onset)
	"filtenv_enabled": 1,
	"filtenv_amt":     0.6,
	"filtenv_decay":   0.12,
	// continuous breath/air noise — low, band-passed (real horn has some air)
	"gen1_source": 2, "gen1_gain": 0.03, "gen1_filt_type": 5, "gen1_filt_freq": 1200, "gen1_filt_q": 0.5,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 14, "gen1_env_tail_mix": 0.8, "gen1_env_tail_rate": 0.0,
	"gain": 0.85,
}

// frenchHornLoudSeed: forte horn — same SUBTRACTIVE saw→LP→filtenv architecture as
// frenchHornSeed, but BRIGHTER (higher LP cutoff, bigger filtenv bloom) and a slightly
// faster forte onset. Still mellow/dark vs trumpet, P1-dominated, with evolving brightness.
var frenchHornLoudSeed = RecipeParams{
	// LOUD/forte french horn — the SAME additive H2-dominant "covered" tone as the
	// base horn but brassier: boosted upper harmonics (more H3/H4/H5 = the forte
	// "blare"), brighter roll-off, higher gain. Was an old saw; re-synced.
	"osc_enabled":        0,
	"filter_type":        0,
	"filter_cutoff":      3200,
	"filter_resonance":   1.0,
	"unison_voices":      3,
	"unison_detune":      6.0,
	"unison_mix":         0.35,
	"unison_drift_rate":  2.5,
	"unison_drift_depth": 8.0,
	"amp_attack":         0.06,
	"amp_decay":          0.06,
	"amp_sustain":        0.92,
	"amp_release":        0.15,
	"lfo_enabled":        1,
	"lfo_rate":           5.2,
	"lfo_depth":          0.02,
	"lfo_target":         0,
	"lfo_delay":          0.3,
	"gen2_source":        1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.32, // H1
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.70, // H2 DOMINANT
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.48, // H3 boosted (brassy)
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 4.0, "gen5_gain": 0.24, // H4 boosted
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 5.0, "gen6_gain": 0.12, // H5 boosted
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.05, // H6
	"gen1_source": 2, "gen1_gain": 0.03, "gen1_filt_type": 5, "gen1_filt_freq": 1300, "gen1_filt_q": 0.5,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 14, "gen1_env_tail_mix": 0.8, "gen1_env_tail_rate": 0.0,
	"gain": 1.0,
}

// ---------------------------------------------------------------------------
// Task 6 (this task): Dedicated synth bass
// ---------------------------------------------------------------------------

// bassGuitarSeed: plucked/punchy electric bass — strong fundamental, controlled harmonics,
// clear attack + decay, not a sustained organ drone. (Renamed from synthBassSeed.)
// Saw OSC at -1 octave for that warm bass register; filter envelope gives the "pluck wow";
// sub-sine at moderate gain for body weight without mud; no drive (removes grit/saturation).
var bassGuitarSeed = RecipeParams{
	// PLUCKED electric bass (finger), matched to the reference (ixwolf C2~=65Hz). The
	// ref's harmonic profile (FFT, reliable at 65Hz; synth-analyze mis-detects low
	// bass): H1:H2:H3:H4:H5 ~= 1:0.34:0.18:0.08:0.10 — fundamental-dominant, warm, a
	// few harmonics, NOT bright. A sine fundamental + that harmonic set + a short
	// finger-PLUCK noise click + a PLUCKED DECAY envelope (amp_sustain 0: each note
	// plucks and rings DOWN over a few seconds, articulating — not a flat synth drone,
	// not the harsh sustained sizzle of the earlier attempt).
	"osc_enabled":   1,
	"osc_type":      0,  // sine fundamental (warm, dominant)
	"osc_octave":    -1, // bass register (octave below the played note)
	"filter_type":   0,
	"filter_cutoff": 2200,
	"amp_attack":    0.002, // sharp finger pluck
	"amp_decay":     4,     // honest ParamDef max (was 8, silently clamped to 4 in the recipe path); long natural ring-down
	"amp_sustain":   0.0,   // PLUCKS AND DECAYS
	"amp_release":   0.15,
	// harmonic set matched to the reference (upper partials decay a little faster = the
	// tone warms as it rings down, like a real string)
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 2, "gen1_gain": 0.50, "gen1_env_fast_rate": 0.8,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 3, "gen2_gain": 0.30, "gen2_env_fast_rate": 1.1,
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 4, "gen4_gain": 0.14, "gen4_env_fast_rate": 1.4,
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 5, "gen5_gain": 0.14, "gen5_env_fast_rate": 1.7,
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 6, "gen6_gain": 0.08, "gen6_env_fast_rate": 2.0,
	// short finger-PLUCK click — band-passed noise at the attack only (the ref's pluck noise)
	"gen3_source": 2, "gen3_gain": 0.08, "gen3_filt_type": 5, "gen3_filt_freq": 1100, "gen3_filt_q": 0.5, "gen3_env_fast_rate": 45,
	"gain": 1.0,
}

// bassAcidSeed: TB-303-style acid bass — saw OSC at -1 octave through a low,
// HIGH-resonance low-pass filter that a fast-decaying filter ENVELOPE sweeps up
// then squelches back down. Light drive adds the 303 grit; plucked amp envelope
// (low sustain) keeps each note articulated.
var bassAcidSeed = RecipeParams{
	"osc_enabled":      1,
	"osc_type":         1,  // saw — the acid buzz
	"osc_octave":       -1, // bass register
	"filter_type":      0,  // low-pass
	"filter_cutoff":    350,
	"filter_resonance": 6.0, // squelchy resonant peak
	// the 303 squelch: a filter envelope that opens the cutoff up then decays.
	"filtenv_enabled": 1,
	"filtenv_amt":     3.0, // wide sweep (octaves)
	"filtenv_decay":   0.25,
	"filtenv_attack":  0.0,
	// plucked amp envelope.
	"amp_attack":  0.003,
	"amp_decay":   0.3,
	"amp_sustain": 0.4,
	"amp_release": 0.1,
	// light grit.
	"drive_enabled": 1,
	"drive":         0.4,
	"gain":          1.0,
}

// bassReeseSeed: Reese bass — the classic detuned-growl bass (DnB/dubstep). A
// heavy UNISON stack of saw voices with wide detune + slow drift gives the
// metallic moving beating; a mild low-pass keeps it from getting harsh; the amp
// is sustained so the growl rings.
var bassReeseSeed = RecipeParams{
	"osc_enabled":      1,
	"osc_type":         1,  // saw
	"osc_octave":       -1, // bass register
	"filter_type":      0,  // low-pass
	"filter_cutoff":    1200,
	"filter_resonance": 1.0, // mild
	// heavy unison: the detuned growl.
	"unison_voices":      4,
	"unison_detune":      22,
	"unison_mix":         0.7,
	"unison_drift_rate":  0.3, // slow movement
	"unison_drift_depth": 12,
	// sustained amp — the growl holds.
	"amp_attack":  0.005,
	"amp_decay":   0.4,
	"amp_sustain": 0.8,
	"amp_release": 0.2,
	"gain":        0.9,
}

// bassFMSeed: punchy FM/DX bass — a sine-ish carrier modulated by a decaying FM
// operator for the metallic DX attack that quickly settles into a clean low
// tone. Fast amp attack + short decay + low sustain make it punchy.
var bassFMSeed = RecipeParams{
	"osc_enabled":  1,
	"osc_type":     4, // FM
	"osc_octave":   -1,
	"fm_enabled":   1,
	"fm_algorithm": 0, // 2-op
	"fm_op1_ratio": 1, // carrier
	"fm_op2_ratio": 2, // modulator — metallic overtones
	"fm_op2_depth": 3.0,
	"fm_op1_level": 1,
	"fm_op2_level": 0,
	// a decaying FM index/attack via the modulator's own fast envelope is carried
	// by the amp envelope shaping below — the modulator decays out leaving the
	// clean carrier.
	"filter_type":   0,
	"filter_cutoff": 2500,
	// metallic-attack FM index decay rides the amp envelope: fast attack, short
	// decay, low sustain so the bright FM transient settles into the low body.
	"amp_attack":  0.002,
	"amp_decay":   0.18,
	"amp_sustain": 0.25,
	"amp_release": 0.12,
	"gain":        1.0,
}

// bass808Seed: 808 sub-bass — a sine OSC with a fast downward PITCH envelope (the
// 808 "pew" drop) and a long amp decay with zero sustain (the boom that rings
// out). Light drive thickens the low end without buzz.
var bass808Seed = RecipeParams{
	"osc_enabled":   1,
	"osc_type":      0,  // sine — pure sub
	"osc_octave":    -1, // deep bass register
	"filter_type":   0,
	"filter_cutoff": 1800,
	// the 808 "pew": fast downward pitch drop on attack.
	"pitchenv_enabled": 1,
	"pitchenv_amt":     12, // drop ~12 semitones
	"pitchenv_decay":   0.04,
	// the boom: long decay, no sustain — each hit blooms then rings down.
	"amp_attack":  0.001,
	"amp_decay":   1.5,
	"amp_sustain": 0.0,
	"amp_release": 0.2,
	// light low-end thickening.
	"drive_enabled": 1,
	"drive":         0.2,
	"gain":          1.0,
}

// dnbKickSeed: the configurable KICK stage tuned to a deep, dark DnB sub-kick
// (reference 36010__sandyrb__dnb-kick-003.wav). The kick body is the modular
// gen-bank kick voice (gen-slot 1, source==5, "deep" variant): a harmonic stack
// with a fast downward pitch drop, a short tail, and almost no beater click —
// the reference is bass-dominated (energy concentrated below 160 Hz, nothing
// above 400 Hz). The legacy OSC/amp/filter stages are bypassed so only the kick
// voice's own internal envelope shapes the hit. Tuned via A/B (synth-analyze)
// against the reference; pinned by TestDnbKickSeedIsKickShaped. NOTE: never seed
// gain:0.5 (TestPhase2Native mutates gain→0.5 and asserts a delta).
var dnbKickSeed = RecipeParams{
	// Only the kick voice sounds (osc/filter/drive bypassed). The amp ADSR IS
	// enabled to sculpt the organic amplitude BLOOM: the reference swells to a
	// plateau peak at ~49 ms then rolls off (a real drumhead body resonating),
	// which the kick voice's decay-from-t0 envelope can't do. amp_attack gives
	// the swell, amp_decay the roll-off.
	"osc_enabled":    0,
	"env_enabled":    1,
	"amp_attack":     0.018, // swell → peak ~18 ms (the reference blooms in, no click)
	"amp_decay":      0.10,  // roll-off to silence by ~120 ms — the organic body decay
	"amp_sustain":    0.0,   // sustain 0 keeps the envelope absolute-time (buffer-independent)
	"amp_release":    0.05,
	"amp_curve":      0, // LINEAR decay holds higher early (plateau-like), not a fast exp drop
	"filter_enabled": 0,
	"drive_enabled":  0,
	"gain":           1.0,

	// KICK stage enabled (the Synth-tab enable pill; gates the source==5 voice).
	"kick_enabled": 1,

	// Kick voice in gen-slot 1: BASE variant (4 harmonics incl. h4 — the deep
	// variant's 3-harmonic stack couldn't reach the reference's strong H3/H4
	// body). Fundamental ≈ 48 Hz (the reference's measured autocorrelation f0).
	"gen1_source":       5, // kick harmonic-bank voice
	"gen1_freq_mode":    0, // ratio × voice_freq
	"gen1_freq":         1.0,
	"gen1_gain":         1.0,
	"voice_freq_hz":     48, // settles ~48–53 Hz
	"gen1_kick_variant": 0,  // base

	// ORGANIC pitch glide (the signature of the dflee4 "stuborn" reference): a
	// long smooth drop ~135 → 48 Hz over ~50 ms, like a real drumhead tension
	// relaxing. pe_amt 1.8 (start = 48·2.8) needs the bumped kick_pe_amt bound;
	// the older small drop read as electronic.
	"gen1_kick_pe_amt":  2.2,
	"gen1_kick_pe_rate": 55,

	// NEAR-PURE FUNDAMENTAL: the reference is almost a clean deep sub (2nd
	// harmonic −25 dB, 97% of energy in 40–80 Hz). Strong harmonics read as
	// electronic — keep them tiny.
	"gen1_kick_h2": 0.06,
	"gen1_kick_h3": 0.05,
	"gen1_kick_h4": 0.02,

	// Kick internal envelope kept NEAR-FLAT so the amp ADSR (above) owns the
	// bloom/roll-off shape entirely.
	"gen1_kick_env0": 0.5,
	"gen1_kick_env1": 1,

	// NO beater click (the click + sharp attack are the "electronic" tell — the
	// reference has none), only a whisper of noise for body texture.
	"gen1_kick_click": 0.0,
	"gen1_kick_noise": 0.03,

	// Phase-9 shaping: gentle long tail, almost no attack transient (the organic
	// kick blooms, it doesn't click), light saturation for warmth. MUST be set
	// (an unset gen_kick_sat=0 silences the voice).
	"gen1_kick_fade":   1.0,
	"gen1_kick_attack": 0.0,
	"gen1_kick_sat":    0.6,
}

// kickStageBase returns the shared wiring every configurable-KICK-stage seed
// needs: the source==5 kick voice in gen-slot 1, the legacy stages bypassed, and
// the KICK enable on. Variant character is overlaid by the caller.
func kickStageBase() RecipeParams {
	return RecipeParams{
		"osc_enabled": 0, "env_enabled": 0, "filter_enabled": 0, "drive_enabled": 0, "gain": 1.0,
		"kick_enabled": 1,
		"gen1_source":  5, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 1.0,
	}
}

func kickSeedWith(overrides RecipeParams) RecipeParams {
	rp := kickStageBase()
	for k, v := range overrides {
		rp[k] = v
	}
	return rp
}

// electroKickSeed: a HARD ELECTRONIC / EDM kick (the deliberately-synthetic
// counterpart to the organic dnb-kick). Punchy variant, a strong beater click +
// sharp attack transient, real harmonics for edge, a fast tight pitch snap, and
// drive — everything the organic kick avoids.
var electroKickSeed = kickSeedWith(RecipeParams{
	"voice_freq_hz":     58,
	"gen1_kick_variant": 2,                             // punchy
	"gen1_kick_pe_amt":  0.25, "gen1_kick_pe_rate": 60, // tight fast "thwack"
	"gen1_kick_h2": 0.45, "gen1_kick_h3": 0.18, // harmonic edge
	"gen1_kick_env0": 9, "gen1_kick_env1": 16, // fast, tight
	"gen1_kick_click": 0.22, "gen1_kick_noise": 0.06, // a click, not a clipping spike
	// attack/sat kept MODERATE: 1.6/1.0 pushed the onset past ±1 into the C hard
	// clamp → distorted/crackly. These keep the raw render under full-scale.
	"gen1_kick_fade": 14, "gen1_kick_attack": 0.5, "gen1_kick_sat": 0.45,
})

// punchyKickSeed: a PUNCHY, RAW, BRUTAL kick (tuned toward yellowtree
// "hybrid-kick-1") on the LAYERED kick DSP (variant 5). That voice resolves the
// punch-vs-raw tension a single distorted voice cannot: a pitch-swept body +
// envelope-tracked grit fused through distortion (raw, decays with the body = no
// hiss) + a sharp POST-distortion click (high crest = the punch). The kick knobs
// are re-purposed by variant 5: sat = DISTORTION DRIVE, noise = GRIT amount,
// click = the TRANSIENT SPIKE.
var punchyKickSeed = kickSeedWith(RecipeParams{
	"voice_freq_hz":     45, // settles ~45 Hz (more sub weight); the sweep loads 60-90 Hz
	"gen1_kick_variant": 5,  // the layered hybrid DSP
	// PUNCH BAND: the reference puts 42% of its energy in 60-120 Hz (the thump you
	// FEEL) — a pure <60 Hz sub reads "weak/boomy". A moderate-rate sweep from
	// ~91 Hz that LINGERS in 60-90 during the loud swell (peak ~30 ms) lands the
	// body in the punch band, then settles to 48. NOT "high pitched" (that bug was
	// a 105 Hz body); 60-90 Hz is a thump.
	"gen1_kick_pe_amt": 0.9, "gen1_kick_pe_rate": 35,
	"gen1_kick_h2": 0.22, "gen1_kick_h3": 0.02, // modest h2 punch, tiny h3 (ref has ~1% in 120-250)
	"gen1_kick_env0": 12.0, "gen1_kick_env1": 12, // tight decay + attack-swell = a punchy thump that finishes DRY
	// ORGANIC grit comes from a LOW-MID noise texture (120-500 Hz, steep) in the
	// ATTACK only — that's body thickness + attack thwack (punch), NOT high air.
	// "Airy/spitty" is HF noise (>1 kHz); this band has none, so it's organic +
	// punchy without spit. Fully removing noise made it electronic and unpunchy.
	"gen1_kick_sat":    1.4,  // light tonal distortion for warmth/dirt
	"gen1_kick_noise":  0.16, // low-mid attack grit = organic thickness + punch (no high air)
	"gen1_kick_click":  0.30, // low thud attack = punch without a high tss
	"gen1_kick_attack": 1.5, "gen1_kick_fade": 4,
})

// kick808Seed: a long boomy 808 sub kick — a near-pure deep sub with the 808
// "pew" pitch drop and a LONG ringing tail (the sustained sub-bass note).
// kick808Seed: a deep progressive-house / 808 sub kick, tuned (harness +
// fingerprint metrics) toward 860393__cat-fox_alex__progressivehouse-kick-2:
// sub-dominant (<60 Hz carries most energy) BUT with real body in 60-120 Hz
// (~28% in the ref — a pure sub reads weak), a BIG fast pitch drop (~150→~40 Hz,
// the house "pew"), and a long sustained tail. The old seed barely dropped
// (pe_amt 0.4) and was ~89% sub with no body; these values restore the drop +
// body while keeping the boom.
var kick808Seed = kickSeedWith(RecipeParams{
	"voice_freq_hz":     44,
	"gen1_kick_variant": 1,                            // deep
	"gen1_kick_pe_amt":  2.4, "gen1_kick_pe_rate": 25, // BIG 808/house pitch drop ~150→44 Hz
	"gen1_kick_h2": 0.20, "gen1_kick_h3": 0.05, // sub + a body in 60-120 (not a pure sub)
	"gen1_kick_env0": 2.4, "gen1_kick_env1": 2.5, // sustained boom
	"gen1_kick_click": 0.30, "gen1_kick_noise": 0.04, // a bit of transport punch
	"gen1_kick_fade": 1.3, "gen1_kick_attack": 0.3, "gen1_kick_sat": 1.2, // long tail, warm
})

// acousticKickSeed: a natural tight acoustic kick — a beater click on a woody
// body with controlled harmonics and a short, gated tail (the tight variant's
// band-pass beater + room give the real-drum character).
// acousticKickSeed: a tight, punchy, ORGANIC acoustic-style kick, tuned (harness
// + fingerprint metrics) toward the real recorded sample 36010__sandyrb__dnb-
// kick-003: fundamental ~85-90 Hz with 60-120 Hz DOMINANT (~85% of energy — NOT
// a sub-boom), almost no <60 Hz, a SHARP beater attack (high crest ~7-8, the
// natural-kick transient) and a fast body decay. The old seed sat at ~55 Hz and
// ~50% sub with a soft beater — boomy and un-acoustic; these values move it to a
// real tight-kick character (the tight variant's band-pass beater does the rest).
var acousticKickSeed = kickSeedWith(RecipeParams{
	// Re-tuned toward the reference as a REAL KICK (the earlier bright modal tuning
	// read as a high tom — that sound is now the "High Tom Organic" instrument):
	// LOWER (~60 Hz) and fundamental-DOMINANT at the loud onset (low mid-mode
	// gains → a kick, not a tom), more BRUTAL (harder saturation + click), and a
	// room REVERB tail (the recorded kick's space). Variant 7's inharmonic modal
	// bank keeps the organic timbre; the low fundamental + reverb make it a kick.
	"voice_freq_hz":     54,                            // LOWER / HEAVIER — a deep real kick, not a high tom
	"gen1_kick_variant": 7,                             // acoustic (inharmonic modal bank + beater excitation)
	"gen1_kick_pe_amt":  0.25, "gen1_kick_pe_rate": 55, // subtler downward drop (less "closing-door" sweep)
	// mid modes kept LOW + high modes minimal, and NEAR-HARMONIC (small stretch) so
	// they don't buzz — the deep fundamental dominates. Heavy and organic, not creaky.
	"gen1_kick_h2": 0.35, "gen1_kick_h3": 0.08, "gen1_kick_h4": 0.02,
	"gen1_kick_env0": 8, "gen1_kick_env1": 22, // a bit more fundamental sustain = weight
	"gen1_kick_mode_detune": 0.03, "gen1_kick_mode_gain": 0.45, "gen1_kick_mode_decay": 45,
	// a hard sharp click carries the punch/crest (the aggressive soul); the body
	// saturation is kept LOW — saturating the summed inharmonic modes makes
	// intermodulation roughness that reads as a "creak"/wooden buzz.
	"gen1_kick_click": 0.90, "gen1_kick_noise": 0.03, "gen1_kick_reverb": 0.85,
	"gen1_kick_fade": 2.5, "gen1_kick_attack": 1.0, "gen1_kick_sat": 0.25,
})

// zgumpKickSeed: an ORGANIC, punchy mid-bass kick tuned toward the reference
// 83768__zgump__kick-pack-0708.wav. It rides the MODAL kick DSP (variant 6):
// a struck BATTER mode + a detuned, less-damped, delayed RESONANT mode whose
// beating (Δf = f0·detune) and slow ring produce the reference's organic
// plateau-then-tail-bloom envelope from a real mechanism, not a drawn envelope.
// The reference is a mid-bass thump: dominant ~80 Hz, ~73% of its energy in
// 60-120 Hz + ~24% in 120-250 Hz (strong 2nd harmonic), almost no true sub or
// HF, a big smooth 240→~78 Hz glide, and crest ~3.4 (a thump, not a click).
// Every value is explicit so bakedModularRender bakes it (the seeded-kick
// playback gotcha): a bare gen1_kick_* default would render the wrong voice.
var zgumpKickSeed = kickSeedWith(RecipeParams{
	// Tuned by the native harness (zgump_kick_tune_test.go) against the reference
	// via the fingerprint kick metrics: bands 71/26 (ref 71/25), settle ~66 Hz
	// (ref 67.5), attack peak ~25 ms (ref 25), crest ~3.2 (ref 3.4), tail bloom
	// present, glide ~120 ms (ref 125). Band accuracy was prioritized over the
	// pitch-glide trace where they conflict (bands win perceptually).
	"voice_freq_hz":     78, // settles ~67 Hz (the reference fundamental)
	"gen1_kick_variant": 6,  // modal (coupled two-mode drumhead)
	// a SLOW, gradual glide (~210→67 Hz over ~120 ms) — the drumhead-tension drop,
	// the reference's #1 organic tell (its pitch declines gradually, not a snap).
	"gen1_kick_pe_amt": 2.0, "gen1_kick_pe_rate": 45,
	// modest 2nd harmonic loads the 120-250 band (~25% of ref energy); tiny 3rd.
	"gen1_kick_h2": 0.18, "gen1_kick_h3": 0.04,
	// batter decay (~25 ms peak by construction) + faster harmonic decay.
	"gen1_kick_env0": 11.0, "gen1_kick_env1": 20.0,
	// the modal knobs: Δf≈9 Hz beat (plateau ripple) + a strong, long, delayed
	// resonant ring (the tail bloom) — the organic plateau-then-bloom.
	"gen1_kick_mode_detune": 0.12, "gen1_kick_mode_gain": 1.3, "gen1_kick_mode_decay": 3.0,
	// a soft swell-in onset (low click/grit) — the reference peaks at 25 ms, it is
	// NOT a sharp beater-click kick; a low thud gives punch without a high tick.
	"gen1_kick_click": 0.20, "gen1_kick_noise": 0.04,
	"gen1_kick_sat": 0.8, "gen1_kick_attack": 0.5, "gen1_kick_fade": 4.0,
})

// cloneSeed returns a deep copy of base with overrides applied on top. Used to
// derive a new instrument config from an existing one (the config-first "copy an
// instrument" primitive, at definition time).
func cloneSeed(base RecipeParams, overrides RecipeParams) RecipeParams {
	out := make(RecipeParams, len(base)+len(overrides))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

// highTomOrganicSeed: the variant-7 modal voice tuned (originally as the
// acoustic kick) to a bright, organic, inharmonic-modal timbre — which the user
// judged reads as a HIGH TOM rather than a kick (its ×2.1/×3.4/×5.2 mid modes
// are prominent). Saved as its own instrument because it's a good organic tom;
// kick-acoustic is then re-tuned lower/brutaler/reverberant for a real kick.
// Explicit snapshot (NOT cloneSeed(acousticKickSeed)) so re-tuning the kick does
// not move the tom.
var highTomOrganicSeed = kickSeedWith(RecipeParams{
	"voice_freq_hz":     85,
	"gen1_kick_variant": 7, // acoustic (inharmonic modal bank)
	"gen1_kick_pe_amt":  0.25, "gen1_kick_pe_rate": 60,
	"gen1_kick_h2": 0.50, "gen1_kick_h3": 0.35, "gen1_kick_h4": 0.20,
	"gen1_kick_env0": 16, "gen1_kick_env1": 28,
	"gen1_kick_mode_detune": 0.10, "gen1_kick_mode_gain": 1.0, "gen1_kick_mode_decay": 55,
	"gen1_kick_click": 0.40, "gen1_kick_noise": 0.04,
	"gen1_kick_fade": 5.0, "gen1_kick_attack": 0.6, "gen1_kick_sat": 0.3,
})

// rawKickSeed: a RAW / HEAVY / BRUTAL sibling of zgump-kick — literally CLONED
// from zgumpKickSeed's config (cloneSeed) and pushed toward the reference's
// grittier natural character (which the clean zgump tuning smoothed out). Same
// MODAL variant-6 DSP; the overrides add broadband grit texture (kick_noise↑),
// saturation drive (kick_sat↑), a heavier sub (lower voice_freq + stronger,
// longer resonant mode) and a harder attack (kick_click/kick_attack↑).
// zgump-kick stays untouched; this is its rawer clone. Tuned in the harness.
var rawKickSeed = cloneSeed(zgumpKickSeed, RecipeParams{
	"voice_freq_hz":        70,   // lower/heavier than zgump's 78
	"gen1_kick_h2":         0.30, // more harmonic edge
	"gen1_kick_h3":         0.12,
	"gen1_kick_mode_gain":  1.7,  // heavier body
	"gen1_kick_mode_decay": 2.2,  // longer resonant ring = heavier tail
	"gen1_kick_click":      0.50, // harder attack transient (brutal)
	"gen1_kick_noise":      0.30, // broadband grit (raw / organic)
	"gen1_kick_sat":        2.5,  // strong distortion drive (raw)
	"gen1_kick_attack":     1.5,
})

// ---------------------------------------------------------------------------
// Task 7: Modal conga
// ---------------------------------------------------------------------------

// congaSeed: MODAL synthesis conga — tuned membrane modes + slap transient.
// Not just a brightened tom: three sine oscillators simulate the air-loaded
// membrane modes (ratios ~1 : 1.5 : 2 as in real hand-drums after air-loading
// stretches inharmonic Bessel modes toward quasi-harmonic), each with its own
// fast per-slot decay via gen_env_fast_rate. A short noise burst (gen4) at
// ~4kHz provides the finger-slap crack on attack. A downward pitch drop on
// attack (pitchenv_enabled) gives the characteristic "thump-to-ring" swoosh.
// OSC disabled — all tone comes from the gen bank.
// Base frequency: voice freq ≈ 220 Hz (A3) when node pitch = 0; quinto sits
// a few semitones higher (node pitch +3 to +7), tumba lower (pitch -5 to -3).
var congaSeed = RecipeParams{
	// QUINTO MUTED SLAP — a sharp, BRIGHT, NOISY percussive crack (the slap) over a
	// brief muted membrane thump (no ring; the hand mutes the head). Tuned to a real
	// quinto muted slap (mrrentapercussionist): very short (~50ms), almost all noise
	// (~1.0), bright (centroid ~3874), membrane modes ~380-516Hz. The SLAP NOISE
	// DOMINATES; the tonal membrane modes are a short under-thump.
	"osc_enabled":   0,
	"amp_attack":    0.0008,
	"amp_decay":     0.05, // MUTED — dies in ~50ms (was 0.15 open-tone)
	"amp_sustain":   0.0,
	"amp_release":   0.03,
	"filter_type":   0,
	"filter_cutoff": 4500, // OPEN — let the bright slap crack through
	// attack pitch snap
	"pitchenv_enabled": 1,
	"pitchenv_amt":     2.0,
	"pitchenv_decay":   0.02,
	// membrane under-thump — brief, the quinto's inharmonic modes, SECONDARY to the slap
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 0.75, "gen1_env_fast_rate": 18,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.5, "gen2_gain": 0.45, "gen2_env_fast_rate": 22,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.3, "gen3_gain": 0.30, "gen3_env_fast_rate": 26,
	// THE SLAP — broadband BRIGHT noise crack, DOMINANT, very fast decay
	"gen4_source": 2, "gen4_gain": 0.9, "gen4_filt_type": 5, "gen4_filt_freq": 2700, "gen4_filt_q": 0.7,
	"gen4_env_fast_rate": 70,
	"gain":               0.95,
}

// congaOpenSeed: OPEN TONE — the conga "tone" stroke (hand bounces off, head RINGS):
// tonal/resonant, the membrane modes dominate (the drum's pitch), a light finger
// crack, a medium ring (~180ms). The melodic conga sound under a tumbao.
var congaOpenSeed = RecipeParams{
	"osc_enabled":      0,
	"amp_attack":       0.001,
	"amp_decay":        0.18, // open tone RINGS (~180ms)
	"amp_sustain":      0.0,
	"amp_release":      0.05,
	"filter_type":      0,
	"filter_cutoff":    5000,
	"pitchenv_enabled": 1,
	"pitchenv_amt":     1.5,
	"pitchenv_decay":   0.02,
	// membrane modes — TONAL, prominent (the open tone's pitch)
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 1.0, "gen1_env_fast_rate": 6,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.5, "gen2_gain": 0.5, "gen2_env_fast_rate": 8,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.3, "gen3_gain": 0.3, "gen3_env_fast_rate": 10,
	// light finger crack — short, secondary
	"gen4_source": 2, "gen4_gain": 0.3, "gen4_filt_type": 5, "gen4_filt_freq": 2500, "gen4_filt_q": 0.8, "gen4_env_fast_rate": 120,
	"gain": 0.95,
}

// congaTumbaSeed: TUMBA — the largest/lowest conga, a deep full BASS tone (tuned an
// octave below the quinto). Darker/fuller, strong fundamental, medium ring.
var congaTumbaSeed = RecipeParams{
	"osc_enabled":      0,
	"osc_octave":       -1, // the bass drum of the conga set — an octave below the quinto
	"amp_attack":       0.001,
	"amp_decay":        0.20,
	"amp_sustain":      0.0,
	"amp_release":      0.05,
	"filter_type":      0,
	"filter_cutoff":    2600, // darker/fuller (bass)
	"pitchenv_enabled": 1,
	"pitchenv_amt":     1.2,
	"pitchenv_decay":   0.025,
	"gen1_source":      1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 1.0, "gen1_gain": 1.0, "gen1_env_fast_rate": 5,
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.5, "gen2_gain": 0.4, "gen2_env_fast_rate": 7,
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.2, "gen3_env_fast_rate": 9,
	"gen4_source": 2, "gen4_gain": 0.25, "gen4_filt_type": 5, "gen4_filt_freq": 1500, "gen4_filt_q": 0.8, "gen4_env_fast_rate": 130,
	"gain": 0.95,
}

// ---------------------------------------------------------------------------
// Masterpiece template set: two new instruments (organ, sax).
// All reuse renderModular — no new C/DSP. Starting seeds; ear-tune as needed.
// ---------------------------------------------------------------------------

// organSeed: additive Hammond-style drawbars — sine partials at organ footage
// ratios (16',8',5⅓',4',2⅔',2',1⅓') with a SUSTAINED envelope and a shallow
// Leslie chorus (LFO→pitch). OSC disabled; gen slots set the drawbar spectrum.
// A static additive sine spectrum is exactly the "organ" timbre the brass seed
// warns against — here that is the intent. Used by Bach (full pipe voicing via
// per-template SynthParams), house pads, techno held chords, reggae bubble.
var organSeed = RecipeParams{
	"osc_enabled":   0,
	"filter_type":   0,
	"filter_cutoff": 9000, // was 6000 — open up so the high mixtures sparkle (air band)
	"amp_attack":    0.012,
	"amp_decay":     0.05,
	"amp_sustain":   1.0,
	"amp_release":   0.10,
	// Shallow Leslie chorus.
	"lfo_enabled": 1,
	"lfo_rate":    6.2,
	"lfo_depth":   0.25,
	"lfo_target":  1, // pitch
	"lfo_delay":   0.0,
	// vs real cathedral organ (BWV532): scoop toward sub + air, away from low/mid.
	"gen8_source": 1, "gen8_wave": 0, "gen8_freq_mode": 0, "gen8_freq": 0.25, "gen8_gain": 0.26, // 32' sub (deep pipe body)
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 0.5, "gen1_gain": 0.40, // 16' (was 0.30)
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.38, // 8' fundamental (was 0.55 — cut low/mid)
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 1.5, "gen3_gain": 0.18, // 5⅓' (was 0.22)
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 2.0, "gen4_gain": 0.28, // 4' (was 0.45 — cut mid)
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 3.0, "gen5_gain": 0.18, // 2⅔'
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 4.0, "gen6_gain": 0.22, // 2'
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.22, // 1⅓' (was 0.10 — more air)
	"gain": 0.85,
}

// saxSeed: SUBTRACTIVE reed (saw → band-pass formant → opening filtenv), the SAME
// architecture as oboeSeed but with TENOR-SAX character: lower/broader formant
// (BP @1000Hz vs oboe's 1450), airier breath chiff (gen2 with a small sustained
// tail), and a slower-blooming filtenv. Used as the So What 2nd horn and the
// Girl from Ipanema melody (Stan Getz tenor).
var saxSeed = RecipeParams{
	// SAXOPHONE = hybrid additive + formant-filtered saw. Tuned against the
	// TRUE-grid measurement of 360251__mtg__sax-baritone-c3 (f0 ≈ 122.8 Hz/B2 —
	// the earlier "H3-dominant, breath 0.45" profile was an OCTAVE-UP analysis
	// artifact: DetectF0 locked onto H2 because the bari fundamental sits ~7 dB
	// below it; fixed via the odd-harmonic-evidence octave guard). Real bari
	// signature (rel H2=1.0): H1 .47, H2 1.00, H3 .97, H4 .68, H5 .39, H6 .67
	// (the ~740 Hz knee formant), H7 .37, then a REBOUND plateau H14-H16 ≈ .3
	// (the 1.9/2.6 kHz formants); even/odd 1.25 (conical bore = even-strong);
	// noise only ~0.08; vibrato ~2¢ (nearly straight); rise10-90 ≈ 0.22 s; and
	// brightness tracks loudness (+0.77) via the note's slow decrescendo.
	// Architecture: sines pin H1-H4+H6 exactly; a saw→BP@820 builds the knee
	// fill (H5/H7-H10); a saw→BP@2050 builds the high formant plateau; a narrow
	// breath-noise band sits at the ref's residual centroid (~1.2-1.5 kHz) so
	// louder breath = brighter (positive coupling), and a slow amp decrescendo
	// supplies the macro brightness-loudness coupling.
	"osc_enabled":      0,
	"filter_type":      0,
	"filter_cutoff":    2200, // shoulder just above the 2 kHz formant plateau (ref cliffs at ~2.6 kHz)
	"filter_resonance": 1.0,
	"amp_attack":       0.5, // measured rise10-90 0.21 s ≈ ref 0.22 s (env curve is nonlinear)
	"amp_decay":        2.5, // slow decrescendo — drives the ref's +0.77 brightness↔loudness coupling
	"amp_sustain":      0.45,
	"amp_release":      0.2,
	// subtle vibrato (the ref reads 2.4¢, below the 3¢ detection threshold)
	"lfo_enabled": 1,
	"lfo_rate":    5.0,
	"lfo_depth":   0.002, // renders ~2.1¢ ≈ ref 2.4¢ (0.008 rendered 8.5¢)
	"lfo_target":  1,
	"lfo_delay":   0.2,
	// sines pin the measured low harmonics (rel H2 ×0.5 gain scale)
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.22, // H1 (weak fundamental)
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 2.0, "gen3_gain": 0.50, // H2 DOMINANT
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 3.0, "gen4_gain": 0.45, // H3 near-equal
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 4.0, "gen5_gain": 0.37, // H4
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 6.0, "gen6_gain": 0.53, // H6 knee-formant bump
	// saw → BP@820 Q1.8: the ~740 Hz knee fill (H5, H7-H10)
	"gen7_source": 1, "gen7_wave": 1, "gen7_freq_mode": 0, "gen7_freq": 1.0, "gen7_gain": 1.9,
	"gen7_filt_type": 5, "gen7_filt_freq": 820, "gen7_filt_q": 1.8,
	// saw → BP@2050 Q2.0: the H14-H18 plateau (1.9 + 2.6 kHz formants).
	// gain 4.0 = the gen_gain ParamDef Max — the recipe path CLAMPS on merge,
	// so a larger value silently renders as 4.0 anyway, and the baked one-shot
	// path (which does NOT clamp) would diverge from playback.
	"gen8_source": 1, "gen8_wave": 1, "gen8_freq_mode": 0, "gen8_freq": 1.0, "gen8_gain": 4.0,
	"gen8_filt_type": 5, "gen8_filt_freq": 2050, "gen8_filt_q": 2.0,
	// breath noise at the ref's residual centroid (~1.5 kHz), narrow + quiet
	// (true breathiness is ~0.08, not the octave-artifact 0.45)
	"gen1_source": 2, "gen1_gain": 0.18, "gen1_filt_type": 5, "gen1_filt_freq": 1200, "gen1_filt_q": 2.5,
	"gen1_env_fast_mix": 0.5, "gen1_env_fast_rate": 8, "gen1_env_tail_mix": 0.85, "gen1_env_tail_rate": 0.0,
	"gain": 0.7,
}

// ── Phase-15 voice/choir family. Source–filter vocal synthesis: glottal pulse
// (osc_type 12) or detuned saws → formant vowel bank (Csound Appendix D
// tables) + humanized ensemble. Values are research-derived starting points
// (spec 2026-07-08); final values are set by the EAR-TUNING pass — the user's
// ear is the acceptance gate, not these numbers.

// ensembleLeadSeed: glottal-ensemble lead — born as a choir "ahh" attempt,
// kept as a synth per ear review 2026-07-08; cello-like sustained ensemble, 5
// humanized glottal voices, tenor "a" formants, moderate singer's formant,
// soft 100 ms attack.
var ensembleLeadSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1,
	"unison_voices": 5, "unison_detune": 8, "unison_mix": 0.7,
	"unison_drift_rate": 0.3, "unison_drift_depth": 4,
	"ens_scatter": 10, "ens_vib_rate": 5.5, "ens_vib_depth": 8, "ens_humanize": 0.8,
	"filter_type": 0, "filter_cutoff": 6000, "filter_resonance": 0.707,
	"amp_attack": 0.10, "amp_decay": 0.3, "amp_sustain": 0.85, "amp_release": 0.5,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 2,
	"formant_mix": 0.85, "formant_breath": 0.06, "formant_sing": 0.35,
	"gain": 0.55, // not 0.5 — TestPhase2Native mutates gain->0.5 and needs a delta
}

// ensembleLeadDarkSeed: darker sustained ensemble-lead pad — born as a choir
// "ooh" attempt, kept as a synth per ear review 2026-07-08; alto "u" vowel,
// more breath, slower attack, long release.
var ensembleLeadDarkSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1,
	"unison_voices": 5, "unison_detune": 7, "unison_mix": 0.75,
	"unison_drift_rate": 0.25, "unison_drift_depth": 4,
	"ens_scatter": 9, "ens_vib_rate": 5.0, "ens_vib_depth": 7, "ens_humanize": 0.85,
	"filter_type": 0, "filter_cutoff": 4200, "filter_resonance": 0.707,
	"amp_attack": 0.15, "amp_decay": 0.4, "amp_sustain": 0.9, "amp_release": 1.0,
	"formant_enabled": 1, "formant_vowel": 4, "formant_voice_type": 1,
	"formant_mix": 0.85, "formant_breath": 0.12, "formant_sing": 0.2,
	"gain": 0.55,
}

// voiceSopranoSeed: solo soprano — single glottal voice, expressive ±60¢
// vibrato after a 0.3 s onset, onset scoop via a small negative pitch env,
// bright 3.1 kHz singer's formant.
var voiceSopranoSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1, "osc_octave": 1,
	"unison_voices": 1,
	"lfo_enabled":   1, "lfo_target": 1, "lfo_rate": 5.5, "lfo_depth": 0.6, "lfo_delay": 0.3,
	"pitchenv_enabled": 1, "pitchenv_amt": -0.5, "pitchenv_decay": 0.09,
	"filter_type": 0, "filter_cutoff": 8000, "filter_resonance": 0.707,
	"amp_attack": 0.06, "amp_decay": 0.2, "amp_sustain": 0.9, "amp_release": 0.4,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 0,
	"formant_mix": 0.9, "formant_breath": 0.05, "formant_sing": 0.5,
	"gain": 0.6,
}

// ghostBassSeed: solo glottal bass synth — born as a "voice-bass" solo singer
// attempt, kept as a synth (Bass) per ear review 2026-07-08; glottal an octave
// down, bass "o" vowel, restrained ±30¢ vibrato, low 2.4 kHz singer's formant.
var ghostBassSeed = RecipeParams{
	"osc_type": 12, "osc_enabled": 1, "osc_octave": -1,
	"unison_voices": 1,
	"lfo_enabled":   1, "lfo_target": 1, "lfo_rate": 5.0, "lfo_depth": 0.3, "lfo_delay": 0.35,
	"pitchenv_enabled": 1, "pitchenv_amt": -0.4, "pitchenv_decay": 0.1,
	"filter_type": 0, "filter_cutoff": 5000, "filter_resonance": 0.707,
	"amp_attack": 0.09, "amp_decay": 0.25, "amp_sustain": 0.9, "amp_release": 0.45,
	"formant_enabled": 1, "formant_vowel": 3, "formant_voice_type": 3,
	"formant_mix": 0.9, "formant_breath": 0.05, "formant_sing": 0.3,
	"gain": 0.6,
}

// violaPadSeed: lush vowel-morphing string pad — born as a "voice-pad" vocal
// attempt, kept as a synth (Strings) per ear review 2026-07-08; 7 detuned SAWS
// (deliberately synthy), slow ah↔oh morph, wide humanized ensemble.
var violaPadSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1,
	"unison_voices": 7, "unison_detune": 12, "unison_mix": 0.85,
	"unison_drift_rate": 0.4, "unison_drift_depth": 6,
	"ens_scatter": 8, "ens_vib_rate": 5.0, "ens_vib_depth": 6, "ens_humanize": 0.6,
	"filter_type": 0, "filter_cutoff": 4500, "filter_resonance": 0.707,
	"amp_attack": 0.25, "amp_decay": 0.5, "amp_sustain": 0.9, "amp_release": 1.2,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 1,
	"formant_mix": 0.9, "formant_morph_rate": 0.15, "formant_morph_to": 3,
	"formant_breath": 0.04, "formant_sing": 0.15,
	"gain": 0.55,
}

// voiceWhisperSeed: unvoiced texture — NO pitch source (osc off), pure breath
// noise through "ee" formants. Unpitched: NOT in melodicRecipeIDs.
var voiceWhisperSeed = RecipeParams{
	"osc_enabled":     0,
	"formant_enabled": 1, "formant_vowel": 2, "formant_voice_type": 1,
	"formant_mix": 1.0, "formant_breath": 1.0,
	"filter_type": 0, "filter_cutoff": 9000, "filter_resonance": 0.707,
	"amp_attack": 0.08, "amp_decay": 0.3, "amp_sustain": 0.9, "amp_release": 0.4,
	"gain": 0.6,
}

// voiceAhhSeed: sung male "ahh" — tuned against a real 7.5 s C3 (131 Hz) "ahh"
// recording (tmp/voice_tuning/ref_ahhhh.wav). The ref has virtually NO
// fundamental energy (fixed-grid tristimulus [0.00,0.33,0.67]; the loudest
// harmonics are H4-H8 ≈ 525-1050 Hz, the F1 region), a large non-harmonic
// component (NoiseRatio 0.63, residual centered ~1178 Hz), ~7.7 Hz vibrato
// over a big slow pitch wander, and a slow ~1.3 s swell.
// Design (found by iteration): the glottal osc's 1/n² slope is too steep for
// the formant bank to recover H5+ above the hard-coded 15% dry leak, so the
// source is a SAW (1/n) whose fundamental is stripped by a 400 Hz HIGH-PASS
// before the tenor "ah" formant bank at full mix; formant_shift 1.25 puts
// F1 ≈ 810 Hz on the ref's broad ~864 Hz hump and formant_sing supplies the
// ref's -11 dB ~2959 Hz hill. Breathiness = formant_breath (white,
// vowel-colored) + two band-passed gen-bank noise taps at 1100/600 Hz
// bracketing the ref's residual centroid (the cello bow-noise pattern).
// Pitch aliveness = 7.5 Hz ±80¢ vibrato after 0.4 s, a -2 st onset scoop,
// 3 drifting unison voices with full ens humanization, and bow_dynamics for
// slow breath-pressure loudness wander. Fixed-grid match (4 s render at
// -9 st = 130.8 Hz): tri [0.07,0.27,0.66] vs ref [0.00,0.33,0.67]. Known
// gap: the engine has no cycle-level glottal jitter knob (ref Jitter 0.53),
// so the render is cleaner/steadier than a real throat.
var voiceAhhSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1,
	"unison_voices": 3, "unison_detune": 6, "unison_mix": 0.6,
	"unison_drift_rate": 0.3, "unison_drift_depth": 14,
	"ens_scatter": 6, "ens_vib_rate": 6.5, "ens_vib_depth": 30, "ens_humanize": 1.0,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 7.5, "lfo_depth": 0.8, "lfo_delay": 0.4,
	"pitchenv_enabled": 1, "pitchenv_amt": -2.0, "pitchenv_decay": 0.7,
	"bow_dynamics": 0.35,
	"filter_type":  1, "filter_cutoff": 400, "filter_resonance": 0.707,
	"amp_attack": 0.55, "amp_decay": 0.3, "amp_sustain": 0.9, "amp_release": 0.6,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 2,
	"formant_mix": 1.0, "formant_shift": 1.25, "formant_breath": 0.07, "formant_sing": 0.28,
	"gen1_source": 2, "gen1_gain": 0.25, "gen1_filt_type": 5, "gen1_filt_freq": 1100, "gen1_filt_q": 1.7,
	"gen1_env_fast_mix": 0.2, "gen1_env_fast_rate": 8, "gen1_env_tail_mix": 0.8, "gen1_env_tail_rate": 0.0,
	"gen2_source": 2, "gen2_gain": 0.18, "gen2_filt_type": 5, "gen2_filt_freq": 600, "gen2_filt_q": 1.2,
	"gen2_env_fast_mix": 0.1, "gen2_env_fast_rate": 8, "gen2_env_tail_mix": 0.9, "gen2_env_tail_rate": 0.0,
	"gain": 0.6,
}

// voiceOperaSeed: breathier "old-man opera" variant of voiceAhhSeed, ear-
// approved per the 2026-07-08 round-2 review (auditioned as
// tmp/voice_audition_v2/voice-ahh_alt_breathier.wav — a temp-edit alternate
// rendered during voice-ahh tuning, ruled genuinely human-sounding and
// promoted to its own instrument rather than folded into voice-ahh). Exact
// voiceAhhSeed values with three deltas that add more air/huskiness and
// brighter noise: formant_breath 0.07→0.18, gen1_gain 0.25→0.38, gen2_gain
// 0.18→0.30. Every other key (osc, unison, ensemble, lfo, pitch env, bow
// dynamics, filter, amp envelope, formant shift/sing, gen filter
// type/freq/Q, gain) is unchanged from voice-ahh.
var voiceOperaSeed = RecipeParams{
	"osc_type": 1, "osc_enabled": 1,
	"unison_voices": 3, "unison_detune": 6, "unison_mix": 0.6,
	"unison_drift_rate": 0.3, "unison_drift_depth": 14,
	"ens_scatter": 6, "ens_vib_rate": 6.5, "ens_vib_depth": 30, "ens_humanize": 1.0,
	"lfo_enabled": 1, "lfo_target": 1, "lfo_rate": 7.5, "lfo_depth": 0.8, "lfo_delay": 0.4,
	"pitchenv_enabled": 1, "pitchenv_amt": -2.0, "pitchenv_decay": 0.7,
	"bow_dynamics": 0.35,
	"filter_type":  1, "filter_cutoff": 400, "filter_resonance": 0.707,
	"amp_attack": 0.55, "amp_decay": 0.3, "amp_sustain": 0.9, "amp_release": 0.6,
	"formant_enabled": 1, "formant_vowel": 0, "formant_voice_type": 2,
	"formant_mix": 1.0, "formant_shift": 1.25, "formant_breath": 0.18, "formant_sing": 0.28,
	"gen1_source": 2, "gen1_gain": 0.38, "gen1_filt_type": 5, "gen1_filt_freq": 1100, "gen1_filt_q": 1.7,
	"gen1_env_fast_mix": 0.2, "gen1_env_fast_rate": 8, "gen1_env_tail_mix": 0.8, "gen1_env_tail_rate": 0.0,
	"gen2_source": 2, "gen2_gain": 0.30, "gen2_filt_type": 5, "gen2_filt_freq": 600, "gen2_filt_q": 1.2,
	"gen2_env_fast_mix": 0.1, "gen2_env_fast_rate": 8, "gen2_env_tail_mix": 0.9, "gen2_env_tail_rate": 0.0,
	"gain": 0.6,
}
