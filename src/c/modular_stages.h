#ifndef MODULAR_STAGES_H
#define MODULAR_STAGES_H

#include "modular.h"

#define MODULAR_GEN_SLOTS 12

/* ── RBJ biquad (LP/HP/BP), shared between modular.c's FILTER stage and the
 * gen-bank slot filters. Moved here verbatim from modular.c (de-static'd, the
 * inline tick renamed mod_biquad_tick_f) so both consumers use the IDENTICAL
 * math — the modular FILTER stage keeps its exact bytes. ── */
typedef struct {
    float b0, b1, b2, a1, a2;
    float x1, x2, y1, y2;
} mod_biquad;

void mod_biquad_set(mod_biquad *f, int type, double cutoff, double q, int sr);
/* Update coefficients WITHOUT resetting the delay line (x1/x2/y1/y2). For
 * per-block cutoff modulation where zeroing state would click. */
void mod_biquad_set_coeffs(mod_biquad *f, int type, double cutoff, double q, int sr);
float mod_biquad_tick_f(mod_biquad *f, float x);

/* Renders the gen bank and ACCUMULATES into out[0..samples-1] (slot-index
 * sum order — byte-identity rule from the unification spec §2). voice_freq
 * is the already-computed modular voice frequency (Hz); seed is the decoded
 * noise_seed. No-op when every slot's source is 0.
 *
 * osc_wrote: 1 if the legacy osc stage ASSIGNED into out[] before this call.
 * When 0, the FIRST active slot ASSIGNS (out[i] = v) instead of accumulating,
 * so a legacy -0.0 sample is not flipped to +0.0 by `0.0f + (-0.0f)`
 * (byte-identity hazard, spec §2 / plan Task-1). */
void modular_gen_bank_render(float *out, int sampleRate, int samples,
                             const modular_params *p, double voice_freq,
                             unsigned int seed, int osc_wrote);

/* Phase-15 FORMANT stage: parallel 5-band vowel bandpass bank + singer's
 * formant. Runs between the static filter and the body resonator. Exact no-op
 * when mix <= 0 (caller also gates on formant_enabled).
 * Phase-16: `dry` (0..1) scales the stage's dry-blend amount; dry=1 (identity)
 * reproduces the pre-Phase-16 dryAmt exactly. */
void modular_formant_process(float *out, int sampleRate, int samples,
                             int voice_type, float vowel, float mix,
                             float shift, float sing,
                             float morph_rate, float morph_to, float dry);

#endif /* MODULAR_STAGES_H */
