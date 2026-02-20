#include <math.h>
#include <string.h>
#include "insert_fx.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* ── Helpers ─────────────────────────────────────────────────────────────── */

static inline float clampf(float v, float lo, float hi) {
    if (v < lo) return lo;
    if (v > hi) return hi;
    return v;
}

/* ==== Distortion ==== */

static void distortion_recalc_lp(ifx_distortion_t *d) {
    float fc = d->tone;
    if (fc <= 0) fc = 4000;
    float sr = (float)d->sr;
    if (sr <= 0) sr = 44100;
    d->lp_a = expf(-2.0f * (float)M_PI * fc / sr);
}

EXPORT void ifx_distortion_init(ifx_distortion_t *d, int sr,
                                 float drive, float tone, float mix) {
    d->sr    = sr;
    d->drive = clampf(drive, 1, 20);
    d->tone  = clampf(tone, 200, 8000);
    d->mix   = clampf(mix, 0, 1);
    d->lp_y1 = 0;
    distortion_recalc_lp(d);
}

EXPORT void ifx_distortion_process(ifx_distortion_t *d,
                                    const float *in, float *out, int samples) {
    float drive = d->drive;
    float mix   = d->mix;
    float lp_a  = d->lp_a;
    float lp_y1 = d->lp_y1;

    for (int i = 0; i < samples; i++) {
        float x   = in[i];
        float wet = tanhf(drive * x);
        /* One-pole LP tone filter on wet signal */
        lp_y1 = wet * (1 - lp_a) + lp_y1 * lp_a;
        wet   = lp_y1;
        out[i] = x * (1 - mix) + wet * mix;
    }
    d->lp_y1 = lp_y1;
}

EXPORT void ifx_distortion_reset(ifx_distortion_t *d) {
    d->lp_y1 = 0;
}

EXPORT void ifx_distortion_set_param(ifx_distortion_t *d,
                                      const char *name, float value) {
    if (strcmp(name, "drive") == 0) {
        d->drive = clampf(value, 1, 20);
    } else if (strcmp(name, "tone") == 0) {
        d->tone = clampf(value, 200, 8000);
        distortion_recalc_lp(d);
    } else if (strcmp(name, "mix") == 0) {
        d->mix = clampf(value, 0, 1);
    }
}

/* ==== Delay ==== */

static void delay_recalc(ifx_delay_t *d) {
    float sr = (float)d->sr;
    if (sr <= 0) sr = 44100;
    d->delay_samples = (int)(d->time_ms * 0.001f * sr + 0.5f);
    if (d->delay_samples < 1) d->delay_samples = 1;
    if (d->delay_samples >= d->buf_len) d->delay_samples = d->buf_len - 1;
    /* LP for feedback darkening: cutoff at 3 kHz */
    d->lp_a = expf(-2.0f * (float)M_PI * 3000.0f / sr);
}

EXPORT void ifx_delay_init(ifx_delay_t *d, int sr, float *buf, int buf_len,
                            float time_ms, float feedback, float mix) {
    d->sr       = sr;
    d->buf      = buf;
    d->buf_len  = buf_len;
    d->pos      = 0;
    d->time_ms  = clampf(time_ms, 10, 1000);
    d->feedback = clampf(feedback, 0, 0.95f);
    d->mix      = clampf(mix, 0, 1);
    d->lp_y1    = 0;
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    delay_recalc(d);
}

EXPORT void ifx_delay_process(ifx_delay_t *d,
                               const float *in, float *out, int samples) {
    int   buf_len   = d->buf_len;
    int   dly       = d->delay_samples;
    float feedback  = d->feedback;
    float mix       = d->mix;
    float lp_a      = d->lp_a;
    float lp_y1     = d->lp_y1;
    int   pos       = d->pos;
    float *buf      = d->buf;

    if (!buf || buf_len <= 0) {
        /* Passthrough */
        if (in != out) memcpy(out, in, (size_t)samples * sizeof(float));
        return;
    }

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        /* Read from delay line */
        int read_pos = pos - dly;
        if (read_pos < 0) read_pos += buf_len;
        float delayed = buf[read_pos];

        /* LP filter on feedback */
        lp_y1 = delayed * (1 - lp_a) + lp_y1 * lp_a;
        float filtered = lp_y1;

        /* Write input + filtered feedback to buffer */
        buf[pos] = x + filtered * feedback;

        pos++;
        if (pos >= buf_len) pos = 0;

        out[i] = x * (1 - mix) + delayed * mix;
    }
    d->pos   = pos;
    d->lp_y1 = lp_y1;
}

