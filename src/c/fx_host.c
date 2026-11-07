#include <stdlib.h>
#include <string.h>
#include "insert_fx.h"
#include "fx_host.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* buf_len is the float buffer the effect's init takes (0 = none), the sizes desktop always allocated, so an effect
 * behaves exactly as before. */
typedef struct {
    const char *type;
    const char *params;
    int nparams;
    size_t state_size;
    int (*buf_len)(int sr);
    void (*init)(void *st, int sr, float *buf, int buf_len, const float *v);
    void (*process)(void *st, const float *in, float *out, int n);
    void (*reset)(void *st);
    void (*set)(void *st, const char *name, float v);
} fx_kind;

static double fx_sr(int sr) { return sr > 0 ? (double)sr : 44100.0; }

static int buf_delay(int sr) { return sr > 0 ? sr : 44100; }
static int buf_reverb(int sr) { return ifx_reverb_mem_size(sr); }
static int buf_chorus(int sr) { return (int)(20.0 * 0.001 * fx_sr(sr) * 2) + 64; }
static int buf_flanger(int sr) { return (int)(0.02 * fx_sr(sr)) + 64; }
static int buf_tape(int sr) { return (int)(0.01 * fx_sr(sr)) + 64; }
static int buf_pitchshift(int sr) { return (int)(0.15 * fx_sr(sr)) + 64; }

#define FX_ADAPT(name, T) \
    static void process_##name(void *st, const float *in, float *out, int n) { ifx_##name##_process((T *)st, in, out, n); } \
    static void reset_##name(void *st) { ifx_##name##_reset((T *)st); } \
    static void set_##name(void *st, const char *k, float v) { ifx_##name##_set_param((T *)st, k, v); }

FX_ADAPT(distortion, ifx_distortion_t)
FX_ADAPT(delay, ifx_delay_t)
FX_ADAPT(reverb, ifx_reverb_t)
FX_ADAPT(chorus, ifx_chorus_t)
FX_ADAPT(bitcrusher, ifx_bitcrusher_t)
FX_ADAPT(filter, ifx_filter_t)
FX_ADAPT(waveshaper, ifx_waveshaper_t)
FX_ADAPT(ringmod, ifx_ringmod_t)
FX_ADAPT(tremolo, ifx_tremolo_t)
FX_ADAPT(gate, ifx_gate_t)
FX_ADAPT(limiter, ifx_limiter_t)
FX_ADAPT(flanger, ifx_flanger_t)
FX_ADAPT(phaser, ifx_phaser_t)
FX_ADAPT(autowah, ifx_autowah_t)
FX_ADAPT(compressor, ifx_compressor_t)
FX_ADAPT(transient, ifx_transient_t)
FX_ADAPT(tape, ifx_tape_t)
FX_ADAPT(pitchshift, ifx_pitchshift_t)

static void init_distortion(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_distortion_init(st, sr, v[0], v[1], v[2]); }
static void init_delay(void *st, int sr, float *b, int bl, const float *v) { ifx_delay_init(st, sr, b, bl, v[0], v[1], v[2]); }
static void init_reverb(void *st, int sr, float *b, int bl, const float *v) { (void)bl; ifx_reverb_init(st, sr, b, v[0], v[1], v[2]); }
static void init_chorus(void *st, int sr, float *b, int bl, const float *v) { ifx_chorus_init(st, sr, b, bl, v[0], v[1], v[2]); }
static void init_bitcrusher(void *st, int sr, float *b, int bl, const float *v) { (void)sr; (void)b; (void)bl; ifx_bitcrusher_init(st, v[0], v[1], v[2]); }
static void init_filter(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_filter_init(st, sr, v[0], v[1], v[2], v[3]); }
static void init_waveshaper(void *st, int sr, float *b, int bl, const float *v) { (void)sr; (void)b; (void)bl; ifx_waveshaper_init(st, v[0], v[1], v[2]); }
static void init_ringmod(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_ringmod_init(st, sr, v[0], v[1], v[2]); }
static void init_tremolo(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_tremolo_init(st, sr, v[0], v[1], v[2], v[3]); }
static void init_gate(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_gate_init(st, sr, v[0], v[1], v[2], v[3]); }
static void init_limiter(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_limiter_init(st, sr, v[0], v[1], v[2]); }
static void init_flanger(void *st, int sr, float *b, int bl, const float *v) { ifx_flanger_init(st, sr, b, bl, v[0], v[1], v[2], v[3]); }
static void init_phaser(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_phaser_init(st, sr, v[0], v[1], v[2], v[3], v[4]); }
static void init_autowah(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_autowah_init(st, sr, v[0], v[1], v[2], v[3]); }
static void init_compressor(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_compressor_init(st, sr, v[0], v[1], v[2], v[3], v[4], v[5]); }
static void init_transient(void *st, int sr, float *b, int bl, const float *v) { (void)b; (void)bl; ifx_transient_init(st, sr, v[0], v[1], v[2]); }
static void init_tape(void *st, int sr, float *b, int bl, const float *v) { ifx_tape_init(st, sr, b, bl, v[0], v[1], v[2], v[3], v[4]); }
static void init_pitchshift(void *st, int sr, float *b, int bl, const float *v) { ifx_pitchshift_init(st, sr, b, bl, v[0], v[1], v[2]); }

