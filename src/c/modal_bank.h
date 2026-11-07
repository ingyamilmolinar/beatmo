#ifndef MODAL_BANK_H
#define MODAL_BANK_H

#include "modular_params.gen.h"

/* Both ADD into out (render_modular_p zeroes it first) and are exact no-ops while disabled. */

/* α_i = decay_i·decay_x·(a(f/1k)^p + b(f/10k)^6) and A_i = amp_i·tilt·(strike: S(f; tc)·hardness), all at the
 * mode's ACTUAL frequency. */
void modular_modal_render(float *out, int sampleRate, int samples,
                          const modular_params *p, double voice_freq, unsigned int seed);

/* One seeded source; a raised-cosine attack then a two-rate decay; a row's level is its RMS in dB after the attack. */
void modular_noiseband_render(float *out, int sampleRate, int samples,
                              const modular_params *p, unsigned int seed);

/* Of the force 0.5(1 − cos(2πt/tc)) on [0, tc]: 1/√(1 + (π(f·tc)³)²), 1 at DC, -18 dB/oct, no nulls. */
double modal_strike_spectrum(double f, double tc);

#endif
