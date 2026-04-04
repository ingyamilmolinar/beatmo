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

/* One-pole smoothing coefficient for the given sample rate. */
static inline float ifx_smooth_coeff(int sr) {
    if (sr <= 0) sr = 44100;
    float samples = (float)sr * IFX_SMOOTH_TIME_MS * 0.001f;
    return 1.0f - expf(-1.0f / samples);
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
    d->drive = d->drive_tgt = clampf(drive, 1, 20);
    d->tone  = d->tone_tgt  = clampf(tone, 200, 8000);
    d->mix   = d->mix_tgt   = clampf(mix, 0, 1);
    d->smooth_coeff = ifx_smooth_coeff(sr);
    d->lp_y1 = 0;
    distortion_recalc_lp(d);
}

EXPORT void ifx_distortion_process(ifx_distortion_t *d,
                                    const float *in, float *out, int samples) {
    float drive = d->drive;
    float tone  = d->tone;
    float mix   = d->mix;
    float lp_y1 = d->lp_y1;
    float coeff = d->smooth_coeff;
    float sr_f  = (float)d->sr;
    if (sr_f <= 0) sr_f = 44100;

    for (int i = 0; i < samples; i++) {
        /* Tick smoothers */
        drive += (d->drive_tgt - drive) * coeff;
        tone  += (d->tone_tgt  - tone)  * coeff;
        mix   += (d->mix_tgt   - mix)   * coeff;
        float lp_a = expf(-2.0f * (float)M_PI * tone / sr_f);

        float x   = in[i];
        float wet = tanhf(drive * x);
        /* One-pole LP tone filter on wet signal */
        lp_y1 = wet * (1 - lp_a) + lp_y1 * lp_a;
        wet   = lp_y1;
        out[i] = x * (1 - mix) + wet * mix;
    }
    d->drive = drive;
    d->tone  = tone;
    d->mix   = mix;
    d->lp_y1 = lp_y1;
}

EXPORT void ifx_distortion_reset(ifx_distortion_t *d) {
    d->lp_y1 = 0;
}

