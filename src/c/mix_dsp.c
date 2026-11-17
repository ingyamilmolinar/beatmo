#include <math.h>
#include <stdlib.h>
#include <string.h>
#include "mix_dsp.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

#define EQ_MAX_STAGES 64
#define EQ_MAX_BANDS 64
#define EQ_MAX_FILTERS 8
/* Samples processed per pass over the stages; the filters are per sample,
 * so the chunk size never changes a result. */
#define EQ_CHUNK 256

typedef struct {
    double b0, b1, b2, a1, a2;
    double x1, x2, y1, y2;
} eq_bq;

typedef struct {
    int muted;
    float gain;
    int nf;
    eq_bq f[EQ_MAX_FILTERS];
} eq_band;

typedef struct {
    int kind; /* 0 biquad, 1 split, 2 silence */
    eq_bq q;
    int nb;
    eq_band *b;
} eq_stage;

typedef struct {
    int ns;
    eq_stage s[EQ_MAX_STAGES];
    float cur[EQ_CHUNK];
    float src[EQ_CHUNK];
    float tmp[EQ_CHUNK];
} eq_t;

/* Direct Form I, double state, float in/out — biquad.ProcessBlockBuf. */
static void bq_run(eq_bq *q, const float *in, float *out, int n) {
    double b0 = q->b0, b1 = q->b1, b2 = q->b2, a1 = q->a1, a2 = q->a2;
    double x1 = q->x1, x2 = q->x2, y1 = q->y1, y2 = q->y2;
    const double denorm = 1e-20;
    for (int i = 0; i < n; i++) {
        double x = (double)in[i];
        double y = b0 * x + b1 * x1 + b2 * x2 - a1 * y1 - a2 * y2;
        if (y > -denorm && y < denorm) y = 0;
        x2 = x1; x1 = x;
        y2 = y1; y1 = y;
        out[i] = (float)y;
    }
    q->x1 = x1; q->x2 = x2; q->y1 = y1; q->y2 = y2;
}

static void bq_coeffs(eq_bq *q, const double *c) {
    q->b0 = c[0]; q->b1 = c[1]; q->b2 = c[2]; q->a1 = c[3]; q->a2 = c[4];
}

/* Parse prog into stages. With into == 0 it only validates; with keep != 0
 * it requires the shape already in `into` and swaps values only. Returns 1 on
 * success. */
static int eq_parse(eq_t *into, const double *p, int n, int keep) {
    int k = 0;
#define NEXT(v) do { if (k >= n) return 0; (v) = p[k++]; } while (0)
    double d;
    NEXT(d);
    int ns = (int)d;
    if (ns < 0 || ns > EQ_MAX_STAGES || (double)ns != d) return 0;
    if (keep && into->ns != ns) return 0;
    for (int s = 0; s < ns; s++) {
        NEXT(d);
        int kind = (int)d;
        if (keep && into->s[s].kind != kind) return 0;
        if (kind == 0) {
            if (k + 5 > n) return 0;
            if (into) bq_coeffs(&into->s[s].q, p + k);
            k += 5;
        } else if (kind == 1) {
            NEXT(d);
            int nb = (int)d;
            if (nb < 1 || nb > EQ_MAX_BANDS) return 0;
            if (keep && into->s[s].nb != nb) return 0;
            if (into && !keep) {
                into->s[s].b = calloc((size_t)nb, sizeof(eq_band));
                if (!into->s[s].b) return 0;
                into->s[s].nb = nb;
            }
            for (int b = 0; b < nb; b++) {
                double muted, gain, nfd;
                NEXT(muted);
                NEXT(gain);
                NEXT(nfd);
                int nf = (int)nfd;
                if (nf < 0 || nf > EQ_MAX_FILTERS || k + 5 * nf > n) return 0;
                if (keep && (into->s[s].b[b].nf != nf || into->s[s].b[b].muted != (muted != 0))) return 0;
                if (into) {
                    eq_band *bd = &into->s[s].b[b];
                    bd->muted = muted != 0;
                    bd->gain = (float)gain;
                    bd->nf = nf;
                    for (int f = 0; f < nf; f++) bq_coeffs(&bd->f[f], p + k + 5 * f);
                }
                k += 5 * nf;
            }
        } else if (kind != 2) {
            return 0;
        }
        if (into && !keep) into->s[s].kind = kind;
    }
    if (into && !keep) into->ns = ns;
#undef NEXT
    return k == n;
}

