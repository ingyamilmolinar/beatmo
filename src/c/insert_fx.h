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

/* ---- Waveshaper (nonlinear curve + drive) ---- */

typedef struct {
    int   curve;  /* 0=tanh, 1=clip, 2=fold, 3=sine */
    float drive;  /* 1-20 */
    float mix;    /* wet/dry 0-1 */
} ifx_waveshaper_t;

void ifx_waveshaper_init(ifx_waveshaper_t *w, float curve, float drive, float mix);
void ifx_waveshaper_process(ifx_waveshaper_t *w,
                             const float *in, float *out, int samples);
void ifx_waveshaper_reset(ifx_waveshaper_t *w);
void ifx_waveshaper_set_param(ifx_waveshaper_t *w,
                               const char *name, float value);

/* ---- Ring Modulator (carrier * input) ---- */

typedef struct {
    float frequency; /* carrier Hz */
    float shape;     /* 0=sine, 1=square */
    float mix;       /* wet/dry 0-1 */
    int   sr;
    float phase;     /* 0..2π */
    float phase_inc; /* per-sample increment */
} ifx_ringmod_t;

void ifx_ringmod_init(ifx_ringmod_t *r, int sr,
                       float frequency, float shape, float mix);
void ifx_ringmod_process(ifx_ringmod_t *r,
                          const float *in, float *out, int samples);
void ifx_ringmod_reset(ifx_ringmod_t *r);
void ifx_ringmod_set_param(ifx_ringmod_t *r,
                            const char *name, float value);

/* ---- Tremolo (LFO amplitude modulation) ---- */

typedef struct {
    float rate;      /* LFO Hz */
    float depth;     /* modulation depth 0-1 */
    int   shape;     /* 0=sine, 1=triangle, 2=square */
    float mix;       /* wet/dry 0-1 */
    int   sr;
    float phase;     /* 0..2π */
    float phase_inc; /* per-sample increment */
} ifx_tremolo_t;

void ifx_tremolo_init(ifx_tremolo_t *t, int sr,
                       float rate, float depth, float shape, float mix);
void ifx_tremolo_process(ifx_tremolo_t *t,
                          const float *in, float *out, int samples);
void ifx_tremolo_reset(ifx_tremolo_t *t);
void ifx_tremolo_set_param(ifx_tremolo_t *t,
                            const char *name, float value);

/* ---- Gate (noise gate with envelope follower) ---- */

typedef struct {
    float threshold_lin; /* linear threshold */
    float attack_coef;   /* envelope attack coefficient */
    float release_coef;  /* envelope release coefficient */
    float range_lin;     /* attenuation floor (linear) */
    int   sr;
    float envelope;      /* current envelope value */
} ifx_gate_t;

void ifx_gate_init(ifx_gate_t *g, int sr,
                    float threshold_db, float attack_ms,
                    float release_ms, float range_db);
void ifx_gate_process(ifx_gate_t *g,
                       const float *in, float *out, int samples);
void ifx_gate_reset(ifx_gate_t *g);
void ifx_gate_set_param(ifx_gate_t *g,
                         const char *name, float value);

/* ---- Limiter (brick-wall) ---- */

typedef struct {
    float threshold_lin; /* linear threshold */
    float release_coef;  /* envelope release coefficient */
    float ceiling_lin;   /* output ceiling (linear) */
    int   sr;
    float envelope;      /* current envelope */
} ifx_limiter_t;

void ifx_limiter_init(ifx_limiter_t *l, int sr,
                       float threshold_db, float release_ms, float ceiling_db);
void ifx_limiter_process(ifx_limiter_t *l,
                          const float *in, float *out, int samples);
void ifx_limiter_reset(ifx_limiter_t *l);
void ifx_limiter_set_param(ifx_limiter_t *l,
                            const char *name, float value);

/* ---- Flanger (short modulated delay + feedback) ---- */

typedef struct {
    float  rate;          /* LFO Hz */
    float  depth;         /* depth in ms */
    float  feedback;      /* -0.95..0.95 */
    float  mix;           /* wet/dry 0-1 */
    int    sr;
    float *buf;           /* circular buffer (caller-owned) */
    int    buf_len;
    int    pos;
    float  phase;         /* LFO phase 0..2π */
    float  phase_inc;     /* per-sample phase increment */
    float  depth_samples; /* depth in samples */
    float  last_read;     /* feedback sample */
} ifx_flanger_t;

void ifx_flanger_init(ifx_flanger_t *f, int sr, float *buf, int buf_len,
                       float rate, float depth, float feedback, float mix);
void ifx_flanger_process(ifx_flanger_t *f,
                          const float *in, float *out, int samples);
void ifx_flanger_reset(ifx_flanger_t *f);
void ifx_flanger_set_param(ifx_flanger_t *f,
                            const char *name, float value);

/* ---- Phaser (allpass chain + LFO) ---- */

typedef struct {
    int   stages;     /* 2-12 */
    float rate;       /* LFO Hz */
    float depth;      /* sweep depth 0-1 */
    float feedback;   /* 0-0.95 */
    float mix;        /* wet/dry 0-1 */
    int   sr;
    float phase;      /* LFO phase 0..2π */
    float x1[12];     /* allpass input state */
    float y1[12];     /* allpass output state */
    float fb_sample;  /* feedback from last stage */
} ifx_phaser_t;

