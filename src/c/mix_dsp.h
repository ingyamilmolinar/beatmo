#ifndef MIX_DSP_H
#define MIX_DSP_H

/* The channel EQ and master soft clip, one implementation for both platforms. An EQ is a PROGRAM Go builds:
 * [nStages, stage...] with biquad (0,b0,b1,b2,a1,a2), split (1,nBands,{muted,gain,nFilt,coeffs×nFilt}) or silence (2),
 * in series; Direct Form I with double state, the exact arithmetic desktop's Go EQ used. */

/* A new EQ for prog (n doubles), or 0 when prog is malformed. */
void *eq_create(const double *prog, int n);

/* Same shape: keeps filter state and returns 1. Otherwise 0 and nothing changes: a new topology starts from silence
 * on both platforms. */
int eq_update(void *h, const double *prog, int n);

void eq_run(void *h, const float *in, float *out, int n);
void eq_clear(void *h);
void eq_destroy(void *h);

/* Identity up to t, then t + (1−t)·tanh((|x|−t)/(1−t)), odd-symmetric: continuous and smooth toward ±1. */
double soft_clip(double x, double t);
void soft_clip_run(const float *in, float *out, int n, double t);
void soft_clip_run_d(const double *in, double *out, int n, double t);

#endif /* MIX_DSP_H */