static void eq_free_stages(eq_t *e) {
    for (int s = 0; s < EQ_MAX_STAGES; s++) {
        free(e->s[s].b);
        e->s[s].b = 0;
    }
}

EXPORT void *eq_create(const double *prog, int n) {
    if (!prog || n < 1 || !eq_parse(0, prog, n, 0)) return 0;
    eq_t *e = calloc(1, sizeof(eq_t));
    if (!e) return 0;
    if (!eq_parse(e, prog, n, 0)) {
        eq_free_stages(e);
        free(e);
        return 0;
    }
    return e;
}

EXPORT int eq_update(void *h, const double *prog, int n) {
    eq_t *e = h;
    if (!e || !prog || n < 1) return 0;
    /* Validate the whole program against the live shape first, so a
     * mismatch half way leaves the EQ untouched. */
    eq_t *probe = calloc(1, sizeof(eq_t));
    if (!probe) return 0;
    int ok = eq_parse(probe, prog, n, 0);
    if (ok) {
        ok = probe->ns == e->ns;
        for (int s = 0; ok && s < e->ns; s++) {
            if (probe->s[s].kind != e->s[s].kind) ok = 0;
            else if (e->s[s].kind == 1) {
                if (probe->s[s].nb != e->s[s].nb) ok = 0;
                for (int b = 0; ok && b < e->s[s].nb; b++) {
                    if (probe->s[s].b[b].nf != e->s[s].b[b].nf || probe->s[s].b[b].muted != e->s[s].b[b].muted) ok = 0;
                }
            }
        }
    }
    eq_free_stages(probe);
    free(probe);
    if (!ok) return 0;
    return eq_parse(e, prog, n, 1);
}

static void eq_split(eq_t *e, eq_stage *st, int n) {
    memcpy(e->src, e->cur, (size_t)n * sizeof(float));
    for (int i = 0; i < n; i++) e->cur[i] = 0;
    for (int b = 0; b < st->nb; b++) {
        eq_band *bd = &st->b[b];
        if (bd->muted) continue;
        memcpy(e->tmp, e->src, (size_t)n * sizeof(float));
        for (int f = 0; f < bd->nf; f++) bq_run(&bd->f[f], e->tmp, e->tmp, n);
        float g = bd->gain;
        for (int i = 0; i < n; i++) e->cur[i] += e->tmp[i] * g;
    }
}

EXPORT void eq_run(void *h, const float *in, float *out, int n) {
    eq_t *e = h;
    if (!e) return;
    for (int off = 0; off < n; off += EQ_CHUNK) {
        int m = n - off < EQ_CHUNK ? n - off : EQ_CHUNK;
        memcpy(e->cur, in + off, (size_t)m * sizeof(float));
        for (int s = 0; s < e->ns; s++) {
            eq_stage *st = &e->s[s];
            if (st->kind == 0) bq_run(&st->q, e->cur, e->cur, m);
            else if (st->kind == 1) eq_split(e, st, m);
            else for (int i = 0; i < m; i++) e->cur[i] = 0;
        }
        memcpy(out + off, e->cur, (size_t)m * sizeof(float));
    }
}

static void bq_clear(eq_bq *q) { q->x1 = q->x2 = q->y1 = q->y2 = 0; }

EXPORT void eq_clear(void *h) {
    eq_t *e = h;
    if (!e) return;
    for (int s = 0; s < e->ns; s++) {
        bq_clear(&e->s[s].q);
        for (int b = 0; b < e->s[s].nb; b++) {
            for (int f = 0; f < e->s[s].b[b].nf; f++) bq_clear(&e->s[s].b[b].f[f]);
        }
    }
}

EXPORT void eq_destroy(void *h) {
    eq_t *e = h;
    if (!e) return;
    eq_free_stages(e);
    free(e);
}

EXPORT double soft_clip(double x, double t) {
    if (x != x) return 0; /* a NaN never reaches the device */
    if (t >= 1) return x > 1 ? 1 : (x < -1 ? -1 : x);
    double a = fabs(x);
    if (a <= t) return x;
    double y = t + (1 - t) * tanh((a - t) / (1 - t));
    return x < 0 ? -y : y;
}

EXPORT void soft_clip_run(const float *in, float *out, int n, double t) {
    for (int i = 0; i < n; i++) out[i] = (float)soft_clip((double)in[i], t);
}

EXPORT void soft_clip_run_d(const double *in, double *out, int n, double t) {
    for (int i = 0; i < n; i++) out[i] = soft_clip(in[i], t);
}
