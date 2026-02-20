#include <math.h>
#include <stdlib.h>
#include "lfo.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* Static global sine table, initialized on first use. */
static float        g_sine_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t  g_sine_wt;
static int          g_sine_initialized = 0;

static void ensure_sine_table(void) {
    if (g_sine_initialized) return;
    wt_generate_sine(&g_sine_wt, g_sine_buf, WT_DEFAULT_LENGTH);
    g_sine_initialized = 1;
}

/* Simple pseudo-random for S&H (deterministic, no seed) */
static unsigned int lfo_rng_state = 1;
static float lfo_random(void) {
    lfo_rng_state = lfo_rng_state * 1103515245 + 12345;
    return ((float)(lfo_rng_state >> 16 & 0x7FFF) / 32767.0f) * 2.0f - 1.0f;
}

EXPORT void lfo_init(lfo_t *lfo, int sr, lfo_shape_t shape,
                      float rate, float depth, float center, float *table_buf) {
    lfo->sr     = sr;
    lfo->shape  = shape;
    lfo->rate   = rate;
    lfo->depth  = depth;
    lfo->center = center;
    lfo->table_buf = table_buf;

    /* Select or build wavetable based on shape */
    int harmonics = 64; /* plenty for LFO range */
    switch (shape) {
    case LFO_SINE:
        ensure_sine_table();
        lfo->wt = g_sine_wt;
        break;
    case LFO_TRIANGLE:
        if (table_buf) {
            wt_generate_triangle(&lfo->wt, table_buf, WT_DEFAULT_LENGTH, harmonics);
        }
        break;
    case LFO_SAW:
        if (table_buf) {
            wt_generate_saw(&lfo->wt, table_buf, WT_DEFAULT_LENGTH, harmonics);
        }
        break;
    case LFO_SQUARE:
        if (table_buf) {
            wt_generate_square(&lfo->wt, table_buf, WT_DEFAULT_LENGTH, harmonics);
        }
        break;
    case LFO_RANDOM_SH:
        /* S&H doesn't use wavetable but we init the osc anyway for consistency */
        ensure_sine_table();
        lfo->wt = g_sine_wt;
        break;
    }

    wt_osc_init(&lfo->osc, &lfo->wt, (double)rate, sr);

    /* S&H state */
    lfo->sh_value   = 0;
    lfo->sh_counter = 0;
    lfo->sh_period  = (rate > 0 && sr > 0) ? (int)((float)sr / rate) : sr;
    if (lfo->sh_period < 1) lfo->sh_period = 1;
}

EXPORT void lfo_set_rate(lfo_t *lfo, float rate) {
    lfo->rate = rate;
    wt_osc_set_freq(&lfo->osc, (double)rate, lfo->sr);
    lfo->sh_period = (rate > 0 && lfo->sr > 0) ? (int)((float)lfo->sr / rate) : lfo->sr;
    if (lfo->sh_period < 1) lfo->sh_period = 1;
}

EXPORT void lfo_set_depth(lfo_t *lfo, float depth) {
    lfo->depth = depth;
}

EXPORT void lfo_process(lfo_t *lfo, float *out, int samples) {
    float depth  = lfo->depth;
    float center = lfo->center;

    if (lfo->shape == LFO_RANDOM_SH) {
        /* Sample-and-hold: output constant until counter hits period */
        float sh_val = lfo->sh_value;
        int   counter = lfo->sh_counter;
        int   period  = lfo->sh_period;
        for (int i = 0; i < samples; i++) {
            counter++;
            if (counter >= period) {
                counter = 0;
                sh_val = lfo_random();
            }
            out[i] = center + sh_val * depth;
        }
        lfo->sh_value   = sh_val;
        lfo->sh_counter = counter;
    } else {
        /* Wavetable-based LFO */
        for (int i = 0; i < samples; i++) {
            float raw = wt_osc_tick(&lfo->osc);
            out[i] = center + raw * depth;
        }
    }
}

EXPORT void lfo_modulate(lfo_t *lfo, float *buf, int samples) {
    float depth  = lfo->depth;
    float center = lfo->center;

    if (lfo->shape == LFO_RANDOM_SH) {
        float sh_val = lfo->sh_value;
        int   counter = lfo->sh_counter;
        int   period  = lfo->sh_period;
        for (int i = 0; i < samples; i++) {
            counter++;
            if (counter >= period) {
                counter = 0;
                sh_val = lfo_random();
            }
            buf[i] *= center + sh_val * depth;
        }
        lfo->sh_value   = sh_val;
        lfo->sh_counter = counter;
    } else {
        for (int i = 0; i < samples; i++) {
            float raw = wt_osc_tick(&lfo->osc);
            buf[i] *= center + raw * depth;
        }
    }
}

EXPORT void lfo_reset(lfo_t *lfo) {
    lfo->osc.phase = 0;
    lfo->sh_value   = 0;
    lfo->sh_counter = 0;
}
