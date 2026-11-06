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

/* Thread-safe: the first renders of a session run on several prewarm workers
 * at once (once.h). */
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

/* Non-static accessor for the gen-bank slot oscillators (modular_stages.c):
 * lazily builds the shared tables then returns the SAME wavetable object the
 * legacy osc stage uses, so a slot sine is byte-identical to the legacy osc. */
const wavetable_t *modular_shared_table_for(int wave) {
    ensure_modular_tables();
    return modular_table_for(wave);
}

/* The RBJ biquad (mod_biquad + mod_biquad_set + mod_biquad_tick_f) lives in
 * modular_stages.c/.h now — shared with the gen-bank slot filters. */

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
    /* op1 is always a carrier. Its level (fm_op1_level) controls output gain;
     * we do NOT force it audible, so the level knob has real authority. */
    out->ops[0].is_carrier = 1;

    /* op1 self-feedback makes fm_op1_depth meaningful (DX7-style operator
     * feedback adds harmonics) instead of being an inert final-carrier knob. */
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

/* KNOWN GAP (2026-09-01): this model cannot produce a bow BITE, and that is a
 * signal-chain limitation rather than a tuning one.
 *
 * Measured per-frame noise fraction over eighths of one note:
 *
 *   reference violin  0.201 0.013 0.011 0.010 0.010 0.009 0.013 0.011
 *   synth violin      0.021 0.019 0.017 0.017 0.017 0.017 0.018 0.018
 *
 * In STEADY STATE the synth is already close (0.017 vs 0.010). What it lacks
 * entirely is the ATTACK: the reference's broadband 0.20 scrape as the hair
 * grabs the string before Helmholtz motion locks in. A note that fades in
 * without a bite is what a BLOWN instrument does, and the listener reported the
 * viola and violin as "a wind instrument, like a flute instead of a string" and
 * the cello as "too airy" -- see noise.persistence in
 * internal/audio/fingerprint/registry_perceptual.go, which agrees with that
 * wording on 5 of the 6 families where it was used.
 *
 * Three fixes were tried and MEASURED, and none moved the onset past 0.023:
 *   1. scaling the multiplicative friction term 0.03 -> 0.35. It perturbs bow
 *      VELOCITY, and the waveguide shapes the result into tone, not noise.
 *   2. injecting broadband noise additively into newVel (the string state).
 *      The string is a resonator: energy put in comes back out AT THE
 *      HARMONICS.
 *   3. adding it directly to the radiated output y. Removed by the voice's own
 *      low-pass -- the violin runs filter_cutoff 3100 Hz, and a scrape is
 *      mostly above that. Confirmed by rendering at filter_cutoff 1200, which
 *      drops the onset further to 0.011.
 *
 * The blocker is ORDER: every noise source in the modular pipeline is injected
 * PRE-filter (see the Phase-15 breath stage), so nothing broadband can survive
 * to the output of a voice that low-passes for its tone. Producing a bite needs
 * a POST-filter transient/noise stage, which is a new pipeline stage with
 * schema and ABI consequences -- deliberately not attempted blind. Until then
 * the bowed strings will keep reading as blown, and noise.persistence is the
 * axis that will show it moving when the stage lands.
 */


/* Linear-interpolated fractional read from a circular delay buffer (so non-integer
 * delay lengths don't quantize the pitch — required by the bowed-string,
 * wind and brass waveguides). */