EXPORT void ifx_distortion_set_param(ifx_distortion_t *d,
                                      const char *name, float value) {
    if (strcmp(name, "drive") == 0) {
        d->drive_tgt = clampf(value, 1, 20);
    } else if (strcmp(name, "tone") == 0) {
        d->tone_tgt = clampf(value, 200, 8000);
    } else if (strcmp(name, "mix") == 0) {
        d->mix_tgt = clampf(value, 0, 1);
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
    /* Crossfade duration: ~5ms */
    d->xfade_len = (int)(sr * IFX_SMOOTH_TIME_MS * 0.001f + 0.5f);
    if (d->xfade_len < 1) d->xfade_len = 1;
}

EXPORT void ifx_delay_init(ifx_delay_t *d, int sr, float *buf, int buf_len,
                            float time_ms, float feedback, float mix) {
    d->sr       = sr;
    d->buf      = buf;
    d->buf_len  = buf_len;
    d->pos      = 0;
    d->time_ms  = clampf(time_ms, 10, 1000);
    d->feedback = d->feedback_tgt = clampf(feedback, 0, 0.95f);
    d->mix      = d->mix_tgt      = clampf(mix, 0, 1);
    d->smooth_coeff = ifx_smooth_coeff(sr);
    d->old_delay_samples = 0;
    d->xfade_pos = 0;
    d->lp_y1    = 0;
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    delay_recalc(d);
}

static inline float delay_read_at(ifx_delay_t *d, int pos, int dly_samples) {
    int read_pos = pos - dly_samples;
    if (read_pos < 0) read_pos += d->buf_len;
    if (read_pos >= d->buf_len) read_pos = 0;
    return d->buf[read_pos];
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
    float coeff     = d->smooth_coeff;

    if (!buf || buf_len <= 0) {
        /* Passthrough */
        if (in != out) memcpy(out, in, (size_t)samples * sizeof(float));
        return;
    }

    for (int i = 0; i < samples; i++) {
        /* Tick smoothers */
        feedback += (d->feedback_tgt - feedback) * coeff;
        mix      += (d->mix_tgt      - mix)      * coeff;

        float x = in[i];

        /* Read from delay line, crossfading if time was recently changed */
        float delayed;
        if (d->xfade_pos < d->xfade_len && d->old_delay_samples > 0) {
            float t = (float)d->xfade_pos / (float)d->xfade_len;
            float old_val = delay_read_at(d, pos, d->old_delay_samples);
            float new_val = delay_read_at(d, pos, dly);
            delayed = old_val * (1 - t) + new_val * t;
            d->xfade_pos++;
            if (d->xfade_pos >= d->xfade_len) {
                d->old_delay_samples = 0;
            }
        } else {
            int read_pos = pos - dly;
            if (read_pos < 0) read_pos += buf_len;
            delayed = buf[read_pos];
        }

        /* LP filter on feedback */
        lp_y1 = delayed * (1 - lp_a) + lp_y1 * lp_a;
        float filtered = lp_y1;

        /* Write input + filtered feedback to buffer */
        buf[pos] = x + filtered * feedback;

        pos++;
        if (pos >= buf_len) pos = 0;

        out[i] = x * (1 - mix) + delayed * mix;
    }
    d->pos      = pos;
    d->lp_y1    = lp_y1;
    d->feedback = feedback;
    d->mix      = mix;
}

EXPORT void ifx_delay_reset(ifx_delay_t *d) {
    if (d->buf && d->buf_len > 0) {
        memset(d->buf, 0, (size_t)d->buf_len * sizeof(float));
    }
    d->pos   = 0;
    d->lp_y1 = 0;
    d->old_delay_samples = 0;
    d->xfade_pos = 0;
}

EXPORT void ifx_delay_set_param(ifx_delay_t *d, const char *name, float value) {
    if (strcmp(name, "time") == 0) {
        float new_time = clampf(value, 10, 1000);
        float sr = (float)d->sr;
        if (sr <= 0) sr = 44100;
        int new_dly = (int)(new_time * 0.001f * sr + 0.5f);
        if (new_dly < 1) new_dly = 1;
        if (new_dly >= d->buf_len) new_dly = d->buf_len - 1;
        if (new_dly != d->delay_samples) {
            d->old_delay_samples = d->delay_samples;
            d->delay_samples = new_dly;
            d->xfade_pos = 0;
        }
        d->time_ms = new_time;
    } else if (strcmp(name, "feedback") == 0) {
        d->feedback_tgt = clampf(value, 0, 0.95f);
    } else if (strcmp(name, "mix") == 0) {
        d->mix_tgt = clampf(value, 0, 1);
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
    r->room    = r->room_tgt    = clampf(room, 0, 1);
    r->damping = r->damping_tgt = clampf(damping, 0, 1);
    r->mix     = r->mix_tgt     = clampf(mix, 0, 1);
    r->smooth_coeff = ifx_smooth_coeff(sr);
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
    float room    = r->room;
    float damping = r->damping;
    float mix     = r->mix;
    float coeff   = r->smooth_coeff;

    for (int i = 0; i < samples; i++) {
        /* Tick smoothers */
        room    += (r->room_tgt    - room)    * coeff;
        damping += (r->damping_tgt - damping) * coeff;
        mix     += (r->mix_tgt     - mix)     * coeff;

        /* Update comb parameters per sample */
        float fb = 0.7f + 0.28f * room;
        for (int c = 0; c < 4; c++) {
            r->combs[c].feedback = fb;
            r->combs[c].damping  = damping;
        }

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
    r->room    = room;
    r->damping = damping;
    r->mix     = mix;
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
        r->room_tgt = clampf(value, 0, 1);
    } else if (strcmp(name, "damping") == 0) {
        r->damping_tgt = clampf(value, 0, 1);
    } else if (strcmp(name, "mix") == 0) {
        r->mix_tgt = clampf(value, 0, 1);
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
    c->rate    = c->rate_tgt  = clampf(rate, 0.1f, 10);
    c->depth   = c->depth_tgt = clampf(depth, 0, 20);
    c->mix     = c->mix_tgt   = clampf(mix, 0, 1);
    c->smooth_coeff = ifx_smooth_coeff(sr);
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    chorus_recalc(c);
}

EXPORT void ifx_chorus_process(ifx_chorus_t *c,
                                const float *in, float *out, int samples) {
    int    buf_len = c->buf_len;
    float  rate    = c->rate;
    float  depth   = c->depth;
    float  mix     = c->mix;
    float  phase   = c->phase;
    int    pos     = c->pos;
    float *buf     = c->buf;
    float  coeff   = c->smooth_coeff;
    float  sr_f    = (float)c->sr;
    if (sr_f <= 0) sr_f = 44100;

    if (!buf || buf_len <= 0) {
        if (in != out) memcpy(out, in, (size_t)samples * sizeof(float));
        return;
    }

    for (int i = 0; i < samples; i++) {
        /* Tick smoothers */
        rate  += (c->rate_tgt  - rate)  * coeff;
        depth += (c->depth_tgt - depth) * coeff;
        mix   += (c->mix_tgt   - mix)   * coeff;

        float depth_samples = depth * 0.001f * sr_f;
        float phase_inc = 2.0f * (float)M_PI * rate / sr_f;

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
    c->rate  = rate;
    c->depth = depth;
    c->mix   = mix;
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
        c->rate_tgt = clampf(value, 0.1f, 10);
    } else if (strcmp(name, "depth") == 0) {
        c->depth_tgt = clampf(value, 0, 20);
    } else if (strcmp(name, "mix") == 0) {
        c->mix_tgt = clampf(value, 0, 1);
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

/* ==== Waveshaper ==== */

EXPORT void ifx_waveshaper_init(ifx_waveshaper_t *w,
                                 float curve, float drive, float mix) {
    w->curve = (int)(clampf(curve, 0, 3) + 0.5f);
    w->drive = clampf(drive, 1, 20);
    w->mix   = clampf(mix, 0, 1);
}

EXPORT void ifx_waveshaper_process(ifx_waveshaper_t *w,
                                    const float *in, float *out, int samples) {
    int   curve = w->curve;
    float drive = w->drive;
    float mix   = w->mix;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float driven = x * drive;
        float wet;
        switch (curve) {
        case 1: /* hard clip */
            wet = clampf(driven, -1, 1);
            break;
        case 2: /* fold-back */
            while (driven > 1.0f || driven < -1.0f) {
                if (driven > 1.0f)  driven = 2.0f - driven;
                if (driven < -1.0f) driven = -2.0f - driven;
            }
            wet = driven;
            break;
        case 3: /* sine fold */
            wet = sinf(driven * (float)M_PI * 0.5f);
            break;
        default: /* tanh */
            wet = tanhf(driven);
            break;
        }
        out[i] = x * (1 - mix) + wet * mix;
    }
}

EXPORT void ifx_waveshaper_reset(ifx_waveshaper_t *w) {
    (void)w; /* stateless */
}

EXPORT void ifx_waveshaper_set_param(ifx_waveshaper_t *w,
                                      const char *name, float value) {
    if (strcmp(name, "curve") == 0) {
        w->curve = (int)(clampf(value, 0, 3) + 0.5f);
    } else if (strcmp(name, "drive") == 0) {
        w->drive = clampf(value, 1, 20);
    } else if (strcmp(name, "mix") == 0) {
        w->mix = clampf(value, 0, 1);
    }
}

/* ==== Ring Modulator ==== */

static void ringmod_recalc(ifx_ringmod_t *r) {
    float sr = (float)r->sr;
    if (sr <= 0) sr = 44100;
    r->phase_inc = 2.0f * (float)M_PI * r->frequency / sr;
}

EXPORT void ifx_ringmod_init(ifx_ringmod_t *r, int sr,
                               float frequency, float shape, float mix) {
    r->sr        = sr;
    r->frequency = clampf(frequency, 20, 5000);
    r->shape     = clampf(shape, 0, 1);
    r->mix       = clampf(mix, 0, 1);
    r->phase     = 0;
    ringmod_recalc(r);
}

EXPORT void ifx_ringmod_process(ifx_ringmod_t *r,
                                  const float *in, float *out, int samples) {
    float mix       = r->mix;
    float shape     = r->shape;
    float phase     = r->phase;
    float phase_inc = r->phase_inc;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float carrier_sin = sinf(phase);
        float carrier_sq  = carrier_sin >= 0 ? 1.0f : -1.0f;
        float carrier = carrier_sin * (1 - shape) + carrier_sq * shape;
        float wet = x * carrier;
        out[i] = x * (1 - mix) + wet * mix;
        phase += phase_inc;
        if (phase >= 2.0f * (float)M_PI) phase -= 2.0f * (float)M_PI;
    }
    r->phase = phase;
}

EXPORT void ifx_ringmod_reset(ifx_ringmod_t *r) {
    r->phase = 0;
}

EXPORT void ifx_ringmod_set_param(ifx_ringmod_t *r,
                                    const char *name, float value) {
    if (strcmp(name, "frequency") == 0) {
        r->frequency = clampf(value, 20, 5000);
        ringmod_recalc(r);
    } else if (strcmp(name, "shape") == 0) {
        r->shape = clampf(value, 0, 1);
    } else if (strcmp(name, "mix") == 0) {
        r->mix = clampf(value, 0, 1);
    }
}

/* ==== Tremolo ==== */

static void tremolo_recalc(ifx_tremolo_t *t) {
    float sr = (float)t->sr;
    if (sr <= 0) sr = 44100;
    t->phase_inc = 2.0f * (float)M_PI * t->rate / sr;
}

EXPORT void ifx_tremolo_init(ifx_tremolo_t *t, int sr,
                               float rate, float depth, float shape, float mix) {
    t->sr    = sr;
    t->rate  = clampf(rate, 0.5f, 20);
    t->depth = clampf(depth, 0, 1);
    t->shape = (int)(clampf(shape, 0, 2) + 0.5f);
    t->mix   = clampf(mix, 0, 1);
    t->phase = 0;
    tremolo_recalc(t);
}

EXPORT void ifx_tremolo_process(ifx_tremolo_t *t,
                                  const float *in, float *out, int samples) {
    float depth     = t->depth;
    float mix       = t->mix;
    int   shape     = t->shape;
    float phase     = t->phase;
    float phase_inc = t->phase_inc;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float lfo;
        switch (shape) {
        case 1: { /* triangle */
            float norm = phase / (2.0f * (float)M_PI);
            lfo = norm < 0.5f ? (norm * 4.0f - 1.0f) : (3.0f - norm * 4.0f);
            lfo = (lfo + 1.0f) * 0.5f; /* map to 0..1 */
            break;
        }
        case 2: /* square */
            lfo = phase < (float)M_PI ? 1.0f : 0.0f;
            break;
        default: /* sine */
            lfo = (sinf(phase) + 1.0f) * 0.5f;
            break;
        }
        float gain = 1.0f - depth * (1.0f - lfo);
        float wet = x * gain;
        out[i] = x * (1 - mix) + wet * mix;
        phase += phase_inc;
        if (phase >= 2.0f * (float)M_PI) phase -= 2.0f * (float)M_PI;
    }
    t->phase = phase;
}

EXPORT void ifx_tremolo_reset(ifx_tremolo_t *t) {
    t->phase = 0;
}

EXPORT void ifx_tremolo_set_param(ifx_tremolo_t *t,
                                    const char *name, float value) {
    if (strcmp(name, "rate") == 0) {
        t->rate = clampf(value, 0.5f, 20);
        tremolo_recalc(t);
    } else if (strcmp(name, "depth") == 0) {
        t->depth = clampf(value, 0, 1);
    } else if (strcmp(name, "shape") == 0) {
        t->shape = (int)(clampf(value, 0, 2) + 0.5f);
    } else if (strcmp(name, "mix") == 0) {
        t->mix = clampf(value, 0, 1);
    }
}

/* ==== Gate ==== */

static void gate_recalc(ifx_gate_t *g) {
    float sr = (float)g->sr;
    if (sr <= 0) sr = 44100;
    /* Coefficients will be recomputed from dB/ms in set_param */
}

EXPORT void ifx_gate_init(ifx_gate_t *g, int sr,
                            float threshold_db, float attack_ms,
                            float release_ms, float range_db) {
    g->sr       = sr;
    g->envelope = 0;
    float s = (float)sr;
    if (s <= 0) s = 44100;
    threshold_db = clampf(threshold_db, -60, 0);
    attack_ms    = clampf(attack_ms, 0.1f, 50);
    release_ms   = clampf(release_ms, 1, 500);
    range_db     = clampf(range_db, -90, 0);
    g->threshold_lin = powf(10.0f, threshold_db / 20.0f);
    g->attack_coef   = expf(-1.0f / (attack_ms * 0.001f * s));
    g->release_coef  = expf(-1.0f / (release_ms * 0.001f * s));
    g->range_lin     = powf(10.0f, range_db / 20.0f);
}

EXPORT void ifx_gate_process(ifx_gate_t *g,
                               const float *in, float *out, int samples) {
    float threshold = g->threshold_lin;
    float att_coef  = g->attack_coef;
    float rel_coef  = g->release_coef;
    float range     = g->range_lin;
    float env       = g->envelope;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float ax = fabsf(x);
        if (ax > env)
            env = att_coef * env + (1 - att_coef) * ax;
        else
            env = rel_coef * env;
        float gain = env >= threshold ? 1.0f : range;
        out[i] = x * gain;
    }
    g->envelope = env;
}

EXPORT void ifx_gate_reset(ifx_gate_t *g) {
    g->envelope = 0;
}

EXPORT void ifx_gate_set_param(ifx_gate_t *g,
                                 const char *name, float value) {
    float sr = (float)g->sr;
    if (sr <= 0) sr = 44100;
    if (strcmp(name, "threshold") == 0) {
        value = clampf(value, -60, 0);
        g->threshold_lin = powf(10.0f, value / 20.0f);
    } else if (strcmp(name, "attack") == 0) {
        value = clampf(value, 0.1f, 50);
        g->attack_coef = expf(-1.0f / (value * 0.001f * sr));
    } else if (strcmp(name, "release") == 0) {
        value = clampf(value, 1, 500);
        g->release_coef = expf(-1.0f / (value * 0.001f * sr));
    } else if (strcmp(name, "range") == 0) {
        value = clampf(value, -90, 0);
        g->range_lin = powf(10.0f, value / 20.0f);
    }
}

/* ==== Limiter ==== */

EXPORT void ifx_limiter_init(ifx_limiter_t *l, int sr,
                               float threshold_db, float release_ms,
                               float ceiling_db) {
    l->sr       = sr;
    l->envelope = 0;
    float s = (float)sr;
    if (s <= 0) s = 44100;
    threshold_db = clampf(threshold_db, -20, 0);
    release_ms   = clampf(release_ms, 1, 500);
    ceiling_db   = clampf(ceiling_db, -6, 0);
    l->threshold_lin = powf(10.0f, threshold_db / 20.0f);
    l->release_coef  = expf(-1.0f / (release_ms * 0.001f * s));
    l->ceiling_lin   = powf(10.0f, ceiling_db / 20.0f);
}

EXPORT void ifx_limiter_process(ifx_limiter_t *l,
                                  const float *in, float *out, int samples) {
    float threshold = l->threshold_lin;
    float rel_coef  = l->release_coef;
    float ceiling   = l->ceiling_lin;
    float env       = l->envelope;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float ax = fabsf(x);
        /* Instant attack, exponential release */
        if (ax > env) env = ax;
        else          env = rel_coef * env;
        float gain = 1.0f;
        if (env > threshold && env > 1e-10f) {
            gain = threshold / env;
        }
        out[i] = x * gain * (ceiling / threshold);
    }
    l->envelope = env;
}

