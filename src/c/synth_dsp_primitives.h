#ifndef SYNTH_DSP_PRIMITIVES_H
#define SYNTH_DSP_PRIMITIVES_H

/* synth_dsp_primitives.h — small, composable DSP building blocks that the
 * kick/tom/snare/cymbal voices and the bowed-string dynamics compose instead
 * of open-coding the same state machines inline. Each primitive is a
 * `static inline` function so it is inlined into the caller's sample loop:
 * same generated code, same float-op ORDER → BYTE-IDENTICAL to the
 * hand-written loops it replaces (verified by TestVariant7RefactorByteIdentity
 * and TestSynthGoldenByteIdentity). Reusing them removes duplication (the same
 * one-pole LP appeared in the click, the texture and the reverb; the same
 * phase-accumulator-into-wavetable idiom appeared across every drum voice) and
 * makes each unit testable in isolation.
 *
 * Byte-identity rule: a primitive must reproduce the exact expression it
 * replaces. Where the caller's math associates in a particular order (e.g. a
 * phase increment `2*pi*f*mul/sr`), the caller computes that scalar and passes
 * it in, rather than the primitive re-deriving it in a different order (IEEE
 * float multiply is not associative).
 *
 * Deliberate non-migration: the crash and ride cymbal "bell" partials
 * evaluate their phase accumulator via a bare `sin(phase)` rather than
 * `osc_wave_shared(wave, phase)` — a different evaluation function than
 * `sp_osc_tick` encodes, and 2 sites don't justify a `sp_osc_sin` variant, so
 * those two stay hand-rolled. */

#include <math.h>
#include "synth_post.h" /* osc_wave_shared */

/* sp_onepole_tick: one-pole low-pass. `*state += coef*(in - *state)` and returns
 * the new state. The workhorse behind the beater click's band-pass, the low-mid
 * texture LP, and the reverb feedback LP. coef ≈ 2*pi*fc/sr (small = darker). */
static inline double sp_onepole_tick(double *state, double coef, double in) {
    *state += coef * (in - *state);
    return *state;
}

/* sp_onepole_mix: the second one-pole LP idiom in the codebase,
 * `*state = *state*keep + in*mix` (keep+mix ~= 1, written as two literals so
 * the historical float rounding is reproduced exactly — do NOT fold to the
 * sp_onepole_tick form, the op order differs and output would drift). */
static inline double sp_onepole_mix(double *state, double keep, double mix, double in) {
    *state = *state * keep + in * mix;
    return *state;
}

/* sp_pitch_glide: the drumhead-tension pitch multiplier `1 + amt*exp(-rate*t)`
 * (starts at 1+amt, decays to 1). Stateless. */
static inline double sp_pitch_glide(double tSec, double rate, double amt) {
    return 1.0 + amt * exp(-rate * tSec);
}

/* sp_dcblock: one-pole DC blocker `y = x - x1 + r*y1` (a high-pass at ~(1-r)).
 * The self-oscillating physical models (brass/reed/flute/sax) each open-coded
 * this on their output; `r` is the pole (0.99–0.995). Op order preserved exactly
 * so composing it is byte-identical to the inline form. */
typedef struct {
    double x1, y1;
} sp_dcblock;

static inline double sp_dcblock_tick(sp_dcblock *d, double in, double r) {
    double y = in - d->x1 + r * d->y1;
    d->x1 = in;
    d->y1 = y;
    return y;
}

/* sp_delay_write: the shared waveguide delay-line write+wrap. Reads go through
 * the existing pm_frac_read helper; this pairs it with the identical
 * write-then-increment-then-wrap the four physical models each open-coded. */
static inline void sp_delay_write(float *buf, int len, int *wi, double in) {
    buf[*wi] = (float)in;
    (*wi)++;
    if (*wi >= len) *wi = 0;
}

/* sp_biquad: Direct-Form-I biquad TICK. Coefficients are computed AT THE CALL
 * SITE exactly as the historical inline code did (normalization order varies
 * per site and IEEE float math is not associative) and stored pre-normalized;
 * the tick reproduces the shared update
 *   y = b0*x + b1*x1 + b2*x2 - a1*y1 - a2*y2;  x2=x1; x1=x; y2=y1; y1=y;
 * verbatim. 10 drum/cymbal voices open-coded this exact block. */
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

/* sp_osc: a phase-accumulating oscillator over the shared wavetable. The caller
 * passes the exact per-sample phase INCREMENT (so byte-identity with the
 * hand-written `phase += 2*M_PI*f*mul/sr` is preserved regardless of how the
 * increment associates). */
typedef struct {
    double phase;
} sp_osc;

static inline void sp_osc_init(sp_osc *o, double phase0) { o->phase = phase0; }

static inline double sp_osc_tick(sp_osc *o, int wave, double phaseInc) {
    o->phase += phaseInc;
    return osc_wave_shared(wave, o->phase);
}

/* sp_reverb3: a dense 3-tap feedback-comb reverb TAIL (a recorded kick's room).
 * Three closely-spaced taps summed + a dark LP in the feedback → a diffuse
 * decaying wash (NOT a single slap echo, and low-feedback so it never blooms
 * over the attack). Delay-line length is a power of two so the circular index
 * masks cheaply. Declared on the caller's stack (per-voice, re-entrant). */
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

/* sp_reverb3_tick: feed `in` (the dry sample) into the tank and return the
 * reverb output to ADD to the dry (scaled by `amount`). */
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
