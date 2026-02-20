#include "pan.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

EXPORT void pan_process_mono_to_lr(const pan_t *p, const float *in,
                                    float *left, float *right, int samples) {
    float gl = p->gain_l;
    float gr = p->gain_r;
    for (int i = 0; i < samples; i++) {
        float s = in[i];
        left[i]  += s * gl;
        right[i] += s * gr;
    }
}

EXPORT void pan_process_mono_to_interleaved(const pan_t *p, const float *in,
                                             float *out, int samples) {
    float gl = p->gain_l;
    float gr = p->gain_r;
    for (int i = 0; i < samples; i++) {
        float s = in[i];
        out[i * 2]     = s * gl;
        out[i * 2 + 1] = s * gr;
    }
}
