#ifndef WAVETABLE_H
#define WAVETABLE_H

/* The guard point table[length] == table[0] removes a branch from linear interpolation; 4096 entries give < 0.01 % THD. */

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

#define WT_DEFAULT_LENGTH 4096

/* The buffer is caller-owned, at least length+1 floats. */
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

/* 1/n² harmonics (-12 dB/oct), the classic vocal source for formant synthesis. */
void wt_generate_glottal(wavetable_t *wt, float *buf, int length, int harmonics);

/* ── Oscillator functions ────────────────────────────────────── */

/* Initialize oscillator for a given frequency and sample rate. */
void wt_osc_init(wt_osc_t *osc, const wavetable_t *wt,
                  double freq, int sampleRate);

/* Phase-preserving frequency change. */
void wt_osc_set_freq(wt_osc_t *osc, double freq, int sampleRate);

/* Values outside [0,1) wrap. Only for a non-zero start phase. */
void wt_osc_set_phase(wt_osc_t *osc, double phase01);

/* Single-sample tick (inline for use in tight inner loops). */
static inline float wt_osc_tick(wt_osc_t *osc) {
    double pos = osc->phase;
    int idx = (int)pos;
    float frac = (float)(pos - (double)idx);
    float val = osc->wt->table[idx] * (1.0f - frac)
              + osc->wt->table[idx + 1] * frac;
    osc->phase += osc->phase_inc;
    int len = osc->wt->length;
    /* Per-sample frequency modulation can push phase_inc past len, and a single subtract then leaves the index out of
     * bounds (a SIGSEGV). The loop is byte-identical below Nyquist. */
    while (osc->phase >= (double)len) osc->phase -= (double)len;
    while (osc->phase < 0.0) osc->phase += (double)len;
    return val;
}

/* phase_mod in table units, not radians. */
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