EXPORT void ifx_limiter_reset(ifx_limiter_t *l) {
    l->envelope = 0;
}

EXPORT void ifx_limiter_set_param(ifx_limiter_t *l,
                                    const char *name, float value) {
    float sr = (float)l->sr;
    if (sr <= 0) sr = 44100;
    if (strcmp(name, "threshold") == 0) {
        value = clampf(value, -20, 0);
        l->threshold_lin = powf(10.0f, value / 20.0f);
    } else if (strcmp(name, "release") == 0) {
        value = clampf(value, 1, 500);
        l->release_coef = expf(-1.0f / (value * 0.001f * sr));
    } else if (strcmp(name, "ceiling") == 0) {
        value = clampf(value, -6, 0);
        l->ceiling_lin = powf(10.0f, value / 20.0f);
    }
}

/* ==== Flanger ==== */

static void flanger_recalc(ifx_flanger_t *f) {
    float sr = (float)f->sr;
    if (sr <= 0) sr = 44100;
    f->depth_samples = f->depth * 0.001f * sr;
    f->phase_inc = 2.0f * (float)M_PI * f->rate / sr;
}

EXPORT void ifx_flanger_init(ifx_flanger_t *f, int sr, float *buf, int buf_len,
                               float rate, float depth, float feedback, float mix) {
    f->sr        = sr;
    f->buf       = buf;
    f->buf_len   = buf_len;
    f->pos       = 0;
    f->phase     = 0;
    f->last_read = 0;
    f->rate      = clampf(rate, 0.1f, 10);
    f->depth     = clampf(depth, 0.5f, 10);
    f->feedback  = clampf(feedback, -0.95f, 0.95f);
    f->mix       = clampf(mix, 0, 1);
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    flanger_recalc(f);
}

