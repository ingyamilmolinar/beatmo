#ifndef MODULAR_H
#define MODULAR_H

/* ================================================================
 * Unified modular synth voice.
 *
 * The modular voice assembles a fully user-editable signal path from the
 * shared DSP primitives (every instrument, drums and FM included, renders
 * here):
 *
 *     OSC (selectable shape, incl. FM)  ->  ADSR (amp)  ->  filter  ->  drive/post
 *
 * Every stage is driven by modular_params, so the UI can expose the
 * whole pipeline. The pre-render-and-cache model is unchanged: the
 * whole note is rendered once on a param change and cached, so engine
 * complexity costs only at edit time, never at trigger time.
 *
 * modular_params is the flat param block every instrument renders through
 * (it is the only block that crosses the ABI). It is GENERATED, together with
 * the JS ABI, from src/go/internal/audio/modular_fields.go — see
 * modular_params.gen.h. Discrete choices (osc type, filter type, FM
 * algorithm, amp curve) are integer-valued doubles read via (int)lrint(...).
 * The engine has no defaults of its own: an absent param takes its table
 * Identity on the Go/JS side, before the block reaches C.
 * ================================================================ */

#include "modular_params.gen.h"

/* Render the modular voice. Every param is read from the block as given — a
 * pure function of params (NULL renders silence). */
int modular_param_count(void);
void render_modular_p(float *out, int sampleRate, int samples,
                      const modular_params *params);

/* The FINISH stage alone, in place over a caller's buffer. A test seam: a
 * filter is measured with a KNOWN signal (sines, impulses), and a voice is not
 * one. Not a render entry point — render_modular_p already runs it. */
void modular_finish_apply(float *buf, int sampleRate, int samples,
                          const modular_params *params);

#endif /* MODULAR_H */