static inline double pm_frac_read(const float *buf, int len, int wi, double delay) {
    double rp = (double)wi - delay;
    while (rp < 0.0) rp += (double)len;
    int i0 = (int)rp;
    int i1 = i0 + 1; if (i1 >= len) i1 = 0;
    double frac = rp - (double)i0;
    return (double)buf[i0] * (1.0 - frac) + (double)buf[i1] * frac;
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
                                double freq, double bow_pos, double slope,
                                double bowVel, double loss, unsigned int noise_seed) {
    enum { DMAX = 4096 };
    float neck[DMAX], bridge[DMAX];
    /* Round-trip delay. The -2 models the loss filter's own phase delay, which
     * is why the two delay lines are SHORTER than the raw period.
     *
     * Both lengths are FRACTIONAL and read through pm_frac_read, exactly as the
     * brass/flute/clarinet waveguides in this file do. They used to be rounded
     * to integers (`(int)(x + 0.5)`) with a `% len` circular read, which
     * quantised the pitch: the realised period came out a roughly CONSTANT ~1.2
     * samples short, so the error stayed inaudible on low notes and grew with
     * frequency as that deficit became a larger fraction of a shorter period.
     * Measured on the shipped violin before this change: +12 cents at node
     * pitch 0, +26 at +15, +39 at +22, +48 at +27 — nearly a quarter tone sharp
     * at the top of the range, against flute/oboe/piano holding within 2 cents.
     * Cello showed about half the error, as expected: it renders an octave
     * lower, so the same sample deficit spans half the cents.
     *
     * The subtracted constant is 1, not the 2 it used to be. With the pitch no
     * longer quantised the loop's true extra delay can be solved for directly
     * from the rendered audio, and it comes out at 1.02-1.08 samples across the
     * whole range (one sample for the write-then-read structure, plus the
     * one-pole loss filter's small phase) — flat enough to model as a constant.
     * Subtracting 2 therefore made every bowed note about one sample short,
     * which is precisely the sharpening described above.
     *
     * This is a ONE-TIME fractional length, not per-sample modulation — the
     * read pointer still advances by exactly one sample per step — so it does
     * not reopen the numerical-stability question the vibrato note below
     * discusses. Guard: TestEveryPitchedInstrumentSoundsInTune. */
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

    /* slope (bow pressure), bowVel (bow speed), loss (brightness) are now config
     * params. offset + refl stay literals (numerical loop stability, not voice). */
    const double offset = 0.001;
    const double refl   = 0.99;   /* slight per-round-trip energy loss for stability */
    double lp = 0.0;
    noise_gen ng; noise_init(&ng, noise_seed);
    /* Bow ramp-in. This is a MODE-SELECTION control, not just a click guard.
     *
     * The stick-slip friction below is a NONLINEAR self-oscillator, and a
     * waveguide loop of length L is resonant at EVERY k*SR/L, not just the
     * fundamental. Which limit cycle the string settles into therefore depends
     * on how the bow is brought up to speed: ramp it too fast from rest and the
     * string locks onto its 2nd or 3rd mode and stays there, exactly as a real
     * player produces an unintended harmonic by attacking too hard. At the
     * previous ~25 ms the shipped strings did this over wide contiguous bands —
     * cello sounded a TWELFTH high across node pitch +1..+6, viola/violin/
     * double-bass across -11..-7 — with the notated fundamental absent from the
     * spectrum entirely (measured: energy only at 3f and 6f). Templates built
     * from a score then leapt registers mid-phrase, which is what made those
     * circuits sound dissonant rather than merely mistuned.
     *
     * The ramp below establishes the Helmholtz fundamental across the whole
     * playable range (verified -20..+28 semitones on all four bowed voices). It
     * costs nothing audible, because the pre-warm below is sized to OUTLAST it:
     * out[0] still begins at the established steady tone and the amp ADSR still
     * shapes the onset the listener actually hears. The only cost is render
     * time, and the deepest note (the longest ramp) measures ~18 ms/render
     * against ~14 ms for an untouched flute.
     *
     * The ramp is measured in PERIODS, not milliseconds. What must happen
     * during it is that the Helmholtz corner completes enough round trips of
     * the string for that motion to establish itself, and one round trip is L
     * samples — so a fixed 250 ms buys ~110 periods at 440 Hz but only ~14 at
     * 55 Hz, and the low notes were exactly the ones still locking after a
     * flat 250 ms (cello -13..-7, double-bass -20..-19: a notated 52-73 Hz).
     * Scaling with L fixes the bottom of the range without making the top
     * wasteful. Guard: TestSelfOscillatingVoicesHoldTheirFundamental. */
    const double bowRampPeriods = 110.0; /* round trips to establish Helmholtz motion */
    int attack = (int)(bowRampPeriods * L);
    /* Both floors are load-bearing and neither subsumes the other: 110 periods
     * is only ~125 ms at 880 Hz (where the high notes relocked), while 250 ms is
     * only ~14 periods at 55 Hz (where the low ones did). Take whichever is
     * longer. */
    if (attack < sampleRate / 4) attack = sampleRate / 4;   /* >=250 ms in absolute time */
    if (attack > 2 * sampleRate) attack = 2 * sampleRate;   /* ceiling on render cost */
    if (attack < 1) attack = 1;

    /* PRE-WARM: a self-oscillating bowed string takes ~0.5-1 s to build its
     * Helmholtz limit cycle from silence — without this the RENDERED note swells
     * over ~1 s then the raw buildup reads as a slow attack + fake decay (the amp
     * ADSR can't shape a tone that isn't there yet). Run the loop SILENTLY for
     * `warm` samples so out[0] starts at the established full-amplitude tone (same
     * trick as render_sax). The amp ADSR then shapes the audible onset. */
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
    sp_dcblock dc = {0.0, 0.0};
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
        sp_delay_write(bore, LEN, &wi, in); /* breath DC stays in the loop = the drive */
        lastOut = tout;
        /* DC-block the OUTPUT only (not the loop — the breath pressure is a DC
         * term that DRIVES the bore; blocking it in-loop kills self-oscillation). */
        double dy = sp_dcblock_tick(&dc, tout, 0.995);
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
    sp_dcblock dc = {0.0, 0.0};
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
        sp_delay_write(d1, LEN, &w1, temp);
        sp_delay_write(d0, LEN, &w0, breath - pd * r - temp);
        double dy = sp_dcblock_tick(&dc, o, 0.995); /* DC block on OUTPUT */
        double y = dy * 0.4;
        if (!(y > -8.0 && y < 8.0)) y = 0.0;
        if (i >= warm) out[i - warm] = (float)y; /* output only AFTER the limit cycle is established */
    }
}