#define FX_ROW(name, T, params, np, buf) \
    { #name, params, np, sizeof(T), buf, init_##name, process_##name, reset_##name, set_##name }

static const fx_kind fx_kinds[] = {
    FX_ROW(distortion, ifx_distortion_t, "drive,tone,mix", 3, 0),
    FX_ROW(delay, ifx_delay_t, "time,feedback,mix", 3, buf_delay),
    FX_ROW(reverb, ifx_reverb_t, "room,damping,mix", 3, buf_reverb),
    FX_ROW(chorus, ifx_chorus_t, "rate,depth,mix", 3, buf_chorus),
    FX_ROW(bitcrusher, ifx_bitcrusher_t, "bits,rate,mix", 3, 0),
    FX_ROW(filter, ifx_filter_t, "mode,cutoff,q,mix", 4, 0),
    FX_ROW(waveshaper, ifx_waveshaper_t, "curve,drive,mix", 3, 0),
    FX_ROW(ringmod, ifx_ringmod_t, "frequency,shape,mix", 3, 0),
    FX_ROW(tremolo, ifx_tremolo_t, "rate,depth,shape,mix", 4, 0),
    FX_ROW(gate, ifx_gate_t, "threshold,attack,release,range", 4, 0),
    FX_ROW(limiter, ifx_limiter_t, "threshold,release,ceiling", 3, 0),
    FX_ROW(flanger, ifx_flanger_t, "rate,depth,feedback,mix", 4, buf_flanger),
    FX_ROW(phaser, ifx_phaser_t, "stages,rate,depth,feedback,mix", 5, 0),
    FX_ROW(autowah, ifx_autowah_t, "sensitivity,rate,depth,mix", 4, 0),
    FX_ROW(compressor, ifx_compressor_t, "threshold,ratio,attack,release,makeup,mix", 6, 0),
    FX_ROW(transient, ifx_transient_t, "attack,sustain,speed", 3, 0),
    FX_ROW(tape, ifx_tape_t, "drive,warmth,wow,flutter,mix", 5, buf_tape),
    FX_ROW(pitchshift, ifx_pitchshift_t, "pitch,mix,window", 3, buf_pitchshift),
};

#define FX_KIND_COUNT ((int)(sizeof(fx_kinds) / sizeof(fx_kinds[0])))

/* The effect state follows it (aligned), then the buffer. */
typedef struct {
    const fx_kind *k;
    void *state;
    float *buf;
} fx_handle;

static const fx_kind *fx_find(const char *type) {
    if (!type) return 0;
    for (int i = 0; i < FX_KIND_COUNT; i++) {
        if (strcmp(fx_kinds[i].type, type) == 0) return &fx_kinds[i];
    }
    return 0;
}

static size_t fx_align(size_t n) { return (n + 15) & ~(size_t)15; }

EXPORT const char *ifx_param_names(const char *type) {
    const fx_kind *k = fx_find(type);
    return k ? k->params : "";
}

EXPORT void *ifx_create(const char *type, int sr, const float *values, int n) {
    const fx_kind *k = fx_find(type);
    if (!k || n != k->nparams || (n > 0 && !values)) return 0;
    int bl = k->buf_len ? k->buf_len(sr) : 0;
    if (bl < 0) return 0;
    size_t head = fx_align(sizeof(fx_handle));
    size_t st = fx_align(k->state_size);
    unsigned char *mem = calloc(1, head + st + (size_t)bl * sizeof(float));
    if (!mem) return 0;
    fx_handle *h = (fx_handle *)mem;
    h->k = k;
    h->state = mem + head;
    h->buf = bl > 0 ? (float *)(mem + head + st) : 0;
    k->init(h->state, sr, h->buf, bl, values);
    return h;
}

EXPORT void ifx_run(void *hp, const float *in, float *out, int n) {
    fx_handle *h = hp;
    if (!h || n <= 0) return;
    h->k->process(h->state, in, out, n);
}

EXPORT void ifx_set(void *hp, const char *name, float value) {
    fx_handle *h = hp;
    if (!h || !name) return;
    h->k->set(h->state, name, value);
}

EXPORT void ifx_clear(void *hp) {
    fx_handle *h = hp;
    if (!h) return;
    h->k->reset(h->state);
}

EXPORT void ifx_destroy(void *hp) { free(hp); }
