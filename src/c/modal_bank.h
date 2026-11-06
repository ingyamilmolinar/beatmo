#ifndef MODAL_BANK_H
#define MODAL_BANK_H

#include "modular_params.gen.h"

/* WAV-clone stages (spec docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md).
 * Both ADD into out (render_modular_p zeroes it first) and are exact no-ops
 * while their stage is disabled. */

/* MODAL (+ STRIKE): up to MODULAR_MODAL_FREQ_ROWS phasor resonators,
 * x_i(t) = A_i e^{-alpha_i t} cos(2 pi f_i t + phi_i), with f_i, alpha_i and
 * A_i evaluated at the mode's ACTUAL frequency (ratio x voice freq, tune,
 * stretch): alpha_i = decay_i * decay_x * (a (f/1k)^p + b (f/10k)^6), and
 * A_i = amp_i * bright tilt * (strike on: S(f; tc) * hardness tilt). */
void modular_modal_render(float *out, int sampleRate, int samples,
                          const modular_params *p, double voice_freq, unsigned int seed);

/* NOISEBAND: up to MODULAR_NB_CENTER_ROWS band-passed noise rows sharing one
 * seeded source, each with a raised-cosine attack then a two-rate decay; a
 * row's level is its RMS in dB after the attack. */
void modular_noiseband_render(float *out, int sampleRate, int samples,
                              const modular_params *p, unsigned int seed);

/* The smooth envelope of |F(f)| of the force 0.5(1 - cos(2 pi t/tc)) on
 * [0, tc]: 1/sqrt(1 + (pi (f tc)^3)^2) -- 1 at DC, -18 dB/oct, no nulls. */
double modal_strike_spectrum(double f, double tc);

#endif
