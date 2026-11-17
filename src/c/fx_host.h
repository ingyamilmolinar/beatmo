#ifndef FX_HOST_H
#define FX_HOST_H

/* ================================================================
 * fx_host — ONE way to build and run any insert effect, on every platform.
 *
 * Desktop calls it through cgo (insert_fx_c.go), the browser through
 * drums.single.js inside the beatmo-dsp AudioWorklet (dsp_host.js). The
 * table in fx_host.c owns everything that used to be repeated per platform:
 * each effect's init-argument order, its delay-buffer size and its state
 * size. Callers pass the effect's COMPLETE parameter set (Go fills the
 * defaults) as floats in ifx_param_names() order.
 * ================================================================ */

/* Comma-separated parameter names of type, in the order ifx_create reads
 * them ("" for an unknown type). */
const char *ifx_param_names(const char *type);

/* A new effect instance (state + any buffer it needs, one allocation), or 0
 * when the type is unknown, n does not match the parameter count, or memory
 * ran out. */
void *ifx_create(const char *type, int sr, const float *values, int n);

/* Process n samples in → out (separate buffers). */
void ifx_run(void *h, const float *in, float *out, int n);

/* Live retune of one parameter: the effect's own smoothing applies and its
 * state (delay lines, envelopes, LFO phases) is kept. */
void ifx_set(void *h, const char *name, float value);

/* Clear every delay line / envelope / phase, as if freshly created. */
void ifx_clear(void *h);

void ifx_destroy(void *h);

#endif /* FX_HOST_H */