EXPORT void ifx_flanger_process(ifx_flanger_t *f,
                                  const float *in, float *out, int samples) {
    int    buf_len       = f->buf_len;
    float  mix           = f->mix;
    float  feedback      = f->feedback;
    float  depth_samples = f->depth_samples;
    float  phase_inc     = f->phase_inc;
    float  phase         = f->phase;
    float  last_read     = f->last_read;
    int    pos           = f->pos;
    float *buf           = f->buf;

    if (!buf || buf_len <= 0) {
        if (in != out) memcpy(out, in, (size_t)samples * sizeof(float));
        return;
    }

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        buf[pos] = x + feedback * last_read;
        float lfo = (sinf(phase) + 1.0f) * 0.5f;
        float delay_f = 1.0f + lfo * depth_samples;
        float read_f = (float)pos - delay_f;
        if (read_f < 0) read_f += (float)buf_len;
        int idx0 = (int)read_f;
        if (idx0 < 0) idx0 = 0;
        if (idx0 >= buf_len) idx0 = buf_len - 1;
        int idx1 = (idx0 + 1);
        if (idx1 >= buf_len) idx1 = 0;
        float frac = read_f - floorf(read_f);
        float wet = buf[idx0] * (1 - frac) + buf[idx1] * frac;
        last_read = wet;
        phase += phase_inc;
        if (phase >= 2.0f * (float)M_PI) phase -= 2.0f * (float)M_PI;
        pos++;
        if (pos >= buf_len) pos = 0;
        out[i] = x * (1 - mix) + wet * mix;
    }
    f->phase     = phase;
    f->pos       = pos;
    f->last_read = last_read;
}

