#include <math.h>
#include <stdlib.h>
#include "modal_bank.h"
#include "modular_stages.h" /* mod_biquad */
#include "noise.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* The smooth ENVELOPE of the raised-cosine force spectrum
 * |sinc(x)/(1 - x^2)| (x = f*tc): flat below the corner, the exact
 * -18 dB/oct asymptote 1/(pi x^3) above it, and no nulls. The ideal nulls made
 * the clone's inverse ill-posed (a mode near one needs x1000 storage, and a
 * fit could place every mode on a sidelobe peak); a real mallet has none. */
double modal_strike_spectrum(double f, double tc) {
    double x = f * tc;
    double r = M_PI * x * x * x;
    return 1.0 / sqrt(1.0 + r * r);
}

static int clamp_count(double v, int cap) {
    long n = lrint(v);
    if (n < 0) return 0;
    if (n > cap) return cap;
    return (int)n;
}

void modular_modal_render(float *out, int sampleRate, int samples,
                          const modular_params *p, double voice_freq, unsigned int seed) {
    if (lrint(p->modal_enabled) == 0 || samples <= 0) return;
    int count = clamp_count(p->modal_count, MODULAR_MODAL_FREQ_ROWS);
    int keep = clamp_count(p->modal_keep, MODULAR_MODAL_FREQ_ROWS);
    if (keep < count) count = keep;
    const double sr = (double)sampleRate;
    const int ratio = lrint(p->modal_freq_mode) == 0;
    const double ref = ratio ? voice_freq : 1000.0;
    const double limit = fmin(20000.0, 0.45 * sr);
    const double tune = pow(2.0, p->modal_tune / 1200.0);
    const int strike = lrint(p->strike_enabled) != 0;

    double fr[MODULAR_MODAL_FREQ_ROWS], al[MODULAR_MODAL_FREQ_ROWS], am[MODULAR_MODAL_FREQ_ROWS];
    double bound = 0.0;
    for (int i = 0; i < count; ++i) {
        am[i] = 0.0;
        double f = p->modal_freq[i] * (ratio ? voice_freq : 1.0);
        if (!(f > 0.0)) continue;
        if (p->modal_stretch != 0.0) f *= pow(f / ref, p->modal_stretch);
        f *= tune;
        if (!(f < limit)) continue;
        double law = p->modal_law_a * pow(f / 1000.0, p->modal_law_p)
                   + p->modal_law_b * pow(f / 10000.0, 6.0);
        double alpha = p->modal_decay[i] * p->modal_decay_x * law;
        if (alpha < 0.0) alpha = 0.0;
        double oct = log2(f / 1000.0);
        double a = p->modal_amp[i];
        if (p->modal_bright != 0.0) a *= pow(10.0, p->modal_bright * oct / 20.0);
        if (strike)
            a *= modal_strike_spectrum(f, p->strike_tc) * pow(10.0, p->strike_hardness * oct / 20.0);
        fr[i] = f; al[i] = alpha; am[i] = a;
        bound += fabs(a);
    }
    if (bound > 0.0) {
        const double stop = bound * 1e-5; /* 100 dB under the bank's peak bound */
        for (int i = 0; i < count; ++i) {
            double a = am[i];
            if (fabs(a) <= stop) continue;
            int nLive = samples;
            if (al[i] > 0.0) {
                double t = log(fabs(a) / stop) / al[i] * sr + 1.0;
                if (t < (double)samples) nLive = (int)t;
            }
            double dec = exp(-al[i] / sr);
            double w = 2.0 * M_PI * fr[i] / sr;
            double cr = dec * cos(w), ci = dec * sin(w);
            double re = a * cos(p->modal_phase[i]), im = a * sin(p->modal_phase[i]);
            int att = (int)(p->modal_attack[i] * sr);
            if (att > nLive) att = nLive;
            for (int j = 0; j < nLive; ++j) {
                double v = re;
                if (j < att) v *= 0.5 * (1.0 - cos(M_PI * (double)j / (double)att));
                out[j] += (float)v;
                double nr = re * cr - im * ci;
                im = re * ci + im * cr;
                re = nr;
            }
        }
    }
    if (strike && p->strike_click > 0.0 && bound > 0.0) {
        int nc = (int)(fmax(p->strike_tc, 0.001) * sr);
        if (nc > samples) nc = samples;
        noise_ma_gen g;
        noise_ma_init(&g, (int)(seed ^ 0x5157u));
        double amp = p->strike_click * bound;
        for (int j = 0; j < nc; ++j) {
            double win = 0.5 * (1.0 - cos(2.0 * M_PI * (double)j / (double)nc));
            out[j] += (float)(amp * win * (double)noise_ma_white_tick(&g));
        }
    }
}

void modular_noiseband_render(float *out, int sampleRate, int samples,
                              const modular_params *p, unsigned int seed) {
    if (lrint(p->noiseband_enabled) == 0 || samples <= 0) return;
    int count = clamp_count(p->noiseband_count, MODULAR_NB_CENTER_ROWS);
    if (count == 0) return;
    const double sr = (double)sampleRate;
    float *src = (float *)malloc((size_t)samples * sizeof(float));
    if (!src) return;
    noise_ma_white_fill(src, samples, (int)(seed ^ 0x6e62u));
    /* white uniform [-1,1): sigma = 1/sqrt(3). An RBJ 0 dB-peak band-pass passes
     * a noise-equivalent bandwidth pi*fc/(2Q) of the sr/2 band: output RMS =
     * sigma*sqrt(pi*fc/(Q*sr)). Scale so a row's RMS is its level. */
    const double sigma = 1.0 / sqrt(3.0);
    const double offset = pow(10.0, p->noiseband_level / 20.0);
    for (int r = 0; r < count; ++r) {
        double fc = p->nb_center[r], q = p->nb_q[r];
        if (!(fc > 0.0) || !(q > 0.0) || !(fc < 0.49 * sr)) continue;
        mod_biquad bq;
        mod_biquad_set(&bq, 2, fc, q, sampleRate);
        double scale = offset * pow(10.0, p->nb_level[r] / 20.0) / (sigma * sqrt(M_PI * fc / (q * sr)));
        int att = (int)(p->nb_attack[r] * sr);
        double mix = fmin(fmax(p->nb_fast_mix[r], 0.0), 1.0);
        double dF = exp(-p->nb_fast_rate[r] / sr), dT = exp(-p->nb_tail_rate[r] / sr);
        double eF = mix, eT = 1.0 - mix;
        for (int j = 0; j < samples; ++j) {
            double s = (double)mod_biquad_tick_f(&bq, src[j]);
            double env;
            if (j < att) {
                env = 0.5 * (1.0 - cos(M_PI * (double)j / (double)att));
            } else {
                env = eF + eT;
                /* 100 dB under the row's own level: done (like a mode's stop
                 * rule; a fast row is silent after a few ms and was filtered
                 * to the end of the voice). Deterministic: a function of the
                 * row alone. */
                if (env < 1e-5) break;
                eF *= dF;
                eT *= dT;
            }
            out[j] += (float)(s * env * scale);
        }
    }
    free(src);
}
