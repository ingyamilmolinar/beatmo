#ifndef LFO_H
#define LFO_H

#include "wavetable.h"

/* ================================================================
 * LFO (Low-Frequency Oscillator) module.
 *
 * Uses the wavetable oscillator for sine/triangle/saw/square shapes.
 * Adds random sample-and-hold mode for noise modulation.
 * Output is centered around `center` with ±depth range.
 * ================================================================ */

typedef enum {
    LFO_SINE,
    LFO_TRIANGLE,
    LFO_SAW,
    LFO_SQUARE,
    LFO_RANDOM_SH    /* random sample-and-hold */
} lfo_shape_t;

typedef struct {
    wt_osc_t    osc;
    lfo_shape_t shape;
    float       rate;      /* Hz */
    float       depth;     /* modulation amplitude */
    float       center;    /* output center value */
    int         sr;

    /* For random S&H */
    float       sh_value;
    int         sh_counter;
    int         sh_period;  /* samples between S&H updates */

    /* LFO owns its own wavetable for non-sine shapes.
     * For sine, it reuses a global sine table. */
    float      *table_buf;  /* caller-owned buffer if custom table needed */
    wavetable_t wt;
} lfo_t;

/* Initialize LFO. table_buf must be at least (WT_DEFAULT_LENGTH+1) floats
 * for non-sine shapes, or NULL for sine (uses internal static sine table). */
void lfo_init(lfo_t *lfo, int sr, lfo_shape_t shape,
              float rate, float depth, float center, float *table_buf);

/* Change LFO rate (Hz). */
void lfo_set_rate(lfo_t *lfo, float rate);

/* Change LFO depth. */
void lfo_set_depth(lfo_t *lfo, float depth);

/* Block render: writes LFO output (center ± depth) to out. */
void lfo_process(lfo_t *lfo, float *out, int samples);

/* Block modulate: multiplies buf[i] by LFO value for each sample. */
void lfo_modulate(lfo_t *lfo, float *buf, int samples);

/* Reset LFO phase to 0. */
void lfo_reset(lfo_t *lfo);

#endif /* LFO_H */
