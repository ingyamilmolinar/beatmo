#include <math.h>
#include <string.h>
#include "modular.h"
#include "modular_stages.h"
#include "wavetable.h"
#include "adsr.h"
#include "fmsynth.h"
#include "noise.h"
#include "synth_post.h" /* apply_post_params + post_config + synth_params (POST stage) */

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
static wavetable_t modular_sine_wt, modular_saw_wt, modular_square_wt, modular_triangle_wt;
static int         modular_tables_ready = 0;

static void ensure_modular_tables(void) {
    if (modular_tables_ready) return;
    wt_generate_sine(&modular_sine_wt, modular_sine_buf, WT_DEFAULT_LENGTH);
    wt_generate_saw(&modular_saw_wt, modular_saw_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_square(&modular_square_wt, modular_square_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    wt_generate_triangle(&modular_triangle_wt, modular_triangle_buf, WT_DEFAULT_LENGTH, MODULAR_WT_HARMONICS);
    modular_tables_ready = 1;
}

static const wavetable_t *modular_table_for(int osc_type) {
    switch (osc_type) {
    case 1: return &modular_saw_wt;
    case 2: return &modular_square_wt;
    case 3: return &modular_triangle_wt;
    default: return &modular_sine_wt;
    }
}

/* Non-static accessor for the gen-bank slot oscillators (modular_stages.c):
 * lazily builds the shared tables then returns the SAME wavetable object the
 * legacy osc stage uses, so a slot sine is byte-identical to the legacy osc. */
const wavetable_t *modular_shared_table_for(int wave) {
    ensure_modular_tables();
    return modular_table_for(wave);
}

/* The RBJ biquad (mod_biquad + mod_biquad_set + mod_biquad_tick_f) lives in
 * modular_stages.c/.h now — shared with the gen-bank slot filters. */

/* ── Param accessors with NULL => default ─────────────────────────────────── */

static float mp_get(const modular_params *p, float fallback,
                    float value, int have) {
    return have ? value : fallback;
}

/* Build an fm_preset from live params for osc_type == FM. Operators run with a
 * steady (sustain=1) internal envelope so the voice-level amp ADSR shapes the
 * whole sound uniformly. */
static void build_fm_preset(const modular_params *p, double base_freq,
                            fm_preset *out) {
    memset(out, 0, sizeof(*out));
    out->num_ops = FM_MAX_OPS;
    out->base_freq = (float)base_freq;
    out->pitch_env_amount = 0.0f;
    out->pitch_env_decay = 0.0f;

    float ratio[FM_MAX_OPS] = {p->fm_op1_ratio, p->fm_op2_ratio, p->fm_op3_ratio, p->fm_op4_ratio};
    const float level[FM_MAX_OPS] = {p->fm_op1_level, p->fm_op2_level, p->fm_op3_level, p->fm_op4_level};
    const float depth[FM_MAX_OPS] = {p->fm_op1_depth, p->fm_op2_depth, p->fm_op3_depth, p->fm_op4_depth};

    for (int i = 0; i < FM_MAX_OPS; i++) {
        out->ops[i].freq_ratio = (ratio[i] > 0.0f) ? ratio[i] : 1.0f;
        out->ops[i].freq_offset = 0.0f;
        out->ops[i].amplitude = level[i];
        out->ops[i].attack_sec = 0.0f;
        out->ops[i].decay_sec = 0.0f;
        out->ops[i].sustain_level = 1.0f;
        out->ops[i].release_sec = 0.0f;
        out->ops[i].is_carrier = 0;
    }
    /* op1 is always a carrier. Its level (fm_op1_level) controls output gain;
     * we do NOT force it audible, so the level knob has real authority. */
    out->ops[0].is_carrier = 1;

    /* op1 self-feedback makes fm_op1_depth meaningful (DX7-style operator
     * feedback adds harmonics) instead of being an inert final-carrier knob. */
    out->mod_matrix[0][0] = depth[0];

    int alg = (int)lrintf(p->fm_algorithm);
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

/* ── Digital-waveguide bowed string (osc_type 7) ───────────────────────────
 * STK "Bowed" model (Smith 1986, after McIntyre-Schumacher-Woodhouse): the
 * string is two VELOCITY delay lines split at the bow point; the bow applies a
 * nonlinear stick-slip FRICTION (the bow grips the string, drags it, then slips)
 * driven continuously by a constant bow velocity. A bridge reflection LOSS
 * filter sets decay/brightness. Continuous excitation produces a SUSTAINED bowed
 * Helmholtz tone with the organic, self-oscillating character a static saw can't
 * make — this is what actually sounds like a bowed string vs. an electronic tone.
 * Bow position/pressure/brightness are sensible hardcoded defaults for now. */
static void render_bowed_string(float *out, int sampleRate, int samples,
                                double freq, unsigned int noise_seed) {
    enum { DMAX = 4096 };
    float neck[DMAX], bridge[DMAX];
    double L = (double)sampleRate / freq - 2.0; /* round-trip delay; -2 = loss-filter phase */
    if (L < 4.0) L = 4.0;
    const double bow_pos = 0.13;                 /* bow ~1/8 from the bridge (STK betaRatio) */
    int bridgeLen = (int)(L * bow_pos + 0.5);
    int neckLen   = (int)(L - (double)bridgeLen + 0.5);
    if (bridgeLen < 1) bridgeLen = 1;
    if (neckLen < 1) neckLen = 1;
    if (bridgeLen > DMAX - 1) bridgeLen = DMAX - 1;
    if (neckLen > DMAX - 1) neckLen = DMAX - 1;
    for (int k = 0; k < DMAX; k++) { neck[k] = 0.0f; bridge[k] = 0.0f; }
    int ni = 0, bi = 0;

    const double slope  = 3.0;    /* bow-table friction slope (higher = more bow pressure) */
    const double offset = 0.001;
    const double bowVel = 0.25;   /* constant bow speed; the amp ADSR shapes the final output */
    const double loss   = 0.55;   /* bridge one-pole-LP loss → brightness/decay (loop stability) */
    const double refl   = 0.99;   /* slight per-round-trip energy loss for stability */
    double lp = 0.0;
    noise_gen ng; noise_init(&ng, noise_seed);
    int attack = sampleRate / 40; /* ~25 ms bow ramp-in (no hard onset click) */
    if (attack < 1) attack = 1;

    for (int i = 0; i < samples; i++) {
        double neckOut   = neck[ni];
        double bridgeOut = bridge[bi];
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
        ni = (ni + 1) % neckLen;
        bi = (bi + 1) % bridgeLen;
        double y = bridgeOut * 4.0;                       /* string velocity at the bridge */
        if (!(y > -8.0 && y < 8.0)) y = 0.0;              /* guard against blow-up/NaN */
        out[i] = (float)y;
    }
}

/* Linear-interpolated fractional read from a circular delay buffer (so non-integer
 * delay lengths don't quantize the pitch — required for the wind/brass waveguides). */
static inline double pm_frac_read(const float *buf, int len, int wi, double delay) {
    double rp = (double)wi - delay;
    while (rp < 0.0) rp += (double)len;
    int i0 = (int)rp;
    int i1 = i0 + 1; if (i1 >= len) i1 = 0;
    double frac = rp - (double)i0;
    return (double)buf[i0] * (1.0 - frac) + (double)buf[i1] * frac;
}

/* ── Brass lip-reed waveguide (osc_type 8) ─────────────────────────────────
 * STK "Brass" (Cook): a TUBE delay line + a 2nd-order LIP RESONATOR (BiQuad at
 * the lip frequency) + a SQUARED nonlinearity (lip opening area ∝ displacement²,
 * which also rectifies). Self-oscillates into a real brass tone. The tube length
 * uses STK's ·2+3 (the note rides on the lip↔tube interaction). bore_refl is the
 * brightness/loss knob; lip_radius the lip Q; max_pressure the blow strength. */
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
    double dcx1 = 0.0, dcy1 = 0.0;
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
        double dy = in - dcx1 + 0.99 * dcy1; dcx1 = in; dcy1 = dy; in = dy; /* DC block */
        double tout = pm_frac_read(tube, LEN, wi, delay);
        tube[wi] = (float)in; wi++; if (wi >= LEN) wi = 0;
        lastOut = tout;
        if (!(tout > -8.0 && tout < 8.0)) tout = 0.0;
        out[i] = (float)tout;
    }
}

/* ── Reed-woodwind waveguide (osc_type 9) ──────────────────────────────────
 * STK "Clarinet" (Cook/Smith): a single BORE delay line + a nonlinear REED
 * TABLE (reflection coefficient = clamp(offset + slope·Δp, ±1) — the reed slaps
 * shut as breath pressure rises) + a one-zero bell LOSS filter. Self-oscillates
 * into a sustained reed tone. Oboe (double reed, conical bore → ALL harmonics)
 * uses a NON-inverting bell (bell_refl > 0, open-pipe round-trip = SR/f);
 * clarinet (single reed, cylindrical → ODD harmonics) uses an inverting bell
 * (bell_refl < 0, closed-open round-trip = SR/2f). reed_slope = reed stiffness,
 * loss_b0 = bell brightness, breath_target = blow strength. */
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
    double dcx1 = 0.0, dcy1 = 0.0;
    noise_gen ng; noise_init(&ng, noise_seed);
    int attack = sampleRate / 100; if (attack < 1) attack = 1; /* ~10 ms */
    for (int i = 0; i < samples; i++) {
        double env = (i < attack) ? (double)i / (double)attack : 1.0;
        double breath = breath_target * env;
        /* STK breath turbulence — also breaks the static fixed point so the reed
         * actually KICKS INTO self-oscillation rather than settling to silence. */
        breath += breath * 0.2 * (double)noise_white_tick(&ng);
        double lf = loss_b0 * lastOut + loss_b1 * lossPrevX; /* one-zero bell loss */
        lossPrevX = lastOut;
        double pdiff = bell_refl * lf - breath;          /* reflected bore minus mouth */
        double r = reed_offset + reed_slope * pdiff;     /* reed table (nonlinear) */
        if (r > 1.0) r = 1.0;
        if (r < -1.0) r = -1.0;
        double in = breath + pdiff * r;                  /* scatter back into the bore */
        double tout = pm_frac_read(bore, LEN, wi, delay);
        bore[wi] = (float)in; wi++; if (wi >= LEN) wi = 0; /* breath DC stays in the loop = the drive */
        lastOut = tout;
        /* DC-block the OUTPUT only (not the loop — the breath pressure is a DC
         * term that DRIVES the bore; blocking it in-loop kills self-oscillation). */
        double dy = tout - dcx1 + 0.995 * dcy1; dcx1 = tout; dcy1 = dy;
        double y = dy * 0.3;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        out[i] = (float)y;
    }
}