/* ── Source-stage primitives (Phase-15 decomposition). Each renders the OSC
 * stage's buffer for one branch of the osc_type dispatch. Bodies are verbatim
 * transplants from the render_modular_p ladder — byte-identity is gated by
 * TestSynthGoldenByteIdentity. ── */
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
}

/* ── Vibrato for the physical-model waveguides (osc_type 7-11) ─────────────
 *
 * THE BUG THIS FIXES
 * ------------------
 * The oscillator dispatch below is a strict if/else-if chain, and the
 * waveguide branch sits AHEAD of both the vibrato branch
 * (modular_source_wt_modulated) and the unison branch. Every waveguide voice
 * therefore returned before either could run, so `lfo_target:1` vibrato was
 * silently DEAD on every physical-model instrument. Four shipped instruments
 * configure it and got nothing: cello (4.0 Hz / 0.025 st), violin (3.6 /
 * 0.025), viola (4.4 / 0.030) and double-bass (3.8 / 0.025) -- the entire
 * bowed-string family, whose seeds document the effect in prose
 * ("Vibrato sweeps harmonics across the sharp modes") that never happened.
 * A dead-straight bowed tone is a known synthesis tell, and the listener's
 * verdicts on three of those four were "breathy and soft" / "needs
 * refinement".
 *
 * WHY A DELAY RATHER THAN RETUNING THE WAVEGUIDE
 * ---------------------------------------------
 * render_bowed_string and friends derive their delay-line length ONCE from
 * freq and run a self-oscillating limit cycle over it. Modulating that length
 * per sample means fractional-delay interpolation inside a loop whose
 * numerical stability is load-bearing (see render_brass's chaos note), for no
 * perceptual gain: a modulated read pointer over the OUTPUT is the textbook
 * equivalent for the depths in play here, and leaves the model untouched.
 *
 * The excursion is small. A sinusoidal vibrato of D semitones at f Hz needs a
 * peak delay swing of D*ln2/(12*2*pi*f) seconds -- for the cello's 0.025 st at
 * 4 Hz that is 57 us, about 2.5 samples at 44.1 kHz.
 *
 * IDENTITY: returns immediately unless depth and rate are both positive, so
 * every instrument that does not configure vibrato renders byte-identically.
 */