EXPORT void ifx_flanger_reset(ifx_flanger_t *f) {
    if (f->buf && f->buf_len > 0) {
        memset(f->buf, 0, (size_t)f->buf_len * sizeof(float));
    }
    f->pos       = 0;
    f->phase     = 0;
    f->last_read = 0;
}

EXPORT void ifx_flanger_set_param(ifx_flanger_t *f,
                                    const char *name, float value) {
    if (strcmp(name, "rate") == 0) {
        f->rate = clampf(value, 0.1f, 10);
        flanger_recalc(f);
    } else if (strcmp(name, "depth") == 0) {
        f->depth = clampf(value, 0.5f, 10);
        flanger_recalc(f);
    } else if (strcmp(name, "feedback") == 0) {
        f->feedback = clampf(value, -0.95f, 0.95f);
    } else if (strcmp(name, "mix") == 0) {
        f->mix = clampf(value, 0, 1);
    }
}

/* ==== Phaser ==== */

EXPORT void ifx_phaser_init(ifx_phaser_t *p, int sr,
                              float stages, float rate, float depth,
                              float feedback, float mix) {
    p->sr       = sr;
    p->stages   = (int)clampf(stages, 2, 12);
    p->rate     = clampf(rate, 0.1f, 10);
    p->depth    = clampf(depth, 0, 1);
    p->feedback = clampf(feedback, 0, 0.95f);
    p->mix      = clampf(mix, 0, 1);
    p->phase    = 0;
    p->fb_sample = 0;
    memset(p->x1, 0, sizeof(p->x1));
    memset(p->y1, 0, sizeof(p->y1));
}

EXPORT void ifx_phaser_process(ifx_phaser_t *p,
                                 const float *in, float *out, int samples) {
    int   stages   = p->stages;
    float rate     = p->rate;
    float depth    = p->depth;
    float feedback = p->feedback;
    float mix      = p->mix;
    float phase    = p->phase;
    float fb       = p->fb_sample;
    float sr       = (float)p->sr;
    if (sr <= 0) sr = 44100;
    float phase_inc = 2.0f * (float)M_PI * rate / sr;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float sig = x + fb * feedback;
        float lfo = (sinf(phase) + 1.0f) * 0.5f;
        float min_freq = 200.0f;
        float max_freq = 200.0f + depth * 3800.0f;
        float fc = min_freq + lfo * (max_freq - min_freq);
        float w = tanf((float)M_PI * fc / sr);
        float a = (w - 1.0f) / (w + 1.0f);
        for (int s = 0; s < stages; s++) {
            float y = a * sig + p->x1[s] - a * p->y1[s];
            p->x1[s] = sig;
            p->y1[s] = y;
            sig = y;
        }
        fb = sig;
        phase += phase_inc;
        if (phase >= 2.0f * (float)M_PI) phase -= 2.0f * (float)M_PI;
        out[i] = x * (1 - mix) + sig * mix;
    }
    p->phase     = phase;
    p->fb_sample = fb;
}

EXPORT void ifx_phaser_reset(ifx_phaser_t *p) {
    p->phase     = 0;
    p->fb_sample = 0;
    memset(p->x1, 0, sizeof(p->x1));
    memset(p->y1, 0, sizeof(p->y1));
}

EXPORT void ifx_phaser_set_param(ifx_phaser_t *p,
                                   const char *name, float value) {
    if (strcmp(name, "stages") == 0) {
        p->stages = (int)clampf(value, 2, 12);
    } else if (strcmp(name, "rate") == 0) {
        p->rate = clampf(value, 0.1f, 10);
    } else if (strcmp(name, "depth") == 0) {
        p->depth = clampf(value, 0, 1);
    } else if (strcmp(name, "feedback") == 0) {
        p->feedback = clampf(value, 0, 0.95f);
    } else if (strcmp(name, "mix") == 0) {
        p->mix = clampf(value, 0, 1);
    }
}

/* ==== Auto-Wah ==== */

EXPORT void ifx_autowah_init(ifx_autowah_t *a, int sr,
                               float sensitivity, float rate,
                               float depth, float mix) {
    a->sr          = sr;
    a->sensitivity = clampf(sensitivity, 0, 1);
    a->rate        = clampf(rate, 0.5f, 20);
    a->depth       = clampf(depth, 0, 1);
    a->mix         = clampf(mix, 0, 1);
    a->env         = 0;
    a->lfo_phase   = 0;
    a->bp          = 0;
    a->lp          = 0;
    float s = (float)sr;
    if (s <= 0) s = 44100;
    a->lfo_inc = 2.0f * (float)M_PI * a->rate / s;
}

EXPORT void ifx_autowah_process(ifx_autowah_t *a,
                                  const float *in, float *out, int samples) {
    float sensitivity = a->sensitivity;
    float depth       = a->depth;
    float mix         = a->mix;
    float env         = a->env;
    float lfo_phase   = a->lfo_phase;
    float lfo_inc     = a->lfo_inc;
    float bp          = a->bp;
    float lp_state    = a->lp;
    float sr          = (float)a->sr;
    if (sr <= 0) sr = 44100;

    /* Envelope follower coefficients */
    float env_att = expf(-1.0f / (0.001f * sr));  /* 1ms attack */
    float env_rel = expf(-1.0f / (0.05f * sr));   /* 50ms release */

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float ax = fabsf(x);
        if (ax > env)
            env = env_att * env + (1 - env_att) * ax;
        else
            env = env_rel * env;

        float lfo = (sinf(lfo_phase) + 1.0f) * 0.5f;
        /* Map env + LFO to filter frequency */
        float env_mod = env * sensitivity * 5000.0f;
        float lfo_mod = lfo * depth * 3000.0f;
        float fc = 200.0f + env_mod + lfo_mod;
        if (fc > sr * 0.45f) fc = sr * 0.45f;
        if (fc < 50.0f) fc = 50.0f;

        /* State-variable filter (bandpass) */
        float f_coef = 2.0f * sinf((float)M_PI * fc / sr);
        float q = 0.5f + depth * 4.5f; /* Q from 0.5 to 5 */
        float hp = x - lp_state - q * bp;
        bp += f_coef * hp;
        lp_state += f_coef * bp;
        /* Flush denormals */
        if (bp > -1e-20f && bp < 1e-20f) bp = 0;
        if (lp_state > -1e-20f && lp_state < 1e-20f) lp_state = 0;

        float wet = bp;
        lfo_phase += lfo_inc;
        if (lfo_phase >= 2.0f * (float)M_PI) lfo_phase -= 2.0f * (float)M_PI;
        out[i] = x * (1 - mix) + wet * mix;
    }
    a->env       = env;
    a->lfo_phase = lfo_phase;
    a->bp        = bp;
    a->lp        = lp_state;
}