/* ── Air-jet (flute) waveguide (osc_type 10) ───────────────────────────────
 * STK "Flute" (Cook): a BORE delay + a shorter JET delay + a cubic jet
 * nonlinearity (the air sheet saturating, x³−x) + multiplicative BREATH NOISE
 * (flutes are breathy) + an inverting one-pole bore loss filter. Self-oscillates
 * into a breathy flute tone. jet_ratio = timbre/register, noise_gain =
 * breathiness, pole = brightness (higher → darker). */
static void render_flute(float *out, int sampleRate, int samples, double freq,
                         double jet_ratio, double noise_gain, double pole,
                         double max_pressure, unsigned int noise_seed) {
    enum { LEN = 8192, JLEN = 4096 };
    float bore[LEN], jet[JLEN];
    /* STK Flute pre-scales the bore by 0.66666 (the played note rides on the
     * jet↔bore interaction, not the bare bore resonance). Omitting it renders
     * ~1.5× sharp; with it the fundamental lands on `freq`. */
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
    double dcx1 = 0.0, dcy1 = 0.0;
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
        jet[jwi] = (float)pd; jwi++; if (jwi >= JLEN) jwi = 0;
        double jt = jout * (jout * jout - 1.0);   /* cubic jet nonlinearity x³−x */
        if (jt > 1.0) jt = 1.0;
        if (jt < -1.0) jt = -1.0;
        double dy = jt - dcx1 + 0.99 * dcy1; dcx1 = jt; dcy1 = dy; /* DC block */
        double in = dy + endRefl * temp;
        double tout = pm_frac_read(bore, LEN, bwi, boreLen); /* bore delay */
        bore[bwi] = (float)in; bwi++; if (bwi >= LEN) bwi = 0;
        boreLast = tout;
        double y = 0.3 * tout;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        out[i] = (float)y;
    }
}