enum { MODULAR_VIB_DMAX = 4096 };

/* ln2 / (12 * 2*pi): converts a semitone depth into a peak delay in seconds
 * once divided by the vibrato rate. */
#define MODULAR_VIB_SEMI_TO_SEC 0.0091938185

/* Vibrato humanisation.
 *
 * A vibrato at an exactly constant rate and depth reads as a machine. The
 * analysis battery measures this directly as VibratoFingerprint.Jitter
 * (cycle-to-cycle period irregularity): the corpus double-bass REFERENCE
 * measures 0.411, and the first cut of modular_apply_vibrato_delay measured
 * 0.000 -- perfectly metronomic. The listener heard it on the first pass and
 * moved double-bass from "sounds like the real thing" to "the synth one has
 * some vibrato or something that goes up and down in time".
 *
 * The rate is therefore modulated by two SLOW, mutually incommensurate sines
 * so the period never repeats exactly, and the depth by a third. Deterministic
 * by construction -- no RNG -- so renders stay reproducible and the goldens
 * stay meaningful. The frequencies are irrational-ish multiples of each other
 * (0.37, 0.61, 0.29 Hz) precisely so their sum has no short period.
 */
#define MODULAR_VIB_RATE_JITTER_A   0.13   /* +/-13% rate wander */
#define MODULAR_VIB_RATE_JITTER_B   0.08
/* Wander frequencies must be fast enough that a NOTE contains several cycles.
 *
 * The first values (0.37 / 0.61 Hz) had periods of 2.7 s and 1.6 s against
 * one-shot renders of 1-2 s, so a note only ever saw the first, rising half of
 * the modulation. The mean rate over the note came out ~12% high -- exactly
 * MODULAR_VIB_RATE_JITTER_A -- and every bowed string rendered about 0.5 Hz
 * fast: cello measured 4.50 against a configured 4.0, violin 4.10 against 3.6.
 * A seed that says lfo_rate 4.0 must produce 4.0 Hz.
 *
 * At 1.7 and 2.9 Hz a 1 s note contains 2-3 cycles of each, so the modulation
 * averages out and the mean rate converges on the nominal, while still being
 * far slower than the 4 Hz vibrato it modulates -- it reads as an unsteady hand
 * rather than as a second vibrato. Still mutually incommensurate, so the
 * combined period is long and no cycle repeats exactly. */
#define MODULAR_VIB_RATE_HZ_A       1.70
#define MODULAR_VIB_RATE_HZ_B       2.90
#define MODULAR_VIB_DEPTH_JITTER    0.18   /* +/-18% depth breathing */
#define MODULAR_VIB_DEPTH_HZ        1.30

