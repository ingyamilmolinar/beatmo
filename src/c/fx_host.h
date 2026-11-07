#ifndef FX_HOST_H
#define FX_HOST_H

/* ONE way to build and run any insert effect on every platform (cgo on desktop, the beatmo-dsp worklet in the browser).
 * The table owns each effect's init-argument order and buffer and state sizes; callers pass the COMPLETE parameter set
 * in ifx_param_names() order. */

/* "" for an unknown type. */
const char *ifx_param_names(const char *type);

/* One allocation; 0 when the type is unknown, n does not match, or memory ran out. */
void *ifx_create(const char *type, int sr, const float *values, int n);

/* Process n samples in → out (separate buffers). */
void ifx_run(void *h, const float *in, float *out, int n);

/* The effect's own smoothing applies; its state (delay lines, envelopes, LFO phases) is kept. */
void ifx_set(void *h, const char *name, float value);

/* Clear every delay line / envelope / phase, as if freshly created. */
void ifx_clear(void *h);

void ifx_destroy(void *h);

#endif /* FX_HOST_H */
