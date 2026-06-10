#ifndef WAVETABLE_H
#define WAVETABLE_H

/* ================================================================
 * Wavetable oscillator module.
 *
 * Adapted from GoAudio's LookupOscillator guard-point pattern,
 * optimized for C block processing.
 *
 * Guard point: table[length] == table[0] eliminates branch in
 * linear interpolation. Table size 4096 gives <0.01% THD.
 * ================================================================ */

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

#define WT_DEFAULT_LENGTH 4096

/* Wavetable: table[0..length] where table[length] == table[0] (guard point).
 * Caller owns the buffer (must be at least length+1 floats). */
typedef struct {
    float *table;    /* table[0..length] with guard point */
    int    length;   /* unique samples (guard point at index length) */
} wavetable_t;

/* Wavetable oscillator with phase accumulator. */
typedef struct {
    const wavetable_t *wt;
    double phase;     /* 0..length fractional position */
    double phase_inc; /* samples per output sample */
} wt_osc_t;

/* ── Generation functions (band-limited via Fourier addition) ─── */

/* Generate a sine wave table. buf must be at least length+1 floats. */
void wt_generate_sine(wavetable_t *wt, float *buf, int length);

/* Generate a band-limited sawtooth. harmonics = number of harmonics. */
void wt_generate_saw(wavetable_t *wt, float *buf, int length, int harmonics);

/* Generate a band-limited square wave. */
void wt_generate_square(wavetable_t *wt, float *buf, int length, int harmonics);

/* Generate a band-limited triangle wave. */
void wt_generate_triangle(wavetable_t *wt, float *buf, int length, int harmonics);

/* ── Oscillator functions ────────────────────────────────────── */

/* Initialize oscillator for a given frequency and sample rate. */
void wt_osc_init(wt_osc_t *osc, const wavetable_t *wt,
                  double freq, int sampleRate);

/* Phase-preserving frequency change. */
void wt_osc_set_freq(wt_osc_t *osc, double freq, int sampleRate);

/* Set the normalized phase accumulator. phase01 in [0,1) maps to [0,length);
 * values outside [0,1) are wrapped. Used by the modular gen bank for
 * fixed/seeded slot start phases. Default init (phase 0) is unaffected — call
 * this only when a non-zero start phase is requested. */
void wt_osc_set_phase(wt_osc_t *osc, double phase01);

/* Block render with linear interpolation. */
void wt_osc_process(wt_osc_t *osc, float *out, int samples);

/* Block render with per-sample FM (frequency modulation).
 * freq_mod[i] is added to the base frequency for sample i. */
void wt_osc_process_fm(wt_osc_t *osc, const float *freq_mod,
                         float *out, int samples, int sampleRate);

/* Single-sample tick (inline for use in tight inner loops). */
static inline float wt_osc_tick(wt_osc_t *osc) {
    double pos = osc->phase;
    int idx = (int)pos;
    float frac = (float)(pos - (double)idx);
    float val = osc->wt->table[idx] * (1.0f - frac)
              + osc->wt->table[idx + 1] * frac;
    osc->phase += osc->phase_inc;
    int len = osc->wt->length;
    if (osc->phase >= (double)len) osc->phase -= (double)len;
    return val;
}

/* Single-sample tick with phase modulation (for FM synthesis).
 * phase_mod is in table units (not radians). */
static inline float wt_osc_tick_pm(wt_osc_t *osc, double phase_mod) {
    double pos = osc->phase + phase_mod;
    int len = osc->wt->length;
    /* Wrap into [0, length) */
    while (pos < 0) pos += (double)len;
    while (pos >= (double)len) pos -= (double)len;
    int idx = (int)pos;
    float frac = (float)(pos - (double)idx);
    float val = osc->wt->table[idx] * (1.0f - frac)
              + osc->wt->table[idx + 1] * frac;
    osc->phase += osc->phase_inc;
    if (osc->phase >= (double)len) osc->phase -= (double)len;
    return val;
}

#endif /* WAVETABLE_H */