EXPORT void ifx_autowah_reset(ifx_autowah_t *a) {
    a->env       = 0;
    a->lfo_phase = 0;
    a->bp        = 0;
    a->lp        = 0;
}

EXPORT void ifx_autowah_set_param(ifx_autowah_t *a,
                                    const char *name, float value) {
    if (strcmp(name, "sensitivity") == 0) {
        a->sensitivity = clampf(value, 0, 1);
    } else if (strcmp(name, "rate") == 0) {
        a->rate = clampf(value, 0.5f, 20);
        float sr = (float)a->sr;
        if (sr <= 0) sr = 44100;
        a->lfo_inc = 2.0f * (float)M_PI * a->rate / sr;
    } else if (strcmp(name, "depth") == 0) {
        a->depth = clampf(value, 0, 1);
    } else if (strcmp(name, "mix") == 0) {
        a->mix = clampf(value, 0, 1);
    }
}

/* ==== Compressor ==== */

static void compressor_recalc(ifx_compressor_t *c) {
    float sr = (float)c->sr;
    if (sr <= 0) sr = 44100;
    c->threshold_lin = powf(10.0f, c->threshold_db / 20.0f);
    c->makeup_lin    = powf(10.0f, c->makeup_db / 20.0f);
}

EXPORT void ifx_compressor_init(ifx_compressor_t *c, int sr,
                                  float threshold_db, float ratio,
                                  float attack_ms, float release_ms,
                                  float makeup_db, float mix) {
    c->sr           = sr;
    c->envelope     = 0;
    float s = (float)sr;
    if (s <= 0) s = 44100;
    c->threshold_db = clampf(threshold_db, -60, 0);
    c->ratio        = clampf(ratio, 1, 20);
    c->makeup_db    = clampf(makeup_db, 0, 24);
    c->mix          = clampf(mix, 0, 1);
    c->attack_coef  = expf(-1.0f / (clampf(attack_ms, 0.1f, 100) * 0.001f * s));
    c->release_coef = expf(-1.0f / (clampf(release_ms, 10, 1000) * 0.001f * s));
    compressor_recalc(c);
}

EXPORT void ifx_compressor_process(ifx_compressor_t *c,
                                     const float *in, float *out, int samples) {
    float thresh_lin = c->threshold_lin;
    float thresh_db  = c->threshold_db;
    float ratio      = c->ratio;
    float att_coef   = c->attack_coef;
    float rel_coef   = c->release_coef;
    float makeup_lin = c->makeup_lin;
    float mix        = c->mix;
    float env        = c->envelope;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float ax = fabsf(x);
        /* Envelope follower */
        if (ax > env)
            env = att_coef * env + (1 - att_coef) * ax;
        else
            env = rel_coef * env;
        /* Gain computation in dB domain */
        float gain = 1.0f;
        if (env > thresh_lin && env > 1e-10f) {
            float env_db = 20.0f * log10f(env);
            float over = env_db - thresh_db;
            float gain_db = over - over / ratio;
            gain = powf(10.0f, -gain_db / 20.0f);
        }
        float wet = x * gain * makeup_lin;
        out[i] = x * (1 - mix) + wet * mix;
    }
    c->envelope = env;
}

EXPORT void ifx_compressor_reset(ifx_compressor_t *c) {
    c->envelope = 0;
}

EXPORT void ifx_compressor_set_param(ifx_compressor_t *c,
                                       const char *name, float value) {
    float sr = (float)c->sr;
    if (sr <= 0) sr = 44100;
    if (strcmp(name, "threshold") == 0) {
        c->threshold_db = clampf(value, -60, 0);
        compressor_recalc(c);
    } else if (strcmp(name, "ratio") == 0) {
        c->ratio = clampf(value, 1, 20);
    } else if (strcmp(name, "attack") == 0) {
        c->attack_coef = expf(-1.0f / (clampf(value, 0.1f, 100) * 0.001f * sr));
    } else if (strcmp(name, "release") == 0) {
        c->release_coef = expf(-1.0f / (clampf(value, 10, 1000) * 0.001f * sr));
    } else if (strcmp(name, "makeup") == 0) {
        c->makeup_db = clampf(value, 0, 24);
        compressor_recalc(c);
    } else if (strcmp(name, "mix") == 0) {
        c->mix = clampf(value, 0, 1);
    }
}

/* ==== Transient Shaper ==== */

static void transient_recalc(ifx_transient_t *t) {
    float sr = (float)t->sr;
    if (sr <= 0) sr = 44100;
    float speed_s = t->speed_ms * 0.001f;
    if (speed_s <= 0) speed_s = 0.01f;
    t->fast_coef     = expf(-1.0f / (speed_s * sr));
    t->fast_rel_coef = expf(-1.0f / (speed_s * 3.0f * sr));
    t->slow_coef     = expf(-1.0f / (0.1f * sr)); /* fixed 100ms */
}

