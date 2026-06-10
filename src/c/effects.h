#ifndef EFFECTS_H
#define EFFECTS_H

/* ---- Delay line with LP-filtered feedback (warm tape echo) ---- */

typedef struct {
    float *buffer;      /* circular buffer */
    int    length;      /* buffer length in samples */
    int    writePos;    /* write cursor */
    float  feedback;    /* feedback amount 0..1 */
    float  lpState;     /* one-pole LP filter state for feedback damping */
    float  lpCoeff;     /* LP filter coefficient (higher = darker) */
} delay_t;

/* Initialize a delay. Caller provides buffer of at least delaySamples floats. */
void delay_init(delay_t *d, float *buffer, int delaySamples,
                float feedback, float dampingHz, int sampleRate);

/* Process a block of samples (in-place). */
void delay_process(delay_t *d, float *buf, int samples);

/* Reset delay line to silence. */
void delay_reset(delay_t *d);

/* Click-free param updates. delay_set_time_smooth changes the read-back
 * distance without zeroing the buffer (so the existing wet tail keeps
 * playing). newDelaySamples must be <= the capacity passed to delay_init;
 * the caller is responsible for sizing the buffer to the maximum time it
 * intends to support. delay_set_feedback_smooth / delay_set_damping_smooth
 * are stateless and trivially safe to call mid-render. */
void delay_set_time_smooth(delay_t *d, int newDelaySamples);
void delay_set_feedback_smooth(delay_t *d, float feedback);
void delay_set_damping_smooth(delay_t *d, float dampingHz, int sampleRate);

/* ---- Schroeder reverb (4 comb + 2 allpass) ---- */

typedef struct {
    float *buf;
    int    length;
    int    pos;
    float  feedback;
    float  damp;
    float  dampState;
} comb_t;

typedef struct {
    float *buf;
    int    length;
    int    pos;
    float  feedback;
} allpass_t;

typedef struct {
    comb_t    combs[4];
    allpass_t allpasses[2];
    float     wet;
    float     dry;
} reverb_t;

/* Initialize reverb. Caller provides one contiguous buffer for all internal
 * delay lines (must be at least reverb_buffer_size(sampleRate) floats). */
int  reverb_buffer_size(int sampleRate);
void reverb_init(reverb_t *r, float *buffer, int sampleRate,
                 float roomSize, float damping, float wet);

/* Process a block of samples (in-place). */
void reverb_process(reverb_t *r, const float *in, float *out, int samples);

/* Reset reverb to silence. */
void reverb_reset(reverb_t *r);

/* Click-free reverb param updates. Mutates the comb feedback, comb damping,
 * and wet/dry coefficients in place; existing delay-line content keeps
 * playing so the tail decays naturally instead of being zeroed by a
 * fresh reverb_init. */
void reverb_set_params_smooth(reverb_t *r, float roomSize, float damping, float wet);

#endif