/* modular_vibrato_phase integrates the jittered rate to a phase.
 *
 * Integrating the RATE (rather than perturbing the phase directly) is what
 * makes this a genuine tempo wander: perturbing phase would leave the average
 * period untouched and merely add a wobble on top, which measures as a second
 * vibrato rather than as irregularity of the first. The integral of
 * rate*(1 + a*sin(wa*t) + b*sin(wb*t)) is closed-form, so this stays exact and
 * allocation-free. */
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
    /* Keep centre +/- peak strictly inside the ring buffer. A very deep, very
     * slow vibrato would otherwise ask for a delay longer than the buffer;
     * clamping degrades the depth rather than reading out of bounds. */
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
        /* HUMANISED phase: see modular_vibrato_phase. A metronomic vibrato is
         * the single most recognisable "this is an LFO, not a player" tell, and
         * shipping one is a regression the listener caught immediately. */
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
        /* Brass lip-reed waveguide. NOTE: not yet a clean self-oscillator — the
         * lip BiQuad + squared nonlinearity tends to chaos rather than a locked
         * tone, so no instrument ships on it yet (trumpet stays subtractive). The
         * output is GUARDED finite/bounded (see test TestWaveguideOscTypesAreFinite
         * + the y-guard below) so it's safe; the low lip_gain is the documented
         * starting point for future stabilisation. See reference_instrument_synth_tuning. */
        render_brass(out, sampleRate, samples, freq, 0.85, 0.997, 0.012, 0.5);
    } else if (osc_type == 9) {
        /* Reed-woodwind waveguide (single-delay, inverting bell → CLARINET timbre:
         * odd harmonics only). NOTE: this is NOT a correct oboe (a conical double
         * reed needs even+odd). The oboe is synthesized ADDITIVELY instead (stable
         * pitch + exact harmonic profile) — routing it through the conical sax
         * model gave even+odd but the limit cycle wanders 20–140 cents at the
         * oboe's short bore. Kept for clarinet-family use. */
        render_reed(out, sampleRate, samples, freq, -0.95, 0.7, -0.3, 0.8, 0.5, noise_seed);
    } else if (osc_type == 10) {
        /* Air-jet (flute) waveguide. Hardcoded breathy-flute defaults; higher
         * drive + lower loss so it sustains oscillation at the prescaled bore. */
        render_flute(out, sampleRate, samples, freq, 0.4, 0.2, 0.7, 1.0, noise_seed);
    } else if (osc_type == 11) {
        /* Saxofony reed-cone waveguide. Hardcoded BARITONE-SAX defaults: off-centre
         * blow position (all harmonics), positive reed slope, hard breath + low
         * offset for the dense honking low tone, bright loss filter. */
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
    /* Wavetable OSC with per-sample frequency modulation: pitch-env
     * (exp semitone sweep) and/or LFO vibrato (sinusoidal semitone wobble),
     * summed in semitones then applied as a 2^(st/12) multiplier. When
     * vib_on is false this is bit-identical to the legacy pitch-env branch;
     * when both are off control never reaches here (the plain branch below
     * runs), preserving byte-identity for every pre-change instrument. */
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
    /* Phase-16 ensemble cycle jitter: per-voice fast pitch roughness. Identity
     * 0 = no jitter (exact bypass of everything jitter-related below). */
    double ens_jitter  = p->ens_jitter;
    int   ens_vib_on  = (ens_vr > 0.0f && ens_vd > 0.0f);
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
    /* Per-voice drift LFO parameters (rate/phase only used when drift_on).
     * drift_base_cents mirrors the static detune spread computed above for
     * side_freq[v]; it is read whenever mod_on runs the per-sample set_freq
     * path (drift_on, ens_scatter, or ens_vib_on), so it must be populated
     * unconditionally — NOT gated behind drift_on like the rate/phase are. */
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
    /* Deterministic per-voice randoms: LCG seeded by (noise_seed, voice).
     * lcg_norm ∈ [-1,1], lcg_unit ∈ [0,1). Never rand()/time — byte-identity
     * and xplat parity depend on renders being pure functions of the seed. */
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
            /* humanize spreads rate ±15% and randomizes phase; at 0 all voices
             * share rate ens_vr and phase 0 (one synchronized singer). */
            vib_rate_v[v]  = (double)ens_vr * (1.0 + 0.15 * (double)ens_hum * n2);
            vib_phase_v[v] = 2.0 * M_PI * (double)ens_hum * u3;
        }
    }
    /* Phase-16 cycle jitter: per-voice fast pitch roughness, INCLUDING the
     * center voice. jit_onepole[0] = center, jit_onepole[v+1] = side v — same
     * indexing convention as scat_cents/vib_rate_v/vib_phase_v above. A
     * deterministic per-voice LCG stream (seeded off noise_seed) advances once
     * every 16 samples and is smoothed by a one-pole (coef 0.008 ≈ 60 Hz
     * bandwidth at 48k) into jit_onepole; ens_jitter==0 never touches these
     * arrays, so they stay exactly 0.0 for the whole render. */
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

