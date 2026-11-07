#include "noise.h"

/* Full 2^32 period; unsigned arithmetic wraps the same on every C implementation. */
#define NOISE_LCG_A 1664525u
#define NOISE_LCG_C 1013904223u

void noise_init(noise_gen *g, unsigned int seed) {
    /* So adjacent variant seeds give decorrelated streams, not nearly identical ones. */
    g->state = seed * 2654435761u + 0x9E3779B9u;
    g->b0 = g->b1 = g->b2 = g->b3 = g->b4 = g->b5 = g->b6 = 0.0f;
}

float noise_white_tick(noise_gen *g) {
    g->state = g->state * NOISE_LCG_A + NOISE_LCG_C;
    /* 2^24 is exact in a float, so the cast is lossless and identical across platforms. */
    unsigned int u = g->state >> 8;
    return (float)u * (1.0f / 8388608.0f) - 1.0f; /* u / 2^23 - 1 */
}

float noise_pink_tick(noise_gen *g) {
    float white = noise_white_tick(g);
    /* Paul Kellet's refined economy pink-noise filter. */
    g->b0 = 0.99886f * g->b0 + white * 0.0555179f;
    g->b1 = 0.99332f * g->b1 + white * 0.0750759f;
    g->b2 = 0.96900f * g->b2 + white * 0.1538520f;
    g->b3 = 0.86650f * g->b3 + white * 0.3104856f;
    g->b4 = 0.55000f * g->b4 + white * 0.5329522f;
    g->b5 = -0.7616f * g->b5 - white * 0.0168980f;
    float pink = g->b0 + g->b1 + g->b2 + g->b3 + g->b4 + g->b5 + g->b6 + white * 0.5362f;
    g->b6 = white * 0.115926f;
    /* Kellet sum has ~9x the amplitude of the white input; trim toward [-1,1]. */
    return pink * 0.11f;
}

/* ── miniaudio-compatible Lehmer white noise (see noise.h) ──────────────── */

#define NOISE_MA_LCG_A 48271
#define NOISE_MA_LCG_M 2147483647
#define NOISE_MA_DEFAULT_SEED 4321 /* MA_DEFAULT_LCG_SEED */

void noise_ma_init(noise_ma_gen *g, int seed) {
    if (seed == 0) seed = NOISE_MA_DEFAULT_SEED; /* ma_noise_config_init rule */
    g->state = seed;
}

float noise_ma_white_tick(noise_ma_gen *g) {
    /* miniaudio's int32 multiply overflows; a defined unsigned multiply reproduces the wrap (two's complement on x86-64
     * and wasm32). C99 % truncates toward zero on negatives, as miniaudio's expression does. */
    int wrapped = (int)((unsigned int)NOISE_MA_LCG_A * (unsigned int)g->state);
    g->state = wrapped % NOISE_MA_LCG_M;
    /* amplitude is 1.0 at every call site. */
    return (float)((double)g->state / (double)NOISE_MA_LCG_M);
}

void noise_ma_white_fill(float *out, int n, int seed) {
    noise_ma_gen g;
    noise_ma_init(&g, seed);
    for (int i = 0; i < n; i++) out[i] = noise_ma_white_tick(&g);
}
