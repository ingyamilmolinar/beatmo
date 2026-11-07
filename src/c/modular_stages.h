#ifndef MODULAR_STAGES_H
#define MODULAR_STAGES_H

#include "modular.h"

/* The slot counts come from the generated modular_params.gen.h. */

/* Shared by the FILTER stage and the slot filters, so both use the IDENTICAL math. */
typedef struct {
    float b0, b1, b2, a1, a2;
    float x1, x2, y1, y2;
} mod_biquad;

void mod_biquad_set(mod_biquad *f, int type, double cutoff, double q, int sr);
/* Keeps the delay line: zeroing it on per-block cutoff modulation would click. */
void mod_biquad_set_coeffs(mod_biquad *f, int type, double cutoff, double q, int sr);
float mod_biquad_tick_f(mod_biquad *f, float x);

/* ACCUMULATES in slot-index order; a no-op when every source is 0. osc_wrote 0 makes the FIRST active slot ASSIGN, so
 * 0.0f + (-0.0f) cannot flip a -0.0 sample to +0.0. */
void modular_gen_bank_render(float *out, int sampleRate, int samples,
                             const modular_params *p, double voice_freq,
                             unsigned int seed, int osc_wrote);

/* Five vowel band-passes plus the singer's formant, between the filter and the body resonator; an exact no-op at
 * mix ≤ 0. dry (0..1) scales the dry blend. */
void modular_formant_process(float *out, int sampleRate, int samples,
                             int voice_type, double vowel, double mix,
                             double shift, double sing,
                             double morph_rate, double morph_to, double dry);

#endif /* MODULAR_STAGES_H */