EXPORT void ifx_delay_reset(ifx_delay_t *d) {
    if (d->buf && d->buf_len > 0) {
        memset(d->buf, 0, (size_t)d->buf_len * sizeof(float));
    }
    d->pos   = 0;
    d->lp_y1 = 0;
}

EXPORT void ifx_delay_set_param(ifx_delay_t *d, const char *name, float value) {
    if (strcmp(name, "time") == 0) {
        d->time_ms = clampf(value, 10, 1000);
        delay_recalc(d);
    } else if (strcmp(name, "feedback") == 0) {
        d->feedback = clampf(value, 0, 0.95f);
    } else if (strcmp(name, "mix") == 0) {
        d->mix = clampf(value, 0, 1);
    }
}

/* ==== Reverb ==== */

/* Comb delay lengths in samples at 44100 Hz (Schroeder classic). */
static const int COMB_DELAYS[4]    = { 1116, 1188, 1277, 1356 };
static const int AP_DELAYS[2]      = { 556, 441 };

static int scale_delay(int base, int sr) {
    int s = (int)((double)base * (double)sr / 44100.0 + 0.5);
    return s > 0 ? s : 1;
}

EXPORT int ifx_reverb_mem_size(int sr) {
    int total = 0;
    for (int i = 0; i < 4; i++) total += scale_delay(COMB_DELAYS[i], sr);
    for (int i = 0; i < 2; i++) total += scale_delay(AP_DELAYS[i], sr);
    return total;
}

EXPORT void ifx_reverb_init(ifx_reverb_t *r, int sr, float *mem,
                              float room, float damping, float mix) {
    r->sr      = sr;
    r->room    = clampf(room, 0, 1);
    r->damping = clampf(damping, 0, 1);
    r->mix     = clampf(mix, 0, 1);
    r->mem     = mem;

    float *ptr = mem;
    float fb = 0.7f + 0.28f * r->room;
    for (int i = 0; i < 4; i++) {
        int len = scale_delay(COMB_DELAYS[i], sr);
        r->combs[i].buf      = ptr;
        r->combs[i].length   = len;
        r->combs[i].pos      = 0;
        r->combs[i].feedback = fb;
        r->combs[i].damping  = r->damping;
        r->combs[i].lp_state = 0;
        memset(ptr, 0, (size_t)len * sizeof(float));
        ptr += len;
    }
    for (int i = 0; i < 2; i++) {
        int len = scale_delay(AP_DELAYS[i], sr);
        r->aps[i].buf    = ptr;
        r->aps[i].length = len;
        r->aps[i].pos    = 0;
        r->aps[i].g      = 0.5f;
        memset(ptr, 0, (size_t)len * sizeof(float));
        ptr += len;
    }
}

static inline float comb_tick(ifx_comb_t *c, float input) {
    float output = c->buf[c->pos];
    /* LP damped feedback */
    c->lp_state = output * (1 - c->damping) + c->lp_state * c->damping;
    c->buf[c->pos] = input + c->lp_state * c->feedback;
    c->pos++;
    if (c->pos >= c->length) c->pos = 0;
    return output;
}

static inline float allpass_tick(ifx_allpass_t *a, float input) {
    float delayed = a->buf[a->pos];
    a->buf[a->pos] = input + delayed * a->g;
    a->pos++;
    if (a->pos >= a->length) a->pos = 0;
    return delayed - input * a->g;
}

EXPORT void ifx_reverb_process(ifx_reverb_t *r,
                                const float *in, float *out, int samples) {
    float mix = r->mix;
    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float wet = 0;
        for (int c = 0; c < 4; c++) {
            wet += comb_tick(&r->combs[c], x);
        }
        wet *= 0.25f;
        for (int a = 0; a < 2; a++) {
            wet = allpass_tick(&r->aps[a], wet);
        }
        out[i] = x * (1 - mix) + wet * mix;
    }
}

EXPORT void ifx_reverb_reset(ifx_reverb_t *r) {
    for (int i = 0; i < 4; i++) {
        if (r->combs[i].buf && r->combs[i].length > 0) {
            memset(r->combs[i].buf, 0, (size_t)r->combs[i].length * sizeof(float));
        }
        r->combs[i].pos      = 0;
        r->combs[i].lp_state = 0;
    }
    for (int i = 0; i < 2; i++) {
        if (r->aps[i].buf && r->aps[i].length > 0) {
            memset(r->aps[i].buf, 0, (size_t)r->aps[i].length * sizeof(float));
        }
        r->aps[i].pos = 0;
    }
}