EXPORT void ifx_transient_init(ifx_transient_t *t, int sr,
                                 float attack_pct, float sustain_pct,
                                 float speed_ms) {
    t->sr           = sr;
    t->attack_gain  = clampf(attack_pct, 0, 200) * 0.01f;
    t->sustain_gain = clampf(sustain_pct, 0, 200) * 0.01f;
    t->speed_ms     = clampf(speed_ms, 1, 50);
    t->fast_env     = 0;
    t->slow_env     = 0;
    transient_recalc(t);
}

EXPORT void ifx_transient_process(ifx_transient_t *t,
                                    const float *in, float *out, int samples) {
    float att_gain  = t->attack_gain;
    float sus_gain  = t->sustain_gain;
    float fast_coef = t->fast_coef;
    float fast_rel  = t->fast_rel_coef;
    float slow_coef = t->slow_coef;
    float fast_env  = t->fast_env;
    float slow_env  = t->slow_env;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        float ax = fabsf(x);
        /* Fast envelope */
        if (ax > fast_env)
            fast_env = fast_coef * fast_env + (1 - fast_coef) * ax;
        else
            fast_env = fast_rel * fast_env;
        /* Slow envelope */
        slow_env = slow_coef * slow_env + (1 - slow_coef) * ax;
        /* Transient amount */
        float diff = fast_env - slow_env;
        float denom = slow_env > 1e-10f ? slow_env : 1e-10f;
        float trans = diff / denom;
        if (trans < 0) trans = 0;
        if (trans > 1) trans = 1;
        float gain = att_gain * trans + sus_gain * (1 - trans);
        out[i] = x * gain;
    }
    t->fast_env = fast_env;
    t->slow_env = slow_env;
}

EXPORT void ifx_transient_reset(ifx_transient_t *t) {
    t->fast_env = 0;
    t->slow_env = 0;
}

EXPORT void ifx_transient_set_param(ifx_transient_t *t,
                                      const char *name, float value) {
    if (strcmp(name, "attack") == 0) {
        t->attack_gain = clampf(value, 0, 200) * 0.01f;
    } else if (strcmp(name, "sustain") == 0) {
        t->sustain_gain = clampf(value, 0, 200) * 0.01f;
    } else if (strcmp(name, "speed") == 0) {
        t->speed_ms = clampf(value, 1, 50);
        transient_recalc(t);
    }
}

/* ==== Tape Saturation ==== */

static void tape_recalc_lp(ifx_tape_t *t) {
    float sr = (float)t->sr;
    if (sr <= 0) sr = 44100;
    float fc = 2000.0f + (1.0f - t->warmth) * 18000.0f;
    t->lp_a = expf(-2.0f * (float)M_PI * fc / sr);
}

static void tape_recalc_lfo(ifx_tape_t *t) {
    float sr = (float)t->sr;
    if (sr <= 0) sr = 44100;
    t->wow_inc     = 2.0f * (float)M_PI * 0.5f / sr;  /* 0.5 Hz wow */
    t->flutter_inc = 2.0f * (float)M_PI * 6.0f / sr;  /* 6 Hz flutter */
}

EXPORT void ifx_tape_init(ifx_tape_t *t, int sr, float *buf, int buf_len,
                            float drive, float warmth, float wow,
                            float flutter, float mix) {
    t->sr      = sr;
    t->buf     = buf;
    t->buf_len = buf_len;
    t->pos     = 0;
    t->drive   = clampf(drive, 1, 10);
    t->warmth  = clampf(warmth, 0, 1);
    t->wow     = clampf(wow, 0, 1);
    t->flutter = clampf(flutter, 0, 1);
    t->mix     = clampf(mix, 0, 1);
    t->lp_y1   = 0;
    t->wow_phase     = 0;
    t->flutter_phase = 0;
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    tape_recalc_lp(t);
    tape_recalc_lfo(t);
}

EXPORT void ifx_tape_process(ifx_tape_t *t,
                               const float *in, float *out, int samples) {
    float  drive   = t->drive;
    float  mix     = t->mix;
    float  wow     = t->wow;
    float  flutter = t->flutter;
    float  lp_a    = t->lp_a;
    float  lp_y1   = t->lp_y1;
    float  wow_ph  = t->wow_phase;
    float  flu_ph  = t->flutter_phase;
    float  wow_inc = t->wow_inc;
    float  flu_inc = t->flutter_inc;
    int    pos     = t->pos;
    float *buf     = t->buf;
    int    buf_len = t->buf_len;
    float  sr      = (float)t->sr;
    if (sr <= 0) sr = 44100;

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        /* Saturation: normalized tanh */
        float norm = tanhf(drive);
        float sat = (norm > 1e-10f) ? tanhf(drive * x) / norm : x;
        /* Warmth: one-pole LP */
        lp_y1 = sat * (1 - lp_a) + lp_y1 * lp_a;
        float filtered = lp_y1;

        float wet;
        if ((wow > 0 || flutter > 0) && buf && buf_len > 0) {
            /* Write to delay buffer */
            buf[pos] = filtered;
            /* Compute modulated read offset */
            float wow_mod = sinf(wow_ph) * wow * 0.002f * sr;
            float flu_mod = sinf(flu_ph) * flutter * 0.0005f * sr;
            float offset = 1.0f + wow_mod + flu_mod;
            if (offset < 0) offset = 0;
            /* Read with linear interpolation */
            float read_f = (float)pos - offset;
            if (read_f < 0) read_f += (float)buf_len;
            int idx0 = (int)read_f;
            if (idx0 < 0) idx0 = 0;
            if (idx0 >= buf_len) idx0 = buf_len - 1;
            int idx1 = (idx0 + 1);
            if (idx1 >= buf_len) idx1 = 0;
            float frac = read_f - floorf(read_f);
            wet = buf[idx0] * (1 - frac) + buf[idx1] * frac;
            /* Advance LFO phases */
            wow_ph += wow_inc;
            if (wow_ph >= 2.0f * (float)M_PI) wow_ph -= 2.0f * (float)M_PI;
            flu_ph += flu_inc;
            if (flu_ph >= 2.0f * (float)M_PI) flu_ph -= 2.0f * (float)M_PI;
            pos++;
            if (pos >= buf_len) pos = 0;
        } else {
            wet = filtered;
        }
        out[i] = x * (1 - mix) + wet * mix;
    }
    t->lp_y1          = lp_y1;
    t->wow_phase       = wow_ph;
    t->flutter_phase   = flu_ph;
    t->pos             = pos;
}

