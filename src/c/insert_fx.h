#ifndef INSERT_FX_H
#define INSERT_FX_H

/* ================================================================
 * Insert Effects — C implementations for all 6 insert effect types.
 * Each effect has _init(), _process(in, out, N), _reset(), _set_param().
 * All buffers are caller-owned (Go allocates via C.malloc, frees on cleanup).
 * ================================================================ */

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* ---- Distortion (tanh soft-clip + one-pole LP tone filter) ---- */

typedef struct {
    float drive;     /* 1-20: gain before tanh */
    float tone;      /* LP filter cutoff Hz */
    float mix;       /* 0=dry, 1=full wet */
    int   sr;
    float lp_y1;     /* one-pole LP state */
    float lp_a;      /* one-pole coefficient: exp(-2π*fc/sr) */
} ifx_distortion_t;

void ifx_distortion_init(ifx_distortion_t *d, int sr,
                          float drive, float tone, float mix);
void ifx_distortion_process(ifx_distortion_t *d,
                             const float *in, float *out, int samples);
void ifx_distortion_reset(ifx_distortion_t *d);
void ifx_distortion_set_param(ifx_distortion_t *d,
                               const char *name, float value);

/* ---- Delay (mono delay line with LP-filtered feedback) ---- */

typedef struct {
    float  time_ms;       /* delay time in ms */
    float  feedback;      /* feedback amount 0-0.95 */
    float  mix;           /* wet/dry 0-1 */
    int    sr;
    float *buf;           /* circular buffer (caller-owned) */
    int    buf_len;       /* buffer length */
    int    pos;           /* write position */
    int    delay_samples; /* current delay in samples */
    float  lp_y1;         /* one-pole LP state for feedback damping */
    float  lp_a;          /* LP coefficient */
} ifx_delay_t;

/* buf must be at least max_delay_samples floats. */
void ifx_delay_init(ifx_delay_t *d, int sr, float *buf, int buf_len,
                     float time_ms, float feedback, float mix);
void ifx_delay_process(ifx_delay_t *d,
                        const float *in, float *out, int samples);
void ifx_delay_reset(ifx_delay_t *d);
void ifx_delay_set_param(ifx_delay_t *d, const char *name, float value);

/* ---- Reverb (4 comb + 2 allpass Schroeder) ---- */

typedef struct {
    float *buf;
    int    length;
    int    pos;
    float  feedback;
    float  damping;
    float  lp_state;
} ifx_comb_t;

typedef struct {
    float *buf;
    int    length;
    int    pos;
    float  g;
} ifx_allpass_t;

typedef struct {
    float       room;      /* 0-1: scales comb feedback */
    float       damping;   /* 0-1: LP damping in combs */
    float       mix;       /* wet/dry 0-1 */
    int         sr;
    ifx_comb_t    combs[4];
    ifx_allpass_t aps[2];
    float        *mem;     /* contiguous memory for all delay lines (caller-owned) */
} ifx_reverb_t;

/* Returns total floats needed for reverb memory at given sample rate. */
int  ifx_reverb_mem_size(int sr);
void ifx_reverb_init(ifx_reverb_t *r, int sr, float *mem,
                      float room, float damping, float mix);
void ifx_reverb_process(ifx_reverb_t *r,
                         const float *in, float *out, int samples);
void ifx_reverb_reset(ifx_reverb_t *r);
void ifx_reverb_set_param(ifx_reverb_t *r, const char *name, float value);

/* ---- Chorus (LFO-modulated interpolated delay) ---- */

typedef struct {
    float  rate;          /* LFO Hz */
    float  depth;         /* modulation depth in ms */
    float  mix;           /* wet/dry 0-1 */
    int    sr;
    float *buf;           /* delay buffer (caller-owned) */
    int    buf_len;
    int    pos;
    float  phase;         /* LFO phase 0..2π */
    float  depth_samples; /* depth in samples */
    float  phase_inc;     /* per-sample phase increment */
} ifx_chorus_t;

void ifx_chorus_init(ifx_chorus_t *c, int sr, float *buf, int buf_len,
                      float rate, float depth, float mix);
void ifx_chorus_process(ifx_chorus_t *c,
                         const float *in, float *out, int samples);
void ifx_chorus_reset(ifx_chorus_t *c);
void ifx_chorus_set_param(ifx_chorus_t *c, const char *name, float value);

/* ---- Bitcrusher (sample rate & bit depth reduction) ---- */

typedef struct {
    float bits;          /* 2-16 */
    float rate;          /* 0.01-1.0 (fraction of original sr) */
    float mix;           /* wet/dry 0-1 */
    float hold_counter;  /* fractional counter for decimation */
    float hold_value;    /* last held sample */
} ifx_bitcrusher_t;

void ifx_bitcrusher_init(ifx_bitcrusher_t *b,
                          float bits, float rate, float mix);
void ifx_bitcrusher_process(ifx_bitcrusher_t *b,
                             const float *in, float *out, int samples);
void ifx_bitcrusher_reset(ifx_bitcrusher_t *b);
void ifx_bitcrusher_set_param(ifx_bitcrusher_t *b,
                               const char *name, float value);

/* ---- Filter (biquad LP/HP/BP with wet/dry) ---- */

typedef struct {
    float mode;   /* 0=LP, 1=HP, 2=BP */
    float cutoff; /* Hz */
    float q;      /* resonance */
    float mix;    /* wet/dry 0-1 */
    int   sr;
    /* Biquad coefficients (normalized, a0=1) */
    float b0, b1, b2;
    float a1, a2;
    /* Biquad state */
    float x1, x2;
    float y1, y2;
} ifx_filter_t;

void ifx_filter_init(ifx_filter_t *f, int sr,
                      float mode, float cutoff, float q, float mix);
void ifx_filter_process(ifx_filter_t *f,
                         const float *in, float *out, int samples);
void ifx_filter_reset(ifx_filter_t *f);
void ifx_filter_set_param(ifx_filter_t *f, const char *name, float value);

#endif /* INSERT_FX_H */
