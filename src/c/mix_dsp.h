#ifndef MIX_DSP_H
#define MIX_DSP_H

/* ================================================================
 * mix_dsp — the channel EQ and the master soft clip, one implementation
 * for desktop (cgo) and the browser (beatmo-dsp AudioWorklet).
 *
 * An EQ is a PROGRAM built by Go (audio.BuildEQProgram, the same code on
 * both platforms) — a flat array of doubles:
 *
 *   [nStages, stage...]
 *   stage kind 0  biquad:  0, b0, b1, b2, a1, a2
 *   stage kind 1  split:   1, nBands, band...
 *                 band:    muted, gain, nFilt, (b0, b1, b2, a1, a2) × nFilt
 *   stage kind 2  silence: 2
 *
 * Stages run in series. A biquad is Direct Form I with double state and
 * float input/output; a split runs each unmuted band's filters in series on
 * a copy of the input and sums band × gain in float. That is exactly the
 * arithmetic desktop's Go EQ used, so moving it here changed no sample.
 * ================================================================ */

/* A new EQ for prog (n doubles), or 0 when prog is malformed. */
void *eq_create(const double *prog, int n);

/* Swap in prog's coefficients and gains, KEEPING filter state, when prog has
 * the same shape (stage kinds, band count, filters per band, muted flags):
 * returns 1. Otherwise returns 0 and changes nothing (the caller builds a new
 * EQ — a different topology starts from silence on both platforms). */
int eq_update(void *h, const double *prog, int n);

void eq_run(void *h, const float *in, float *out, int n);
void eq_clear(void *h);
void eq_destroy(void *h);

/* The master soft clip: identity up to t, then t + (1−t)·tanh((|x|−t)/(1−t)),
 * odd-symmetric — continuous and smooth, approaching ±1. */
double soft_clip(double x, double t);
void soft_clip_run(const float *in, float *out, int n, double t);
void soft_clip_run_d(const double *in, double *out, int n, double t);

#endif /* MIX_DSP_H */
