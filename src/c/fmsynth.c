#include <math.h>
#include <string.h>
#include "wavetable.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* ── 4-operator FM synthesis engine ──────────────────────────────────────── */

#define FM_MAX_OPS 4
#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif
#define TWO_PI (2.0 * M_PI)

typedef struct {
    float freq_ratio;     /* frequency = base_freq * freq_ratio + freq_offset */
    float freq_offset;    /* Hz, for inharmonic metallic sounds              */
    float amplitude;      /* 0.0–1.0                                         */
    float attack_sec;     /* ADSR attack time                                */
    float decay_sec;      /* ADSR decay time                                 */
    float sustain_level;  /* ADSR sustain level (0.0–1.0)                    */
    float release_sec;    /* ADSR release time                               */
    int   is_carrier;     /* 1 = output to speaker, 0 = modulator only       */
} fm_operator;

typedef struct {
    int   num_ops;
    float base_freq;                              /* Hz                       */
    float mod_matrix[FM_MAX_OPS][FM_MAX_OPS];     /* [src][dst] = mod depth   */
    fm_operator ops[FM_MAX_OPS];
    float pitch_env_amount;                        /* semitones of pitch sweep */
    float pitch_env_decay;                         /* decay rate (seconds)     */
} fm_preset;

/* ── Static global sine wavetable for FM oscillators ─────────────────────── */

static float       fm_sine_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t fm_sine_wt;
static int         fm_sine_initialized = 0;

static void ensure_fm_sine(void) {
    if (fm_sine_initialized) return;
    wt_generate_sine(&fm_sine_wt, fm_sine_buf, WT_DEFAULT_LENGTH);
    fm_sine_initialized = 1;
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

static void fm_render(const fm_preset *p, float *out,
                      int sampleRate, int samples) {
    memset(out, 0, (size_t)samples * sizeof(float));

    int nops = p->num_ops;
    if (nops > FM_MAX_OPS) nops = FM_MAX_OPS;
    if (nops <= 0) return;

    ensure_fm_sine();

    double dt = 1.0 / (double)sampleRate;
    double total_dur = (double)samples * dt;
    int    wt_len = fm_sine_wt.length;
    const float *table = fm_sine_wt.table;

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

/* ── Presets ──────────────────────────────────────────────────────────────── */

/*
 * FM Bass: warm, round bass with slight modulation decay.
 * 2 operators: op0 = carrier (1:1), op1 = modulator (1:1 ratio).
 * Modulator has fast decay for initial brightness that fades to warmth.
 */
static const fm_preset PRESET_FM_BASS = {
    .num_ops   = 2,
    .base_freq = 55.0f,  /* A1 */
    .mod_matrix = {
        /* src\dst  op0    op1 */
        /*  op0 */ {0.0f, 0.0f, 0.0f, 0.0f},
        /*  op1 */ {2.5f, 0.0f, 0.0f, 0.0f},  /* op1 modulates op0 */
        {0}, {0}
    },
    .ops = {
        /* op0: carrier */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.9f,
          .attack_sec = 0.005f, .decay_sec = 0.3f,
          .sustain_level = 0.6f, .release_sec = 0.2f, .is_carrier = 1 },
        /* op1: modulator */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.8f,
          .attack_sec = 0.001f, .decay_sec = 0.15f,
          .sustain_level = 0.2f, .release_sec = 0.1f, .is_carrier = 0 },
    },
    .pitch_env_amount = 3.0f,   /* subtle pitch drop for punch */
    .pitch_env_decay  = 0.06f,
};

/*
 * FM Bell: DX7-style metallic bell.
 * 2 operators: inharmonic ratio (1:3.5) for metallic character.
 * Long decay, no sustain — ring and fade.
 */
static const fm_preset PRESET_FM_BELL = {
    .num_ops   = 2,
    .base_freq = 440.0f,  /* A4 */
    .mod_matrix = {
        {0.0f, 0.0f, 0.0f, 0.0f},
        {3.0f, 0.0f, 0.0f, 0.0f},  /* op1 modulates op0 */
        {0}, {0}
    },
    .ops = {
        /* op0: carrier */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.7f,
          .attack_sec = 0.001f, .decay_sec = 1.5f,
          .sustain_level = 0.0f, .release_sec = 0.3f, .is_carrier = 1 },
        /* op1: modulator — inharmonic ratio for metallic timbre */
        { .freq_ratio = 3.5f, .freq_offset = 0.0f, .amplitude = 0.6f,
          .attack_sec = 0.001f, .decay_sec = 1.2f,
          .sustain_level = 0.0f, .release_sec = 0.2f, .is_carrier = 0 },
    },
    .pitch_env_amount = 0.0f,
    .pitch_env_decay  = 0.0f,
};

/*
 * FM Lead: bright, aggressive lead.
 * 3 operators: op2 → op1 → op0 (modulators in series).
 * Short, punchy envelope with high modulation index.
 */
