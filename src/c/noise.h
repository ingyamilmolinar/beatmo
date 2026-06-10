#ifndef NOISE_H
#define NOISE_H

/* ================================================================
 * Deterministic noise generators for the modular synth voice.
 *
 * The state is a 32-bit integer LCG, and the integer→float
 * conversion uses only exact IEEE-754 operations, so emcc (WASM)
 * and native gcc/clang produce a BIT-IDENTICAL stream for the same
 * seed. This is why the modular voice does NOT use miniaudio's
 * ma_noise here: ma_noise is not guaranteed identical across the
 * two toolchains, which would break the cross-platform audio parity
 * test (src/js/xplat_audio_compare.browser.test.js).
 *
 * A single render with a fixed seed is reproducible (golden-stable);
 * the voice cache renders three round-robin variants by passing
 * distinct seeds (variant index), so repeated noise hits vary.
 * ================================================================ */

typedef struct {
    unsigned int state;                       /* LCG state */
    float b0, b1, b2, b3, b4, b5, b6;         /* Paul Kellet pink filter state */
} noise_gen;

/* Seed the generator. Distinct seeds produce well-separated streams. */
void noise_init(noise_gen *g, unsigned int seed);

/* White noise sample in [-1, 1). */
float noise_white_tick(noise_gen *g);

/* Pink noise sample (~[-1, 1], 1/f spectrum) via the Paul Kellet filter. */
float noise_pink_tick(noise_gen *g);

/* ── miniaudio-compatible Lehmer white noise ──────────────────────────────
 * Bit-identical transplant of ma_noise's white stream (ma_lcg: A=48271,
 * M=2^31-1, C=0; ma_noise_config_init replaces seed 0 with 4321). The
 * legacy drum renderers were captured against ma_noise with seed 0; their
 * modular-preset replacements MUST consume this stream. Verified against
 * real ma_noise by TestNoiseMaParity (noise_ma_parity_test.go).
 * The LCG step intentionally reproduces miniaudio's int32 multiply
 * overflow (state may go negative); do not "fix" it. */
typedef struct {
    int state; /* ma_int32 */
} noise_ma_gen;

void  noise_ma_init(noise_ma_gen *g, int seed);
float noise_ma_white_tick(noise_ma_gen *g);
/* Bulk fill helper (also the CGo seam for tests). */
void  noise_ma_white_fill(float *out, int n, int seed);

#endif /* NOISE_H */