/* Thread-local scratch for the FINISH resample. Grows lazily, never freed: the
 * render thread reuses it on every cache-miss render, bounded by the longest
 * single render. _Thread_local gives each cgo-crossing goroutine its own copy;
 * under single-threaded Emscripten it is a plain static. */
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

/* ── FINISH resampler ──
 * Resample plays the finished voice at 2^(st/12) speed. Reading it back with
 * linear interpolation folded every partial pushed past Nyquist back down as an
 * inharmonic tone at full level (a 15 kHz partial shifted +12 st at 44.1 kHz
 * landed on 14.1 kHz) and dulled the top of the passband by up to 4 dB. It is a
 * band-limited interpolator now: a Kaiser-windowed sinc, its cutoff scaled
 * down by the speed when shifting UP (so content that would land past Nyquist
 * is removed, not folded). The kernel spans FINISH_RS_ZC zero crossings each
 * side (β 5.65: ≈60 dB stopband, passband flat to ≈0.8 of Nyquist), tabulated
 * at FINISH_RS_OS points per crossing and read at the nearest point (that
 * rounding error sits at the stopband floor, and interpolating the table cost
 * as much again as the whole read). Guards:
 * TestFinishResampleDoesNotAlias, TestFinishResampleKeepsThePassbandFlat,
 * TestFinishResampleShiftsPitchExactly. */
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
    /* Cutoff as a fraction of the input's Nyquist: shifting up by `rate` must
     * remove everything above Nyquist/rate before it is read faster. */
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

/* finish_shelf_set: an RBJ (Audio EQ Cookbook) shelf, slope S = 1, gainDB at
 * the shelved end and 0 dB at the other — the same curve at every engine
 * rate (the corner clamp below Nyquist only bites far below 22.05 kHz). */
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

/* The shelves' corners and their gain at a knob's full travel. Low Boost lifts
 * the body (kick boom, cello weight) below FINISH_BODY_HZ; Crisp lifts the
 * presence/air above FINISH_CRISP_HZ. Each lifts ONLY its end: the rest keeps
 * its level, so the knob changes the balance, not the loudness. Gain is linear
 * in dB with the knob (a first step does a first step's worth). */
#define FINISH_BODY_HZ 200.0
#define FINISH_CRISP_HZ 3000.0
#define FINISH_SHELF_MAX_DB 12.0

/* modular_finish — the FINISH stage, a pure function of the post_* fields,
 * always run, in this order: resample (post_pitch), fade (post_decay at
 * post_decay_rate), low boost (post_body, a low shelf), crisp (post_brightness,
 * a high shelf), darken (post_darken), crunch (post_drive), then the output
 * limit (out_clamp). Each op is a no-op at its identity, so a voice that leaves
 * them alone renders exactly as without the stage. Darken and Crunch crossfade
 * their wet signal by their own amount, so a knob's first step does a first
 * step's worth of work. Response contract: finish_stage_response_test.go. */
static void modular_finish(float *out, int sampleRate, int samples, const modular_params *p) {
    /* A non-finite sample from upstream is dropped HERE, before any recursive
     * filter sees it: a biquad / one-pole that takes in one NaN holds it in its
     * state forever, and the output guard below then silenced the whole rest
     * of the voice. */
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
    /* Structural bounds, not defaults: at 0 the exponent is 0·Inf = NaN, which
     * poisons every BiquadFilter downstream; above 1 the exponent turns
     * positive and the "fade" grows without bound (Inf within a second). */
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

    /* A non-finite sample is never audio: it poisons the browser's peak
     * normaliser and every BiquadFilter on the channel. Structural guard. */
    for (int i = 0; i < samples; i++) {
        if (!isfinite(out[i])) out[i] = 0.0f;
    }
}

