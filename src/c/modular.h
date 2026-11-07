#ifndef MODULAR_H
#define MODULAR_H

/* The one voice every instrument renders through, driven by modular_params (GENERATED from modular_fields.go, the only
 * block crossing the ABI). A note renders once per edit and is cached, so engine cost is paid at edit time; 0 is a real
 * value, there are no engine defaults. */

#include "modular_params.gen.h"

/* Every param is read as given: a pure function of the block (NULL renders silence). */
int modular_param_count(void);
void render_modular_p(float *out, int sampleRate, int samples,
                      const modular_params *params);

/* A test seam: a filter is measured with a KNOWN signal, which a voice is not. render_modular_p already runs it. */
void modular_finish_apply(float *buf, int sampleRate, int samples,
                          const modular_params *params);

#endif /* MODULAR_H */