EXPORT void ifx_tape_reset(ifx_tape_t *t) {
    t->lp_y1          = 0;
    t->wow_phase       = 0;
    t->flutter_phase   = 0;
    t->pos             = 0;
    if (t->buf && t->buf_len > 0) {
        memset(t->buf, 0, (size_t)t->buf_len * sizeof(float));
    }
}

EXPORT void ifx_tape_set_param(ifx_tape_t *t,
                                 const char *name, float value) {
    if (strcmp(name, "drive") == 0) {
        t->drive = clampf(value, 1, 10);
    } else if (strcmp(name, "warmth") == 0) {
        t->warmth = clampf(value, 0, 1);
        tape_recalc_lp(t);
    } else if (strcmp(name, "wow") == 0) {
        t->wow = clampf(value, 0, 1);
    } else if (strcmp(name, "flutter") == 0) {
        t->flutter = clampf(value, 0, 1);
    } else if (strcmp(name, "mix") == 0) {
        t->mix = clampf(value, 0, 1);
    }
}

/* ==== Pitch Shifter (dual delay-line) ==== */

static void pitchshift_recalc(ifx_pitchshift_t *p) {
    float sr = (float)p->sr;
    if (sr <= 0) sr = 44100;
    p->window_samples = p->window_ms * 0.001f * sr;
    if (p->window_samples < 2) p->window_samples = 2;
    p->step = 1.0f - powf(2.0f, p->pitch / 12.0f);
}

EXPORT void ifx_pitchshift_init(ifx_pitchshift_t *p, int sr,
                                  float *buf, int buf_len,
                                  float pitch, float mix, float window_ms) {
    p->sr        = sr;
    p->buf       = buf;
    p->buf_len   = buf_len;
    p->write_pos = 0;
    p->pitch     = clampf(pitch, -24, 24);
    p->mix       = clampf(mix, 0, 1);
    p->window_ms = clampf(window_ms, 20, 100);
    if (buf && buf_len > 0) {
        memset(buf, 0, (size_t)buf_len * sizeof(float));
    }
    pitchshift_recalc(p);
    p->head_a = 0;
    p->head_b = p->window_samples * 0.5f;
}

static inline float ps_buf_read(const float *buf, int buf_len, float pos) {
    while (pos < 0) pos += (float)buf_len;
    int idx = (int)pos;
    idx = idx % buf_len;
    if (idx < 0) idx += buf_len;
    int next = (idx + 1) % buf_len;
    float frac = pos - floorf(pos);
    return buf[idx] * (1 - frac) + buf[next] * frac;
}

EXPORT void ifx_pitchshift_process(ifx_pitchshift_t *p,
                                     const float *in, float *out, int samples) {
    float  mix       = p->mix;
    float  step      = p->step;
    float  win       = p->window_samples;
    float  head_a    = p->head_a;
    float  head_b    = p->head_b;
    int    wpos      = p->write_pos;
    float *buf       = p->buf;
    int    buf_len   = p->buf_len;

    if (!buf || buf_len <= 0) {
        if (in != out) memcpy(out, in, (size_t)samples * sizeof(float));
        return;
    }

    for (int i = 0; i < samples; i++) {
        float x = in[i];
        buf[wpos] = x;

        /* Advance heads */
        head_a += step;
        head_b += step;

        /* Wrap within window */
        while (head_a < 0)   head_a += win;
        while (head_a >= win) head_a -= win;
        while (head_b < 0)   head_b += win;
        while (head_b >= win) head_b -= win;

        /* Triangular cross-fade */
        float alpha_a = 1.0f - fabsf(2.0f * head_a / win - 1.0f);
        float alpha_b = 1.0f - fabsf(2.0f * head_b / win - 1.0f);

        /* Read from buffer */
        float read_a = ps_buf_read(buf, buf_len, (float)wpos - head_a);
        float read_b = ps_buf_read(buf, buf_len, (float)wpos - head_b);

        float sum_alpha = alpha_a + alpha_b;
        float wet = 0;
        if (sum_alpha > 1e-10f) {
            wet = (alpha_a * read_a + alpha_b * read_b) / sum_alpha;
        }

        wpos++;
        if (wpos >= buf_len) wpos = 0;

        out[i] = x * (1 - mix) + wet * mix;
    }
    p->head_a    = head_a;
    p->head_b    = head_b;
    p->write_pos = wpos;
}

EXPORT void ifx_pitchshift_reset(ifx_pitchshift_t *p) {
    if (p->buf && p->buf_len > 0) {
        memset(p->buf, 0, (size_t)p->buf_len * sizeof(float));
    }
    p->write_pos = 0;
    p->head_a    = 0;
    p->head_b    = p->window_samples * 0.5f;
}

EXPORT void ifx_pitchshift_set_param(ifx_pitchshift_t *p,
                                       const char *name, float value) {
    if (strcmp(name, "pitch") == 0) {
        p->pitch = clampf(value, -24, 24);
        pitchshift_recalc(p);
    } else if (strcmp(name, "mix") == 0) {
        p->mix = clampf(value, 0, 1);
    } else if (strcmp(name, "window") == 0) {
        p->window_ms = clampf(value, 20, 100);
        pitchshift_recalc(p);
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