static const fm_preset PRESET_FM_LEAD = {
    .num_ops   = 3,
    .base_freq = 220.0f,  /* A3 */
    .mod_matrix = {
        {0.0f, 0.0f, 0.0f, 0.0f},
        {3.5f, 0.0f, 0.0f, 0.0f},  /* op1 modulates op0 */
        {0.0f, 2.0f, 0.0f, 0.0f},  /* op2 modulates op1 */
        {0}
    },
    .ops = {
        /* op0: carrier */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.8f,
          .attack_sec = 0.003f, .decay_sec = 0.2f,
          .sustain_level = 0.5f, .release_sec = 0.15f, .is_carrier = 1 },
        /* op1: modulator 1 */
        { .freq_ratio = 2.0f, .freq_offset = 0.0f, .amplitude = 0.9f,
          .attack_sec = 0.001f, .decay_sec = 0.12f,
          .sustain_level = 0.3f, .release_sec = 0.1f, .is_carrier = 0 },
        /* op2: modulator 2 */
        { .freq_ratio = 3.0f, .freq_offset = 0.0f, .amplitude = 0.5f,
          .attack_sec = 0.001f, .decay_sec = 0.08f,
          .sustain_level = 0.1f, .release_sec = 0.05f, .is_carrier = 0 },
    },
    .pitch_env_amount = 1.5f,
    .pitch_env_decay  = 0.04f,
};

/*
 * FM E-Piano: Rhodes-like warm electric piano.
 * 3 operators: op1 modulates op0 (main tone), op2 is a second carrier
 * at 2x for harmonic richness.
 */
static const fm_preset PRESET_FM_EPIANO = {
    .num_ops   = 3,
    .base_freq = 261.63f,  /* C4 (middle C) */
    .mod_matrix = {
        {0.0f, 0.0f, 0.0f, 0.0f},
        {1.8f, 0.0f, 0.0f, 0.0f},  /* op1 modulates op0 */
        {0.0f, 0.0f, 0.0f, 0.0f},  /* op2 is independent carrier */
        {0}
    },
    .ops = {
        /* op0: carrier (fundamental) */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.7f,
          .attack_sec = 0.002f, .decay_sec = 0.8f,
          .sustain_level = 0.3f, .release_sec = 0.4f, .is_carrier = 1 },
        /* op1: modulator (tine character) */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.6f,
          .attack_sec = 0.001f, .decay_sec = 0.3f,
          .sustain_level = 0.1f, .release_sec = 0.2f, .is_carrier = 0 },
        /* op2: carrier (octave harmonic for shimmer) */
        { .freq_ratio = 2.0f, .freq_offset = 0.0f, .amplitude = 0.25f,
          .attack_sec = 0.001f, .decay_sec = 0.5f,
          .sustain_level = 0.05f, .release_sec = 0.3f, .is_carrier = 1 },
    },
    .pitch_env_amount = 0.0f,
    .pitch_env_decay  = 0.0f,
};

/*
 * FM Pluck: short, percussive plucked string.
 * 2 operators: fast modulator envelope decay creates the pluck transient.
 */
static const fm_preset PRESET_FM_PLUCK = {
    .num_ops   = 2,
    .base_freq = 196.0f,  /* G3 */
    .mod_matrix = {
        {0.0f, 0.0f, 0.0f, 0.0f},
        {4.0f, 0.0f, 0.0f, 0.0f},  /* op1 modulates op0, high index */
        {0}, {0}
    },
    .ops = {
        /* op0: carrier */
        { .freq_ratio = 1.0f, .freq_offset = 0.0f, .amplitude = 0.85f,
          .attack_sec = 0.001f, .decay_sec = 0.2f,
          .sustain_level = 0.0f, .release_sec = 0.05f, .is_carrier = 1 },
        /* op1: modulator — very fast decay for pluck attack */
        { .freq_ratio = 2.0f, .freq_offset = 0.0f, .amplitude = 1.0f,
          .attack_sec = 0.0005f, .decay_sec = 0.04f,
          .sustain_level = 0.0f, .release_sec = 0.02f, .is_carrier = 0 },
    },
    .pitch_env_amount = 2.0f,
    .pitch_env_decay  = 0.03f,
};

/* ── Per-preset render wrappers ──────────────────────────────────────────── */

EXPORT void render_fm_bass(float *out, int sampleRate, int samples) {
    fm_render(&PRESET_FM_BASS, out, sampleRate, samples);
}

EXPORT void render_fm_bell(float *out, int sampleRate, int samples) {
    fm_render(&PRESET_FM_BELL, out, sampleRate, samples);
}

EXPORT void render_fm_lead(float *out, int sampleRate, int samples) {
    fm_render(&PRESET_FM_LEAD, out, sampleRate, samples);
}

EXPORT void render_fm_epiano(float *out, int sampleRate, int samples) {
    fm_render(&PRESET_FM_EPIANO, out, sampleRate, samples);
}

EXPORT void render_fm_pluck(float *out, int sampleRate, int samples) {
    fm_render(&PRESET_FM_PLUCK, out, sampleRate, samples);
}
