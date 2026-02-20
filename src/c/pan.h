#ifndef PAN_H
#define PAN_H

#include <math.h>

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* ================================================================
 * Equal-power panning.
 *
 * Uses constant-power pan law:
 *   gain_l = cos(angle), gain_r = sin(angle)
 *   where angle = (pan + 1) * π/4
 *
 * pan = -1.0: full left
 * pan =  0.0: center (equal power both channels)
 * pan = +1.0: full right
 * ================================================================ */

typedef struct {
    float pan;     /* -1 (left) to +1 (right) */
    float gain_l;  /* left channel gain */
    float gain_r;  /* right channel gain */
} pan_t;

/* Set pan position and recompute gains. */
static inline void pan_set(pan_t *p, float pan) {
    if (pan < -1.0f) pan = -1.0f;
    if (pan >  1.0f) pan =  1.0f;
    p->pan = pan;
    float angle = (pan + 1.0f) * (float)M_PI * 0.25f;
    p->gain_l = cosf(angle);
    p->gain_r = sinf(angle);
}

/* Initialize pan to center. */
static inline void pan_init(pan_t *p) {
    pan_set(p, 0.0f);
}

/* Process a mono block into interleaved stereo left/right buffers.
 * left and right are accumulated (+=), not overwritten. */
void pan_process_mono_to_lr(const pan_t *p, const float *in,
                             float *left, float *right, int samples);

/* Process a mono block into interleaved stereo output.
 * out must be at least samples*2 floats.
 * Output format: [L0, R0, L1, R1, ...] */
void pan_process_mono_to_interleaved(const pan_t *p, const float *in,
                                      float *out, int samples);

#endif /* PAN_H */