EXPORT void ifx_reverb_set_param(ifx_reverb_t *r,
                                  const char *name, float value) {
    if (strcmp(name, "room") == 0) {
        r->room = clampf(value, 0, 1);
        float fb = 0.7f + 0.28f * r->room;
        for (int i = 0; i < 4; i++) {
            r->combs[i].feedback = fb;
        }
    } else if (strcmp(name, "damping") == 0) {
        r->damping = clampf(value, 0, 1);
        for (int i = 0; i < 4; i++) {
            r->combs[i].damping = r->damping;
        }
    } else if (strcmp(name, "mix") == 0) {
        r->mix = clampf(value, 0, 1);
    }
}

/* ==== Chorus ==== */

static void chorus_recalc(ifx_chorus_t *c) {
    float sr = (float)c->sr;
    if (sr <= 0) sr = 44100;
    c->depth_samples = c->depth * 0.001f * sr;
    c->phase_inc = 2.0f * (float)M_PI * c->rate / sr;
}

EXPORT void ifx_chorus_init(ifx_chorus_t *c, int sr, float *buf, int buf_len,
                              float rate, float depth, float mix) {
    c->sr      = sr;
    c->buf     = buf;
    c->buf_len = buf_len;
    c->pos     = 0;
    c->phase   = 0;
    c->rate    = clampf(rate, 0.1f, 10);
    c->depth   = clampf(depth, 0, 20);
    c->mix     = clampf(mix, 0, 1);
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    chorus_recalc(c);
}

EXPORT void ifx_chorus_process(ifx_chorus_t *c,
                                const float *in, float *out, int samples) {
    int    buf_len       = c->buf_len;
    float  mix           = c->mix;
    float  depth_samples = c->depth_samples;
    float  phase_inc     = c->phase_inc;
    float  phase         = c->phase;
    int    pos           = c->pos;
    float *buf           = c->buf;

    if (!buf || buf_len <= 0) {
        if (in != out) memcpy(out, in, (size_t)samples * sizeof(float));
        return;
    }

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        buf[pos] = x;

        /* LFO: sine 0..1 range */
        float lfo = (sinf(phase) + 1.0f) * 0.5f;
        float delay_f = 1.0f + lfo * depth_samples;

        /* Read with linear interpolation */
        float read_f = (float)pos - delay_f;
        if (read_f < 0) read_f += (float)buf_len;
        int idx0 = (int)read_f;
        if (idx0 < 0) idx0 = 0;
        if (idx0 >= buf_len) idx0 = buf_len - 1;
        int idx1 = (idx0 + 1);
        if (idx1 >= buf_len) idx1 = 0;
        float frac = read_f - floorf(read_f);
        float wet = buf[idx0] * (1 - frac) + buf[idx1] * frac;

        phase += phase_inc;
        if (phase >= 2.0f * (float)M_PI) phase -= 2.0f * (float)M_PI;

        pos++;
        if (pos >= buf_len) pos = 0;

        out[i] = x * (1 - mix) + wet * mix;
    }
    c->phase = phase;
    c->pos   = pos;
}

EXPORT void ifx_chorus_reset(ifx_chorus_t *c) {
    if (c->buf && c->buf_len > 0) {
        memset(c->buf, 0, (size_t)c->buf_len * sizeof(float));
    }
    c->pos   = 0;
    c->phase = 0;
}

EXPORT void ifx_chorus_set_param(ifx_chorus_t *c,
                                  const char *name, float value) {
    if (strcmp(name, "rate") == 0) {
        c->rate = clampf(value, 0.1f, 10);
        chorus_recalc(c);
    } else if (strcmp(name, "depth") == 0) {
        c->depth = clampf(value, 0, 20);
        chorus_recalc(c);
    } else if (strcmp(name, "mix") == 0) {
        c->mix = clampf(value, 0, 1);
    }
}

/* ==== Bitcrusher ==== */

EXPORT void ifx_bitcrusher_init(ifx_bitcrusher_t *b,
                                 float bits, float rate, float mix) {
    b->bits         = clampf(bits, 2, 16);
    b->rate         = clampf(rate, 0.01f, 1);
    b->mix          = clampf(mix, 0, 1);
    b->hold_counter = 0;
    b->hold_value   = 0;
}

EXPORT void ifx_bitcrusher_process(ifx_bitcrusher_t *b,
                                    const float *in, float *out, int samples) {
    float bits_f      = b->bits;
    float rate        = b->rate;
    float mix         = b->mix;
    float hold_counter = b->hold_counter;
    float hold_value   = b->hold_value;
    float levels       = powf(2.0f, bits_f);

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        hold_counter += rate;
        if (hold_counter >= 1.0f) {
            hold_counter -= 1.0f;
            hold_value = roundf(x * levels) / levels;
        }
        out[i] = x * (1 - mix) + hold_value * mix;
    }
    b->hold_counter = hold_counter;
    b->hold_value   = hold_value;
}