/* ── Saxofony reed-cone waveguide (osc_type 11) ─────────────────────────────
 * STK "Saxofony" (Cook/Scavone): the SAME inverting reed loop as the clarinet,
 * but the bore is split into TWO delay lines at a BLOW POSITION and the reed is
 * injected OFF-CENTRE (position < 0.5). That asymmetry restores the EVEN
 * harmonics → the full even+odd series of a CONICAL bore (sax/oboe) instead of
 * the clarinet's odd-only. It stays stable because the inverting half-wave
 * resonator still pins the pitch (a NON-inverting "cone" reflection has no node
 * to lock onto → chaos — that was the earlier mistake). Reed slope is POSITIVE
 * for a sax. The output is tapped at the reed junction, NOT a delay read. */
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
    /* Seed the delay lines with energy so the limit cycle starts near full
     * amplitude — the loop gain is barely >1, so building from pure silence
     * takes ~2 s. The pre-warm below then settles this into the steady tone. */
    for (int k = 0; k < LEN; k++) {
        d0[k] = (float)(0.8 * (double)noise_white_tick(&ng));
        d1[k] = (float)(0.8 * (double)noise_white_tick(&ng));
    }
    int w0 = 0, w1 = 0;
    double ozx1 = 0.0;          /* one-zero loss filter previous input */
    double dcx1 = 0.0, dcy1 = 0.0;
    double vibPhase = 0.0, vibInc = 2.0 * M_PI * 5.2 / (double)sampleRate;
    int attack = sampleRate / 100; if (attack < 1) attack = 1; /* ~10 ms breath ramp */
    /* PRE-WARM: a self-oscillating reed takes ~0.2 s to build its limit cycle from
     * silence (and the timbre differs while it settles). Run the loop SILENTLY for
     * `warm` samples so out[0] starts at the established full-amplitude tone — no
     * initial quiet/different gap. (The amp ADSR shapes the audible onset.) */
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
        d1[w1] = (float)temp; w1++; if (w1 >= LEN) w1 = 0;
        d0[w0] = (float)(breath - pd * r - temp); w0++; if (w0 >= LEN) w0 = 0;
        double dy = o - dcx1 + 0.995 * dcy1; dcx1 = o; dcy1 = dy; /* DC block on OUTPUT */
        double y = dy * 0.4;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        if (i >= warm) out[i - warm] = (float)y; /* output only AFTER the limit cycle is established */
    }
}

