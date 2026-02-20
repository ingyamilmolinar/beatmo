#include <math.h>
#include <string.h>
#include "effects.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* ==== Delay line ==== */

EXPORT void delay_init(delay_t *d, float *buffer, int delaySamples,
                       float feedback, float dampingHz, int sampleRate) {
    d->buffer   = buffer;
    d->length   = delaySamples;
    d->writePos = 0;
    d->feedback = feedback;
    d->lpState  = 0.0f;

    /* One-pole LP coefficient: higher dampingHz = less damping (brighter).
     * coeff = exp(-2*PI*dampingHz/sr) — 0 = no filtering, 1 = full damp. */
    if (dampingHz > 0 && sampleRate > 0) {
        double rc = 1.0 / (2.0 * M_PI * dampingHz);
        double dt = 1.0 / (double)sampleRate;
        d->lpCoeff = (float)(dt / (rc + dt));
    } else {
        d->lpCoeff = 1.0f; /* no filtering */
    }

    memset(buffer, 0, (size_t)delaySamples * sizeof(float));
}

EXPORT void delay_process(delay_t *d, float *buf, int samples) {
    for (int i = 0; i < samples; i++) {
        int readPos = d->writePos - d->length;
        if (readPos < 0) readPos += d->length;
        /* Guard against out-of-bounds if length is 0 or 1. */
        if (d->length <= 0) continue;
        if (readPos < 0) readPos = 0;
        if (readPos >= d->length) readPos = d->length - 1;

        float delayed = d->buffer[readPos];

        /* LP-filtered feedback for warm tape echo character. */
        d->lpState += d->lpCoeff * (delayed - d->lpState);
        float fbSample = d->lpState * d->feedback;

        d->buffer[d->writePos] = buf[i] + fbSample;
        buf[i] = buf[i] + delayed; /* wet = delayed signal added to dry */

        d->writePos++;
        if (d->writePos >= d->length) d->writePos = 0;
    }
}

EXPORT void delay_reset(delay_t *d) {
    if (d->buffer && d->length > 0) {
        memset(d->buffer, 0, (size_t)d->length * sizeof(float));
    }
    d->writePos = 0;
    d->lpState  = 0.0f;
}

/* ==== Schroeder Reverb ==== */

/* Prime-spaced comb delays (in milliseconds at 44100 Hz reference).
 * Scaled proportionally for other sample rates. */
static const double COMB_DELAYS_MS[4]    = { 29.7, 37.1, 41.1, 43.7 };
static const double ALLPASS_DELAYS_MS[2]  = { 5.0, 1.7 };

static int ms_to_samples(double ms, int sampleRate) {
    int s = (int)(ms * 0.001 * sampleRate + 0.5);
    return s > 0 ? s : 1;
}

EXPORT int reverb_buffer_size(int sampleRate) {
    int total = 0;
    for (int i = 0; i < 4; i++)
        total += ms_to_samples(COMB_DELAYS_MS[i], sampleRate);
    for (int i = 0; i < 2; i++)
        total += ms_to_samples(ALLPASS_DELAYS_MS[i], sampleRate);
    return total;
}

EXPORT void reverb_init(reverb_t *r, float *buffer, int sampleRate,
                        float roomSize, float damping, float wet) {
    float *ptr = buffer;
    r->wet = wet;
    r->dry = 1.0f;

    for (int i = 0; i < 4; i++) {
        int len = ms_to_samples(COMB_DELAYS_MS[i], sampleRate);
        r->combs[i].buf      = ptr;
        r->combs[i].length   = len;
        r->combs[i].pos      = 0;
        r->combs[i].feedback = roomSize;
        r->combs[i].damp     = damping;
        r->combs[i].dampState = 0.0f;
        memset(ptr, 0, (size_t)len * sizeof(float));
        ptr += len;
    }

    for (int i = 0; i < 2; i++) {
        int len = ms_to_samples(ALLPASS_DELAYS_MS[i], sampleRate);
        r->allpasses[i].buf      = ptr;
        r->allpasses[i].length   = len;
        r->allpasses[i].pos      = 0;
        r->allpasses[i].feedback = 0.5f;
        memset(ptr, 0, (size_t)len * sizeof(float));
        ptr += len;
    }
}

static float comb_process(comb_t *c, float input) {
    float output = c->buf[c->pos];

    /* LP-damped feedback. */
    c->dampState = output * (1.0f - c->damp) + c->dampState * c->damp;
    c->buf[c->pos] = input + c->dampState * c->feedback;

    c->pos++;
    if (c->pos >= c->length) c->pos = 0;

    return output;
}

static float allpass_process(allpass_t *a, float input) {
    float delayed = a->buf[a->pos];
    float output = -input + delayed;

    a->buf[a->pos] = input + delayed * a->feedback;

    a->pos++;
    if (a->pos >= a->length) a->pos = 0;

    return output;
}

EXPORT void reverb_process(reverb_t *r, const float *in, float *out,
                           int samples) {
    for (int i = 0; i < samples; i++) {
        float input = in[i];
        float wet = 0.0f;

        /* Sum of 4 parallel comb filters. */
        for (int c = 0; c < 4; c++) {
            wet += comb_process(&r->combs[c], input);
        }

        /* Series allpass diffusers. */
        for (int a = 0; a < 2; a++) {
            wet = allpass_process(&r->allpasses[a], wet);
        }

        out[i] = input * r->dry + wet * r->wet;
    }
}

EXPORT void reverb_reset(reverb_t *r) {
    for (int i = 0; i < 4; i++) {
        if (r->combs[i].buf && r->combs[i].length > 0) {
            memset(r->combs[i].buf, 0,
                   (size_t)r->combs[i].length * sizeof(float));
        }
        r->combs[i].pos = 0;
        r->combs[i].dampState = 0.0f;
    }
    for (int i = 0; i < 2; i++) {
        if (r->allpasses[i].buf && r->allpasses[i].length > 0) {
            memset(r->allpasses[i].buf, 0,
                   (size_t)r->allpasses[i].length * sizeof(float));
        }
        r->allpasses[i].pos = 0;
    }
}
