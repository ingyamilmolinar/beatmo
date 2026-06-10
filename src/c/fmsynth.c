#include <math.h>
#include <string.h>
#include "wavetable.h"
#include "synth_post.h"
#include "fmsynth.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* ── 4-operator FM synthesis engine ──────────────────────────────────────── */
/* fm_operator / fm_preset / fm_render are declared in fmsynth.h so the
 * modular voice engine (modular.c) can reuse the core renderer. */

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif
#define TWO_PI (2.0 * M_PI)

/* ── Static global sine wavetable for FM oscillators ─────────────────────── */

static float       fm_sine_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t fm_sine_wt;
static int         fm_sine_initialized = 0;

static void ensure_fm_sine(void) {
    if (fm_sine_initialized) return;
    wt_generate_sine(&fm_sine_wt, fm_sine_buf, WT_DEFAULT_LENGTH);
    fm_sine_initialized = 1;
}

/* Optional operator waveforms for the fm_wave knob (band-limited, same
 * harmonic budget as the modular voice). Lazily built; the default sine
 * path never touches them, so wave==0 renders stay bit-identical. */
#define FM_WT_HARMONICS 48
static float       fm_saw_buf[WT_DEFAULT_LENGTH + 1];
static float       fm_square_buf[WT_DEFAULT_LENGTH + 1];
static float       fm_triangle_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t fm_saw_wt, fm_square_wt, fm_triangle_wt;
static int         fm_alt_tables_initialized = 0;

static const wavetable_t *fm_table_for(int wave) {
    if (wave <= 0 || wave > 3) {
        ensure_fm_sine();
        return &fm_sine_wt;
    }
    if (!fm_alt_tables_initialized) {
        wt_generate_saw(&fm_saw_wt, fm_saw_buf, WT_DEFAULT_LENGTH, FM_WT_HARMONICS);
        wt_generate_square(&fm_square_wt, fm_square_buf, WT_DEFAULT_LENGTH, FM_WT_HARMONICS);
        wt_generate_triangle(&fm_triangle_wt, fm_triangle_buf, WT_DEFAULT_LENGTH, FM_WT_HARMONICS);
        fm_alt_tables_initialized = 1;
    }
    switch (wave) {
    case 1: return &fm_saw_wt;
    case 2: return &fm_square_wt;
    default: return &fm_triangle_wt;
    }
}

/* ── ADSR envelope (time-based, unchanged semantics) ─────────────────────── */

static float adsr_env(float t, float total_dur,
                      float a, float d, float s, float r) {
    float release_start = total_dur - r;
    if (release_start < a + d) release_start = a + d;

    if (t < 0.0f) return 0.0f;

    /* Attack */
    if (t < a) {
        return (a > 0.0f) ? t / a : 1.0f;
    }
    /* Decay */
    if (t < a + d) {
        float decay_t = (t - a) / d;
        return 1.0f - (1.0f - s) * decay_t;
    }
    /* Sustain */
    if (t < release_start) {
        return s;
    }
    /* Release */
    if (t < total_dur) {
        float rel_t = (t - release_start) / r;
        if (rel_t > 1.0f) rel_t = 1.0f;
        return s * (1.0f - rel_t);
    }
    return 0.0f;
}

/* ── Soft saturation (matches drums.c) ───────────────────────────────────── */

static inline float softsat(float x) {
    return (float)tanh((double)x * 1.5);
}

/* ── Core FM render (wavetable-based) ────────────────────────────────────── */

