#include <math.h>
#include <string.h>
#include "wavetable.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

#define TWO_PI (2.0 * M_PI)

/* ── Waveform generation ─────────────────────────────────────────────────── */

EXPORT void wt_generate_sine(wavetable_t *wt, float *buf, int length) {
    wt->table  = buf;
    wt->length = length;
    double inc = TWO_PI / (double)length;
    for (int i = 0; i < length; i++) {
        buf[i] = (float)sin(inc * (double)i);
    }
    /* Guard point */
    buf[length] = buf[0];
}

EXPORT void wt_generate_saw(wavetable_t *wt, float *buf, int length, int harmonics) {
    wt->table  = buf;
    wt->length = length;
    memset(buf, 0, (size_t)(length + 1) * sizeof(float));

    /* Fourier series: saw(x) = (2/π) * Σ (-1)^(n+1) * sin(n*x) / n */
    for (int h = 1; h <= harmonics; h++) {
        double sign = (h % 2 == 0) ? -1.0 : 1.0;
        double amp  = sign * 2.0 / (M_PI * (double)h);
        double inc  = TWO_PI * (double)h / (double)length;
        for (int i = 0; i < length; i++) {
            buf[i] += (float)(amp * sin(inc * (double)i));
        }
    }
    buf[length] = buf[0];
}

EXPORT void wt_generate_square(wavetable_t *wt, float *buf, int length, int harmonics) {
    wt->table  = buf;
    wt->length = length;
    memset(buf, 0, (size_t)(length + 1) * sizeof(float));

    /* Fourier series: square(x) = (4/π) * Σ sin((2n-1)*x) / (2n-1) */
    for (int h = 1; h <= harmonics; h++) {
        int n = 2 * h - 1; /* odd harmonics only */
        double amp = 4.0 / (M_PI * (double)n);
        double inc = TWO_PI * (double)n / (double)length;
        for (int i = 0; i < length; i++) {
            buf[i] += (float)(amp * sin(inc * (double)i));
        }
    }
    buf[length] = buf[0];
}

EXPORT void wt_generate_triangle(wavetable_t *wt, float *buf, int length, int harmonics) {
    wt->table  = buf;
    wt->length = length;
    memset(buf, 0, (size_t)(length + 1) * sizeof(float));

    /* Fourier series: tri(x) = (8/π²) * Σ (-1)^n * sin((2n+1)*x) / (2n+1)² */
    for (int h = 0; h < harmonics; h++) {
        int n = 2 * h + 1; /* odd harmonics */
        double sign = (h % 2 == 0) ? 1.0 : -1.0;
        double amp  = sign * 8.0 / (M_PI * M_PI * (double)n * (double)n);
        double inc  = TWO_PI * (double)n / (double)length;
        for (int i = 0; i < length; i++) {
            buf[i] += (float)(amp * sin(inc * (double)i));
        }
    }
    buf[length] = buf[0];
}

/* ── Oscillator functions ────────────────────────────────────────────────── */

EXPORT void wt_osc_init(wt_osc_t *osc, const wavetable_t *wt,
                          double freq, int sampleRate) {
    osc->wt    = wt;
    osc->phase = 0;
    /* phase_inc = (freq / sampleRate) * tableLength */
    if (sampleRate > 0) {
        osc->phase_inc = freq * (double)wt->length / (double)sampleRate;
    } else {
        osc->phase_inc = 0;
    }
}

EXPORT void wt_osc_set_freq(wt_osc_t *osc, double freq, int sampleRate) {
    if (sampleRate > 0 && osc->wt) {
        osc->phase_inc = freq * (double)osc->wt->length / (double)sampleRate;
    }
}

EXPORT void wt_osc_set_phase(wt_osc_t *osc, double phase01) {
    if (!osc->wt) return;
    double len = (double)osc->wt->length;
    double p = phase01 - floor(phase01); /* wrap into [0,1) */
    osc->phase = p * len;
    if (osc->phase >= len) osc->phase -= len; /* guard floating edge */
    if (osc->phase < 0.0) osc->phase = 0.0;
}

EXPORT void wt_osc_process(wt_osc_t *osc, float *out, int samples) {
    const float  *table = osc->wt->table;
    int           len   = osc->wt->length;
    double        phase = osc->phase;
    double        inc   = osc->phase_inc;
    double        dlen  = (double)len;

    for (int i = 0; i < samples; i++) {
        int idx = (int)phase;
        float frac = (float)(phase - (double)idx);
        out[i] = table[idx] * (1.0f - frac) + table[idx + 1] * frac;
        phase += inc;
        if (phase >= dlen) phase -= dlen;
    }
    osc->phase = phase;
}

EXPORT void wt_osc_process_fm(wt_osc_t *osc, const float *freq_mod,
                                float *out, int samples, int sampleRate) {
    const float  *table = osc->wt->table;
    int           len   = osc->wt->length;
    double        phase = osc->phase;
    double        base_inc = osc->phase_inc;
    double        dlen  = (double)len;
    double        sr    = (double)sampleRate;

    for (int i = 0; i < samples; i++) {
        int idx = (int)phase;
        float frac = (float)(phase - (double)idx);
        out[i] = table[idx] * (1.0f - frac) + table[idx + 1] * frac;
        /* Modulate phase increment by freq_mod */
        double mod_inc = (double)freq_mod[i] * dlen / sr;
        phase += base_inc + mod_inc;
        while (phase >= dlen) phase -= dlen;
        while (phase < 0)    phase += dlen;
    }
    osc->phase = phase;
}