EXPORT void ifx_bitcrusher_reset(ifx_bitcrusher_t *b) {
    b->hold_counter = 0;
    b->hold_value   = 0;
}

EXPORT void ifx_bitcrusher_set_param(ifx_bitcrusher_t *b,
                                      const char *name, float value) {
    if (strcmp(name, "bits") == 0) {
        b->bits = clampf(value, 2, 16);
    } else if (strcmp(name, "rate") == 0) {
        b->rate = clampf(value, 0.01f, 1);
    } else if (strcmp(name, "mix") == 0) {
        b->mix = clampf(value, 0, 1);
    }
}

/* ==== Filter (biquad LP/HP/BP) ==== */

/* Compute biquad coefficients using RBJ Audio EQ Cookbook formulas. */
static void filter_compute_coeffs(ifx_filter_t *f) {
    float sr = (float)f->sr;
    if (sr <= 0) sr = 44100;
    float freq = f->cutoff;
    if (freq <= 0) freq = 1000;
    if (freq > sr * 0.5f) freq = sr * 0.5f;
    float q = f->q;
    if (q <= 0) q = 0.707f;

    float w0   = 2.0f * (float)M_PI * freq / sr;
    float cosw = cosf(w0);
    float sinw = sinf(w0);
    float alpha = sinw / (2.0f * q);

    float b0, b1, b2, a0, a1, a2;
    int mode = (int)(f->mode + 0.5f);

    switch (mode) {
    case 1: /* Highpass */
        b0 = (1 + cosw) / 2;
        b1 = -(1 + cosw);
        b2 = (1 + cosw) / 2;
        a0 = 1 + alpha;
        a1 = -2 * cosw;
        a2 = 1 - alpha;
        break;
    case 2: /* Bandpass */
        b0 = alpha;
        b1 = 0;
        b2 = -alpha;
        a0 = 1 + alpha;
        a1 = -2 * cosw;
        a2 = 1 - alpha;
        break;
    default: /* Lowpass (mode 0) */
        b0 = (1 - cosw) / 2;
        b1 = 1 - cosw;
        b2 = (1 - cosw) / 2;
        a0 = 1 + alpha;
        a1 = -2 * cosw;
        a2 = 1 - alpha;
        break;
    }

    if (a0 != 0) {
        f->b0 = b0 / a0;
        f->b1 = b1 / a0;
        f->b2 = b2 / a0;
        f->a1 = a1 / a0;
        f->a2 = a2 / a0;
    }
}

EXPORT void ifx_filter_init(ifx_filter_t *f, int sr,
                              float mode, float cutoff, float q, float mix) {
    f->sr     = sr;
    f->mode   = clampf(mode, 0, 2);
    f->cutoff = clampf(cutoff, 20, 20000);
    f->q      = clampf(q, 0.1f, 10);
    f->mix    = clampf(mix, 0, 1);
    f->x1 = f->x2 = f->y1 = f->y2 = 0;
    filter_compute_coeffs(f);
}

EXPORT void ifx_filter_process(ifx_filter_t *f,
                                const float *in, float *out, int samples) {
    float b0  = f->b0, b1 = f->b1, b2 = f->b2;
    float a1  = f->a1, a2 = f->a2;
    float x1  = f->x1, x2 = f->x2;
    float y1  = f->y1, y2 = f->y2;
    float mix = f->mix;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float y = b0 * x + b1 * x1 + b2 * x2 - a1 * y1 - a2 * y2;
        /* Flush denormals */
        if (y > -1e-20f && y < 1e-20f) y = 0;
        x2 = x1; x1 = x;
        y2 = y1; y1 = y;
        out[i] = x * (1 - mix) + y * mix;
    }
    f->x1 = x1; f->x2 = x2;
    f->y1 = y1; f->y2 = y2;
}

EXPORT void ifx_filter_reset(ifx_filter_t *f) {
    f->x1 = f->x2 = f->y1 = f->y2 = 0;
}

EXPORT void ifx_filter_set_param(ifx_filter_t *f,
                                  const char *name, float value) {
    if (strcmp(name, "mode") == 0) {
        f->mode = clampf(value, 0, 2);
        filter_compute_coeffs(f);
    } else if (strcmp(name, "cutoff") == 0) {
        f->cutoff = clampf(value, 20, 20000);
        filter_compute_coeffs(f);
    } else if (strcmp(name, "q") == 0) {
        f->q = clampf(value, 0.1f, 10);
        filter_compute_coeffs(f);
    } else if (strcmp(name, "mix") == 0) {
        f->mix = clampf(value, 0, 1);
    }
}
