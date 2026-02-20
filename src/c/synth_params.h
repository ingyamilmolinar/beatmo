#ifndef SYNTH_PARAMS_H
#define SYNTH_PARAMS_H

#include <math.h>

/* Shared synthesis parameter struct for parameterized instrument rendering.
 * All fields have sensible defaults when zero-initialized.
 * Pass NULL to any _p() function to use the original hardcoded defaults. */
typedef struct {
    float pitch;       /* Semitone offset from default (0 = default) */
    float decay;       /* Decay multiplier (1.0 = default, 0.5 = half, 2.0 = double) */
    float tone;        /* Tone/brightness: -1 = dark, 0 = default, 1 = bright */
    float attack;      /* Attack multiplier (1.0 = default, smaller = sharper) */
    float drive;       /* Saturation/overdrive amount (0 = none, 1 = heavy) */
    float body;        /* Body/resonance emphasis (0 = default, 1 = max) */
    float color;       /* Timbral character shift: -1..1 (instrument-specific) */
    float brightness;  /* High-frequency content: 0 = default, 1 = max shimmer */
} synth_params;

/* Helper: read a param or return default if params is NULL. */
static inline float sp_pitch(const synth_params *p)      { return p ? p->pitch      : 0.0f; }
static inline float sp_decay(const synth_params *p)      { return p ? p->decay      : 1.0f; }
static inline float sp_tone(const synth_params *p)       { return p ? p->tone       : 0.0f; }
static inline float sp_attack(const synth_params *p)     { return p ? p->attack     : 1.0f; }
static inline float sp_drive(const synth_params *p)      { return p ? p->drive      : 0.0f; }
static inline float sp_body(const synth_params *p)       { return p ? p->body       : 0.0f; }
static inline float sp_color(const synth_params *p)      { return p ? p->color      : 0.0f; }
static inline float sp_brightness(const synth_params *p) { return p ? p->brightness : 0.0f; }

/* Apply pitch offset: multiply a base frequency by 2^(semitones/12). */
static inline double sp_freq(double baseFreq, const synth_params *p) {
    float st = sp_pitch(p);
    if (st == 0.0f) return baseFreq;
    return baseFreq * pow(2.0, (double)st / 12.0);
}

/* Apply decay multiplier to a base envelope time constant. */
static inline double sp_env_decay(double baseSec, const synth_params *p) {
    float d = sp_decay(p);
    if (d <= 0.0f) d = 0.01f; /* clamp to avoid infinite decay */
    return baseSec * (double)d;
}

/* Apply attack multiplier. */
static inline double sp_env_attack(double baseSec, const synth_params *p) {
    float a = sp_attack(p);
    if (a <= 0.0f) a = 0.01f;
    return baseSec * (double)a;
}

/* Apply tone to a filter cutoff: shift by ±1 octave. */
static inline double sp_filter_cutoff(double baseCutoff, const synth_params *p) {
    float t = sp_tone(p);
    return baseCutoff * pow(2.0, (double)t);
}

/* Apply drive as tanh saturation. */
static inline float sp_saturate(float x, const synth_params *p) {
    float d = sp_drive(p);
    if (d <= 0.0f) return x;
    float driven = x * (1.0f + d * 3.0f);
    return (float)tanh((double)driven) / (float)tanh(1.0 + (double)d * 3.0);
}

#endif
