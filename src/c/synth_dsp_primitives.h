#ifndef SYNTH_DSP_PRIMITIVES_H
#define SYNTH_DSP_PRIMITIVES_H

/* Composable static-inline blocks the drum voices and bow dynamics share, inlined into the caller's loop. A primitive
 * must reproduce the exact expression it replaces: where the caller's math associates in a particular order, the
 * caller computes the scalar and passes it in (float multiply is not associative). */

#include <math.h>
#include "synth_post.h" /* osc_wave_shared */

/* coef ≈ 2π·fc/sr (smaller = darker). */
static inline double sp_onepole_tick(double *state, double coef, double in) {
    *state += coef * (in - *state);
    return *state;
}

/* keep and mix as two literals reproduce that idiom's rounding: do NOT fold into sp_onepole_tick, whose op order differs. */
static inline double sp_onepole_mix(double *state, double keep, double mix, double in) {
    *state = *state * keep + in * mix;
    return *state;
}

/* 1 + amt·exp(-rate·t): starts at 1+amt, decays to 1. */
static inline double sp_pitch_glide(double tSec, double rate, double amt) {
    return 1.0 + amt * exp(-rate * tSec);
}

/* y = x - x1 + r·y1 (a high-pass at ~(1-r)), op order preserved. */
typedef struct {
    double x1, y1;
} sp_dcblock;

static inline double sp_dcblock_tick(sp_dcblock *d, double in, double r) {
    double y = in - d->x1 + r * d->y1;
    d->x1 = in;
    d->y1 = y;
    return y;
}

/* Write, increment, wrap; reads go through pm_frac_read. */
static inline void sp_delay_write(float *buf, int len, int *wi, double in) {
    buf[*wi] = (float)in;
    (*wi)++;
    if (*wi >= len) *wi = 0;
}

/* Coefficients are computed at the call site, since normalization order varies per site and float math is not
 * associative; the tick is the shared Direct Form I update verbatim. */
typedef struct {
    double b0, b1, b2, a1, a2;
    double x1, x2, y1, y2;
} sp_biquad;

static inline double sp_biquad_tick(sp_biquad *q, double x) {
    double y = q->b0 * x + q->b1 * q->x1 + q->b2 * q->x2 - q->a1 * q->y1 - q->a2 * q->y2;
    q->x2 = q->x1;
    q->x1 = x;
    q->y2 = q->y1;
    q->y1 = y;
    return y;
}

/* The caller passes the exact per-sample increment, so the result does not depend on how it associates. */
typedef struct {
    double phase;
} sp_osc;

static inline void sp_osc_init(sp_osc *o, double phase0) { o->phase = phase0; }

static inline double sp_osc_tick(sp_osc *o, int wave, double phaseInc) {
    o->phase += phaseInc;
    return osc_wave_shared(wave, o->phase);
}

/* Three close taps and a dark LP in the feedback: a diffuse wash, not a slap echo, low-feedback so it never blooms
 * over the attack. A power-of-two length masks cheaply; on the caller's stack, so re-entrant. */
#define SP_REVERB_LEN 4096
#define SP_REVERB_MASK (SP_REVERB_LEN - 1)

typedef struct {
    double buf[SP_REVERB_LEN];
    int idx, d1, d2, d3;
    double lp;
} sp_reverb3;

static inline void sp_reverb3_init(sp_reverb3 *r, int sampleRate) {
    for (int z = 0; z < SP_REVERB_LEN; ++z) r->buf[z] = 0.0;
    r->d1 = (int)(0.019 * (double)sampleRate); if (r->d1 >= SP_REVERB_LEN) r->d1 = SP_REVERB_LEN - 1;
    r->d2 = (int)(0.029 * (double)sampleRate); if (r->d2 >= SP_REVERB_LEN) r->d2 = SP_REVERB_LEN - 1;
    r->d3 = (int)(0.041 * (double)sampleRate); if (r->d3 >= SP_REVERB_LEN) r->d3 = SP_REVERB_LEN - 1;
    r->idx = 0;
    r->lp = 0.0;
}

/* Returns the reverb to ADD to the dry, scaled by amount. */
static inline double sp_reverb3_tick(sp_reverb3 *r, double in, double amount) {
    double fbTap = r->buf[(r->idx - r->d1) & SP_REVERB_MASK]
                 + r->buf[(r->idx - r->d2) & SP_REVERB_MASK]
                 + r->buf[(r->idx - r->d3) & SP_REVERB_MASK];
    r->lp += 0.30 * (fbTap * 0.33 - r->lp);
    double revState = in * 0.25 + r->lp * 0.45;
    r->buf[r->idx] = revState;
    r->idx = (r->idx + 1) & SP_REVERB_MASK;
    return r->lp * amount;
}

#endif /* SYNTH_DSP_PRIMITIVES_H */
