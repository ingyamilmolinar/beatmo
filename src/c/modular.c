#include <math.h>
#include <stdlib.h>
#include <string.h>
#include "modular.h"
#include "once.h"
#include "modular_stages.h"
#include "modal_bank.h"
#include "wavetable.h"
#include "adsr.h"
#include "fmsynth.h"
#include "noise.h"
#include "synth_dsp_primitives.h" /* sp_* composable DSP primitives */

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* Nominal base pitch of the modular voice (A3). pitch/octave/detune shift it. */
#define MODULAR_BASE_FREQ 220.0
/* Band-limit harmonic count for the analytic waveform tables. */
#define MODULAR_WT_HARMONICS 48

/* ── Static band-limited oscillator tables (sine/saw/square/triangle) ─────── */

static float       modular_sine_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_saw_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_square_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_triangle_buf[WT_DEFAULT_LENGTH + 1];
static float       modular_glottal_buf[WT_DEFAULT_LENGTH + 1];
static wavetable_t modular_sine_wt, modular_saw_wt, modular_square_wt, modular_triangle_wt;
static wavetable_t modular_glottal_wt;
static beatmo_once_t modular_tables_once = BEATMO_ONCE_INIT;

static void build_modular_tables(void) {
    wt_generate_sine(&modular_sine_wt, modular_sine_buf, WT_DEFAULT_LENGTH);
    wt_generate_saw(&modular_saw_wt, modular_saw_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_square(&modular_square_wt, modular_square_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_triangle(&modular_triangle_wt, modular_triangle_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_glottal(&modular_glottal_wt, modular_glottal_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
}

/* Thread-safe: a session's first renders run on several prewarm workers at once. */
static void ensure_modular_tables(void) {
    beatmo_once(&modular_tables_once, build_modular_tables);
}

static const wavetable_t *modular_table_for(int osc_type) {
    switch (osc_type) {
    case 1: return &modular_saw_wt;
    case 2: return &modular_square_wt;
    case 3: return &modular_triangle_wt;
    case 12: return &modular_glottal_wt;
    default: return &modular_sine_wt;
    }
}

/* The SAME wavetable the osc stage uses, so a slot sine is byte-identical to it. */
const wavetable_t *modular_shared_table_for(int wave) {
    ensure_modular_tables();
    return modular_table_for(wave);
}

/* Operators run a steady internal envelope so the voice amp ADSR shapes the whole sound uniformly. */
static void build_fm_preset(const modular_params *p, double base_freq,
                            fm_preset *out) {
    memset(out, 0, sizeof(*out));
    out->num_ops = FM_MAX_OPS;
    out->base_freq = (float)base_freq;
    out->pitch_env_amount = 0.0f;
    out->pitch_env_decay = 0.0f;

    double ratio[FM_MAX_OPS] = {p->fm_op1_ratio, p->fm_op2_ratio, p->fm_op3_ratio, p->fm_op4_ratio};
    const double level[FM_MAX_OPS] = {p->fm_op1_level, p->fm_op2_level, p->fm_op3_level, p->fm_op4_level};
    const double depth[FM_MAX_OPS] = {p->fm_op1_depth, p->fm_op2_depth, p->fm_op3_depth, p->fm_op4_depth};

    for (int i = 0; i < FM_MAX_OPS; i++) {
        out->ops[i].freq_ratio = ratio[i] > 0.0 ? ratio[i] : 0.0; /* structural: a negative ratio has no meaning */
        out->ops[i].freq_offset = 0.0f;
        out->ops[i].amplitude = level[i];
        out->ops[i].attack_sec = 0.0f;
        out->ops[i].decay_sec = 0.0f;
        out->ops[i].sustain_level = 1.0f;
        out->ops[i].release_sec = 0.0f;
        out->ops[i].is_carrier = 0;
    }
    /* Not forced audible, so the level knob has real authority. */
    out->ops[0].is_carrier = 1;

    /* op1 self-feedback (DX7-style) gives fm_op1_depth meaning on a final carrier. */
    out->mod_matrix[0][0] = depth[0];

    int alg = (int)lrint(p->fm_algorithm);
    switch (alg) {
    case 1: /* parallel: each op with level>0 is an independent carrier */
        for (int i = 1; i < FM_MAX_OPS; i++)
            out->ops[i].is_carrier = (level[i] > 0.0f) ? 1 : 0;
        break;
    case 2: /* 3-op chain: op3 -> op2 -> op1 */
        out->mod_matrix[1][0] = depth[1];
        out->mod_matrix[2][1] = depth[2];
        break;
    case 3: /* 4-op stack: op4 -> op3 -> op2 -> op1 */
        out->mod_matrix[1][0] = depth[1];
        out->mod_matrix[2][1] = depth[2];
        out->mod_matrix[3][2] = depth[3];
        break;
    default: /* alg 0: 2-op stack, op2 -> op1 */
        out->mod_matrix[1][0] = depth[1];
        break;
    }
}

/* No bow BITE, a signal-chain limit, not tuning: every noise source is PRE-filter, so a broadband scrape cannot survive
 * a voice that low-passes for its tone (scaling friction or injecting noise into the string only adds tone). A bite
 * needs a post-filter transient stage. */


/* So a non-integer delay does not quantize the pitch. */
static inline double pm_frac_read(const float *buf, int len, int wi, double delay) {
    double rp = (double)wi - delay;
    while (rp < 0.0) rp += (double)len;
    int i0 = (int)rp;
    int i1 = i0 + 1; if (i1 >= len) i1 = 0;
    double frac = rp - (double)i0;
    return (double)buf[i0] * (1.0 - frac) + (double)buf[i1] * frac;
}

/* STK "Bowed" (Smith 1986): two velocity delay lines split at the bow, a nonlinear stick-slip friction driven by a
 * constant bow velocity and a bridge loss filter. The continuous excitation self-oscillates into a sustained
 * Helmholtz tone a static saw cannot make. */
static void render_bowed_string(float *out, int sampleRate, int samples,
                                double freq, double bow_pos, double slope,
                                double bowVel, double loss, unsigned int noise_seed) {
    enum { DMAX = 4096 };
    float neck[DMAX], bridge[DMAX];
    /* FRACTIONAL lengths, as in the other waveguides: an integer length leaves each period a constant ~1 sample short,
     * which sharpens high notes by up to a quarter tone. −1 is the loop's measured extra delay (write-then-read plus the
     * loss filter's phase). A one-time length, not per-sample modulation, so loop stability is unaffected. */
    double L = (double)sampleRate / freq - 1.0; /* round-trip delay; -1 = loop + loss-filter phase */
    if (L < 4.0) L = 4.0;                        /* bow_pos: ~1/8 from the bridge (STK betaRatio) */
    double bridgeLen = L * bow_pos;
    double neckLen   = L - bridgeLen;
    if (bridgeLen < 1.0) bridgeLen = 1.0;
    if (neckLen < 1.0) neckLen = 1.0;
    if (bridgeLen > (double)(DMAX - 2)) bridgeLen = (double)(DMAX - 2);
    if (neckLen > (double)(DMAX - 2)) neckLen = (double)(DMAX - 2);
    for (int k = 0; k < DMAX; k++) { neck[k] = 0.0f; bridge[k] = 0.0f; }
    int ni = 0, bi = 0;

    /* offset and refl stay literals: they hold the loop numerically stable, they are not voice. */
    const double offset = 0.001;
    const double refl   = 0.99;   /* slight per-round-trip energy loss for stability */
    double lp = 0.0;
    noise_gen ng; noise_init(&ng, noise_seed);
    /* A MODE-SELECTION control, not a click guard: the stick-slip loop is resonant at every k·SR/L, and a bow brought up to
     * speed too fast locks onto the 2nd or 3rd mode (a cello a twelfth high, the fundamental absent). Measured in PERIODS,
     * since establishing Helmholtz motion takes round trips; the pre-warm outlasts it, so nothing is heard. */
    const double bowRampPeriods = 110.0; /* round trips to establish Helmholtz motion */
    int attack = (int)(bowRampPeriods * L);
    /* Both floors bind: 110 periods is short at high notes, 250 ms short at low ones. The longer wins. */
    if (attack < sampleRate / 4) attack = sampleRate / 4;   /* >=250 ms in absolute time */
    if (attack > 2 * sampleRate) attack = 2 * sampleRate;   /* ceiling on render cost */
    if (attack < 1) attack = 1;

    /* A bowed string takes ~0.5-1 s to build its limit cycle; running the loop silently first means out[0] starts at the
     * established tone and the amp ADSR shapes the onset. */
    int warm = attack + sampleRate / 4;                /* ramp, then ~0.25 s settled */
    if (warm < sampleRate / 2) warm = sampleRate / 2;  /* never below the old ~0.5 s */
    if (warm < 1) warm = 1;
    int totalN = warm + samples;
    for (int i = 0; i < totalN; i++) {
        double neckOut   = pm_frac_read(neck, DMAX, ni, neckLen);
        double bridgeOut = pm_frac_read(bridge, DMAX, bi, bridgeLen);
        lp = (1.0 - loss) * bridgeOut + loss * lp;        /* loss LP (high-freq damping) */
        double bridgeRefl = -refl * lp;
        double nutRefl    = -refl * neckOut;
        double stringVel  = bridgeRefl + nutRefl;
        double bv = bowVel * (i < attack ? (double)i / (double)attack : 1.0);
        bv *= 1.0 + 0.03 * (double)noise_white_tick(&ng); /* friction irregularity = natural */
        double dv = bv - stringVel;                       /* differential velocity (bow - string) */
        double bt = fabs((dv + offset) * slope) + 0.75;
        bt = pow(bt, -4.0);                               /* stick-slip friction curve */
        if (bt > 1.0) bt = 1.0;
        double newVel = dv * bt;
        neck[ni]   = (float)(bridgeRefl + newVel);
        bridge[bi] = (float)(nutRefl + newVel);
        ni = (ni + 1) % DMAX;
        bi = (bi + 1) % DMAX;
        double y = bridgeOut * 4.0;                       /* string velocity at the bridge */
        if (!(y > -8.0 && y < 8.0)) y = 0.0;              /* guard against blow-up/NaN */
        if (i >= warm) out[i - warm] = (float)y;
    }
}

/* STK "Brass" (Cook): a tube delay, a lip resonator and a squared nonlinearity (lip area ∝ displacement², which
 * rectifies). The tube length uses STK's ·2+3: the note rides on the lip-tube interaction. */
static void render_brass(float *out, int sampleRate, int samples, double freq,
                         double bore_refl, double lip_radius, double lip_gain,
                         double max_pressure) {
    enum { LEN = 8192 };
    float tube[LEN];
    double delay = (double)sampleRate / freq - 2.0; /* non-inverting loop oscillates ~SR/delay = f */
    if (delay > (double)(LEN - 2)) delay = (double)(LEN - 2);
    if (delay < 1.0) delay = 1.0;
    for (int k = 0; k < LEN; k++) tube[k] = 0.0f;
    int wi = 0;
    double lastOut = 0.0;
    double a2 = lip_radius * lip_radius;
    double a1 = -2.0 * lip_radius * cos(2.0 * M_PI * freq / (double)sampleRate);
    double ly1 = 0.0, ly2 = 0.0;
    sp_dcblock dc = {0.0, 0.0};
    int attack = sampleRate / 200; if (attack < 1) attack = 1; /* ~5 ms */
    for (int i = 0; i < samples; i++) {
        double env = (i < attack) ? (double)i / (double)attack : 1.0;
        double breath = max_pressure * env;
        double mouth = 0.3 * breath;
        double bore = bore_refl * lastOut;
        double dp = mouth - bore;
        double y = lip_gain * dp - a1 * ly1 - a2 * ly2; /* lip resonator (BiQuad) */
        ly2 = ly1; ly1 = y;
        dp = y * y; if (dp > 1.0) dp = 1.0;             /* displacement → area (nonlinear) */
        double in = dp * mouth + (1.0 - dp) * bore;     /* scattering junction */
        double dy = sp_dcblock_tick(&dc, in, 0.99); in = dy; /* DC block */
        double tout = pm_frac_read(tube, LEN, wi, delay);
        sp_delay_write(tube, LEN, &wi, in);
        lastOut = tout;
        if (!(tout > -8.0 && tout < 8.0)) tout = 0.0;
        out[i] = (float)tout;
    }
}

/* STK "Clarinet": a bore delay, a reed table (reflection = clamp(offset + slope·Δp, ±1)) and a one-zero bell loss. A
 * non-inverting bell is an open pipe (all harmonics, round trip SR/f); an inverting one closed-open (odd, SR/2f). */
static void render_reed(float *out, int sampleRate, int samples, double freq,
                        double bell_refl, double reed_offset, double reed_slope,
                        double breath_target, double loss_b0, unsigned int noise_seed) {
    enum { LEN = 8192 };
    float bore[LEN];
    double loss_b1 = 1.0 - loss_b0;
    double delay = (bell_refl >= 0.0) ? ((double)sampleRate / freq)
                                      : ((double)sampleRate / freq * 0.5);
    delay -= 0.5; /* one-zero loss-filter phase delay */
    if (delay > (double)(LEN - 2)) delay = (double)(LEN - 2);
    if (delay < 1.0) delay = 1.0;
    for (int k = 0; k < LEN; k++) bore[k] = 0.0f;
    int wi = 0;
    double lastOut = 0.0, lossPrevX = 0.0;
    sp_dcblock dc = {0.0, 0.0};
    noise_gen ng; noise_init(&ng, noise_seed);
    int attack = sampleRate / 100; if (attack < 1) attack = 1; /* ~10 ms */
    for (int i = 0; i < samples; i++) {
        double env = (i < attack) ? (double)i / (double)attack : 1.0;
        double breath = breath_target * env;
        /* Also breaks the static fixed point, so the reed kicks into self-oscillation instead of settling to silence. */
        breath += breath * 0.2 * (double)noise_white_tick(&ng);
        double lf = loss_b0 * lastOut + loss_b1 * lossPrevX; /* one-zero bell loss */
        lossPrevX = lastOut;
        double pdiff = bell_refl * lf - breath;          /* reflected bore minus mouth */
        double r = reed_offset + reed_slope * pdiff;     /* reed table (nonlinear) */
        if (r > 1.0) r = 1.0;
        if (r < -1.0) r = -1.0;
        double in = breath + pdiff * r;                  /* scatter back into the bore */
        double tout = pm_frac_read(bore, LEN, wi, delay);
        sp_delay_write(bore, LEN, &wi, in); /* breath DC stays in the loop = the drive */
        lastOut = tout;
        /* The OUTPUT only: the breath pressure is a DC term that DRIVES the bore; blocking it in-loop kills oscillation. */
        double dy = sp_dcblock_tick(&dc, tout, 0.995);
        double y = dy * 0.3;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        out[i] = (float)y;
    }
}

/* STK "Flute": a bore delay, a shorter jet delay, a cubic jet nonlinearity (x³−x), multiplicative breath noise and
 * an inverting one-pole bore loss. */
static void render_flute(float *out, int sampleRate, int samples, double freq,
                         double jet_ratio, double noise_gain, double pole,
                         double max_pressure, unsigned int noise_seed) {
    enum { LEN = 8192, JLEN = 4096 };
    float bore[LEN], jet[JLEN];
    /* STK pre-scales the bore by 2/3: the note rides on the jet-bore interaction. Without it the voice renders ~1.5× sharp. */
    double boreLen = (double)sampleRate / freq - 0.5;
    if (boreLen > (double)(LEN - 2)) boreLen = (double)(LEN - 2);
    if (boreLen < 1.0) boreLen = 1.0;
    double jetLen = boreLen * jet_ratio;
    if (jetLen > (double)(JLEN - 2)) jetLen = (double)(JLEN - 2);
    if (jetLen < 1.0) jetLen = 1.0;
    for (int k = 0; k < LEN; k++) bore[k] = 0.0f;
    for (int k = 0; k < JLEN; k++) jet[k] = 0.0f;
    int bwi = 0, jwi = 0;
    double boreLast = 0.0;
    double b0 = (pole > 0.0) ? (1.0 - pole) : (1.0 + pole); /* one-pole, DC gain 1 */
    double fpY = 0.0;
    sp_dcblock dc = {0.0, 0.0};
    const double jetRefl = 0.5, endRefl = 0.5;
    noise_gen ng; noise_init(&ng, noise_seed);
    int attack = sampleRate / 200; if (attack < 1) attack = 1; /* ~5 ms */
    for (int i = 0; i < samples; i++) {
        double env = (i < attack) ? (double)i / (double)attack : 1.0;
        double bp = max_pressure * env;
        bp += bp * noise_gain * (double)noise_white_tick(&ng); /* multiplicative breath noise */
        fpY = b0 * boreLast + pole * fpY;        /* one-pole loss on bore feedback */
        double temp = -fpY;                       /* inverting bore reflection */
        double pd = bp - jetRefl * temp;
        double jout = pm_frac_read(jet, JLEN, jwi, jetLen); /* jet delay */
        sp_delay_write(jet, JLEN, &jwi, pd);
        double jt = jout * (jout * jout - 1.0);   /* cubic jet nonlinearity x³−x */
        if (jt > 1.0) jt = 1.0;
        if (jt < -1.0) jt = -1.0;
        double dy = sp_dcblock_tick(&dc, jt, 0.99); /* DC block */
        double in = dy + endRefl * temp;
        double tout = pm_frac_read(bore, LEN, bwi, boreLen); /* bore delay */
        sp_delay_write(bore, LEN, &bwi, in);
        boreLast = tout;
        double y = 0.3 * tout;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        out[i] = (float)y;
    }
}

/* STK "Saxofony": the clarinet's inverting reed loop with the bore split at a BLOW POSITION and the reed injected
 * off-centre, which restores the even harmonics of a conical bore; the inverting resonator still pins the pitch (a
 * non-inverting cone reflection has nothing to lock onto: chaos). Tapped at the reed junction. */
static void render_sax(float *out, int sampleRate, int samples, double freq,
                       double position, double reed_offset, double reed_slope,
                       double reflect, double breath_target, double loss_b0,
                       unsigned int noise_seed) {
    enum { LEN = 8192 };
    float d0[LEN], d1[LEN];
    double loss_b1 = 1.0 - loss_b0;
    double total = (double)sampleRate / freq - 1.5; /* one-zero phase + junction */
    double delay0 = (1.0 - position) * total; /* bridge side (long) */
    double delay1 = position * total;         /* blow side (short) */
    if (delay0 > (double)(LEN - 2)) delay0 = (double)(LEN - 2);
    if (delay1 > (double)(LEN - 2)) delay1 = (double)(LEN - 2);
    if (delay0 < 1.0) delay0 = 1.0;
    if (delay1 < 1.0) delay1 = 1.0;
    noise_gen ng; noise_init(&ng, noise_seed);
    /* The loop gain is barely above 1, so from pure silence the limit cycle takes ~2 s to build. */
    for (int k = 0; k < LEN; k++) {
        d0[k] = (float)(0.8 * (double)noise_white_tick(&ng));
        d1[k] = (float)(0.8 * (double)noise_white_tick(&ng));
    }
    int w0 = 0, w1 = 0;
    double ozx1 = 0.0;          /* one-zero loss filter previous input */
    sp_dcblock dc = {0.0, 0.0};
    double vibPhase = 0.0, vibInc = 2.0 * M_PI * 5.2 / (double)sampleRate;
    int attack = sampleRate / 100; if (attack < 1) attack = 1; /* ~10 ms breath ramp */
    /* So out[0] starts at the established tone; the amp ADSR shapes the audible onset. */
    int warm = sampleRate / 2; if (warm < 1) warm = 1; /* ~0.5 s */
    int saxN = warm + samples;
    for (int i = 0; i < saxN; i++) {
        double env = (i < attack) ? (double)i / (double)attack : 1.0;
        double breath = breath_target * env;
        breath += breath * 0.2 * (double)noise_white_tick(&ng); /* breath turbulence (also kick-starts) */
        breath += breath * 0.08 * sin(vibPhase);                /* vibrato */
        vibPhase += vibInc; if (vibPhase > 2.0 * M_PI) vibPhase -= 2.0 * M_PI;
        /* reflection from the bridge-side delay through the one-zero loss, INVERTING */
        double d0out = pm_frac_read(d0, LEN, w0, delay0);
        double filt = loss_b0 * d0out + loss_b1 * ozx1; ozx1 = d0out;
        double temp = reflect * filt;
        double d1out = pm_frac_read(d1, LEN, w1, delay1);
        double o = temp - d1out;            /* reed junction = output tap */
        double pd = breath - o;             /* differential pressure across the reed */
        double r = reed_offset + reed_slope * pd; /* reed table (positive slope for sax) */
        if (r > 1.0) r = 1.0;
        if (r < -1.0) r = -1.0;
        sp_delay_write(d1, LEN, &w1, temp);
        sp_delay_write(d0, LEN, &w0, breath - pd * r - temp);
        double dy = sp_dcblock_tick(&dc, o, 0.995); /* DC block on OUTPUT */
        double y = dy * 0.4;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        if (i >= warm) out[i - warm] = (float)y; /* output only AFTER the limit cycle is established */
    }
}

/* Each renders the OSC stage for one branch of the osc_type dispatch. */
static void modular_source_noise(float *out, int samples, int osc_type,
                                 unsigned int noise_seed) {
    noise_gen ng;
    noise_init(&ng, noise_seed);
    if (osc_type == 6) {
        for (int i = 0; i < samples; i++) out[i] = noise_pink_tick(&ng);
    } else {
        for (int i = 0; i < samples; i++) out[i] = noise_white_tick(&ng);
    }
}

static void modular_source_fm(float *out, int sampleRate, int samples,
                              const modular_params *p, double freq,
                              int fm_on, int pe_on, double pe_amt, double pe_decay) {
    fm_preset preset;
    build_fm_preset(p, freq, &preset);
    if (!fm_on) {
        /* Carrier-only, including op1's self-feedback. */
        memset(preset.mod_matrix, 0, sizeof(preset.mod_matrix));
    }
    if (pe_on) {
        /* build_fm_preset zeroes the preset's pitch sweep, so stage off leaves it at 0. */
        preset.pitch_env_amount = pe_amt;
        preset.pitch_env_decay  = pe_decay;
    }
    fm_render(&preset, out, sampleRate, samples);
    /* fm_render applies its own steady envelope and softsat; the voice amp ADSR still shapes it. */
}

/* The waveguide branch returns before the generic vibrato and unison branches, so vibrato is applied here, as a
 * modulated read over the OUTPUT: retuning the loop per sample would risk its stability for no perceptual gain at these
 * depths. Identity unless depth and rate are both positive. */
enum { MODULAR_VIB_DMAX = 4096 };

/* Divided by the rate, converts a semitone depth to a peak delay in seconds. */
#define MODULAR_VIB_SEMI_TO_SEC 0.0091938185

/* A constant rate and depth reads as a machine, so the rate wanders by two slow incommensurate sines and the depth by
 * a third. No RNG: renders stay reproducible. */
#define MODULAR_VIB_RATE_JITTER_A   0.13   /* +/-13% rate wander */
#define MODULAR_VIB_RATE_JITTER_B   0.08
/* Fast enough that a note holds several cycles, so the mean rate stays the nominal one; far slower than the vibrato, so
 * it reads as an unsteady hand, not a second vibrato. */
#define MODULAR_VIB_RATE_HZ_A       1.70
#define MODULAR_VIB_RATE_HZ_B       2.90
#define MODULAR_VIB_DEPTH_JITTER    0.18   /* +/-18% depth breathing */
#define MODULAR_VIB_DEPTH_HZ        1.30

/* Integrating the RATE makes a genuine tempo wander; perturbing phase would add a second vibrato on top. The integral is
 * closed-form: exact and allocation-free. */
static double modular_vibrato_phase(double t, double rate) {
    double wa = 2.0 * M_PI * MODULAR_VIB_RATE_HZ_A;
    double wb = 2.0 * M_PI * MODULAR_VIB_RATE_HZ_B;
    double integ = t
        + MODULAR_VIB_RATE_JITTER_A * (1.0 - cos(wa * t)) / wa
        + MODULAR_VIB_RATE_JITTER_B * (1.0 - cos(wb * t)) / wb;
    return 2.0 * M_PI * rate * integ;
}

static void modular_apply_vibrato_delay(float *out, int sampleRate, int samples,
                                        double lfo_depth, double lfo_rate,
                                        double lfo_delay) {
    if (!(lfo_depth > 0.0f) || !(lfo_rate > 0.0f) || samples <= 0 || sampleRate <= 0) return;

    double peak = (double)lfo_depth * MODULAR_VIB_SEMI_TO_SEC
                / (double)lfo_rate * (double)sampleRate;
    /* A deep, slow vibrato would ask for more than the ring buffer: the depth degrades rather than reading out of bounds. */
    const double maxPeak = (double)(MODULAR_VIB_DMAX / 2 - 4);
    if (peak > maxPeak) peak = maxPeak;
    if (!(peak > 0.0)) return;
    double centre = peak + 2.0;

    float buf[MODULAR_VIB_DMAX];
    for (int k = 0; k < MODULAR_VIB_DMAX; k++) buf[k] = 0.0f;

    int wi = 0;
    for (int i = 0; i < samples; i++) {
        buf[wi] = out[i];
        double t = (double)i / (double)sampleRate;
        /* Same delay-ramp semantics as modular_source_wt_modulated. */
        double ramp = (lfo_delay > 0.0f) ? fmin(t / (double)lfo_delay, 1.0) : 1.0;
        /* A metronomic vibrato is the most recognisable "LFO, not a player" tell. */
        double d = centre + peak * ramp * cos(modular_vibrato_phase(t, (double)lfo_rate))
                 * (1.0 + MODULAR_VIB_DEPTH_JITTER * sin(2.0 * M_PI * MODULAR_VIB_DEPTH_HZ * t + 0.7));

        double rp = (double)wi - d;
        while (rp < 0.0) rp += (double)MODULAR_VIB_DMAX;
        int i0 = (int)rp;
        if (i0 >= MODULAR_VIB_DMAX) i0 = MODULAR_VIB_DMAX - 1;
        double fr = rp - (double)i0;
        int i1 = i0 + 1; if (i1 >= MODULAR_VIB_DMAX) i1 = 0;
        out[i] = (float)((1.0 - fr) * (double)buf[i0] + fr * (double)buf[i1]);

        wi++; if (wi >= MODULAR_VIB_DMAX) wi = 0;
    }
}

static void modular_source_physical(float *out, int sampleRate, int samples,
                                    const modular_params *p, int osc_type,
                                    double freq, unsigned int noise_seed) {
    if (osc_type == 7) {
        /* Bowed-string digital waveguide (physical model) — see render_bowed_string. */
        render_bowed_string(out, sampleRate, samples, freq,
                            p->osc_bow_pos,
                            p->osc_bow_slope,
                            p->osc_bow_vel,
                            p->osc_bow_loss,
                            noise_seed);
    } else if (osc_type == 8) {
        /* Not yet a clean self-oscillator (it tends to chaos), so no instrument ships on it; the output is guarded finite. */
        render_brass(out, sampleRate, samples, freq, 0.85, 0.997, 0.012, 0.5);
    } else if (osc_type == 9) {
        /* Clarinet timbre (odd harmonics only). The oboe is additive: through the conical model its limit cycle wanders 20-140
         * cents at an oboe's short bore. */
        render_reed(out, sampleRate, samples, freq, -0.95, 0.7, -0.3, 0.8, 0.5, noise_seed);
    } else if (osc_type == 10) {
        /* Higher drive and lower loss so it sustains at the prescaled bore. */
        render_flute(out, sampleRate, samples, freq, 0.4, 0.2, 0.7, 1.0, noise_seed);
    } else if (osc_type == 11) {
        /* Baritone-sax values: off-centre blow, positive reed slope, hard breath and low offset for a dense honk. */
        render_sax(out, sampleRate, samples, freq,
                   p->osc_sax_blow,
                   p->osc_sax_reed_off,
                   p->osc_sax_reed_slope,
                   p->osc_sax_reflect,
                   p->osc_sax_breath,
                   p->osc_sax_loss,
                   noise_seed);
    }
}

static void modular_source_wt_modulated(float *out, int sampleRate, int samples,
                                        int osc_type, double freq,
                                        int pe_active, double pe_amt, double pe_decay,
                                        int vib_on, double lfo_depth, double lfo_rate,
                                        double lfo_delay) {
    /* Pitch env and vibrato summed in semitones, applied as 2^(st/12); with both off the plain branch runs, so every other
     * instrument is byte-identical. */
    ensure_modular_tables();
    wt_osc_t osc;
    wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
    for (int i = 0; i < samples; i++) {
        double t = (double)i / (double)sampleRate;
        double semis = 0.0;
        if (pe_active) semis += (double)pe_amt * exp(-t / (double)pe_decay);
        if (vib_on) {
            double ramp = (lfo_delay > 0.0f) ? fmin(t / (double)lfo_delay, 1.0) : 1.0;
            semis += (double)lfo_depth * ramp
                              * sin(2.0 * M_PI * (double)lfo_rate * t);
        }
        double mult = pow(2.0, semis / 12.0);
        wt_osc_set_freq(&osc, freq * mult, sampleRate);
        out[i] = wt_osc_tick(&osc);
    }
}

static void modular_source_unison(float *out, int sampleRate, int samples,
                                  const modular_params *p,
                                  int osc_type, double freq,
                                  int unison_nv, double unison_detune, double unison_mix,
                                  double unison_drift_rate, double unison_drift_depth,
                                  unsigned int noise_seed) {
    /* Phase-15 ENSEMBLE humanization params (scatter + per-voice vibrato). */
    double ens_scatter = p->ens_scatter;
    double ens_vr      = p->ens_vib_rate;
    double ens_vd      = p->ens_vib_depth;
    double ens_hum     = p->ens_humanize;
    /* 0 is an exact bypass of everything jitter-related. */
    double ens_jitter  = p->ens_jitter;
    int   ens_vib_on  = (ens_vr > 0.0f && ens_vd > 0.0f);
    /* Centre at exact pitch; side voices detuned ±detune/2 with phase offsets i·π/nv; the sum normalised by 1/√nv.
     * unison_mix blends centre-only (0) to ensemble (1). */
    ensure_modular_tables();
    const wavetable_t *wt = modular_table_for(osc_type);
    wt_osc_t osc;
    wt_osc_init(&osc, wt, freq, sampleRate);
    /* side-voice oscillators (max 6 side voices for nv up to 7). */
    wt_osc_t side[6];
    double side_freq[6];
    int ns = unison_nv - 1; /* number of side voices */
    for (int v = 0; v < ns; v++) {
        /* max(ns-1, 1) so a single side voice lands at -detune, not the centre. */
        double spread = -1.0 + 2.0 * (double)v / (double)(ns > 1 ? ns - 1 : 1);
        double cents_v = (double)unison_detune * spread;
        side_freq[v] = freq * pow(2.0, cents_v / 1200.0);
        wt_osc_init(&side[v], wt, side_freq[v], sampleRate);
        /* Phase offset for decorrelation: i*π/nv mapped to [0,1). */
        wt_osc_set_phase(&side[v], (double)(v + 1) / (double)unison_nv);
    }
    double norm = 1.0 / sqrt((double)unison_nv);
    double mix_blend = (double)unison_mix;
    int drift_on = (unison_drift_rate > 0.0f && unison_drift_depth > 0.0f);
    /* drift_base_cents is read whenever the per-sample path runs (drift, scatter or vibrato), so it is filled
     * unconditionally, unlike the drift rate and phase. */
    double drift_rate_v[6] = {0}, drift_phase_v[6] = {0}, drift_base_cents[6] = {0};
    for (int v = 0; v < ns; v++) {
        double spread = -1.0 + 2.0 * (double)v / (double)(ns > 1 ? ns - 1 : 1);
        drift_base_cents[v] = (double)unison_detune * spread;
    }
    if (drift_on) {
        for (int v = 0; v < ns; v++) {
            /* Each voice gets a slightly different rate for decorrelation. */
            drift_rate_v[v]  = (double)unison_drift_rate * (1.0 + 0.13 * (double)v);
            drift_phase_v[v] = 2.0 * M_PI * (double)(v + 1) / (double)unison_nv;
        }
    }
    /* Never rand()/time: byte-identity and cross-platform parity need renders to be pure functions of the seed. */
    double scat_cents[7] = {0}, vib_rate_v[7] = {0}, vib_phase_v[7] = {0};
    if (ens_scatter > 0.0f || ens_vib_on) {
        for (int v = 0; v < unison_nv; v++) {
            unsigned int r = (noise_seed + 1u) * 2654435761u + (unsigned int)(v + 1) * 40503u;
            r = r * 1664525u + 1013904223u;
            double n1 = ((double)(r >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
            r = r * 1664525u + 1013904223u;
            double n2 = ((double)(r >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
            r = r * 1664525u + 1013904223u;
            double u3 = (double)(r >> 8) / (double)(1u << 24);
            /* Center voice keeps exact pitch center: scatter applies to sides only. */
            scat_cents[v] = (v == 0) ? 0.0 : (double)ens_scatter * n1;
            /* At 0 every voice shares one rate and phase: one synchronized singer. */
            vib_rate_v[v]  = (double)ens_vr * (1.0 + 0.15 * (double)ens_hum * n2);
            vib_phase_v[v] = 2.0 * M_PI * (double)ens_hum * u3;
        }
    }
    /* Includes the centre voice (index 0, sides at v+1). An LCG stepped every 16 samples through a ~60 Hz one-pole; at 0
     * these stay exactly 0.0. */
    double jit_onepole[7] = {0};
    unsigned int jit_r[7] = {0};
    if (ens_jitter > 0.0f) {
        for (int v = 0; v < unison_nv; v++) {
            jit_r[v] = (noise_seed + 7u) * 2654435761u + (unsigned int)(v + 1) * 48271u;
        }
    }
    int mod_on = drift_on || ens_scatter > 0.0f || ens_vib_on || ens_jitter > 0.0f;
    for (int i = 0; i < samples; i++) {
        if (mod_on) {
            double t = (double)i / (double)sampleRate;
            if (ens_jitter > 0.0f && (i & 15) == 0) {
                for (int v = 0; v < unison_nv; v++) {
                    jit_r[v] = jit_r[v] * 1664525u + 1013904223u;
                    double jn = ((double)(jit_r[v] >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
                    sp_onepole_tick(&jit_onepole[v], 0.008, jn);
                }
            }
            for (int v = 0; v < ns; v++) {
                double drift_cents = drift_on
                    ? (double)unison_drift_depth
                        * sin(2.0 * M_PI * drift_rate_v[v] * t + drift_phase_v[v])
                    : 0.0;
                double cents = drift_base_cents[v]                          /* static spread (existing) */
                             + drift_cents                                   /* existing drift term */
                             + scat_cents[v + 1]                             /* Phase-15 scatter */
                             + (ens_vib_on ? (double)ens_vd
                                   * sin(2.0 * M_PI * vib_rate_v[v + 1] * t + vib_phase_v[v + 1]) : 0.0);
                if (ens_jitter > 0.0f) {
                    cents += jit_onepole[v + 1] * (double)ens_jitter * 35.0;
                }
                wt_osc_set_freq(&side[v], freq * pow(2.0, cents / 1200.0), sampleRate);
            }
            if (ens_vib_on || ens_jitter > 0.0f) {
                double c0 = 0.0;
                if (ens_vib_on) {
                    c0 = (double)ens_vd * sin(2.0 * M_PI * vib_rate_v[0] * t + vib_phase_v[0]);
                }
                if (ens_jitter > 0.0f) {
                    c0 += jit_onepole[0] * (double)ens_jitter * 35.0;
                }
                wt_osc_set_freq(&osc, freq * pow(2.0, c0 / 1200.0), sampleRate);
            }
        }
        double center = (double)wt_osc_tick(&osc);
        double ensemble = center;
        for (int v = 0; v < ns; v++) ensemble += (double)wt_osc_tick(&side[v]);
        ensemble *= norm;
        out[i] = (float)((1.0 - mix_blend) * center + mix_blend * ensemble);
    }
}

static void modular_source_plain(float *out, int sampleRate, int samples,
                                 int osc_type, double freq) {
    ensure_modular_tables();
    wt_osc_t osc;
    wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
    for (int i = 0; i < samples; i++) {
        out[i] = wt_osc_tick(&osc);
    }
}

/* Grows lazily, never freed, bounded by the longest render. _Thread_local gives each cgo-crossing goroutine its own;
 * single-threaded Emscripten makes it a plain static. */
static _Thread_local float *g_finish_scratch = NULL;
static _Thread_local size_t g_finish_scratch_cap = 0;

static float *finish_scratch(size_t want) {
    if (want > g_finish_scratch_cap) {
        float *grown = (float *)realloc(g_finish_scratch, want * sizeof(float));
        if (!grown) return NULL;
        g_finish_scratch = grown;
        g_finish_scratch_cap = want;
    }
    return g_finish_scratch;
}

/* A Kaiser-windowed sinc with the cutoff scaled down by the speed when shifting up, so partials past Nyquist are removed
 * rather than folded back as inharmonic tones. Tabulated and read at the nearest point: that error sits at the
 * stopband floor, and interpolating cost as much again as the whole read. */
#define FINISH_RS_ZC 16
#define FINISH_RS_OS 2048
#define FINISH_RS_ROLLOFF 0.9
#define FINISH_RS_TAB (FINISH_RS_ZC * FINISH_RS_OS + 2)
static float g_rs_kernel[FINISH_RS_TAB];
static beatmo_once_t g_rs_kernel_once = BEATMO_ONCE_INIT;

static double bessel_i0(double x) {
    double sum = 1.0, term = 1.0, q = x * x / 4.0;
    for (int k = 1; k < 64; k++) {
        term *= q / ((double)k * (double)k);
        sum += term;
        if (term < sum * 1e-17) break;
    }
    return sum;
}

static void build_rs_kernel(void) {
    const double beta = 5.65;
    double norm = bessel_i0(beta);
    for (int j = 0; j < FINISH_RS_TAB; j++) {
        double x = (double)j / FINISH_RS_OS; /* in zero crossings */
        if (x >= FINISH_RS_ZC) { g_rs_kernel[j] = 0.0f; continue; }
        double t = x / FINISH_RS_ZC;
        double w = bessel_i0(beta * sqrt(1.0 - t * t)) / norm;
        double sinc = j == 0 ? 1.0 : sin(M_PI * x) / (M_PI * x);
        g_rs_kernel[j] = (float)(sinc * w);
    }
}

static void finish_resample(float *out, int samples, double rate, const float *in) {
    beatmo_once(&g_rs_kernel_once, build_rs_kernel);
    /* Shifting up by rate must remove everything above Nyquist/rate before it is read faster. */
    double fc = FINISH_RS_ROLLOFF / (rate > 1.0 ? rate : 1.0);
    double half = FINISH_RS_ZC / fc;      /* kernel half-width, input samples */
    double tabPerSample = fc * FINISH_RS_OS;
    for (int i = 0; i < samples; i++) {
        double pos = (double)i * rate;
        int k0 = (int)ceil(pos - half);
        int k1 = (int)floor(pos + half);
        if (k0 < 0) k0 = 0;
        if (k1 > samples - 1) k1 = samples - 1;
        double acc = 0.0;
        double d = (pos - (double)k0) * tabPerSample; /* table distance of tap k0 */
        for (int k = k0; k <= k1; k++, d -= tabPerSample) {
            double ad = d < 0.0 ? -d : d;
            int j = (int)(ad + 0.5);
            if (j < FINISH_RS_TAB) acc += (double)g_rs_kernel[j] * (double)in[k];
        }
        out[i] = (float)(acc * fc);
    }
}

/* RBJ shelf, slope 1, gain at the shelved end and 0 dB at the other: the same curve at every engine rate. */
static void finish_shelf_set(sp_biquad *q, int high, double f0, double gainDB, int sampleRate) {
    double nyq = 0.5 * (double)sampleRate;
    if (f0 > 0.45 * nyq) f0 = 0.45 * nyq;
    double A = pow(10.0, gainDB / 40.0);
    double w0 = 2.0 * M_PI * f0 / (double)sampleRate;
    double cw = cos(w0), sw = sin(w0);
    double alpha = sw / 2.0 * sqrt(2.0);
    double sa = 2.0 * sqrt(A) * alpha;
    double b0, b1, b2, a0, a1, a2;
    if (high) {
        b0 = A * ((A + 1) + (A - 1) * cw + sa);
        b1 = -2 * A * ((A - 1) + (A + 1) * cw);
        b2 = A * ((A + 1) + (A - 1) * cw - sa);
        a0 = (A + 1) - (A - 1) * cw + sa;
        a1 = 2 * ((A - 1) - (A + 1) * cw);
        a2 = (A + 1) - (A - 1) * cw - sa;
    } else {
        b0 = A * ((A + 1) - (A - 1) * cw + sa);
        b1 = 2 * A * ((A - 1) - (A + 1) * cw);
        b2 = A * ((A + 1) - (A - 1) * cw - sa);
        a0 = (A + 1) + (A - 1) * cw + sa;
        a1 = -2 * ((A - 1) + (A + 1) * cw);
        a2 = (A + 1) + (A - 1) * cw - sa;
    }
    q->b0 = b0 / a0; q->b1 = b1 / a0; q->b2 = b2 / a0;
    q->a1 = a1 / a0; q->a2 = a2 / a0;
    q->x1 = q->x2 = q->y1 = q->y2 = 0.0;
}

/* Each lifts ONLY its end, so the knob changes the balance, not the loudness; gain is linear in dB with the knob. */
#define FINISH_BODY_HZ 200.0
#define FINISH_CRISP_HZ 3000.0
#define FINISH_SHELF_MAX_DB 12.0

/* Always run, in this order: resample, fade, low boost, crisp, darken, crunch, then the limit. Each is a no-op at its
 * identity; Darken and Crunch crossfade by their own amount, so a first step does a first step's work. */
static void modular_finish(float *out, int sampleRate, int samples, const modular_params *p) {
    /* Dropped BEFORE any recursive filter: a biquad that takes one NaN holds it forever, and the output guard then
     * silences the rest of the voice. */
    for (int i = 0; i < samples; i++) {
        if (!isfinite(out[i])) out[i] = 0.0f;
    }

    double pitchSt = p->post_pitch;
    if (pitchSt != 0.0) {
        double rate = pow(2.0, pitchSt / 12.0);
        float *tmp = finish_scratch((size_t)samples);
        if (tmp) {
            memcpy(tmp, out, (size_t)samples * sizeof(float));
            finish_resample(out, samples, rate, tmp);
        }
    }

    double decayMul = p->post_decay;
    /* Structural bounds, not defaults: at 0 the exponent is 0·Inf = NaN; above 1 the fade grows without bound. */
    if (decayMul < 0.01f) decayMul = 0.01f;
    if (decayMul > 1.0) decayMul = 1.0;
    if (decayMul != 1.0) {
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double env = exp(-t * p->post_decay_rate * (1.0 / decayMul - 1.0));
            out[i] *= (float)env;
        }
    }

    double body = p->post_body;
    if (body > 0.0) {
        sp_biquad q;
        finish_shelf_set(&q, 0, FINISH_BODY_HZ, body * FINISH_SHELF_MAX_DB, sampleRate);
        for (int i = 0; i < samples; i++) out[i] = (float)sp_biquad_tick(&q, (double)out[i]);
    }

    double brightness = p->post_brightness;
    if (brightness > 0.0) {
        sp_biquad q;
        finish_shelf_set(&q, 1, FINISH_CRISP_HZ, brightness * FINISH_SHELF_MAX_DB, sampleRate);
        for (int i = 0; i < samples; i++) out[i] = (float)sp_biquad_tick(&q, (double)out[i]);
    }

    double darken = p->post_darken;
    if (darken > 0.0) {
        double cutoff = 2000.0 + (1.0 - darken) * 6000.0;
        double rc = 1.0 / (2.0 * M_PI * cutoff);
        double dt = 1.0 / (double)sampleRate;
        double alpha = dt / (rc + dt);
        double prev = 0;
        for (int i = 0; i < samples; i++) {
            double x = (double)out[i];
            prev += alpha * (x - prev);
            out[i] = (float)(x + darken * (prev - x));
        }
    }

    double crunch = p->post_drive;
    if (crunch > 0.0) {
        double k = 1.0 + crunch * 3.0;
        double norm = tanh(k);
        for (int i = 0; i < samples; i++) {
            double x = (double)out[i];
            out[i] = (float)(x + crunch * (tanh(x * k) / norm - x));
        }
    }

    double clamp = p->out_clamp;
    if (clamp > 0.0) {
        float c = (float)clamp;
        for (int i = 0; i < samples; i++) {
            if (out[i] > c) out[i] = c;
            else if (out[i] < -c) out[i] = -c;
        }
    }

    /* A non-finite sample poisons the browser's peak normaliser and every filter on the channel. */
    for (int i = 0; i < samples; i++) {
        if (!isfinite(out[i])) out[i] = 0.0f;
    }
}

/* Go and JS compare it with their generated layout at load, so a stale engine fails loudly instead of reading later
 * fields at shifted offsets. */
EXPORT int modular_param_count(void) { return MODULAR_PARAM_COUNT; }

void modular_finish_apply(float *buf, int sampleRate, int samples,
                          const modular_params *params) {
    if (!buf || !params || samples <= 0 || sampleRate <= 0) return;
    modular_finish(buf, sampleRate, samples, params);
}

EXPORT void render_modular_p(float *out, int sampleRate, int samples,
                             const modular_params *params) {
    if (!out || samples <= 0 || sampleRate <= 0) return;
    memset(out, 0, (size_t)samples * sizeof(float));

    /* No "absent → default" path: a NULL block is a caller bug and renders silence. */
    if (!params) return;
    const modular_params *p = params;

    int   osc_type   = (int)lrint(p->osc_type);
    double octave     = p->osc_octave;
    double detune     = p->osc_detune;          /* cents */
    double pitch_st   = p->pitch;               /* semitones */
    /* Envelope. */
    double a   = p->amp_attack;
    double d   = p->amp_decay;
    double sus = p->amp_sustain;
    double rel = p->amp_release;
    int   exp_curve = (int)lrint(p->amp_curve);
    int   filt_type = (int)lrint(p->filter_type);
    double cutoff = p->filter_cutoff;
    double q      = p->filter_resonance;
    double drive  = p->drive;
    double gain   = p->gain;

    /* Per-stage bypass toggles. */
    int osc_on   = p->osc_enabled >= 0.5f;
    int fm_on    = p->fm_enabled >= 0.5f;
    int env_on   = p->env_enabled >= 0.5f;
    int filt_on  = p->filter_enabled >= 0.5f;
    int drive_on = p->drive_enabled >= 0.5f;
    unsigned int noise_seed = (unsigned int)lrint(p->noise_seed);

    /* Off skips the stage entirely: its numeric knobs are never read. */
    int   pe_on     = p->pitchenv_enabled >= 0.5f;
    double pe_amt    = p->pitchenv_amt;
    double pe_decay  = p->pitchenv_decay;
    int   lfo_on    = p->lfo_enabled >= 0.5f;
    double lfo_rate  = p->lfo_rate;
    double lfo_depth = p->lfo_depth;
    int   lfo_target = (int)lrint(p->lfo_target);
    double lfo_delay = p->lfo_delay;
    int   burst_on  = p->burst_enabled >= 0.5f;
    int   filtenv_on     = p->filtenv_enabled >= 0.5f;
    double filtenv_amt    = p->filtenv_amt;
    double filtenv_decay  = p->filtenv_decay;
    double filtenv_attack = p->filtenv_attack;
    int   body_model     = (int)lrint(p->body_model);
    double body_mix       = p->body_mix;
    double bow_dynamics   = p->bow_dynamics;

    /* Enum values may be 0; shift is clamped ≥ 0.01, a structural guard against a zero frequency scale. */
    int   formant_on   = p->formant_enabled >= 0.5f;
    double formant_vowel = p->formant_vowel;
    int   formant_vt   = (int)lrint(p->formant_voice_type);
    double formant_mix  = p->formant_mix;
    double formant_shift = p->formant_shift;
    double formant_breath = p->formant_breath;
    double formant_sing = p->formant_sing;
    double formant_mrate = p->formant_morph_rate;
    double formant_mto  = p->formant_morph_to;
    /* Phase-16: dry-blend scaler; identity 1.0 = full dry blend. */
    double formant_dry  = p->formant_dry;

    /* Phase-8E unison: identity default 1 (single osc = no change). */
    int   unison_nv     = (int)lrint(p->unison_voices);
    double unison_detune = p->unison_detune;
    double unison_mix    = p->unison_mix;
    /* Phase-8F drift: identity when either is 0. */
    double unison_drift_rate  = p->unison_drift_rate;
    double unison_drift_depth = p->unison_drift_depth;
    if (unison_nv < 1) unison_nv = 1;
    if (unison_nv > 7) unison_nv = 7;

    if (a < 0.0f) a = 0.0f;
    if (rel < 0.0f) rel = 0.0f;
    if (sus < 0.0f) sus = 0.0f; else if (sus > 1.0f) sus = 1.0f;

    /* Base frequency with octave/semitone/cent shifts. */
    double freq = MODULAR_BASE_FREQ
                * pow(2.0, (double)octave)
                * pow(2.0, (double)pitch_st / 12.0)
                * pow(2.0, (double)detune / 1200.0);
    if (freq < 1.0) freq = 1.0;

    /* > 0 sets an EXACT Hz after the pow derivation, for fundamentals 220·2^x cannot reach (a 45 Hz kick). */
    {
        double vfh = (double)p->voice_freq_hz;
        if (vfh > 0.0) freq = vfh;
    }

    /* When the osc stage did not write, the first active slot ASSIGNS rather than accumulating, or 0.0f + (-0.0f) would
     * flip a -0.0 sample to +0.0. */
    int osc_wrote = 0;
    int vib_on = lfo_on && lfo_target == 1 && lfo_depth > 0.0f && lfo_rate > 0.0f;

    /* ── Oscillator / generator stage ── */
    if (!osc_on) {
        /* The generator is the source, so bypassing it means silence. */
    } else if (osc_type == 5 || osc_type == 6) {
        modular_source_noise(out, samples, osc_type, noise_seed);
        osc_wrote = 1;
    } else if (osc_type == 4) {
        modular_source_fm(out, sampleRate, samples, p, freq, fm_on, pe_on, pe_amt, pe_decay);
        osc_wrote = 1;
    } else if (osc_type == 7 || osc_type == 8 || osc_type == 9 || osc_type == 10 || osc_type == 11) {
        modular_source_physical(out, sampleRate, samples, p, osc_type, freq, noise_seed);
        /* The waveguide returns before the vibrato branch could run, so it is applied here or not at all. */
        if (vib_on) {
            modular_apply_vibrato_delay(out, sampleRate, samples, lfo_depth, lfo_rate, lfo_delay);
        }
        osc_wrote = 1;
    } else if ((pe_on && pe_amt != 0.0f && pe_decay > 0.0f) || vib_on) {
        int pe_active = pe_on && pe_amt != 0.0f && pe_decay > 0.0f;
        modular_source_wt_modulated(out, sampleRate, samples, osc_type, freq,
                                    pe_active, pe_amt, pe_decay,
                                    vib_on, lfo_depth, lfo_rate, lfo_delay);
        osc_wrote = 1;
    } else if (unison_nv >= 2) {
        modular_source_unison(out, sampleRate, samples, p, osc_type, freq,
                              unison_nv, unison_detune, unison_mix,
                              unison_drift_rate, unison_drift_depth, noise_seed);
        osc_wrote = 1;
    } else {
        modular_source_plain(out, sampleRate, samples, osc_type, freq);
        osc_wrote = 1;
    }

    /* Parallel slots added after the osc stage, independent of its gate; no active slot is an exact no-op. */
    modular_gen_bank_render(out, sampleRate, samples, p, freq, noise_seed, osc_wrote);

    /* PRE-envelope/filter like every source; disabled is an exact no-op. */
    modular_modal_render(out, sampleRate, samples, p, freq, noise_seed);
    modular_noiseband_render(out, sampleRate, samples, p, noise_seed);

    /* PRE-envelope so the ADSR shapes it and the filter and formants colour it (whisper = osc off + breath 1). */
    if (formant_on && formant_breath > 0.0f) {
        noise_gen bng;
        noise_init(&bng, noise_seed ^ 0x9E3779B9u);
        for (int i = 0; i < samples; i++)
            out[i] = (float)(out[i] + noise_white_tick(&bng) * formant_breath * 0.5f);
    }

    /* Multiplies the FULL mix, so it shapes every voice: 1 - depth·(0.5 - 0.5·sin(2πft)) stays in [1-depth, 1]. */
    if (lfo_on && lfo_target == 0 && lfo_depth > 0.0f && lfo_rate > 0.0f) {
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double ramp = (lfo_delay > 0.0f) ? fmin(t / (double)lfo_delay, 1.0) : 1.0;
            double eff_depth = (double)lfo_depth * ramp;
            double wob = 1.0 - eff_depth
                       * (0.5 - 0.5 * sin(2.0 * M_PI * (double)lfo_rate * t));
            out[i] = (float)((double)out[i] * wob);
        }
    }

    /* env(t) = Σ amp_j·e^(-sharp·(t-off_j)) multiplies the mix; with every amp 0 an enabled burst is silence, so the UI
     * defaults ship audible amps. */
    if (burst_on) {
        double sharp = p->burst_sharp;
        double boff[4] = {
            p->burst1_off, p->burst2_off,
            p->burst3_off, p->burst4_off,
        };
        double bamp[4] = {
            p->burst1_amp, p->burst2_amp,
            p->burst3_amp, p->burst4_amp,
        };
        for (int i = 0; i < samples; i++) {
            double t = (double)i / (double)sampleRate;
            double env = 0.0;
            for (int j = 0; j < 4; j++) {
                if (bamp[j] != 0.0f && t >= (double)boff[j]) {
                    env += (double)bamp[j] * exp(-(double)sharp * (t - (double)boff[j]));
                }
            }
            out[i] = (float)((double)out[i] * env);
        }
    }

    /* ── Amplitude ADSR stage ── */
    if (env_on) {
        int rel_samples = (int)(rel * (float)sampleRate);
        /* Release takes at most half the note, so attack/decay/sustain keep the first half; a capped release ramps over the
         * half it is given, or the note would still be loud where the voice hard-cuts. */
        double rel_ramp = rel;
        if (rel_samples > samples / 2) {
            rel_samples = samples / 2;
            rel_ramp = (double)rel_samples / (double)sampleRate;
        }
        adsr_t env;
        adsr_init(&env, sampleRate, a, d, sus, rel_ramp, exp_curve);
        adsr_trigger(&env);
        int rel_start = samples - rel_samples;
        if (rel_start < 1) rel_start = 1;
        for (int i = 0; i < samples; i++) {
            if (i == rel_start) adsr_release(&env);
            out[i] = (float)(out[i] * adsr_tick(&env));
        }
    }
    /* env disabled → leave the raw generator output (constant unity gain). */

    /* ── Filter stage ── */
    if (filt_on) {
        mod_biquad filt;
        int cut_sweep   = lfo_on && lfo_target == 2 && lfo_depth > 0.0f && lfo_rate > 0.0f;
        int filt_env_on = filtenv_on && filtenv_amt > 0.0f && filtenv_decay > 0.0f;
        if (cut_sweep) {
            /* ± lfo_depth octaves, recomputed every 64 samples with the state-preserving setter (no click). */
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            for (int i = 0; i < samples; i++) {
                if ((i & 63) == 0) {
                    double t = (double)i / (double)sampleRate;
                    double ramp = (lfo_delay > 0.0f) ? fmin(t / (double)lfo_delay, 1.0) : 1.0;
                    double oct = (double)lfo_depth * ramp
                               * sin(2.0 * M_PI * (double)lfo_rate * t);
                    double c = (double)cutoff * pow(2.0, oct);
                    mod_biquad_set_coeffs(&filt, filt_type, c, (double)q, sampleRate);
                }
                out[i] = mod_biquad_tick_f(&filt, out[i]);
            }
        } else if (filt_env_on) {
            /* Brightens by filtenv_amt octaves at onset and decays over filtenv_decay s; recomputed every 64 samples, no click. */
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            double atk = (double)filtenv_attack;
            for (int i = 0; i < samples; i++) {
                if ((i & 63) == 0) {
                    double t = (double)i / (double)sampleRate;
                    /* attack > 0 rises over atk seconds first (a brass bloom); 0 is the plain onset-then-decay. */
                    double oct;
                    if (atk > 0.0 && t < atk) {
                        oct = (double)filtenv_amt * (t / atk);
                    } else {
                        double td = t - atk; if (td < 0.0) td = 0.0;
                        oct = (double)filtenv_amt * exp(-td / (double)filtenv_decay);
                    }
                    double c = (double)cutoff * pow(2.0, oct);
                    mod_biquad_set_coeffs(&filt, filt_type, c, (double)q, sampleRate);
                }
                out[i] = mod_biquad_tick_f(&filt, out[i]);
            }
        } else {
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            for (int i = 0; i < samples; i++) {
                out[i] = mod_biquad_tick_f(&filt, out[i]);
            }
        }
    }
    /* filter disabled → passthrough. */

    /* Before the body resonator: mouth, then instrument body. */
    if (formant_on) {
        modular_formant_process(out, sampleRate, samples, formant_vt,
                                formant_vowel, formant_mix, formant_shift,
                                formant_sing, formant_mrate, formant_mto,
                                formant_dry);
    }

    /* out = dry + Σ bandpass_k(out)·g_k·body_mix: vibrato sweeping harmonics across fixed body peaks is the shimmer. */
    if (body_model >= 1 && body_mix > 0.0f) {
        /* Moderate Q colours without ringing into a metallic drone; dense in 600-4000 Hz, the 2.2-3 kHz bridge hill carries the
         * brightness. */
        static const double violinF[] = {280, 460, 540, 660, 800, 950, 1100, 1280, 1500, 1700, 1950, 2200,
                                         2450, 2700, 2950, 3250, 3550, 3900, 4300, 4800, 5400, 6000};
        static const double violinQ[] = {  6,   7,   7,   8,   8,   9,    9,   10,   10,   10,    9,    9,
                                          10,   11,  11,   9,    8,    8,    7,    7,    6,    6};
        static const double violinG[] = {0.1, 0.12, 0.15, 0.2, 0.35, 0.45, 0.6, 0.65, 0.7, 0.7, 0.65, 0.85,
                                         0.9, 0.95, 0.9, 0.75, 0.6, 0.5, 0.4, 0.3, 0.25, 0.2};
        /* Helmholtz air (~100 Hz), top plate, back plate, then tapering modes; the 1.6-3.4 kHz modes give nylon presence at LOW
         * Q, since a high-Q upper resonance is what reads as a steel string. */
        static const double guitarF[] = {100, 195, 270, 400, 540, 700, 900, 1200, 1600, 2100, 2700, 3400};
        static const double guitarQ[] = { 12,  14,  12,   9,   8,   7,   6,    5,  2.2,  2.0,  1.8,  1.5};
        static const double guitarG[] = {0.9, 1.0, 0.7, 0.5, 0.4, 0.3, 0.25, 0.20, 0.16, 0.12, 0.08, 0.06};
        /* About an octave below the violin's and broader: the low wood modes reinforce the dominant 2nd partial, and the
         * bridge hill sits near 1.3-1.7 kHz, so the cello reads dark and woody. */
        static const double celloF[] = {104, 140, 190, 260, 350, 460, 600, 780, 1000, 1300, 1650, 2050, 2500, 3100};
        /* High Q made it a humming metallic drone. */
        static const double celloQ[] = {  4,   5,   4,   4,    3,   3,   3,   3,    3,    3,    3,    3,    3,    2};
        static const double celloG[] = {0.7, 1.0, 0.95, 0.7, 0.55, 0.5, 0.45, 0.4, 0.45, 0.5, 0.5, 0.4, 0.3, 0.18};
        /* The guitar box with HIGH-Q, strong upper modes: the ringing top is the steel zing nylon avoids. */
        static const double steelF[] = {100, 195, 270, 400, 540, 700, 900, 1200, 1600, 2100, 2700, 3400, 4200};
        static const double steelQ[] = { 10,  12,  10,   8,   7,   7,   6,    6,    5,    5,    4,    4,    3};
        static const double steelG[] = {0.6, 0.7, 0.55, 0.5, 0.45, 0.42, 0.4, 0.38, 0.35, 0.32, 0.28, 0.24, 0.18};
        /* 2 nylon, 4 steel, 3 cello, 1 (or any other) violin. */
        const double *mF, *mQ, *mG;
        int nM;
        if (body_model == 2) {
            mF = guitarF; mQ = guitarQ; mG = guitarG;
            nM = (int)(sizeof(guitarF) / sizeof(guitarF[0]));
        } else if (body_model == 4) {
            mF = steelF; mQ = steelQ; mG = steelG;
            nM = (int)(sizeof(steelF) / sizeof(steelF[0]));
        } else if (body_model == 3) {
            mF = celloF; mQ = celloQ; mG = celloG;
            nM = (int)(sizeof(celloF) / sizeof(celloF[0]));
        } else {
            mF = violinF; mQ = violinQ; mG = violinG;
            nM = (int)(sizeof(violinF) / sizeof(violinF[0]));
        }
        mod_biquad body[24]; /* 24 slots: fits violin (22) and guitar (8) with headroom */
        for (int k = 0; k < nM; k++) {
            mod_biquad_set(&body[k], 2 /* band-pass */, mF[k], mQ[k], sampleRate);
        }
        /* The body shapes the tone rather than adding peaks onto a dry saw, so the dry drops as mix grows. */
        double dryAmt = 1.0 - 0.85 * fmin((double)body_mix, 1.0);
        for (int i = 0; i < samples; i++) {
            double x = (double)out[i];
            double wet = 0.0;
            for (int k = 0; k < nM; k++) {
                wet += (double)mod_biquad_tick_f(&body[k], (float)x) * mG[k];
            }
            out[i] = (float)(x * dryAmt + wet * (double)body_mix);
        }
        /* A real cello radiates its low fundamental poorly, so the 2nd harmonic dominates; the range bottom (~65 Hz) is
         * realistically attenuated, higher notes unaffected. */
        if (body_model == 3) {
            const double hp_a = 0.987; /* fc ≈ 100 Hz @ 48k */
            double prevX = 0.0, prevY = 0.0;
            for (int i = 0; i < samples; i++) {
                double x = (double)out[i];
                double y = hp_a * (prevY + x - prevX);
                prevX = x;
                prevY = y;
                out[i] = (float)y;
            }
        }
    }

    /* A slow random walk drives loudness AND brightness together (more bow pressure, louder and brighter): the
     * non-repeating motion a steady synth tone lacks. */
    if (bow_dynamics > 0.0f) {
        unsigned int rng = noise_seed * 2654435761u + 2246822519u;
        double m = 0.0, target = 0.0, lp = 0.0;
        for (int i = 0; i < samples; i++) {
            if ((i % 11000) == 0) { /* new bow target every ~250 ms (a gentle bow gesture) */
                rng = rng * 1664525u + 1013904223u;
                target = ((double)(rng >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
            }
            sp_onepole_tick(&m, 0.0004, target); /* ~55 ms ramp → smooth, gentle bow wander */
            double amp = 1.0 + (double)bow_dynamics * m; /* loudness swing */
            if (amp < 0.0) amp = 0.0;
            double a = 0.70 + 0.42 * (double)bow_dynamics * m; /* brightness: LP coeff tracks m (wider swing) */
            if (a < 0.05) a = 0.05; else if (a > 0.995) a = 0.995;
            sp_onepole_tick(&lp, a, (double)out[i]);
            out[i] = (float)(lp * amp);
        }
    }

    /* ── Drive + gain post stage ── */
    if (drive_on && drive > 0.0f) {
        double k = 1.0f + drive * 3.0f;
        float norm = (float)tanh((double)k);
        for (int i = 0; i < samples; i++) {
            out[i] = (float)tanh((double)(out[i] * k)) / norm;
        }
    }
    if (gain != 1.0f) {
        for (int i = 0; i < samples; i++) out[i] = (float)(out[i] * gain);
    }

    modular_finish(out, sampleRate, samples, p);
}