EXPORT void render_modular_p(float *out, int sampleRate, int samples,
                             const modular_params *params) {
    if (!out || samples <= 0 || sampleRate <= 0) return;
    memset(out, 0, (size_t)samples * sizeof(float));

    int have = params != 0;
    const modular_params zero = {0};
    const modular_params *p = have ? params : &zero;

    int   osc_type   = (int)lrintf(p->osc_type);
    float octave     = p->osc_octave;
    float detune     = p->osc_detune;          /* cents */
    float pitch_st   = p->pitch;               /* semitones */
    /* Envelope: defaults when params is NULL or fields are zero-ish. */
    float a   = mp_get(p, 0.005f, p->amp_attack,  have);
    float d   = mp_get(p, 0.300f, p->amp_decay,   have);
    float sus = mp_get(p, 0.600f, p->amp_sustain, have);
    float rel = mp_get(p, 0.200f, p->amp_release, have);
    int   exp_curve = have ? ((int)lrintf(p->amp_curve)) : 1;
    int   filt_type = (int)lrintf(p->filter_type);
    float cutoff = mp_get(p, 8000.0f, p->filter_cutoff, have);
    float q      = mp_get(p, 0.707f, p->filter_resonance, have);
    float drive  = p->drive;
    float gain   = mp_get(p, 1.0f, p->gain, have);

    /* Per-stage bypass toggles. Fallback 1.0 keeps the NULL/zero-struct path
     * (have==false) fully enabled — bit-identical to the pre-toggle voice. */
    int osc_on   = mp_get(p, 1.0f, p->osc_enabled,    have) >= 0.5f;
    int fm_on    = mp_get(p, 1.0f, p->fm_enabled,     have) >= 0.5f;
    int env_on   = mp_get(p, 1.0f, p->env_enabled,    have) >= 0.5f;
    int filt_on  = mp_get(p, 1.0f, p->filter_enabled, have) >= 0.5f;
    int drive_on = mp_get(p, 1.0f, p->drive_enabled,  have) >= 0.5f;
    unsigned int noise_seed = (unsigned int)lrintf(mp_get(p, 0.0f, p->noise_seed, have));

    /* Phase-8C modulator stages. Fallback 0.0 = DISABLED (opposite polarity to
     * the pre-existing toggles above): these stages are NEW, so default-off is
     * what keeps every pre-Phase-8C render byte-identical. enabled=0 skips the
     * stage code entirely (the numeric knobs are never read) — exact bypass. */
    int   pe_on     = mp_get(p, 0.0f, p->pitchenv_enabled, have) >= 0.5f;
    float pe_amt    = mp_get(p, 0.0f, p->pitchenv_amt,     have);
    float pe_decay  = mp_get(p, 0.0f, p->pitchenv_decay,   have);
    int   lfo_on    = mp_get(p, 0.0f, p->lfo_enabled,      have) >= 0.5f;
    float lfo_rate  = mp_get(p, 0.0f, p->lfo_rate,         have);
    float lfo_depth = mp_get(p, 0.0f, p->lfo_depth,        have);
    int   lfo_target = (int)lrintf(mp_get(p, 0.0f, p->lfo_target, have));
    float lfo_delay = mp_get(p, 0.0f, p->lfo_delay, have);
    int   burst_on  = mp_get(p, 0.0f, p->burst_enabled,    have) >= 0.5f;
    int   filtenv_on     = mp_get(p, 0.0f, p->filtenv_enabled, have) >= 0.5f;
    float filtenv_amt    = mp_get(p, 0.0f, p->filtenv_amt,   have);
    float filtenv_decay  = mp_get(p, 0.0f, p->filtenv_decay, have);
    float filtenv_attack = mp_get(p, 0.0f, p->filtenv_attack, have);
    int   body_model     = (int)lrintf(mp_get(p, 0.0f, p->body_model, have));
    float body_mix       = mp_get(p, 0.0f, p->body_mix, have);
    float bow_dynamics   = mp_get(p, 0.0f, p->bow_dynamics, have);

    /* Phase-8E unison: identity default 1 (single osc = no change). */
    int   unison_nv     = (int)lrintf(mp_get(p, 1.0f, p->unison_voices, have));
    float unison_detune = mp_get(p, 0.0f, p->unison_detune, have);
    float unison_mix    = mp_get(p, 0.5f, p->unison_mix,    have);
    /* Phase-8F drift: identity when either is 0. */
    float unison_drift_rate  = mp_get(p, 0.0f, p->unison_drift_rate,  have);
    float unison_drift_depth = mp_get(p, 0.0f, p->unison_drift_depth, have);
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

    /* voice_freq_hz override (Phase 2): >0 sets the voice freq to an EXACT Hz,
     * applied AFTER the pow derivation. Identity 0 keeps the derived freq, so
     * every pre-Phase-2 preset is byte-unchanged. Used by migrated family
     * presets whose fundamental (e.g. 45 Hz) is not reachable via 220·2^x. */
    {
        double vfh = (double)mp_get(p, 0.0f, p->voice_freq_hz, have);
        if (vfh > 0.0) freq = vfh;
    }

    /* osc_wrote tracks whether the legacy osc stage ASSIGNED into out[] this
     * call. When it did not (osc disabled / no source), the gen bank's first
     * active slot must ASSIGN rather than accumulate into the zeroed buffer —
     * otherwise 0.0f + (-0.0f) would flip a legacy -0.0 sample to +0.0f. */
    int osc_wrote = 0;
    int vib_on = lfo_on && lfo_target == 1 && lfo_depth > 0.0f && lfo_rate > 0.0f;

    /* ── Oscillator / generator stage ── */
    if (!osc_on) {
        /* Generator disabled by the user → silence (out[] is already zeroed).
         * The generator is the sound source, so "bypass" means no source. */
    } else if (osc_type == 5 || osc_type == 6) {
        noise_gen ng;
        noise_init(&ng, noise_seed);
        if (osc_type == 6) {
            for (int i = 0; i < samples; i++) out[i] = noise_pink_tick(&ng);
        } else {
            for (int i = 0; i < samples; i++) out[i] = noise_white_tick(&ng);
        }
        osc_wrote = 1;
    } else if (osc_type == 4) {
        fm_preset preset;
        build_fm_preset(p, freq, &preset);
        if (!fm_on) {
            /* FM disabled → carrier-only: drop all modulation routing (incl.
             * op1 self-feedback) so only the base carrier sounds. */
            memset(preset.mod_matrix, 0, sizeof(preset.mod_matrix));
        }
        if (pe_on) {
            /* PITCH ENV on the FM core: the fm_preset already carries the
             * exponential semitone sweep (pitch_env_amount / pitch_env_decay,
             * fm_render lines ~127-129); build_fm_preset zeroes them, so the
             * stage just fills them in. Stage off ⇒ fields stay 0 ⇒ the exact
             * pre-Phase-8C preset. */
            preset.pitch_env_amount = pe_amt;
            preset.pitch_env_decay  = pe_decay;
        }
        fm_render(&preset, out, sampleRate, samples);
        /* fm_render applies its own steady envelope + softsat; we still shape
         * with the voice amp ADSR below. */
        osc_wrote = 1;
    } else if (osc_type == 7) {
        /* Bowed-string digital waveguide (physical model) — see render_bowed_string. */
        render_bowed_string(out, sampleRate, samples, freq, noise_seed);
        osc_wrote = 1;
    } else if (osc_type == 8) {
        /* Brass lip-reed waveguide. NOTE: not yet a clean self-oscillator — the
         * lip BiQuad + squared nonlinearity tends to chaos rather than a locked
         * tone, so no instrument ships on it yet (trumpet stays subtractive). The
         * output is GUARDED finite/bounded (see test TestWaveguideOscTypesAreFinite
         * + the y-guard below) so it's safe; the low lip_gain is the documented
         * starting point for future stabilisation. See reference_instrument_synth_tuning. */
        render_brass(out, sampleRate, samples, freq, 0.85, 0.997, 0.012, 0.5);
        osc_wrote = 1;
    } else if (osc_type == 9) {
        /* Reed-woodwind waveguide. Hardcoded OBOE defaults (double reed: non-
         * inverting bell → all harmonics; reed stiffness + blow + loss filter
         * tuned to oscillate AND give the bright, nasal, reedy oboe spectrum). */
        render_reed(out, sampleRate, samples, freq, -0.95, 0.7, -0.3, 0.8, 0.5, noise_seed);
        osc_wrote = 1;
    } else if (osc_type == 10) {
        /* Air-jet (flute) waveguide. Hardcoded breathy-flute defaults; higher
         * drive + lower loss so it sustains oscillation at the prescaled bore. */
        render_flute(out, sampleRate, samples, freq, 0.4, 0.2, 0.7, 1.0, noise_seed);
        osc_wrote = 1;
    } else if (osc_type == 11) {
        /* Saxofony reed-cone waveguide. Hardcoded BARITONE-SAX defaults: off-centre
         * blow position (all harmonics), positive reed slope, hard breath + low
         * offset for the dense honking low tone, bright loss filter. */
        render_sax(out, sampleRate, samples, freq, 0.15, 0.58, 0.28, -0.94, 0.85, 0.7, noise_seed);
        osc_wrote = 1;
    } else if ((pe_on && pe_amt != 0.0f && pe_decay > 0.0f) || vib_on) {
        /* Wavetable OSC with per-sample frequency modulation: pitch-env
         * (exp semitone sweep) and/or LFO vibrato (sinusoidal semitone wobble),
         * summed in semitones then applied as a 2^(st/12) multiplier. When
         * vib_on is false this is bit-identical to the legacy pitch-env branch;
         * when both are off control never reaches here (the plain branch below
         * runs), preserving byte-identity for every pre-change instrument. */
        ensure_modular_tables();
        wt_osc_t osc;
        wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
        int pe_active = pe_on && pe_amt != 0.0f && pe_decay > 0.0f;
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
        osc_wrote = 1;
    } else if (unison_nv >= 2) {
        /* Phase-8E unison: N detuned wt_osc copies with decorrelated phases.
         * Center voice (i=0) runs at exact pitch; side voices (i=1..nv-1) are
         * detuned symmetrically ±detune_cents/2 with phase offset i*π/nv for
         * decorrelation. Sum all voices and normalize by 1/sqrt(nv) for RMS
         * stability. unison_mix blends from center-only (0) to ensemble (1). */
        ensure_modular_tables();
        const wavetable_t *wt = modular_table_for(osc_type);
        wt_osc_t osc;
        wt_osc_init(&osc, wt, freq, sampleRate);
        /* side-voice oscillators (max 6 side voices for nv up to 7). */
        wt_osc_t side[6];
        double side_freq[6];
        int ns = unison_nv - 1; /* number of side voices */
        for (int v = 0; v < ns; v++) {
            /* Spread ns side voices evenly across [-1, +1].
             * For ns==1 (voices==2) the old code gave spread=0 (no detuning).
             * Fix: use max(ns-1,1) so one side voice lands at -detune, not center. */
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
        /* Per-voice drift LFO parameters (only used when drift_on). */
        double drift_rate_v[6], drift_phase_v[6], drift_base_cents[6];
        if (drift_on) {
            for (int v = 0; v < ns; v++) {
                /* Each voice gets a slightly different rate for decorrelation. */
                drift_rate_v[v]  = (double)unison_drift_rate * (1.0 + 0.13 * (double)v);
                drift_phase_v[v] = 2.0 * M_PI * (double)(v + 1) / (double)unison_nv;
                /* Remember the static base detune so we can add drift on top. */
                double spread = -1.0 + 2.0 * (double)v / (double)(ns > 1 ? ns - 1 : 1);
                drift_base_cents[v] = (double)unison_detune * spread;
            }
        }
        for (int i = 0; i < samples; i++) {
            if (drift_on) {
                double t = (double)i / (double)sampleRate;
                for (int v = 0; v < ns; v++) {
                    double drift_cents = (double)unison_drift_depth
                        * sin(2.0 * M_PI * drift_rate_v[v] * t + drift_phase_v[v]);
                    double total_cents = drift_base_cents[v] + drift_cents;
                    wt_osc_set_freq(&side[v], freq * pow(2.0, total_cents / 1200.0), sampleRate);
                }
            }
            double center = (double)wt_osc_tick(&osc);
            double ensemble = center;
            for (int v = 0; v < ns; v++) ensemble += (double)wt_osc_tick(&side[v]);
            ensemble *= norm;
            out[i] = (float)((1.0 - mix_blend) * center + mix_blend * ensemble);
        }
        osc_wrote = 1;
    } else {
        ensure_modular_tables();
        wt_osc_t osc;
        wt_osc_init(&osc, modular_table_for(osc_type), freq, sampleRate);
        for (int i = 0; i < samples; i++) {
            out[i] = wt_osc_tick(&osc);
        }
        osc_wrote = 1;
    }

    /* ── Gen bank (Phase 1, modular unification): parallel slots accumulate
     * into the mix AFTER the legacy osc stage writes. All-identity params ⇒
     * no active slot ⇒ exact no-op (byte-identity invariant). Runs even when
     * osc_enabled == 0 (slots are independent of the legacy osc gate). ── */
    modular_gen_bank_render(out, sampleRate, samples, p, freq, noise_seed, osc_wrote);

    /* ── Phase-8C LFO stage: post-mix amp wobble. Multiplies the FULL mix
     * (legacy osc + gen-bank family voices), so it is a real stage on every
     * instrument. gain(t) = 1 - depth·(0.5 - 0.5·sin(2πft)) wobbles in
     * [1-depth, 1] — never exceeds unity, starts at the midpoint heading up.
     * Disabled (the default) ⇒ the loop never runs ⇒ exact bypass. ── */
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

    /* ── Phase-8C BURST stage: post-mix multi-burst gate (clap-style staggered
     * transients on any source). env(t) = Σ_j amp_j·e^(-sharp·(t-off_j)) for
     * t ≥ off_j; out is multiplied by the summed envelope. With every amp at 0
     * (the identity) an ENABLED burst stage gates to silence — the UI defaults
     * (modular_recipe.go) ship amp1=1 + staggered 2/3 so enabling is audible.
     * Disabled (the default) ⇒ exact bypass. ── */
    if (burst_on) {
        float sharp = mp_get(p, 0.0f, p->burst_sharp, have);
        float boff[4] = {
            mp_get(p, 0.0f, p->burst1_off, have), mp_get(p, 0.0f, p->burst2_off, have),
            mp_get(p, 0.0f, p->burst3_off, have), mp_get(p, 0.0f, p->burst4_off, have),
        };
        float bamp[4] = {
            mp_get(p, 0.0f, p->burst1_amp, have), mp_get(p, 0.0f, p->burst2_amp, have),
            mp_get(p, 0.0f, p->burst3_amp, have), mp_get(p, 0.0f, p->burst4_amp, have),
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
        adsr_t env;
        adsr_init(&env, sampleRate, a, d, sus, rel, exp_curve);
        adsr_trigger(&env);
        int rel_samples = (int)(rel * (float)sampleRate);
        /* Cap release to at most half the note so attack/decay/sustain always
         * get the first half — otherwise a note shorter than the release time
         * would collapse straight into release and the decay knob would do
         * nothing. */
        if (rel_samples > samples / 2) rel_samples = samples / 2;
        int rel_start = samples - rel_samples;
        if (rel_start < 1) rel_start = 1;
        for (int i = 0; i < samples; i++) {
            if (i == rel_start) adsr_release(&env);
            out[i] *= adsr_tick(&env);
        }
    }
    /* env disabled → leave the raw generator output (constant unity gain). */

    /* ── Filter stage ── */
    if (filt_on) {
        mod_biquad filt;
        int cut_sweep   = lfo_on && lfo_target == 2 && lfo_depth > 0.0f && lfo_rate > 0.0f;
        int filt_env_on = filtenv_on && filtenv_amt > 0.0f && filtenv_decay > 0.0f;
        if (cut_sweep) {
            /* LFO cutoff sweep: ± lfo_depth octaves around the base cutoff,
             * recomputed every 64 samples with the state-preserving coeff
             * setter (no click). Target 0/1 take the static path above, so this
             * branch is reached ONLY for cutoff-target instruments — every
             * pre-change instrument is byte-identical. */
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
            /* Filter envelope: cutoff brightens by filtenv_amt octaves at onset,
             * decaying exponentially with time constant filtenv_decay seconds.
             * Recomputed every 64 samples with the state-preserving coeff setter
             * (no click). When filtenv_on is false, control falls through to the
             * existing static path below — byte-identical. */
            mod_biquad_set(&filt, filt_type, (double)cutoff, (double)q, sampleRate);
            double atk = (double)filtenv_attack;
            for (int i = 0; i < samples; i++) {
                if ((i & 63) == 0) {
                    double t = (double)i / (double)sampleRate;
                    /* With attack>0: RISE 0→amt octaves over `atk` seconds (the
                     * brass crescendo bloom), then decay/hold. attack==0 reduces
                     * EXACTLY to the legacy onset-bloom-then-decay (byte-identical). */
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

    /* ── Body-resonator bank ── (organic string/violin body)
     * out = dry + Σ_k (bandpass_k(out) · g_k) · body_mix. The narrow band-passes
     * are an instrument body's signature modes; vibrato sweeping the harmonics
     * across these fixed peaks modulates each harmonic = the timbre shimmer.
     * body_model 0 (default) skips this entirely → byte-identical. */
    if (body_model >= 1 && body_mix > 0.0f) {
        /* Violin signature modes: air/body (A0/B1) + a dense series up through the
         * ~2.6 kHz "bridge hill" (Hz, Q, relative gain). */
        /* MODERATE Q = body COLORATION (the violin's formant/resonance shape) without
         * ringing into a metallic/organ-like drone. (The dramatic per-note "shimmer"
         * is the PLAYER'S bowing, handled by bow_dynamics — not the static body.)
         * ~22 modes, dense in the 600-4000Hz region where bowed-string harmonics live;
         * the bridge-hill region (2.2-3kHz) carries the brightness. */
        static const double violinF[] = {280, 460, 540, 660, 800, 950, 1100, 1280, 1500, 1700, 1950, 2200,
                                         2450, 2700, 2950, 3250, 3550, 3900, 4300, 4800, 5400, 6000};
        static const double violinQ[] = {  6,   7,   7,   8,   8,   9,    9,   10,   10,   10,    9,    9,
                                          10,   11,  11,   9,    8,    8,    7,    7,    6,    6};
        static const double violinG[] = {0.1, 0.12, 0.15, 0.2, 0.35, 0.45, 0.6, 0.65, 0.7, 0.7, 0.65, 0.85,
                                         0.9, 0.95, 0.9, 0.75, 0.6, 0.5, 0.4, 0.3, 0.25, 0.2};
        /* Classical guitar body modes: Helmholtz air resonance (~100 Hz), top-plate
         * monopole (~195 Hz), back-plate coupling (~270 Hz), then body modes tapering
         * up. The low-mid emphasis is the guitar "box" character; the upper modes
         * (1.6–3.4 kHz) carry the bright nylon "presence" the real reference has —
         * without them (and with the post-LP opened up) the render reads too dull
         * (centroid ~990 vs the real ~1710). */
        /* NYLON character: the upper modes (1.6–3.4 kHz) are kept for presence but
         * given LOW Q (broad, ~2 not ~4) so they color the attack WITHOUT a high-Q
         * metallic RING — a high-Q upper resonance is exactly what makes a render
         * read as a bright STEEL/acoustic string. Nylon = warm sustain, bright but
         * non-ringing top. (User feedback: bright bank read "metallic acoustic".) */
        static const double guitarF[] = {100, 195, 270, 400, 540, 700, 900, 1200, 1600, 2100, 2700, 3400};
        static const double guitarQ[] = { 12,  14,  12,   9,   8,   7,   6,    5,  2.2,  2.0,  1.8,  1.5};
        static const double guitarG[] = {0.9, 1.0, 0.7, 0.5, 0.4, 0.3, 0.25, 0.20, 0.16, 0.12, 0.08, 0.06};
        /* Cello body modes: the cello box is ~2x the violin, so its resonances sit
         * roughly an octave LOWER and broader. A0 air (~104 Hz) + the main wood
         * resonances (B1-/B1+, ~140-190 Hz) carry the woody warmth and REINFORCE the
         * low harmonics (the real cello's dominant 2nd partial), tapering up through a
         * broad low-mid formant; the cello "bridge hill" (~1.3-1.7 kHz) is much lower
         * than the violin's 2.5 kHz, so the cello reads dark/woody, not bright/buzzy. */
        static const double celloF[] = {104, 140, 190, 260, 350, 460, 600, 780, 1000, 1300, 1650, 2050, 2500, 3100};
        /* LOW Q (gentle body COLORATION, not ringing) — high Q here made it a humming
         * metallic/radio drone, not a bowed string (see the moderate-Q note above). */
        static const double celloQ[] = {  4,   5,   4,   4,    3,   3,   3,   3,    3,    3,    3,    3,    3,    2};
        static const double celloG[] = {0.7, 1.0, 0.95, 0.7, 0.55, 0.5, 0.45, 0.4, 0.45, 0.5, 0.5, 0.4, 0.3, 0.18};
        /* STEEL-string acoustic guitar body (body_model:4): the SAME box as the
         * classical guitar but with HIGH-Q, STRONG upper modes (1.6–4.2 kHz) — that
         * bright, RINGING top is the "metallic" steel zing (exactly what made the
         * nylon read as steel before its upper modes were broadened). Nylon
         * (body_model:2) keeps those modes low-Q/warm; steel rings. */
        static const double steelF[] = {100, 195, 270, 400, 540, 700, 900, 1200, 1600, 2100, 2700, 3400, 4200};
        static const double steelQ[] = { 10,  12,  10,   8,   7,   7,   6,    6,    5,    5,    4,    4,    3};
        static const double steelG[] = {0.6, 0.7, 0.55, 0.5, 0.45, 0.42, 0.4, 0.38, 0.35, 0.32, 0.28, 0.24, 0.18};
        /* Select mode table by body_model: 2 = classical/nylon guitar, 4 = steel
         * guitar (bright/ringing), 3 = cello, 1 (or any other) = violin. */
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
        /* As body_mix rises the BODY shapes the tone (the harmonics move THROUGH
         * the resonances) rather than peaks merely added onto a steady dry saw —
         * so suppress the dry as mix grows. dryAmt(0)=1 → byte-identical off. */
        double dryAmt = 1.0 - 0.85 * fmin((double)body_mix, 1.0);
        for (int i = 0; i < samples; i++) {
            double x = (double)out[i];
            double wet = 0.0;
            for (int k = 0; k < nM; k++) {
                wet += (double)mod_biquad_tick_f(&body[k], (float)x) * mG[k];
            }
            out[i] = (float)(x * dryAmt + wet * (double)body_mix);
        }
        /* Cello (body_model 3): gentle one-pole high-pass (~100 Hz) to suppress the
         * boomy fundamental — on a real cello the low fundamental radiates poorly, so
         * the 2nd harmonic dominates. This removes the low "hum" and gives the dominant-
         * 2nd-harmonic signature. Note-safe: cello range bottoms at C2 (~65 Hz) whose
         * weak fundamental is realistically attenuated; higher notes are unaffected. */
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

    /* ── Bow dynamics (bowing expression) ──
     * A slow smoothed random walk m(t) (the bow pressure/speed wandering) drives
     * BOTH loudness and brightness together (more bow pressure → louder AND
     * brighter), giving the organic, non-repeating performance motion a steady
     * synth tone lacks. Brightness = a one-pole LP whose cutoff follows m.
     * bow_dynamics 0 (default) skips this → byte-identical. */
    if (bow_dynamics > 0.0f) {
        unsigned int rng = noise_seed * 2654435761u + 2246822519u;
        double m = 0.0, target = 0.0, lp = 0.0;
        for (int i = 0; i < samples; i++) {
            if ((i % 11000) == 0) { /* new bow target every ~250 ms (a gentle bow gesture) */
                rng = rng * 1664525u + 1013904223u;
                target = ((double)(rng >> 8) / (double)(1u << 24)) * 2.0 - 1.0;
            }
            m += 0.0004 * (target - m); /* ~55 ms ramp → smooth, gentle bow wander */
            double amp = 1.0 + (double)bow_dynamics * m; /* loudness swing */
            if (amp < 0.0) amp = 0.0;
            double a = 0.70 + 0.42 * (double)bow_dynamics * m; /* brightness: LP coeff tracks m (wider swing) */
            if (a < 0.05) a = 0.05; else if (a > 0.995) a = 0.995;
            lp += a * ((double)out[i] - lp);
            out[i] = (float)(lp * amp);
        }
    }

    /* ── Drive + gain post stage ── */
    if (drive_on && drive > 0.0f) {
        float k = 1.0f + drive * 3.0f;
        float norm = (float)tanh((double)k);
        for (int i = 0; i < samples; i++) {
            out[i] = (float)tanh((double)(out[i] * k)) / norm;
        }
    }
    if (gain != 1.0f) {
        for (int i = 0; i < samples; i++) out[i] *= gain;
    }

    /* ── Shared POST stage (Phase 2). Runs at the VERY END, after gain, when
     * post_enabled >= 0.5. It calls the SAME compiled apply_post_params used by
     * the legacy drums.c _p() renderers, so a migrated family preset is
     * byte-identical to its legacy post-processing by construction. A
     * synth_params base is assembled from the post_* params and a post_config
     * from post_decay_rate + the post_*_on gates. Legacy renderers apply NO
     * safety clamp after post, so when POST runs the modular clamp is skipped
     * (the clamp only guards the pre-Phase-2 path). ── */
    int post_on = mp_get(p, 0.0f, p->post_enabled, have) >= 0.5f;
    if (post_on) {
        synth_params base;
        base.pitch      = mp_get(p, 0.0f, p->post_pitch, have);
        base.decay      = mp_get(p, 1.0f, p->post_decay, have);
        base.tone       = mp_get(p, 0.0f, p->post_tone, have);
        base.drive      = mp_get(p, 0.0f, p->post_drive, have);
        base.body       = mp_get(p, 0.0f, p->post_body, have);
        base.brightness = mp_get(p, 0.0f, p->post_brightness, have);
        base.fundamental = 0.0f; /* unread by apply_post_params */
        post_config cfg;
        cfg.decay_rate = (double)mp_get(p, 0.0f, p->post_decay_rate, have);
        cfg.pitch      = mp_get(p, 1.0f, p->post_pitch_on, have) >= 0.5f;
        cfg.decay      = mp_get(p, 1.0f, p->post_decay_on, have) >= 0.5f;
        cfg.drive      = mp_get(p, 1.0f, p->post_drive_on, have) >= 0.5f;
        cfg.tone       = mp_get(p, 1.0f, p->post_tone_on, have) >= 0.5f;
        cfg.body       = mp_get(p, 1.0f, p->post_body_on, have) >= 0.5f;
        cfg.brightness = mp_get(p, 1.0f, p->post_brightness_on, have) >= 0.5f;

        int post_order = (int)lrintf(mp_get(p, 0.0f, p->post_order, have));
        if (post_order == 1) {
            /* Legacy BASE kick (render_kick_p) op ORDER: pitch → decay → drive →
             * body. apply_post_params applies body BEFORE drive; the base kick's
             * bespoke inline post applies drive BEFORE body. The op IMPLEMENTATIONS
             * are byte-identical (same resample, same decay formula+clamp, same
             * sp_saturate, same one-pole body mix), so we reproduce the legacy
             * SEQUENCE by calling the shared helper twice with disjoint configs:
             * first {pitch,decay,drive} (body off → pitch→decay→drive), then
             * {body}. This shares the op code while transplanting the order. The
             * caller only sets order 1 for the base kick, whose POST wiring is
             * {pitch,decay,drive,body}; brightness/tone are off there. */
            post_config first = cfg;
            first.body = 0;
            first.brightness = 0;
            first.tone = 0;
            apply_post_params(out, sampleRate, samples, &base, &first);
            post_config bodyOnly;
            bodyOnly.decay_rate = cfg.decay_rate;
            bodyOnly.pitch = 0;
            bodyOnly.decay = 0;
            bodyOnly.drive = 0;
            bodyOnly.tone = 0;
            bodyOnly.body = cfg.body;
            bodyOnly.brightness = 0;
            apply_post_params(out, sampleRate, samples, &base, &bodyOnly);
            return;
        }
        if (post_order == 2) {
            /* Legacy BASE snare (render_snare_p) op ORDER: pitch → decay → drive →
             * tone. apply_post_params applies tone BEFORE drive; the base snare's
             * bespoke inline post applies drive BEFORE tone. The op IMPLEMENTATIONS
             * are byte-identical (same resample, same decay formula+clamp, same
             * sp_saturate, same tone<0 one-pole LP), so we reproduce the legacy
             * SEQUENCE by calling the shared helper twice with disjoint configs:
             * first {pitch,decay,drive} (tone off → pitch→decay→drive), then {tone}.
             * The base snare's POST wiring is {pitch,decay,drive,tone}; body/
             * brightness are off there. */
            post_config first = cfg;
            first.tone = 0;
            first.body = 0;
            first.brightness = 0;
            apply_post_params(out, sampleRate, samples, &base, &first);
            post_config toneOnly;
            toneOnly.decay_rate = cfg.decay_rate;
            toneOnly.pitch = 0;
            toneOnly.decay = 0;
            toneOnly.drive = 0;
            toneOnly.tone = cfg.tone;
            toneOnly.body = 0;
            toneOnly.brightness = 0;
            apply_post_params(out, sampleRate, samples, &base, &toneOnly);
            return;
        }

        apply_post_params(out, sampleRate, samples, &base, &cfg);
        return;
    }

    /* Safety clamp (pre-Phase-2 path only — legacy post renderers don't clamp). */
    for (int i = 0; i < samples; i++) {
        if (out[i] > 1.5f) out[i] = 1.5f;
        else if (out[i] < -1.5f) out[i] = -1.5f;
    }
}

EXPORT void render_modular(float *out, int sampleRate, int samples) {
    render_modular_p(out, sampleRate, samples, 0);
}
