#ifndef MODULAR_H
#define MODULAR_H

/* ================================================================
 * Unified modular synth voice.
 *
 * Unlike the bespoke drum/FM renderers (which bake oscillator shape,
 * harmonic structure, envelope and filter as constants), the modular
 * voice assembles a fully user-editable signal path from the shared
 * DSP primitives:
 *
 *     OSC (selectable shape, incl. FM)  ->  ADSR (amp)  ->  filter  ->  drive/post
 *
 * Every stage is driven by modular_params, so the UI can expose the
 * whole pipeline. The pre-render-and-cache model is unchanged: the
 * whole note is rendered once on a param change and cached, so engine
 * complexity costs only at edit time, never at trigger time.
 *
 * modular_params is a SECOND flat-float param block, distinct from the
 * 7-field synth_params shared by the legacy renderers. Discrete choices
 * (osc type, filter type, FM algorithm, amp curve) are carried as
 * integer-valued floats and read via (int)lrintf(...). Field order is
 * the canonical ABI order mirrored by modularParamSchema in
 * src/go/internal/audio/synth_param_schema.go and the generated
 * src/js/synth_param_abi.gen.js. Pass NULL to render_modular_p to use
 * the built-in defaults (same result as render_modular).
 * ================================================================ */

typedef struct {
    /* Oscillator stage. */
    float osc_type;        /* 0=sine 1=saw 2=square 3=triangle 4=FM 5=noise-white 6=noise-pink */
    float osc_detune;      /* cents, -100..100 */
    float osc_octave;      /* integer octave shift, -2..2 */

    /* FM stage (used when osc_type == 4). */
    float fm_algorithm;    /* 0..3 operator routing */
    float fm_op1_ratio, fm_op2_ratio, fm_op3_ratio, fm_op4_ratio; /* 0.5..8 */
    float fm_op1_depth, fm_op2_depth, fm_op3_depth, fm_op4_depth;  /* 0..8 mod index */
    float fm_op1_level, fm_op2_level, fm_op3_level, fm_op4_level;   /* 0..1 */

    /* Amplitude ADSR. */
    float amp_attack;      /* seconds */
    float amp_decay;       /* seconds */
    float amp_sustain;     /* 0..1 */
    float amp_release;     /* seconds */
    float amp_curve;       /* 0=linear 1=exponential */

    /* Filter stage. */
    float filter_type;     /* 0=LP 1=HP 2=BP */
    float filter_cutoff;   /* Hz, 20..20000 */
    float filter_resonance;/* Q, 0.5..16 */

    /* Post stage. */
    float drive;           /* 0..1 tanh saturation */
    float pitch;           /* semitone offset, -24..24 */
    float gain;            /* output trim, 0..1.5 */

    /* ── Per-stage bypass toggles. APPENDED to the ABI (never insert above —
     *    every consumer fills heap[MODULAR_PARAM_INDEX[name]] by position).
     *    1.0 = enabled (the default). Read in modular.c via mp_get with a
     *    fallback of 1.0 so the NULL / zero-filled-struct path stays fully
     *    enabled, i.e. byte-identical to the pre-toggle voice. ── */
    float osc_enabled;     /* 0 = silence the generator (it is the source) */
    float fm_enabled;      /* osc_type==FM only; 0 = carrier-only (no modulation) */
    float env_enabled;     /* 0 = constant unity gain (skip the amp ADSR) */
    float filter_enabled;  /* 0 = filter bypassed (passthrough) */
    float drive_enabled;   /* 0 = drive bypassed (clean) */
    float noise_seed;      /* deterministic seed for osc_type 5/6; default 0 */

    /* ── Phase-1 gen bank (modular unification, spec 2026-06-05). APPEND-ONLY:
     *    by-field [12] arrays so later phases can append new per-slot fields
     *    without reordering existing flat indices. All identity values are
     *    no-ops (see ModularParamSchemaIdentity); a preset that never touches
     *    the bank renders byte-identically to the pre-bank engine. ── */
    float noise_draws;             /* bus draws per sample (default 1) */
    float noise_prelude;           /* bus draws consumed before sample 0 */
    float gen_source[12];          /* 0=off 1=osc 2=noise-tap */
    float gen_wave[12];            /* 0=sine 1=saw 2=square 3=triangle */
    float gen_freq_mode[12];       /* 0=ratio×voice-freq 1=absolute Hz */
    float gen_freq[12];
    float gen_gain[12];
    float gen_env_fast_rate[12];   /* s^-1; 0 => constant */
    float gen_env_tail_rate[12];
    float gen_env_fast_mix[12];
    float gen_env_tail_mix[12];
    float gen_filt_type[12];       /* 0=none 1=1pLP 2=1pHP 3=rbjLP 4=rbjHP 5=rbjBP */
    float gen_filt_alpha[12];      /* 1-pole input coeff: lp = lp*(1-a) + x*a */
    float gen_filt_freq[12];       /* RBJ Hz */
    float gen_filt_q[12];
    float gen_phase_mode[12];      /* 0=zero 1=fixed-radians 2=prelude-index */
    float gen_phase[12];
    float gen_noise_offset[12];    /* which per-sample bus draw this tap reads */

    /* ── Phase-2 (bass family): per-slot analytic-voice fields (source==4).
     *    APPEND-ONLY — new by-field [12] columns after every existing slot
     *    field. Identity values are no-ops when the slot's source is not 4, so
     *    every pre-Phase-2 preset renders byte-identically. Each field is read
     *    via kp_get(field, legacy_literal) inside the analytic voice so the
     *    NaN sentinel reproduces the legacy double literal at default. ── */
    float gen_pitch_env_amt[12];   /* sub-bass pitch-punch fraction (NaN→0.15 per binding) */
    float gen_pitch_env_rate[12];  /* pitch-env decay /s (NaN→40 for sub-bass) */
    float gen_harm_mix[12];        /* 2nd-harmonic companion mix (NaN→0.08) */
    float gen_atk_amt[12];         /* attack-boost amount (NaN→0.2) */
    float gen_atk_rate[12];        /* attack-boost decay /s (NaN→60) */
    float gen_sat_k[12];           /* tanh input scale; 0 = no sat (NaN→1.2) */
    float gen_out_scale[12];       /* trailing output scale (NaN→0.95 analytic; the KS voice overrides the fallback to 0.85 — see modular_gen_slot_ks) */

    /* ── Phase-2 globals: voice-freq override + shared POST stage. APPENDED
     *    after the per-slot arrays (flat schema order). All identities are
     *    no-ops: voice_freq_hz=0 keeps the pow derivation, post_enabled=0
     *    skips the POST stage entirely. When post runs, a synth_params base is
     *    assembled from post_* and gated by the post_*_on flags. ── */
    float voice_freq_hz;   /* >0 overrides the derived modular voice freq (Hz) */
    float post_enabled;    /* >=0.5 runs apply_post_params at the very end */
    float post_pitch;      /* → synth_params.pitch  (semitones) */
    float post_decay;      /* → synth_params.decay  (multiplier; 1 = no-op) */
    float post_decay_rate; /* post_config.decay_rate time constant */
    float post_body;       /* → synth_params.body */
    float post_brightness; /* → synth_params.brightness */
    float post_tone;       /* → synth_params.tone */
    float post_drive;      /* → synth_params.drive */
    float post_pitch_on;   /* post_config.pitch gate (1=on) */
    float post_decay_on;   /* post_config.decay gate */
    float post_body_on;    /* post_config.body gate */
    float post_brightness_on; /* post_config.brightness gate */
    float post_tone_on;    /* post_config.tone gate */
    float post_drive_on;   /* post_config.drive gate */

    /* ── Phase-2 (bass family, Task 2): Karplus-Strong (source==3) per-slot
     *    fields. APPEND-ONLY at the very tail (after the Phase-2 globals) — the
     *    KS slot voice was added after the analytic source + globals, so these
     *    two columns land here to keep every pre-existing flat index frozen.
     *    Identity 0 each: only read when a slot's source is 3. The KS voice
     *    reads them via kp_get(field, legacy_literal) so the NaN sentinel
     *    reproduces the exact legacy double literal at default (bSustain=0.996,
     *    bPluck=0.35). KS reuses gen_atk_amt/gen_atk_rate (legacy 0.25/400),
     *    gen_env_fast_rate (envGlobal, 1.8), gen_out_scale (0.85 pre-softsat),
     *    and gen_freq/gen_freq_mode (the KS fundamental). ── */
    float gen_ks_sustain[12]; /* KS decay factor (NaN→0.996) */
    float gen_ks_pluck[12];   /* KS pluck-LP alpha (NaN→0.35) */
    float gen_ks_blow[12];    /* KS continuous-noise excitation (NaN→0 = plucked; >0 = blown tube / wind) */

    /* ── Phase-3 (kick family): harmonic-bank kick voice (source==5) per-slot
     *    fields. APPEND-ONLY at the very tail (after the Phase-2 KS columns) —
     *    these landed after every prior column so the pre-existing flat indices
     *    stay frozen (append-only ABI). Identity 0 each: read only when a slot's
     *    source is 5.
     *
     *    gen_kick_variant is a DISCRIMINATOR (0=base 1=deep 2=punchy 3=lofi
     *    4=tight) the binding sets explicitly per recipe — the five legacy kick
     *    loops differ structurally (noise-draw topology, harmonic count, final
     *    shaping: dual-tanh+bit-reduce+LP on lofi, dual-BP+gate on tight), so
     *    one fused loop cannot hold all five byte-identically. The source==5
     *    voice therefore branches on the variant and runs the matching legacy
     *    loop transplanted VERBATIM, with every CURATED kick knob (the user-
     *    editable fields) read via kp_get so the NaN sentinel reproduces the
     *    exact legacy double literal at default. The structural per-variant
     *    constants (filter coeffs, fixed harmonic env rates, attack/fade rates,
     *    final scales) stay as literals inside each branch exactly like the
     *    legacy render_kick_*_internal functions (they are not knobs). ── */
    float gen_kick_variant[12]; /* 0=base 1=deep 2=punchy 3=lofi 4=tight */
    float gen_kick_h2[12];      /* kick_h2_gain (NaN→variant literal) */
    float gen_kick_h3[12];      /* kick_h3_gain (NaN→variant literal) */
    float gen_kick_h4[12];      /* kick_h4_gain (NaN→variant literal; base only) */
    float gen_kick_env0[12];    /* kick_env0_rate (NaN→variant literal) */
    float gen_kick_env1[12];    /* kick_env1_rate (NaN→variant literal) */
    float gen_kick_pe_amt[12];  /* kick_pitch_env_amount (NaN→variant literal) */
    float gen_kick_pe_rate[12]; /* kick_pitch_env_rate (NaN→variant literal) */
    float gen_kick_click[12];   /* kick_click (NaN→variant literal) */
    float gen_kick_noise[12];   /* kick_noise (NaN→variant literal) */

    /* ── Phase-3 (kick post-order fix): selects the POST-stage op ORDER when the
     *    shared POST stage runs. APPEND-ONLY at the very tail so every prior flat
     *    index stays frozen. Identity 0 = the shared apply_post_params order
     *    (pitch → decay → body → brightness → tone → drive), used by every
     *    migrated family whose legacy wrapper called apply_post_params. Order 1 =
     *    the legacy BASE kick (render_kick_p) bespoke order: pitch → decay →
     *    drive → body. The per-param oracle sweeps never set two post knobs at
     *    once, so they cannot tell the orders apart; the combo oracle case does,
     *    and pins order 1 for the base kick. ── */
    float post_order; /* 0=apply_post_params order; 1=base-kick (drive before body); 2=base-snare (drive before tone) */

    /* ── Phase-4 (tom family): 808-style tom voice (source==6) per-slot fields.
     *    APPEND-ONLY at the VERY tail (after post_order) — every prior flat index
     *    stays frozen (append-only ABI). Identity 0 each: read only when a slot's
     *    source is 6.
     *
     *    The three legacy tom renderers (render_tom_internal +
     *    render_tom_high_internal + render_tom_low_internal, drums.c) share ONE
     *    loop shape and differ ONLY in literal sets (sweep/ring defaults, the
     *    osc1/osc2 env rates, the fundamental-env mult, resonant-boost amount/rate,
     *    stick LP/HP alphas, attack-env rate, ambient-env rate, global-fade slope).
     *    The source==6 voice therefore runs ONE parameterized loop and selects the
     *    per-variant literal set by gen_tom_variant (0=tom 1=high 2=low) — a single
     *    fused loop, NOT three branches. The CURATED tom knobs (sweep/ring rates,
     *    o1/o2 gains, stick, room) are read via kp_get so the NaN sentinel
     *    reproduces the exact per-variant legacy double literal at default; the
     *    STRUCTURAL per-variant constants stay as variant-indexed literal tables
     *    exactly like the legacy functions (they are not user knobs). tom_wave
     *    reuses gen_wave (NaN→0). ── */
    float gen_tom_variant[12]; /* 0=tom 1=high 2=low */
    float gen_tom_sweep[12];   /* tom_sweep_rate (NaN→per-variant literal) */
    float gen_tom_ring[12];    /* tom_ring_rate  (NaN→per-variant literal) */
    float gen_tom_o1[12];      /* tom_o1_gain    (NaN→per-variant literal) */
    float gen_tom_o2[12];      /* tom_o2_gain    (NaN→per-variant literal) */
    float gen_tom_stick[12];   /* tom_stick      (NaN→per-variant literal) */
    float gen_tom_room[12];    /* tom_room       (NaN→per-variant literal) */

    /* ── Phase-5 (snare family): snare-ish voice (source==7, 3 variant branches:
     *    0=snare 1=rimshot 2=sidestick) + clap voice (source==8) per-slot fields.
     *    APPEND-ONLY at the VERY tail (after the Phase-4 tom columns) — every prior
     *    flat index stays frozen (append-only ABI). Identities are 0 each: read
     *    only when a slot's source is 7 or 8.
     *
     *    The three snare-ish renderers (render_snare_internal +
     *    render_snare_rimshot_internal + render_snare_sidestick_internal, drums.c)
     *    share the SAME noise-draw topology (1 seed draw, then 1 white draw per
     *    sample) but differ STRUCTURALLY (snare: 2 body oscs + 4 RBJ noise bands +
     *    wire LFO + multi-term mix + fade; rimshot: 4 inharmonic partials + HP
     *    transient + 1st-order HP on the sum + hard-tanh×4 drive; sidestick: 2
     *    fixed sines + 1 low-Q BP). Like the kick family they therefore branch on
     *    gen_snare_variant rather than fusing one loop. The CLAP renderer
     *    (render_clap_internal) is PURE noise with a different draw topology (NO
     *    seed draw — the first draw is per-sample) so it is a SEPARATE source (8).
     *
     *    The CURATED snare knobs (tone2/tune/tone-decay/noise-decay/tail-decay/
     *    tone-mix/noise-mix/wire-mix/attack) are read via kp_get so the NaN
     *    sentinel reproduces the exact per-variant legacy double literal at default;
     *    the STRUCTURAL per-variant constants (the fixed RBJ band centers/Qs, the
     *    fixed partial ratios/decays, the HP cutoffs, the fade slopes) stay as
     *    literals inside each branch exactly like the legacy functions (they are
     *    not user knobs). snare_wave reuses gen_wave (NaN→0). Each curated column
     *    carries DIFFERENT per-variant fallbacks, so the binding passes NaN and the
     *    branch supplies the legacy literal. ── */
    float gen_snare_variant[12]; /* 0=snare 1=rimshot 2=sidestick (source==7) */
    float gen_snare_tone2[12];   /* snare_tone2_freq (NaN→per-variant literal) */
    float gen_snare_tune[12];    /* snare_noise_tune (NaN→1.0) */
    float gen_snare_tone_d[12];  /* snare_tone_decay (NaN→per-variant literal) */
    float gen_snare_noise_d[12]; /* snare_noise_decay (NaN→per-variant literal) */
    float gen_snare_tail_d[12];  /* snare_tail_decay (NaN→per-variant literal; snare/clap only) */
    float gen_snare_tone_m[12];  /* snare_tone_mix (NaN→per-variant literal) */
    float gen_snare_noise_m[12]; /* snare_noise_mix (NaN→per-variant literal) */
    float gen_snare_wire_m[12];  /* snare_wire_mix (NaN→0.9; snare only) */
    float gen_snare_attack[12];  /* snare_attack (NaN→per-variant literal) */

    /* ── Phase-6 (cymbal family): metallic voice (source==9, 6 variant branches:
     *    0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash) per-slot fields.
     *    APPEND-ONLY at the VERY tail (after the Phase-5 snare columns) — every
     *    prior flat index stays frozen (append-only ABI). Identities are 0 each:
     *    read only when a slot's source is 9.
     *
     *    The six legacy metallic renderers (render_hihat_internal +
     *    render_open_hihat_internal + render_cowbell_internal +
     *    render_shaker_internal + render_ride_internal + render_crash_internal,
     *    drums.c) split into FOUR distinct loop shapes: the hi-hat PAIR
     *    (hihat/open-hihat) shares one 6-square-partial inharmonic-cluster loop
     *    differing only in literal sets; the ride/crash PAIR shares one
     *    N-sine-partial + bell + HP-noise + LFO loop (10 vs 8 partials, a
     *    table-driven count); cowbell (4 partials + RBJ-BP-1200 transient + HP
     *    metal rasp) and shaker (noise→1-pole-HP + RBJ-BP-8k + 4 staggered burst
     *    envelopes) each have their own structure. ALL SIX live in ONE source
     *    (the kick/snare precedent: one source, variant-branched) because they
     *    share the cymbal knob ABI + post wiring; the gen_cym_variant
     *    discriminator selects the matching legacy loop, transplanted VERBATIM.
     *
     *    The CURATED cymbal knobs (tune/env_fast/env_tail/tone_mix/noise_mix/
     *    noise_decay) are read via kp_get so the NaN sentinel reproduces the exact
     *    per-variant legacy double literal at default; the STRUCTURAL per-variant
     *    constants (partial-frequency/gain tables, fixed LFO rates, HP-LP alphas,
     *    bell freq/decay, burst offsets/amps, fade slopes) stay as literals inside
     *    each branch exactly like the legacy functions (they are not user knobs).
     *    cym_wave reuses gen_wave (NaN→per-variant literal: hats default 2, the
     *    pitched-partial variants default 0; shaker has no oscillator). The noise
     *    stream is noise_ma seeded 0 (→4321), bit-identical to the legacy ma_noise
     *    seed-0 stream (TestNoiseMaParity), drawn in the SAME order as the legacy
     *    loop (per-variant prelude draws then 1 per-sample draw). ── */
    float gen_cym_variant[12];   /* 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash */
    float gen_cym_tune[12];      /* cym_tune (NaN→1.0) */
    float gen_cym_env_fast[12];  /* cym_env_fast / transient rate (NaN→per-variant literal) */
    float gen_cym_env_tail[12];  /* cym_env_tail / ring rate (NaN→per-variant literal) */
    float gen_cym_tone_m[12];    /* cym_tone_mix (NaN→per-variant literal) */
    float gen_cym_noise_m[12];   /* cym_noise_mix (NaN→per-variant literal) */
    float gen_cym_noise_d[12];   /* cym_noise_decay (NaN→per-variant literal; not shaker) */

    /* ── Phase-7 (FM family, the LAST legacy family): 4-operator FM preset voice
     *    (source==10, 5 variant branches: 0=bass 1=bell 2=lead 3=epiano 4=pluck)
     *    per-slot fields. APPEND-ONLY at the VERY tail (after the Phase-6 cymbal
     *    columns) — every prior flat index stays frozen (append-only ABI).
     *    Identities are 0 each: read only when a slot's source is 10.
     *
     *    The five legacy FM renderers (render_fm_bass_p / bell_p / lead_p /
     *    epiano_p / pluck_p, fmsynth.c) all assemble a full fm_preset (num_ops,
     *    base_freq, per-op ratio/offset/amplitude/ADSR/carrier, mod_matrix
     *    routing, pitch env, wavetable) and call the SHARED fm_render — the SAME
     *    compiled core the modular osc_type==4 FM stage already uses (untouched).
     *    They differ ONLY in their preset's structural constants, so the
     *    source==10 voice runs ONE parameterized path and selects the per-variant
     *    structural preset by gen_fm_variant. The CURATED FM knobs (base_freq,
     *    pitch-env amount/decay, per-op ratio/depth/decay, wave) are read via
     *    kp_get so the NaN sentinel reproduces the exact per-variant legacy double
     *    literal at default (the same overlay the legacy fm_apply_params performed);
     *    the STRUCTURAL per-variant constants (op amplitudes, freq offsets, ADSR
     *    attack/sustain/release, carrier flags, num_ops, mod_matrix ROUTING) stay
     *    as literals inside the per-variant builder exactly like the legacy
     *    PRESET_FM_* tables (they are not user knobs).
     *
     *    fm_op_depth[i] sets op i's outgoing mod_matrix edge depth (mirroring the
     *    legacy fm_apply_params, which replaced op i's FIRST nonzero outgoing edge);
     *    the routing topology is fixed by the variant. gen_fm_wave reuses gen_wave
     *    (NaN→0, the legacy preset.wave default sine). The voice renders into a
     *    private scratch buffer via fm_render (which memset-zeros its own output)
     *    then assign/accumulates into out[] — so a legacy -0.0 sample is preserved
     *    when this is the first writer (osc off), and the POST stage that follows
     *    (post_enabled=1, PostOrder=0) calls the SAME apply_post_params the legacy
     *    _p wrappers used. ── */
    float gen_fm_variant[12];    /* 0=bass 1=bell 2=lead 3=epiano 4=pluck */
    float gen_fm_base[12];       /* preset.base_freq (NaN→per-variant literal) */
    float gen_fm_pe_amt[12];     /* preset.pitch_env_amount (NaN→per-variant literal) */
    float gen_fm_pe_decay[12];   /* preset.pitch_env_decay (NaN→per-variant literal) */
    float gen_fm_r1[12];         /* ops[0].freq_ratio (NaN→per-variant literal) */
    float gen_fm_r2[12];         /* ops[1].freq_ratio (NaN→per-variant literal) */
    float gen_fm_r3[12];         /* ops[2].freq_ratio (NaN→per-variant literal) */
    float gen_fm_r4[12];         /* ops[3].freq_ratio (NaN→per-variant literal) */
    float gen_fm_d1[12];         /* op0 outgoing edge depth (NaN→per-variant literal) */
    float gen_fm_d2[12];         /* op1 outgoing edge depth (NaN→per-variant literal) */
    float gen_fm_d3[12];         /* op2 outgoing edge depth (NaN→per-variant literal) */
    float gen_fm_d4[12];         /* op3 outgoing edge depth (NaN→per-variant literal) */
    float gen_fm_dec1[12];       /* ops[0].decay_sec (NaN→per-variant literal) */
    float gen_fm_dec2[12];       /* ops[1].decay_sec (NaN→per-variant literal) */
    float gen_fm_dec3[12];       /* ops[2].decay_sec (NaN→per-variant literal) */
    float gen_fm_dec4[12];       /* ops[3].decay_sec (NaN→per-variant literal) */

    /* ── Phase-8C modulator stages (gap closure vs spec §1: PITCH ENV / LFO /
     *    BURST as REAL togglable global stages). APPEND-ONLY at the VERY tail
     *    (after the Phase-7 FM columns) — every prior flat index stays frozen.
     *    All identities are 0 and every *_enabled defaults DISABLED (mp_get
     *    fallback 0.0), the OPPOSITE polarity of the pre-existing stage toggles:
     *    those stages pre-dated the toggles (default-on = byte-identity), these
     *    stages are NEW (default-off = byte-identity). enabled=0 skips the
     *    stage code entirely, so the numeric knobs are unread — an exact bypass
     *    by construction (TestPitchEnvDisabledIsExactBypass et al).
     *
     *    PITCH ENV: exponential semitone sweep on the OSC-stage oscillator
     *    (wavetable types 0-3 via per-sample wt_osc_set_freq; FM core via
     *    fm_preset.pitch_env_*). Context-dependent like fm_enabled: silent
     *    until the OSC stage is enabled with a pitched osc type. It does NOT
     *    reach inside gen-bank family voices (their transplanted loops own
     *    their pitch math; byte-identity forbids editing them).
     *    LFO: post-mix amp wobble out[i] *= 1 - depth*(0.5 - 0.5*sin(2πft)) —
     *    range [1-depth, 1], shapes EVERY voice incl. migrated families.
     *    BURST: post-mix multi-burst gate out[i] *= Σ_j amp_j·e^(-sharp·(t-off_j))
     *    for t ≥ off_j (clap-style staggered transients on any source). ── */
    float pitchenv_enabled; /* >=0.5 runs the OSC pitch envelope */
    float pitchenv_amt;     /* semitones of sweep (decays to 0) */
    float pitchenv_decay;   /* exp time constant, seconds (fm_preset convention) */
    float lfo_enabled;      /* >=0.5 runs the amp-wobble LFO */
    float lfo_rate;         /* Hz */
    float lfo_depth;        /* 0..1 wobble depth (1 = full dips to silence) */
    float burst_enabled;    /* >=0.5 runs the burst gate */
    float burst_sharp;      /* per-burst exp decay rate, s^-1 */
    float burst1_off;       /* burst onsets, seconds */
    float burst1_amp;       /* burst peak amplitudes */
    float burst2_off;
    float burst2_amp;
    float burst3_off;
    float burst3_amp;
    float burst4_off;
    float burst4_amp;
    /* LFO routing target: 0 = amplitude tremolo (default, identity), 1 = pitch
     * vibrato (wavetable OSC path), 2 = filter-cutoff sweep. Appended at the
     * struct tail so the WASM flat param-block indices stay frozen. */
    float lfo_target;

    /* Filter envelope: cutoff *= 2^(amt * exp(-t/decay)) — brightens on attack,
     * decays to base. Separate from the LFO cutoff sweep so it composes with
     * vibrato. Default 0 = off = byte-identical. */
    float filtenv_enabled;
    float filtenv_amt;
    float filtenv_decay;
    /* filtenv_attack: seconds for the cutoff to RISE from base to +filtenv_amt
     * octaves before the decay/hold phase. Default 0 = no rise = onset bloom
     * then decay (byte-identical). >0 = a slow brightening swell — the brass
     * "spectral envelope tracks loudness" crescendo (set filtenv_decay large to
     * HOLD bright after the rise). */
    float filtenv_attack;

    /* ── Phase-8E (unison/ensemble): each wavetable oscillator (OSC stage + gen-bank
     *    source==1 slots) renders N detuned copies with decorrelated phases.
     *    APPEND-ONLY at the VERY tail — every prior flat index stays frozen.
     *    unison_voices==1 (the default, identity 1) means single oscillator →
     *    byte-identical to before this change (the unison path is gated strictly
     *    on nv >= 2). Phase accumulators for side voices are stack-allocated per
     *    render call (the whole note is rendered in one shot), so no persistent
     *    state is needed in the params struct. ── */
    float unison_voices; /* 1..7, integer-valued, default 1 (identity = single osc) */
    float unison_detune; /* cents spread ±detune/2 across side voices, 0..50, default 0 */
    float unison_mix;    /* 0=only center, 1=full ensemble blend, default 0.5 */
    /* Phase-8F unison drift: slow per-voice decorrelated LFO on detune.
     * APPEND-ONLY at very tail. Identity: both 0 → byte-identical to before. */
    float unison_drift_rate;  /* Hz, 0..8, base rate (each voice uses rate*(1+0.13*v)) */
    float unison_drift_depth; /* cents, 0..30, per-voice drift amplitude */

    /* ── Phase-8G LFO onset delay: ramps LFO depth from 0 at t=0 to full at
     * t=lfo_delay (linear ramp min(t/lfo_delay, 1)). Identity 0 = ramp factor
     * always 1.0 = byte-identical to pre-Phase-8G behavior. Applied to ALL
     * three LFO targets (amp/pitch/cutoff). APPEND-ONLY at very tail. ── */
    float lfo_delay;  /* seconds, 0..3, default 0 (identity) */

    /* ── Body-resonator bank (organic string/violin body). A parallel bank of
     * narrow band-pass resonators ≈ an instrument body's signature modes, driven
     * by the tonal signal AFTER the filter stage. The fixed narrow peaks make
     * each harmonic's level swing as vibrato sweeps it across them → the timbre
     * "shimmer" a single biquad cannot make. APPEND-ONLY at very tail.
     * body_model 0 = OFF = byte-identical. */
    float body_model; /* 0=off, 1=violin (hardcoded mode table) */
    float body_mix;   /* 0..~1.5, wet (resonant peaks) added to the dry signal */

    /* ── Bow dynamics (bowing expression). A slow smoothed-random modulator (the
     * bow pressure/speed wandering) drives loudness AND brightness together (more
     * pressure → louder + brighter), the organic non-repeating performance motion
     * a steady synth tone lacks (the "sounds like an organ" fix). APPEND-ONLY at
     * very tail. 0 = off = byte-identical. */
    float bow_dynamics; /* 0..1, modulation depth */

    /* ── Phase-9 (kick family extras): the structural shaping constants of the
     * source==5 kick voice promoted to per-slot knobs, so the configurable KICK
     * stage can match real kicks the curated h2/h3/env0/env1/pe/click/noise set
     * cannot (a too-short tail, a harder transient, more saturation weight).
     * APPEND-ONLY at the VERY tail — every prior flat index stays frozen. Each
     * is NaN-driven (kp_get(field, per-variant literal)): an unset value
     * reproduces the exact legacy per-variant constant, so every existing kick
     * variant renders byte-identically. Read only when a slot's source is 5. ── */
    float gen_kick_attack[12]; /* attack-boost amount (NaN→variant literal: base .3 deep .15 punchy .5 lofi .2 tight .3) */
    float gen_kick_fade[12];   /* global-fade rate /tNorm (NaN→variant literal: base 4 deep 3 punchy 5 lofi 3 tight 4.5) — higher = shorter tail */
    float gen_kick_sat[12];    /* saturation pre-gain (NaN→variant literal: base .55 deep 1.2 punchy .8 lofi 1.5 tight .45) */

    /* ── Phase-10 (KICK stage enable): the Synth-tab KICK stage's enable pill.
     * Gates the source==5 kick voice: <0.5 silences it. APPEND-ONLY at the VERY
     * tail. Identity 0 (off, the new-stage byte-identity polarity) — but every
     * source==5 consumer (the legacy kick binding/push + the dnb-kick seed) sets
     * it to 1, and a render with no kick slot (gen_source!=5) never reads it, so
     * pre-Phase-10 renders stay byte-identical. ── */
    float kick_enabled; /* >=0.5 runs the source==5 kick voice; <0.5 silences it */
} modular_params;

/* Render the modular voice with built-in defaults (sine, percussive env). */
void render_modular(float *out, int sampleRate, int samples);

/* Render the modular voice with explicit params. NULL => built-in defaults. */
void render_modular_p(float *out, int sampleRate, int samples,
                      const modular_params *params);

#endif /* MODULAR_H */