void fm_render(const fm_preset *p, float *out,
               int sampleRate, int samples) {
    memset(out, 0, (size_t)samples * sizeof(float));

    int nops = p->num_ops;
    if (nops > FM_MAX_OPS) nops = FM_MAX_OPS;
    if (nops <= 0) return;

    const wavetable_t *wt = fm_table_for(p->wave);

    double dt = 1.0 / (double)sampleRate;
    double total_dur = (double)samples * dt;
    int    wt_len = wt->length;
    const float *table = wt->table;

    /* Per-operator phase accumulators (in table units) and output buffers. */
    double phase[FM_MAX_OPS];
    float  op_out[FM_MAX_OPS] = {0};
    memset(phase, 0, sizeof(phase));

    for (int i = 0; i < samples; i++) {
        double t = (double)i * dt;

        /* Pitch envelope: exponential sweep in semitones. */
        double pitch_mult = 1.0;
        if (p->pitch_env_amount != 0.0f && p->pitch_env_decay > 0.0f) {
            double env = exp(-t / (double)p->pitch_env_decay);
            pitch_mult = pow(2.0, ((double)p->pitch_env_amount * env) / 12.0);
        }

        /* Compute modulation contributions from previous sample's output. */
        float mod_input[FM_MAX_OPS];
        memset(mod_input, 0, sizeof(mod_input));
        for (int src = 0; src < nops; src++) {
            for (int dst = 0; dst < nops; dst++) {
                float depth = p->mod_matrix[src][dst];
                if (depth != 0.0f) {
                    mod_input[dst] += op_out[src] * depth;
                }
            }
        }

        /* Render each operator using wavetable lookup. */
        float mix = 0.0f;
        for (int op = 0; op < nops; op++) {
            const fm_operator *o = &p->ops[op];
            double freq = (double)p->base_freq * (double)o->freq_ratio
                        + (double)o->freq_offset;
            freq *= pitch_mult;

            float env = adsr_env((float)t, (float)total_dur,
                                 o->attack_sec, o->decay_sec,
                                 o->sustain_level, o->release_sec);

            /* Phase modulation from other operators.
             * Convert from radians-equivalent to table units:
             * mod_input is in radians-like units, scale to table length / (2π). */
            double phase_mod = (double)mod_input[op] * (double)wt_len / TWO_PI;

            /* Advance phase in table units: freq * tableLength / sampleRate */
            phase[op] += freq * (double)wt_len * dt;
            /* Keep phase bounded to avoid precision loss. */
            double dlen = (double)wt_len;
            phase[op] = fmod(phase[op], dlen);
            if (phase[op] < 0) phase[op] += dlen;

            /* Wavetable lookup with linear interpolation + phase modulation. */
            double lookup = phase[op] + phase_mod;
            lookup = fmod(lookup, dlen);
            if (lookup < 0) lookup += dlen;
            int idx = (int)lookup;
            if (idx < 0) idx = 0;
            if (idx >= wt_len) idx = wt_len - 1;
            float frac = (float)(lookup - (double)idx);
            float val = table[idx] * (1.0f - frac) + table[idx + 1] * frac;

            op_out[op] = val * o->amplitude * env;

            if (o->is_carrier) {
                mix += op_out[op];
            }
        }

        out[i] = softsat(mix);
    }
}

/* ── Presets / per-preset wrappers / parameterized variants (DELETED) ────────
 *
 * The five static PRESET_FM_* tables, the render_fm_* unparameterized wrappers,
 * the render_fm_*_p parameterized wrappers, and fm_apply_params were all DELETED
 * in the Phase-7 FM-family migration (the LAST legacy family). The FM presets now
 * render through the unified modular engine: the per-variant fm_preset is built by
 * modular_gen_slot_fm (src/c/modular_stages.c, source==10) — which bakes the SAME
 * structural constants the PRESET_FM_* tables held and overlays the curated knobs
 * via kp_get (the exact overlay fm_apply_params performed) — then calls the SHARED
 * fm_render core below, followed by the shared apply_post_params POST stage. The
 * fm_params ABI block was removed from fmsynth.h; FM knobs travel in the wide
 * modular_params block (gen_fm_* columns). Byte-identity to the deleted presets is
 * proven by the FM oracle fixtures + TestFMPresetGolden.
 *
 * fm_render + the fm_preset/fm_operator types REMAIN (above + in fmsynth.h) —
 * they are the shared core used by BOTH the modular osc_type==4 FM stage and the
 * source==10 FM-family voice. ──────────────────────────────────────────────── */