void ifx_phaser_init(ifx_phaser_t *p, int sr,
                      float stages, float rate, float depth,
                      float feedback, float mix);
void ifx_phaser_process(ifx_phaser_t *p,
                         const float *in, float *out, int samples);
void ifx_phaser_reset(ifx_phaser_t *p);
void ifx_phaser_set_param(ifx_phaser_t *p,
                           const char *name, float value);

/* ---- Auto-Wah (envelope follower + LFO → bandpass) ---- */

typedef struct {
    float sensitivity; /* envelope sensitivity 0-1 */
    float rate;        /* LFO Hz */
    float depth;       /* sweep depth 0-1 */
    float mix;         /* wet/dry 0-1 */
    int   sr;
    float env;         /* envelope follower state */
    float lfo_phase;   /* LFO phase 0..2π */
    float lfo_inc;     /* per-sample LFO increment */
    /* State-variable filter state */
    float bp;          /* bandpass output */
    float lp;          /* lowpass output */
} ifx_autowah_t;

void ifx_autowah_init(ifx_autowah_t *a, int sr,
                       float sensitivity, float rate,
                       float depth, float mix);
void ifx_autowah_process(ifx_autowah_t *a,
                          const float *in, float *out, int samples);
void ifx_autowah_reset(ifx_autowah_t *a);
void ifx_autowah_set_param(ifx_autowah_t *a,
                            const char *name, float value);

/* ---- Compressor (per-instrument dynamics) ---- */

typedef struct {
    float threshold_db;
    float threshold_lin;
    float ratio;
    float attack_coef;
    float release_coef;
    float makeup_db;
    float makeup_lin;
    float mix;
    int   sr;
    float envelope;
} ifx_compressor_t;

void ifx_compressor_init(ifx_compressor_t *c, int sr,
                          float threshold_db, float ratio,
                          float attack_ms, float release_ms,
                          float makeup_db, float mix);
void ifx_compressor_process(ifx_compressor_t *c,
                             const float *in, float *out, int samples);
void ifx_compressor_reset(ifx_compressor_t *c);
void ifx_compressor_set_param(ifx_compressor_t *c,
                               const char *name, float value);

/* ---- Transient Shaper (attack/sustain control) ---- */

typedef struct {
    float attack_gain;    /* 0-2 (100% = 1.0) */
    float sustain_gain;   /* 0-2 */
    float speed_ms;       /* detection speed ms */
    int   sr;
    float fast_env;       /* fast envelope follower */
    float slow_env;       /* slow envelope follower */
    float fast_coef;      /* fast attack coefficient */
    float fast_rel_coef;  /* fast release coefficient */
    float slow_coef;      /* slow coefficient (~100ms) */
} ifx_transient_t;

void ifx_transient_init(ifx_transient_t *t, int sr,
                         float attack_pct, float sustain_pct, float speed_ms);
void ifx_transient_process(ifx_transient_t *t,
                            const float *in, float *out, int samples);
void ifx_transient_reset(ifx_transient_t *t);
void ifx_transient_set_param(ifx_transient_t *t,
                              const char *name, float value);

/* ---- Tape Saturation (warm saturation + wow/flutter) ---- */

typedef struct {
    float  drive;      /* 1-10 */
    float  warmth;     /* 0-1 (LP filter amount) */
    float  wow;        /* 0-1 (slow pitch modulation) */
    float  flutter;    /* 0-1 (fast pitch modulation) */
    float  mix;        /* wet/dry 0-1 */
    int    sr;
    float  lp_y1;      /* warmth LP filter state */
    float  lp_a;       /* LP coefficient */
    float *buf;        /* circular buffer for wow/flutter (caller-owned) */
    int    buf_len;
    int    pos;
    float  wow_phase;
    float  flutter_phase;
    float  wow_inc;     /* per-sample phase increment */
    float  flutter_inc;
} ifx_tape_t;

void ifx_tape_init(ifx_tape_t *t, int sr, float *buf, int buf_len,
                    float drive, float warmth, float wow,
                    float flutter, float mix);
void ifx_tape_process(ifx_tape_t *t,
                       const float *in, float *out, int samples);
void ifx_tape_reset(ifx_tape_t *t);
void ifx_tape_set_param(ifx_tape_t *t,
                         const char *name, float value);

/* ---- Pitch Shifter (dual delay-line) ---- */

typedef struct {
    float  pitch;          /* semitones -24..+24 */
    float  mix;            /* wet/dry 0-1 */
    float  window_ms;      /* grain window in ms */
    int    sr;
    float *buf;            /* circular buffer (caller-owned) */
    int    buf_len;
    int    write_pos;
    float  head_a;         /* read head A position within window */
    float  head_b;         /* read head B position within window */
    float  window_samples; /* window size in samples */
    float  step;           /* per-sample head drift */
} ifx_pitchshift_t;

void ifx_pitchshift_init(ifx_pitchshift_t *p, int sr, float *buf, int buf_len,
                          float pitch, float mix, float window_ms);
void ifx_pitchshift_process(ifx_pitchshift_t *p,
                             const float *in, float *out, int samples);
void ifx_pitchshift_reset(ifx_pitchshift_t *p);
void ifx_pitchshift_set_param(ifx_pitchshift_t *p,
                               const char *name, float value);

#endif /* INSERT_FX_H */
