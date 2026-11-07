#ifndef NOISE_H
#define NOISE_H

/* An integer LCG with an exact integer→float conversion, so emcc and native compilers produce a BIT-IDENTICAL stream;
 * ma_noise is not guaranteed identical across the two toolchains. A fixed seed is reproducible; distinct seeds vary. */

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

/* A bit-identical transplant of ma_noise's white stream (Lehmer 48271, 2^31-1; seed 0 becomes 4321). It
 * reproduces miniaudio's int32 overflow on purpose (state may go negative): do not "fix" it. */
typedef struct {
    int state; /* ma_int32 */
} noise_ma_gen;

void  noise_ma_init(noise_ma_gen *g, int seed);
float noise_ma_white_tick(noise_ma_gen *g);
/* Bulk fill helper (also the CGo seam for tests). */
void  noise_ma_white_fill(float *out, int n, int seed);

#endif /* NOISE_H */
