#include <math.h>
#include <string.h>
#include "modular.h"
#include "modular_stages.h"
#include "wavetable.h"
#include "adsr.h"
#include "fmsynth.h"
#include "noise.h"
#include "synth_post.h" /* apply_post_params + post_config + synth_params (POST stage) */

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* Nominal base pitch of the modular voice (A3). pitch/octave/detune shift it. */
#define MODULAR_BASE_FREQ 220.0
/* Band-limit harmonic count for the analytic waveform tables. */
#define MODULAR_WT_HARMONICS 48

/* ── Static band-limited oscillator tables (sine/saw/square/triangle) ─────── */

static float       modular_sine_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_saw_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_square_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_triangle_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t modular_sine_wt, modular_saw_wt, modular_square_wt, modular_triangle_wt;
static int         modular_tables_ready = 0;

static void ensure_modular_tables(void) {
    if (modular_tables_ready) return;
    wt_generate_sine(&modular_sine_wt, modular_sine_buf, WT_DEFAULT_LENGTH);
    wt_generate_saw(&modular_saw_wt, modular_saw_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_square(&modular_square_wt, modular_square_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_triangle(&modular_triangle_wt, modular_triangle_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    modular_tables_ready = 1;
}

static const wavetable_t *modular_table_for(int osc_type) {
    switch (osc_type) {
    case 1: return &modular_saw_wt;
    case 2: return &modular_square_wt;
    case 3: return &modular_triangle_wt;
    default: return &modular_sine_wt;
    }
}

/* Non-static accessor for the gen-bank slot oscillators (modular_stages.c):
 * lazily builds the shared tables then returns the SAME wavetable object the
 * legacy osc stage uses, so a slot sine is byte-identical to the legacy osc. */
const wavetable_t *modular_shared_table_for(int wave) {
    ensure_modular_tables();
    return modular_table_for(wave);
}

/* The RBJ biquad (mod_biquad + mod_biquad_set + mod_biquad_tick_f) lives in
 * modular_stages.c/.h now — shared with the gen-bank slot filters. */

/* ── Param accessors with NULL => default ─────────────────────────────────── */

static float mp_get(const modular_params *p, float fallback,
                    float value, int have) {
    return have ? value : fallback;
}

/* Build an fm_preset from live params for osc_type == FM. Operators run with a
 * steady (sustain=1) internal envelope so the voice-level amp ADSR shapes the
 * whole sound uniformly. */
static void build_fm_preset(const modular_params *p, double base_freq,
                            fm_preset *out) {
    memset(out, 0, sizeof(*out));
    out->num_ops = FM_MAX_OPS;
    out->base_freq = (float)base_freq;
    out->pitch_env_amount = 0.0f;
    out->pitch_env_decay = 0.0f;

    float ratio[FM_MAX_OPS] = {p->fm_op1_ratio, p->fm_op2_ratio, p->fm_op3_ratio, p->fm_op4_ratio};
    const float level[FM_MAX_OPS] = {p->fm_op1_level, p->fm_op2_level, p->fm_op3_level, p->fm_op4_level};
    const float depth[FM_MAX_OPS] = {p->fm_op1_depth, p->fm_op2_depth, p->fm_op3_depth, p->fm_op4_depth};

    for (int i = 0; i < FM_MAX_OPS; i++) {
        out->ops[i].freq_ratio = (ratio[i] > 0.0f) ? ratio[i] : 1.0f;
        out->ops[i].freq_offset = 0.0f;
        out->ops[i].amplitude = level[i];
        out->ops[i].attack_sec = 0.0f;
        out->ops[i].decay_sec = 0.0f;
        out->ops[i].sustain_level = 1.0f;
        out->ops[i].release_sec = 0.0f;
        out->ops[i].is_carrier = 0;
    }
    /* op1 is always a carrier. Its level (fm_op1_level) controls output gain;
     * we do NOT force it audible, so the level knob has real authority. */
    out->ops[0].is_carrier = 1;

    /* op1 self-feedback makes fm_op1_depth meaningful (DX7-style operator
     * feedback adds harmonics) instead of being an inert final-carrier knob. */
    out->mod_matrix[0][0] = depth[0];

    int alg = (int)lrintf(p->fm_algorithm);
    switch (alg) {
    case 1: /* parallel: each op with level>0 is an independent carrier */
        for (int i = 1; i < FM_MAX_OPS; i++)
            out->ops[i].is_carrier = (level[i] > 0.0f) ? 1 : 0;
        break;
    case 2: /* 3-op chain: op3 -> op2 -> op1 */
        out->mod_matrix[1][0] = depth[1];
        out->mod_matrix[2][1] = depth[2];
        break;
    case 3: /* 4-op stack: op4 -> op3 -> op2 -> op1 */
        out->mod_matrix[1][0] = depth[1];
        out->mod_matrix[2][1] = depth[2];
        out->mod_matrix[3][2] = depth[3];
        break;
    default: /* alg 0: 2-op stack, op2 -> op1 */
        out->mod_matrix[1][0] = depth[1];
        break;
    }
}

EXPORT void render_modular_p(float *out, int sampleRate, int samples,
                             const modular_params *params) {
    if (!out || samples <= 0 || sampleRate <= 0) return;
    memset(out, 0, (size_t)samples * sizeof(float));

    int have = params != 0;
    const modular_params zero = {0};
    const modular_params *p = have ? params : &zero;

    int   osc_type   = (int)lrintf(p->osc_type);
    float octave     = p->osc_octave;
    float detune     = p->osc_detune;          /* cents */
    float pitch_st   = p->pitch;               /* semitones */
    /* Envelope: defaults when params is NULL or fields are zero-ish. */
    float a   = mp_get(p, 0.005f, p->amp_attack,  have);
    float d   = mp_get(p, 0.300f, p->amp_decay,   have);
    float sus = mp_get(p, 0.600f, p->amp_sustain, have);
    float rel = mp_get(p, 0.200f, p->amp_release, have);
    int   exp_curve = have ? ((int)lrintf(p->amp_curve)) : 1;
    int   filt_type = (int)lrintf(p->filter_type);
    float cutoff = mp_get(p, 8000.0f, p->filter_cutoff, have);
    float q      = mp_get(p, 0.707f, p->filter_resonance, have);
    float drive  = p->drive;
    float gain   = mp_get(p, 1.0f, p->gain, have);

    /* Per-stage bypass toggles. Fallback 1.0 keeps the NULL/zero-struct path
     * (have==false) fully enabled — bit-identical to the pre-toggle voice. */
    int osc_on   = mp_get(p, 1.0f, p->osc_enabled,    have) >= 0.5f;
    int fm_on    = mp_get(p, 1.0f, p->fm_enabled,     have) >= 0.5f;
    int env_on   = mp_get(p, 1.0f, p->env_enabled,    have) >= 0.5f;
    int filt_on  = mp_get(p, 1.0f, p->filter_enabled, have) >= 0.5f;
    int drive_on = mp_get(p, 1.0f, p->drive_enabled,  have) >= 0.5f;
    unsigned int noise_seed = (unsigned int)lrintf(mp_get(p, 0.0f, p->noise_seed, have));

    /* Phase-8C modulator stages. Fallback 0.0 = DISABLED (opposite polarity to
     * the pre-existing toggles above): these stages are NEW, so default-off is
     * what keeps every pre-Phase-8C render byte-identical. enabled=0 skips the
     * stage code entirely (the numeric knobs are never read) — exact bypass. */
    int   pe_on     = mp_get(p, 0.0f, p->pitchenv_enabled, have) >= 0.5f;
    float pe_amt    = mp_get(p, 0.0f, p->pitchenv_amt,     have);
    float pe_decay  = mp_get(p, 0.0f, p->pitchenv_decay,   have);
    int   lfo_on    = mp_get(p, 0.0f, p->lfo_enabled,      have) >= 0.5f;
    float lfo_rate  = mp_get(p, 0.0f, p->lfo_rate,         have);
    float lfo_depth = mp_get(p, 0.0f, p->lfo_depth,        have);
    int   burst_on  = mp_get(p, 0.0f, p->burst_enabled,    have) >= 0.5f;

    if (a < 0.0f) a = 0.0f;
    if (rel < 0.0f) rel = 0.0f;
    if (sus < 0.0f) sus = 0.0f; else if (sus > 1.0f) sus = 1.0f;

    /* Base frequency with octave/semitone/cent shifts. */
    double freq = MODULAR_BASE_FREQ
                * pow(2.0, (double)octave)
                * pow(2.0, (double)pitch_st / 12.0)
                * pow(2.0, (double)detune / 1200.0);
    if (freq < 1.0) freq = 1.0;

    /* voice_freq_hz override (Phase 2): >0 sets the voice freq to an EXACT Hz,
     * applied AFTER the pow derivation. Identity 0 keeps the derived freq, so
     * every pre-Phase-2 preset is byte-unchanged. Used by migrated family
     * presets whose fundamental (e.g. 45 Hz) is not reachable via 220·2^x. */
    {
        double vfh = (double)mp_get(p, 0.0f, p->voice_freq_hz, have);
        if (vfh > 0.0) freq = vfh;
    }

    /* osc_wrote tracks whether the legacy osc stage ASSIGNED into out[] this
     * call. When it did not (osc disabled / no source), the gen bank's first
     * active slot must ASSIGN rather than accumulate into the zeroed buffer —
     * otherwise 0.0f + (-0.0f) would flip a legacy -0.0 sample to +0.0f. */
    int osc_wrote = 0;

    /* ── Oscillator / generator stage ── */
    if (!osc_on) {
        /* Generator disabled by the user → silence (out[] is already zeroed).
         * The generator is the sound source, so "bypass" means no source. */
    } else if (osc_type == 5 || osc_type == 6) {
        noise_gen ng;
        noise_init(&ng, noise_seed);
        if (osc_type == 6) {
            for (int i = 0; i < samples; i++) out[i] = noise_pink_tick(&ng);
        } else {
            for (int i = 0; i < samples; i++) out[i] = noise_white_tick(&ng);
        }
        osc_wrote = 1;
    } else if (osc_type == 4) {
        fm_preset preset;
        build_fm_preset(p, freq, &preset);
        if (!fm_on) {
            /* FM disabled → carrier-only: drop all modulation routing (incl.
             * op1 self-feedback) so only the base carrier sounds. */
            memset(preset.mod_matrix, 0, sizeof(preset.mod_matrix));
        }
        if (pe_on) {
            /* PITCH ENV on the FM core: the fm_preset already carries the
             * exponential semitone sweep (pitch_env_amount / pitch_env_decay,
             * fm_render lines ~127-129); build_fm_preset zeroes them, so the
             * stage just fills them in. Stage off ⇒ fields stay 0 ⇒ the exact
             * pre-Phase-8C preset. */
            preset.pitch_env_amount = pe_amt;
            preset.pitch_env_decay  = pe_decay;
        }
        fm_render(&preset, out, sampleRate, samples);
        /* fm_render applies its own steady envelope + softsat; we still shape
         * with the voice amp ADSR below. */
        osc_wrote = 1;
    } else if (pe_on && pe_amt != 0.0f && pe_decay > 0.0f) {
        /* PITCH ENV on the wavetable oscillator: per-sample exponential
         * semitone sweep, mirroring the FM core's formula (exp(-t/decay) decay
         * shape, 2^(st/12) pitch multiplier). This branch only exists when the
         * stage is ACTIVE — the disabled path below is the untouched legacy
         * loop, so enabled=0 is an exact bypass by construction. */
        ensure_modular_tables();
        wt_osc_t osc;
        wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double env = exp(-t / (double)pe_decay);
            double mult = pow(2.0, ((double)pe_amt * env) / 12.0);
            wt_osc_set_freq(&osc, freq * mult, sampleRate);
            out[i] = wt_osc_tick(&osc);
        }
        osc_wrote = 1;
    } else {
        ensure_modular_tables();
        wt_osc_t osc;
        wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
        for (int i = 0; i < samples; i++) {
            out[i] = wt_osc_tick(&osc);
        }
        osc_wrote = 1;
    }

    /* ── Gen bank (Phase 1, modular unification): parallel slots accumulate
     * into the mix AFTER the legacy osc stage writes. All-identity params ⇒
     * no active slot ⇒ exact no-op (byte-identity invariant). Runs even when
     * osc_enabled == 0 (slots are independent of the legacy osc gate). ── */
    modular_gen_bank_render(out, sampleRate, samples, p, freq, noise_seed, osc_wrote);

    /* ── Phase-8C LFO stage: post-mix amp wobble. Multiplies the FULL mix
     * (legacy osc + gen-bank family voices), so it is a real stage on every
     * instrument. gain(t) = 1 - depth·(0.5 - 0.5·sin(2πft)) wobbles in
     * [1-depth, 1] — never exceeds unity, starts at the midpoint heading up.
     * Disabled (the default) ⇒ the loop never runs ⇒ exact bypass. ── */
    if (lfo_on && lfo_depth > 0.0f && lfo_rate > 0.0f) {
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double wob = 1.0 - (double)lfo_depth
                       * (0.5 - 0.5 * sin(2.0 * M_PI * (double)lfo_rate * t));
            out[i] = (float)((double)out[i] * wob);
        }
    }

    /* ── Phase-8C BURST stage: post-mix multi-burst gate (clap-style staggered
     * transients on any source). env(t) = Σ_j amp_j·e^(-sharp·(t-off_j)) for
     * t ≥ off_j; out is multiplied by the summed envelope. With every amp at 0
     * (the identity) an ENABLED burst stage gates to silence — the UI defaults
     * (modular_recipe.go) ship amp1=1 + staggered 2/3 so enabling is audible.
     * Disabled (the default) ⇒ exact bypass. ── */
    if (burst_on) {
        float sharp = mp_get(p, 0.0f, p->burst_sharp, have);
        float boff[4] = {
            mp_get(p, 0.0f, p->burst1_off, have), mp_get(p, 0.0f, p->burst2_off, have),
            mp_get(p, 0.0f, p->burst3_off, have), mp_get(p, 0.0f, p->burst4_off, have),
        };
        float bamp[4] = {
            mp_get(p, 0.0f, p->burst1_amp, have), mp_get(p, 0.0f, p->burst2_amp, have),
            mp_get(p, 0.0f, p->burst3_amp, have), mp_get(p, 0.0f, p->burst4_amp, have),
        };
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double env = 0.0;
            for (int j = 0; j < 4; j++) {
                if (bamp[j] != 0.0f && t >= (double)boff[j]) {
                    env += (double)bamp[j] * exp(-(double)sharp * (t - (double)boff[j]));
                }
            }
            out[i] = (float)((double)out[i] * env);
        }
    }

    /* ── Amplitude ADSR stage ── */
    if (env_on) {
        adsr_t env;
        adsr_init(&env, sampleRate, a, d, sus, rel, exp_curve);
        adsr_trigger(&env);
        int rel_samples = (int)(rel * (float)sampleRate);
        /* Cap release to at most half the note so attack/decay/sustain always
         * get the first half — otherwise a note shorter than the release time
         * would collapse straight into release and the decay knob would do
         * nothing. */
        if (rel_samples > samples / 2) rel_samples = samples / 2;
        int rel_start = samples - rel_samples;
        if (rel_start < 1) rel_start = 1;
        for (int i = 0; i < samples; i++) {
            if (i == rel_start) adsr_release(&env);
            out[i] *= adsr_tick(&env);
        }
    }
    /* env disabled → leave the raw generator output (constant unity gain). */

    /* ── Filter stage ── */
    if (filt_on) {
        mod_biquad filt;
        mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
        for (int i = 0; i < samples; i++) {
            out[i] = mod_biquad_tick_f(&filt, out[i]);
        }
    }
    /* filter disabled → passthrough. */

    /* ── Drive + gain post stage ── */
    if (drive_on && drive > 0.0f) {
        float k = 1.0f + drive * 3.0f;
        float norm = (float)tanh((double)k);
        for (int i = 0; i < samples; i++) {
            out[i] = (float)tanh((double)(out[i] * k)) / norm;
        }
    }
    if (gain != 1.0f) {
        for (int i = 0; i < samples; i++) out[i] *= gain;
    }

    /* ── Shared POST stage (Phase 2). Runs at the VERY END, after gain, when
     * post_enabled >= 0.5. It calls the SAME compiled apply_post_params used by
     * the legacy drums.c _p() renderers, so a migrated family preset is
     * byte-identical to its legacy post-processing by construction. A
     * synth_params base is assembled from the post_* params and a post_config
     * from post_decay_rate + the post_*_on gates. Legacy renderers apply NO
     * safety clamp after post, so when POST runs the modular clamp is skipped
     * (the clamp only guards the pre-Phase-2 path). ── */
    int post_on = mp_get(p, 0.0f, p->post_enabled, have) >= 0.5f;
    if (post_on) {
        synth_params base;
        base.pitch      = mp_get(p, 0.0f, p->post_pitch, have);
        base.decay      = mp_get(p, 1.0f, p->post_decay, have);
        base.tone       = mp_get(p, 0.0f, p->post_tone, have);
        base.drive      = mp_get(p, 0.0f, p->post_drive, have);
        base.body       = mp_get(p, 0.0f, p->post_body, have);
        base.brightness = mp_get(p, 0.0f, p->post_brightness, have);
        base.fundamental = 0.0f; /* unread by apply_post_params */
        post_config cfg;
        cfg.decay_rate = (double)mp_get(p, 0.0f, p->post_decay_rate, have);
        cfg.pitch      = mp_get(p, 1.0f, p->post_pitch_on, have) >= 0.5f;
        cfg.decay      = mp_get(p, 1.0f, p->post_decay_on, have) >= 0.5f;
        cfg.drive      = mp_get(p, 1.0f, p->post_drive_on, have) >= 0.5f;
        cfg.tone       = mp_get(p, 1.0f, p->post_tone_on, have) >= 0.5f;
        cfg.body       = mp_get(p, 1.0f, p->post_body_on, have) >= 0.5f;
        cfg.brightness = mp_get(p, 1.0f, p->post_brightness_on, have) >= 0.5f;

        int post_order = (int)lrintf(mp_get(p, 0.0f, p->post_order, have));
        if (post_order == 1) {
            /* Legacy BASE kick (render_kick_p) op ORDER: pitch → decay → drive →
             * body. apply_post_params applies body BEFORE drive; the base kick's
             * bespoke inline post applies drive BEFORE body. The op IMPLEMENTATIONS
             * are byte-identical (same resample, same decay formula+clamp, same
             * sp_saturate, same one-pole body mix), so we reproduce the legacy
             * SEQUENCE by calling the shared helper twice with disjoint configs:
             * first {pitch,decay,drive} (body off → pitch→decay→drive), then
             * {body}. This shares the op code while transplanting the order. The
             * caller only sets order 1 for the base kick, whose POST wiring is
             * {pitch,decay,drive,body}; brightness/tone are off there. */
            post_config first = cfg;
            first.body = 0;
            first.brightness = 0;
            first.tone = 0;
            apply_post_params(out, sampleRate, samples, &base, &first);
            post_config bodyOnly;
            bodyOnly.decay_rate = cfg.decay_rate;
            bodyOnly.pitch = 0;
            bodyOnly.decay = 0;
            bodyOnly.drive = 0;
            bodyOnly.tone = 0;
            bodyOnly.body = cfg.body;
            bodyOnly.brightness = 0;
            apply_post_params(out, sampleRate, samples, &base, &bodyOnly);
            return;
        }
        if (post_order == 2) {
            /* Legacy BASE snare (render_snare_p) op ORDER: pitch → decay → drive →
             * tone. apply_post_params applies tone BEFORE drive; the base snare's
             * bespoke inline post applies drive BEFORE tone. The op IMPLEMENTATIONS
             * are byte-identical (same resample, same decay formula+clamp, same
             * sp_saturate, same tone<0 one-pole LP), so we reproduce the legacy
             * SEQUENCE by calling the shared helper twice with disjoint configs:
             * first {pitch,decay,drive} (tone off → pitch→decay→drive), then {tone}.
             * The base snare's POST wiring is {pitch,decay,drive,tone}; body/
             * brightness are off there. */
            post_config first = cfg;
            first.tone = 0;
            first.body = 0;
            first.brightness = 0;
            apply_post_params(out, sampleRate, samples, &base, &first);
            post_config toneOnly;
            toneOnly.decay_rate = cfg.decay_rate;
            toneOnly.pitch = 0;
            toneOnly.decay = 0;
            toneOnly.drive = 0;
            toneOnly.tone = cfg.tone;
            toneOnly.body = 0;
            toneOnly.brightness = 0;
            apply_post_params(out, sampleRate, samples, &base, &toneOnly);
            return;
        }

        apply_post_params(out, sampleRate, samples, &base, &cfg);
        return;
    }

    /* Safety clamp (pre-Phase-2 path only — legacy post renderers don't clamp). */
    for (int i = 0; i < samples; i++) {
        if (out[i] > 1.5f) out[i] = 1.5f;
        else if (out[i] < -1.5f) out[i] = -1.5f;
    }
}

EXPORT void render_modular(float *out, int sampleRate, int samples) {
    render_modular_p(out, sampleRate, samples, 0);
}