/* modular_param_count: the param count this engine was COMPILED with. Go and
 * JS compare it with their own (generated) layout at load, so a header
 * regenerated without rebuilding the engine fails loudly instead of reading
 * every later field at a shifted offset. */
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

    /* Every param is read directly: there is no "absent → built-in default"
     * path. A NULL block is a caller bug; render silence. */
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

    /* Phase-8C modulator stages. Each enable's identity is 0 (off); enabled=0
     * skips the stage code entirely (the numeric knobs are never read) — an
     * exact bypass. */
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

    /* Phase-15 formant stage. enabled/mix gate everything; vowel/voice_type/
     * morph_to are enum values (0 is a valid one); shift is clamped >= 0.01
     * below (a structural guard against a zero/negative frequency scale). */
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

    /* voice_freq_hz override (Phase 2): >0 sets the voice freq to an EXACT Hz,
     * applied AFTER the pow derivation. Identity 0 keeps the derived freq, so
     * every pre-Phase-2 preset is byte-unchanged. Used by migrated family
     * presets whose fundamental (e.g. 45 Hz) is not reachable via 220·2^x. */
    {
        double vfh = (double)p->voice_freq_hz;
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
        modular_source_noise(out, samples, osc_type, noise_seed);
        osc_wrote = 1;
    } else if (osc_type == 4) {
        modular_source_fm(out, sampleRate, samples, p, freq, fm_on, pe_on, pe_amt, pe_decay);
        osc_wrote = 1;
    } else if (osc_type == 7 || osc_type == 8 || osc_type == 9 || osc_type == 10 || osc_type == 11) {
        modular_source_physical(out, sampleRate, samples, p, osc_type, freq, noise_seed);
        /* The waveguide branch returns before the vibrato branch below could
         * ever run, so vibrato has to be applied here or not at all. Identity
         * unless depth and rate are both positive -- see
         * modular_apply_vibrato_delay. */
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

    /* ── Gen bank (Phase 1, modular unification): parallel slots accumulate
     * into the mix AFTER the legacy osc stage writes. All-identity params ⇒
     * no active slot ⇒ exact no-op (byte-identity invariant). Runs even when
     * osc_enabled == 0 (slots are independent of the legacy osc gate). ── */
    modular_gen_bank_render(out, sampleRate, samples, p, freq, noise_seed, osc_wrote);

    /* ── WAV-clone stages: MODAL (+ STRIKE) resonators and NOISEBAND rows add
     * into the mix after the gen bank, PRE-envelope/filter like every source.
     * Disabled (the default) => exact no-op. ── */
    modular_modal_render(out, sampleRate, samples, p, freq, noise_seed);
    modular_noiseband_render(out, sampleRate, samples, p, noise_seed);

    /* ── Phase-15 breath: white noise mixed into the source PRE-envelope so the
     * ADSR shapes it and the filter + formant bank color it. Whisper = osc off
     * + breath 1. Gated on the formant stage (it is that card's knob). ── */
    if (formant_on && formant_breath > 0.0f) {
        noise_gen bng;
        noise_init(&bng, noise_seed ^ 0x9E3779B9u);
        for (int i = 0; i < samples; i++)
            out[i] = (float)(out[i] + noise_white_tick(&bng) * formant_breath * 0.5f);
    }

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
        /* Cap release to at most half the note so attack/decay/sustain always
         * get the first half — otherwise a note shorter than the release time
         * would collapse straight into release and the decay knob would do
         * nothing. A capped release ramps over the half it is given: keeping
         * the full release time left the note loud at its end, where the voice
         * hard-cut (a release value past half the note sounded LOUDER, not
         * longer). An uncapped release is untouched. */
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

    /* ── Phase-15 FORMANT stage (vowel bank) — before the body resonator so
     * FORMANT (mouth) and RESONATOR (instrument body) compose in that order. */
    if (formant_on) {
        modular_formant_process(out, sampleRate, samples, formant_vt,
                                formant_vowel, formant_mix, formant_shift,
                                formant_sing, formant_mrate, formant_mto,
                                formant_dry);
    }

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

