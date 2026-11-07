#include <math.h>
#include <stdlib.h>
#include <string.h>
#include "modular_stages.h"
#include "wavetable.h"
#include "noise.h"
#include "synth_post.h"          /* osc_wave_shared (analytic source==4) */
#include "synth_dsp_primitives.h" /* sp_onepole / sp_osc / sp_pitch_glide / sp_reverb3 */
#include "fmsynth.h"             /* fm_preset + fm_render (source==10 FM-family voice) */

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* Static in modular.c; the slot oscillator MUST use the same wavetables and tick loop so a slot sine is
 * byte-identical to the osc stage. */
const wavetable_t *modular_shared_table_for(int wave);

/* The RBJ biquad, shared by the FILTER stage and the slot filters. */

void mod_biquad_set_coeffs(mod_biquad *f, int type, double cutoff, double q, int sr) {
    if (cutoff < 20.0) cutoff = 20.0;
    double nyq = 0.5 * (double)sr;
    if (cutoff > nyq * 0.99) cutoff = nyq * 0.99;
    if (q < 0.25) q = 0.25;

    double w0 = 2.0 * M_PI * cutoff / (double)sr;
    double cw = cos(w0), sw = sin(w0);
    double alpha = sw / (2.0 * q);
    double a0, b0, b1, b2, a1, a2;

    switch (type) {
    case 1: /* high-pass */
        b0 = (1.0 + cw) / 2.0;
        b1 = -(1.0 + cw);
        b2 = (1.0 + cw) / 2.0;
        a0 = 1.0 + alpha;
        a1 = -2.0 * cw;
        a2 = 1.0 - alpha;
        break;
    case 2: /* band-pass (constant 0 dB peak) */
        b0 = alpha;
        b1 = 0.0;
        b2 = -alpha;
        a0 = 1.0 + alpha;
        a1 = -2.0 * cw;
        a2 = 1.0 - alpha;
        break;
    default: /* low-pass */
        b0 = (1.0 - cw) / 2.0;
        b1 = 1.0 - cw;
        b2 = (1.0 - cw) / 2.0;
        a0 = 1.0 + alpha;
        a1 = -2.0 * cw;
        a2 = 1.0 - alpha;
        break;
    }
    f->b0 = (float)(b0 / a0);
    f->b1 = (float)(b1 / a0);
    f->b2 = (float)(b2 / a0);
    f->a1 = (float)(a1 / a0);
    f->a2 = (float)(a2 / a0);
}

void mod_biquad_set(mod_biquad *f, int type, double cutoff, double q, int sr) {
    mod_biquad_set_coeffs(f, type, cutoff, q, sr);
    f->x1 = f->x2 = f->y1 = f->y2 = 0.0f;
}

float mod_biquad_tick_f(mod_biquad *f, float x) {
    float y = f->b0 * x + f->b1 * f->x1 + f->b2 * f->x2
            - f->a1 * f->y1 - f->a2 * f->y2;
    f->x2 = f->x1; f->x1 = x;
    f->y2 = f->y1; f->y1 = y;
    return y;
}

/* ── Slot branch filter state ── */
typedef struct {
    int        type;
    double     alpha, keep; /* 1-pole: lp = lp*keep + x*alpha (keep = 1-alpha) */
    double     lp;
    mod_biquad bq;
} gen_slot_filter;

static void gen_slot_filter_init(gen_slot_filter *f, const modular_params *p,
                                 int k, int sr) {
    f->type = (int)lrint(p->gen_filt_type[k]);
    f->alpha = (double)p->gen_filt_alpha[k];
    f->keep = 1.0 - f->alpha;
    f->lp = 0.0;
    if (f->type >= 3) {
        /* RBJ types: 3=LP 4=HP 5=BP map onto mod_biquad types 0/1/2. */
        mod_biquad_set(&f->bq, f->type - 3, (double)p->gen_filt_freq[k],
                       (double)p->gen_filt_q[k], sr);
    }
}

static inline double gen_slot_filter_tick(gen_slot_filter *f, double x) {
    switch (f->type) {
    case 1: /* 1-pole LP — legacy literal-pair form (lp = lp*keep + x*alpha). */
        f->lp = f->lp * f->keep + x * f->alpha;
        return f->lp;
    case 2: /* 1-pole HP = x - LP(x) (the legacy click-extraction form). */
        f->lp = f->lp * f->keep + x * f->alpha;
        return x - f->lp;
    case 3: case 4: case 5:
        return (double)mod_biquad_tick_f(&f->bq, (float)x);
    default:
        return x;
    }
}

/* The source-4 analytic dual-osc voice: every value read per slot, with a fixed float-op order, normalization form and
 * cast positions. Base freq = gen_freq × voice_freq in ratio mode, else absolute. assign 1 ASSIGNS the first write so a
 * -0.0 sample is not flipped to +0.0. */
static void modular_gen_slot_analytic(float *out, int sampleRate, int samples,
                                      const modular_params *p, int k,
                                      double voice_freq, int assign, double gain) {
    /* Appended as a FLOAT, not folded in: `(float)x * scale` and `(float)(x * scale * gain)` round differently, while ×1.0f
     * is exact, so unity gain stays byte-identical. */
    const float gf = (float)gain;
    double bHarm    = p->gen_harm_mix[k];
    double bPitchE  = p->gen_pitch_env_amt[k];
    double bPitchER = p->gen_pitch_env_rate[k];
    double bAttack  = p->gen_atk_amt[k];
    double bAttackR = p->gen_atk_rate[k];
    double bEnvRate = p->gen_env_fast_rate[k];
    double satK     = p->gen_sat_k[k];
    int    bWave    = (int)p->gen_wave[k];

    double baseFreq = (double)p->gen_freq[k];
    if ((int)lrint(p->gen_freq_mode[k]) == 0) baseFreq *= voice_freq;

    double phase  = p->gen_phase[k];
    double phase2 = phase;
    double harmonicMix = bHarm;

    /* out scale read as a FLOAT (legacy `(float)sat * 0.95f` — float×float). */
    float outScale = (float)p->gen_out_scale[k];

    for (int i = 0; i < samples; i++) {
        double tSec = (double)i / (double)sampleRate;

        double pitchEnv = exp(-bPitchER * tSec);
        double freq = baseFreq * (1.0 + bPitchE * pitchEnv);

        double phaseInc = 2.0 * M_PI * freq / (double)sampleRate;
        phase += phaseInc;
        if (phase > 2.0 * M_PI) phase -= 2.0 * M_PI;

        double fund = osc_wave_shared(bWave, phase);

        phase2 += 2.0 * phaseInc; /* Double frequency */
        if (phase2 > 2.0 * M_PI) phase2 -= 2.0 * M_PI;
        double harm = osc_wave_shared(bWave, phase2);

        double ampEnv = exp(-bEnvRate * tSec);
        double attackBoost = 1.0 + bAttack * exp(-bAttackR * tSec);

        double mixed = (fund + harm * harmonicMix) * ampEnv * attackBoost;
        double sat = tanh(mixed * satK);

        if (assign) out[i] = (float)sat * outScale * gf;
        else        out[i] += (float)sat * outScale * gf;
    }
}

/* The source-3 Karplus-Strong voice: a PRIVATE delay line and noise stream (noise_ma, seed 0 → 4321), not the shared
 * noise bus, since its draw order depends on control flow. Base freq as the analytic voice; it writes via softsat, so
 * assign only chooses = over +=. */
static void modular_gen_slot_ks(float *out, int sampleRate, int samples,
                                const modular_params *p, int k,
                                double voice_freq, int assign, double gain) {
    /* Appended as a FLOAT, not folded in: `(float)x * scale` and `(float)(x * scale * gain)` round differently, while ×1.0f
     * is exact, so unity gain stays byte-identical. */
    const float gf = (float)gain;
    /* Family knobs, read as-is. */
    double bSustain = p->gen_ks_sustain[k];
    double bPluck   = p->gen_ks_pluck[k];
    double bBlow    = p->gen_ks_blow[k]; /* 0 = plucked (legacy); >0 = blown tube / wind */
    double bAttack  = p->gen_atk_amt[k];
    double bAttackR = p->gen_atk_rate[k];
    double bEnvRate = p->gen_env_fast_rate[k];
    /* The analytic voice reads the same shared field. */
    float  outScale = (float)p->gen_out_scale[k];

    /* Karplus-Strong delay buffer size for the base frequency. */
    double baseFreq = (double)p->gen_freq[k];
    if ((int)lrint(p->gen_freq_mode[k]) == 0) baseFreq *= voice_freq;
    int delayLen = (int)((double)sampleRate / baseFreq + 0.5);
    if (delayLen < 2) delayLen = 2;
    if (delayLen > 4096) delayLen = 4096;

    /* Local buffer (legacy uses a 4096 float stack buffer, zeroed in full). */
    float delayBuf[4096];
    for (int i = 0; i < 4096; i++) delayBuf[i] = 0.0f;

    /* The first draw (the detune seed) is unused for audio but consumed, so the stream offset stays fixed. */
    noise_ma_gen noise;
    noise_ma_init(&noise, 0);
    float seed = noise_ma_white_tick(&noise);
    double detune = 1.0 + 0.005 * (double)seed;
    (void)detune; /* Applied through playback rate in Go/JS */

    /* Fill delay buffer with filtered noise (simulates pluck position). */
    double lpState = 0.0;
    double lpAlpha = bPluck;
    for (int i = 0; i < delayLen; i++) {
        float n = noise_ma_white_tick(&noise);
        lpState = lpState * (1.0 - lpAlpha) + (double)n * lpAlpha;
        delayBuf[i] = (float)lpState * 0.9f;
    }

    double decayFactor = bSustain;
    double stretchProb = 0.5;

    int readPtr = 0;
    double prev = delayBuf[0];

    int attackSamples = (int)(0.008 * (double)sampleRate); /* ~8ms */
    if (attackSamples > samples) attackSamples = samples;

    /* Blown mode (gen_ks_blow > 0): breath drives a tuned bore through a cubic jet nonlinearity whose saturation
     * SELF-LIMITS the oscillation; a one-pole in the reflection is the radiation loss. Gated on blow > 0, so plucked KS
     * is untouched. blow = pressure, sustain = bore reflection, pluck = air colour. */
    if (bBlow > 0.0) {
        /* A linear comb at high feedback builds without bound; tanh on the recirculated signal makes a self-limiting limit
         * cycle. A tube oscillates only when small-signal loop gain exceeds 1, so sustain (≤ 1) is scaled by 1.05 here. */
        double fb      = decayFactor * 1.05; /* gen_ks_sustain → blown loop gain (>1 = self-oscillating) */
        double lpAlpha = bPluck;             /* reflection lowpass alpha (tube air color) */
        double lpState = 0.0;         /* reflection lowpass state */
        /* Smooth air, not white hiss (which sounds industrial). On the excitation, not the round trip, so pitch is unaffected. */
        const double breathAlpha = 0.10;
        double bA = 0.0, bB = 0.0;    /* 2-pole breath lowpass state */
        /* A tube has nothing musical below f0, so a pitch-relative HP at 0.65·f0 removes the loop's low rumble at any note. */
        double hpFc = 0.65 * baseFreq;
        double hpR = exp(-2.0 * M_PI * hpFc / (double)sampleRate);
        double h1x = 0.0, h1y = 0.0, h2x = 0.0, h2y = 0.0;
        for (int i = 0; i < samples; i++) {
            double bore = (double)delayBuf[readPtr];
            /* reflection filter: one-pole lowpass (open-tube radiation damping) */
            lpState = lpState * (1.0 - lpAlpha) + bore * lpAlpha;
            /* breath excitation: white noise → 2-pole lowpass (smooth air) */
            float nz = noise_ma_white_tick(&noise);
            bA = bA * (1.0 - breathAlpha) + (double)nz * breathAlpha;
            bB = bB * (1.0 - breathAlpha) + bA * breathAlpha;
            /* recirculate feedback + breath, SATURATED → bounded limit cycle */
            double recirc = tanh(lpState * fb + bB * bBlow);
            delayBuf[readPtr] = (float)recirc;
            readPtr = (readPtr + 1) % delayLen;
            /* high-pass the output below the fundamental (kills the rumble floor) */
            double s = bore;
            h1y = hpR * (h1y + s - h1x);
            h1x = s;
            h2y = hpR * (h2y + h1y - h2x);
            h2x = h1y;
            float v = softsat_shared((float)(h2y * outScale));
            if (assign) out[i] = v * gf;
            else        out[i] += v * gf;
        }
        return;
    }

    for (int i = 0; i < samples; i++) {
        double tSec = (double)i / (double)sampleRate;

        double current = (double)delayBuf[readPtr];

        double filtered;
        float rnd = noise_ma_white_tick(&noise);
        if ((double)rnd > stretchProb) {
            filtered = 0.5 * (current + prev) * decayFactor;
        } else {
            filtered = current * decayFactor;
        }
        prev = current;

        delayBuf[readPtr] = (float)filtered;
        readPtr = (readPtr + 1) % delayLen;

        double attack = 0.0;
        if (i < attackSamples) {
            float atkNoise = noise_ma_white_tick(&noise);
            double atkEnv = exp(-tSec * bAttackR);
            attack = (double)atkNoise * atkEnv * bAttack;
        }

        double envGlobal = exp(-tSec * bEnvRate);

        double mixed = (current + attack) * envGlobal;

        float v = softsat_shared((float)mixed * outScale);
        if (assign) out[i] = v * gf;
        else        out[i] += v * gf;
    }
}

/* The source-5 harmonic-bank kick, selected by gen_kick_variant: each branch keeps its float-op order, noise topology
 * (noise_ma seed 0 → 4321) and shaping. The fundamental flows through voice_freq (ratio 1). assign 1 ASSIGNS the first
 * write so a -0.0 sample is not flipped to +0.0. */
static void gen_kick_acoustic(float *out, int sampleRate, int samples,
                              const modular_params *p, int k,
                              double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* A recorded kick is INHARMONIC head/shell modes struck by a beater, which a sine plus a band-passed click cannot
     * imitate: each mode struck at t=0 with its own decay, a bright beater click, broadband air noise, gentle saturation.
     * No new ABI: mode_detune = stretch, mode_gain = high-mode level, mode_decay = high-mode damping. */
    double kNoiseAmt = p->gen_kick_noise[k];
    double kClickAmt = p->gen_kick_click[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k]; /* fundamental decay /s */
    double kEnv1Rate = p->gen_kick_env1[k]; /* low-mid mode decay /s */
    double kH2       = p->gen_kick_h2[k];   /* mode-2 gain */
    double kH3       = p->gen_kick_h3[k];   /* mode-3 gain */
    double kH4       = p->gen_kick_h4[k];   /* mode-4 gain */
    double kSat      = p->gen_kick_sat[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kStretch  = p->gen_kick_mode_detune[k]; /* inharmonic stretch */
    double kModeGain = p->gen_kick_mode_gain[k];    /* high-mode level */
    double kHiDecay  = p->gen_kick_mode_decay[k];  /* high-mode damping /s */
    double kReverb   = p->gen_kick_reverb[k];       /* room/echo tail amount */

    /* Composed from synth_dsp_primitives.h with the exact float-op order of the inline loop it replaced. */
    sp_reverb3 rev;
    sp_reverb3_init(&rev, sampleRate); /* zeroed tank; dry kicks just don't read it */

    /* A membrane's modes are non-integer and cluster higher. */
    double r1 = 1.0;
    double r2 = 2.1 * (1.0 + kStretch);
    double r3 = 3.4 * (1.0 + kStretch);
    double r4 = 5.2 * (1.0 + kStretch);
    sp_osc osc1, osc2, osc3, osc4;
    sp_osc_init(&osc1, M_PI * 0.5);
    sp_osc_init(&osc2, M_PI * 0.5);
    sp_osc_init(&osc3, M_PI * 0.5);
    sp_osc_init(&osc4, M_PI * 0.5);
    double lpHi = 0.0, lpLo = 0.0, nzLo = 0.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double freqMul = sp_pitch_glide(tSec, kPeRate, kPeAmt);
        double m1 = sp_osc_tick(&osc1, kWave, 2.0 * M_PI * f0 * r1 * freqMul / (double)sampleRate);
        double m2 = sp_osc_tick(&osc2, kWave, 2.0 * M_PI * f0 * r2 * freqMul / (double)sampleRate);
        double m3 = sp_osc_tick(&osc3, kWave, 2.0 * M_PI * f0 * r3 * freqMul / (double)sampleRate);
        double m4 = sp_osc_tick(&osc4, kWave, 2.0 * M_PI * f0 * r4 * freqMul / (double)sampleRate);

        /* struck modes (start at peak → sharp attack), per-mode decay. */
        double e1 = exp(-kEnv0Rate * tSec);
        double e2 = exp(-kEnv1Rate * tSec);
        double e3 = exp(-kHiDecay * 0.55 * tSec);
        double e4 = exp(-kHiDecay * tSec);
        double body = m1 * e1
                    + kH2 * kModeGain * m2 * e2
                    + kH3 * kModeGain * m3 * e3
                    + kH4 * kModeGain * m4 * e4;
        /* The body ONLY: click and texture go on after, so the transient survives (a real kick's crest is ~8; compressing
         * everything flattens it toward 4). */
        double driven = tanh(body * (1.0 + kSat)) * 0.6;

        /* Two one-pole LPs subtracted: a felt-beater band, not bright white. */
        double cn = (double)noise_ma_white_tick(noise);
        double bpClick = sp_onepole_tick(&lpHi, 0.30, cn) - sp_onepole_tick(&lpLo, 0.05, cn);
        double click = bpClick * kClickAmt * (1.0 + kAttack) * exp(-220.0 * tSec);

        /* LP-limited so the body stays tonal; full white reads hissy. */
        double nz = (double)noise_ma_white_tick(noise);
        double texture = sp_onepole_tick(&nzLo, 0.28, nz) * kNoiseAmt * exp(-45.0 * tSec);

        double g = exp(-kFade * tNorm);
        double s = (driven + click + texture) * g;

        /* reverb/echo TAIL — the recorded-kick "space". Skipped when dry. */
        if (kReverb > 0.0) {
            s += sp_reverb3_tick(&rev, s, kReverb);
        }

        /* A soft limit only, so the click peak stays sharp. */
        float y = (float)(tanh(s) * 0.95);
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_tight(float *out, int sampleRate, int samples,
                           const modular_params *p, int k,
                           double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── tight (render_kick_tight_internal) ── */
    double kClickAmt = p->gen_kick_click[k];
    double kNoiseAmt = p->gen_kick_noise[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k];
    double kEnv1Rate = p->gen_kick_env1[k];
    double kH2Gain   = p->gen_kick_h2[k];
    double kH3Gain   = p->gen_kick_h3[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kSat      = p->gen_kick_sat[k];

    float r0 = noise_ma_white_tick(noise);
    float r1 = noise_ma_white_tick(noise);
    float r2 = noise_ma_white_tick(noise);
    double phase0 = 2.0 * M_PI * (double)r0;
    double phase1 = 2.0 * M_PI * (double)r1;
    double phase2 = 2.0 * M_PI * (double)r2;
    double f1 = f0 * 2.0;
    double f2 = f0 * 3.0;

    double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
    {
        double fc = 2500.0; double Q = 0.7;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
        double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
        b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0; a1_bp = a1/a0; a2_bp = a2/a0;
    }
    sp_biquad bq_bp = {0};
    bq_bp.b0 = b0_bp; bq_bp.b1 = b1_bp; bq_bp.b2 = b2_bp; bq_bp.a1 = a1_bp; bq_bp.a2 = a2_bp;

    double b0_rm, b1_rm, b2_rm, a1_rm, a2_rm;
    {
        double fc = 200.0; double Q = 0.5;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
        double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
        b0_rm = b0/a0; b1_rm = b1/a0; b2_rm = b2/a0; a1_rm = a1/a0; a2_rm = a2/a0;
    }
    sp_biquad bq_rm = {0};
    bq_rm.b0 = b0_rm; bq_rm.b1 = b1_rm; bq_rm.b2 = b2_rm; bq_rm.a1 = a1_rm; bq_rm.a2 = a2_rm;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        float n = noise_ma_white_tick(noise);
        double x = (double)n;

        double ybp = sp_biquad_tick(&bq_bp, x);
        double beaterEnv = exp(-150.0 * tSec);
        double beater = ybp * beaterEnv * kClickAmt;

        double yrm = sp_biquad_tick(&bq_rm, x);
        double roomEnv = exp(-80.0 * tSec);
        double room = yrm * roomEnv * kNoiseAmt;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
        double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
        double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;
        phase0 += step0; phase1 += step1; phase2 += step2;

        double s0 = osc_wave_shared(kWave, phase0);
        double s1 = osc_wave_shared(kWave, phase1);
        double s2 = osc_wave_shared(kWave, phase2);

        double env0 = exp(-kEnv0Rate * tSec);
        double env1 = exp(-kEnv1Rate * tSec);
        double env2 = exp(-17.0 * tSec);

        double tonal = s0 * env0 * 0.85 + s1 * env1 * kH2Gain + s2 * env2 * kH3Gain;
        double attackShape = 1.0 + kAttack * exp(-50.0 * tSec);
        tonal *= attackShape;

        double gate = 1.0;
        if (tNorm > 0.45) {
            gate = exp(-12.0 * (tNorm - 0.45));
        }
        double g = exp(-kFade * tNorm);

        double mixed = (tonal + beater + room) * g * gate;
        float y = softsat_shared((float)mixed * (float)kSat) * 1.1f;
        if (y > 1.0f) y = 1.0f;
        if (y < -1.0f) y = -1.0f;
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_modal(float *out, int sampleRate, int samples,
                           const modular_params *p, int k,
                           double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* Two pitch-swept modes: a damped batter and a detuned, less-damped resonant head with a slow swell; their beating
     * at f0·detune makes the plateau-then-bloom from a REAL mechanism instead of a drawn envelope. Grit is additive, never
     * through the waveshaper. */
    double kNoiseAmt = p->gen_kick_noise[k];
    double kClickAmt = p->gen_kick_click[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k]; /* batter decay /s */
    double kEnv1Rate = p->gen_kick_env1[k]; /* harmonic decay /s */
    double kH2Gain   = p->gen_kick_h2[k];
    double kH3Gain   = p->gen_kick_h3[k];
    double kDrive    = p->gen_kick_sat[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kDetune   = p->gen_kick_mode_detune[k];
    double kModeGain = p->gen_kick_mode_gain[k];
    double kModeDec  = p->gen_kick_mode_decay[k];

    double fR = f0 * (1.0 - kDetune);
    sp_osc oscB, oscR, osc2, osc3;
    sp_osc_init(&oscB, M_PI * 0.5);
    sp_osc_init(&oscR, M_PI * 0.5);
    sp_osc_init(&osc2, M_PI * 0.5);
    sp_osc_init(&osc3, M_PI * 0.5);
    double f2 = f0 * 2.0, f3 = f0 * 3.0;
    double lpClick = 0.0, nzLo = 0.0, nzBand = 0.0, nzBand2 = 0.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        /* batter: fast swell (~11 ms) × exp decay → peaks ~25 ms (the thump). */
        double batterEnv = exp(-kEnv0Rate * tSec) * (1.0 - exp(-90.0 * tSec));
        /* The delayed bloom, beating against the batter at f0·detune. */
        double resoEnv = exp(-kModeDec * tSec) * (1.0 - exp(-25.0 * tSec)) * kModeGain;
        /* The harmonics share the batter's swell, or the onset is all 2f0/3f0 while the fundamental still ramps. */
        double harmEnv = exp(-kEnv1Rate * tSec) * (1.0 - exp(-90.0 * tSec));

        double body = sp_osc_tick(&oscB, kWave, 2.0 * M_PI * f0 * freqMul / (double)sampleRate) * batterEnv
                    + sp_osc_tick(&oscR, kWave, 2.0 * M_PI * fR * freqMul / (double)sampleRate) * resoEnv
                    + kH2Gain * harmEnv * sp_osc_tick(&osc2, kWave, 2.0 * M_PI * f2 * freqMul / (double)sampleRate)
                    + kH3Gain * harmEnv * sp_osc_tick(&osc3, kWave, 2.0 * M_PI * f3 * freqMul / (double)sampleRate);

        /* gentle warmth drive on the summed body (deterministic → no flutter). */
        double driven = tanh(body * (1.0 + kDrive));

        /* low beater thud (1-pole LP ~140 Hz, short) — punch, not a high tick. */
        double cn = (double)noise_ma_white_tick(noise);
        sp_onepole_tick(&lpClick, 0.018, cn);
        double click = lpClick * kClickAmt * exp(-260.0 * tSec) * 10.0;

        /* Attack-only, added on top, never through the waveshaper. */
        double nz = (double)noise_ma_white_tick(noise);
        sp_onepole_tick(&nzLo, 0.016, nz);
        double nzHP = nz - nzLo;
        sp_onepole_tick(&nzBand, 0.065, nzHP);
        sp_onepole_tick(&nzBand2, 0.065, nzBand);
        double texture = nzBand2 * kNoiseAmt * exp(-70.0 * tSec) * 3.0;

        double g = exp(-kFade * tNorm);
        double s = (driven * 0.7 + texture + click * (0.6 + 0.5 * kAttack)) * g;
        float y = (float)(tanh(s) * 0.95);
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_hybrid(float *out, int sampleRate, int samples,
                            const modular_params *p, int k,
                            double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* Three DECOUPLED layers keep it smooth: a waveshaped body, a band-passed texture on the body's ENVELOPE added after
     * the distortion (noise through tanh × the body wave sputters), and a short click. */
    double kNoiseAmt = p->gen_kick_noise[k];
    double kClickAmt = p->gen_kick_click[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k];
    double kEnv1Rate = p->gen_kick_env1[k];
    double kH2Gain   = p->gen_kick_h2[k];
    double kH3Gain   = p->gen_kick_h3[k];
    double kDrive    = p->gen_kick_sat[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];

    sp_osc osc0, osc1, osc2;
    sp_osc_init(&osc0, M_PI * 0.5);
    sp_osc_init(&osc1, M_PI * 0.5);
    sp_osc_init(&osc2, M_PI * 0.5);
    double f1 = f0 * 2.0, f2 = f0 * 3.0;
    double lpClick = 0.0, nzLo = 0.0, nzBand = 0.0, nzBand2 = 0.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        /* A ~15 ms ramp makes the body SWELL to its peak: a pure exp decay peaks at t=0 and feels weak. */
        double decayEnv = exp(-kEnv0Rate * tSec);
        double atkRamp  = 1.0 - exp(-90.0 * tSec);
        double bodyEnv  = decayEnv * atkRamp;
        double harmEnv = exp(-kEnv1Rate * tSec);
        double body = sp_osc_tick(&osc0, kWave, 2.0 * M_PI * f0 * freqMul / (double)sampleRate)
                    + kH2Gain * harmEnv * sp_osc_tick(&osc1, kWave, 2.0 * M_PI * f1 * freqMul / (double)sampleRate)
                    + kH3Gain * harmEnv * sp_osc_tick(&osc2, kWave, 2.0 * M_PI * f2 * freqMul / (double)sampleRate);
        body *= bodyEnv;

        /* The body ALONE, so it is deterministic: no flutter. */
        double distorted = tanh(body * (1.0 + kDrive * 3.0)) * bodyEnv * 0.45;

        /* Low-mid noise is organic thickness; high noise (> 1 kHz) is spit. Attack-only so the tail stays dry. */
        float nz = noise_ma_white_tick(noise);
        sp_onepole_tick(&nzLo, 0.016, (double)nz);   /* HP ~120 Hz (drop sub rumble) */
        double nzHP = (double)nz - nzLo;
        sp_onepole_tick(&nzBand, 0.065, nzHP);       /* 1-pole LP ~500 Hz */
        sp_onepole_tick(&nzBand2, 0.065, nzBand);    /* 2nd pole → steep, kills >800 Hz (no air) */
        /* Attack-only fast decay (gone ~50 ms) → dry tail. */
        double airEnv = exp(-75.0 * tSec);
        double texture = nzBand2 * kNoiseAmt * airEnv * 3.5;

        /* A low-passed thud, not a bright tick: a click reads as "tss" and pulls the perceived pitch up. */
        float cn = noise_ma_white_tick(noise);
        sp_onepole_tick(&lpClick, 0.017, (double)cn);  /* 1-pole LP ~130 Hz */
        double click = lpClick * kClickAmt * exp(-300.0 * tSec) * 12.0;

        double g = exp(-kFade * tNorm);
        double s = (distorted + texture + click * (0.6 + 0.5 * kAttack)) * g;
        /* GENTLE soft-limit (smooth saturation, NOT a hard clamp). */
        float y = (float)(tanh(s) * 0.95);
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_base(float *out, int sampleRate, int samples,
                          const modular_params *p, int k,
                          double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── base (render_kick_internal) ── */
    double kNoiseAmt = p->gen_kick_noise[k];
    double kClickAmt = p->gen_kick_click[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k];
    double kEnv1Rate = p->gen_kick_env1[k];
    double kH2Gain   = p->gen_kick_h2[k];
    double kH3Gain   = p->gen_kick_h3[k];
    double kH4Gain   = p->gen_kick_h4[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kSat      = p->gen_kick_sat[k];

    double lpNoise = 0.0;
    double lpClick = 0.0;

    float r0 = noise_ma_white_tick(noise);
    float r1 = noise_ma_white_tick(noise);
    float r2 = noise_ma_white_tick(noise);
    float r3 = noise_ma_white_tick(noise);
    double phase0 = 2.0 * M_PI * (double)r0;
    double phase1 = 2.0 * M_PI * (double)r1;
    double phase2 = 2.0 * M_PI * (double)r2;
    double phase3 = 2.0 * M_PI * (double)r3;

    double f1 = f0 * 2.0;
    double f2 = f0 * 3.0;
    double f3 = f0 * 4.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        float n = noise_ma_white_tick(noise);
        sp_onepole_mix(&lpNoise, 0.97, 0.03, (double)n);
        double noiseEnv = exp(-16.0 * tSec);
        double noiseThud = lpNoise * noiseEnv * kNoiseAmt;

        float cn = noise_ma_white_tick(noise);
        double clickRaw = (double)cn;
        sp_onepole_mix(&lpClick, 0.85, 0.15, clickRaw);
        double hpClick = clickRaw - lpClick;
        double clickEnv = exp(-120.0 * tSec);
        double click = hpClick * clickEnv * kClickAmt;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
        double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
        double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;
        double step3 = 2.0 * M_PI * f3 * freqMul / (double)sampleRate;
        phase0 += step0; phase1 += step1; phase2 += step2; phase3 += step3;

        double s0 = osc_wave_shared(kWave, phase0);
        double s1 = osc_wave_shared(kWave, phase1);
        double s2 = osc_wave_shared(kWave, phase2);
        double s3 = osc_wave_shared(kWave, phase3);

        double env0 = exp(-kEnv0Rate * tSec);
        double env1 = exp(-kEnv1Rate * tSec);
        double env2 = exp(-12.0 * tSec);
        double env3 = exp(-18.0 * tSec);

        double tonal =
            s0 * env0 * 0.85 +
            s1 * env1 * kH2Gain +
            s2 * env2 * kH3Gain +
            s3 * env3 * kH4Gain;

        double attackShape = 1.0 + kAttack * exp(-40.0 * tSec);
        tonal *= attackShape;

        double g = exp(-kFade * tNorm);
        double mixed = (tonal + noiseThud + click) * g;
        float y = softsat_shared((float)mixed * (float)kSat) * 1.2f;
        if (y > 1.0f) y = 1.0f;
        if (y < -1.0f) y = -1.0f;
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_lofi(float *out, int sampleRate, int samples,
                          const modular_params *p, int k,
                          double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── lofi (render_kick_lofi_internal) ── */
    double kNoiseAmt = p->gen_kick_noise[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k];
    double kEnv1Rate = p->gen_kick_env1[k];
    double kH2Gain   = p->gen_kick_h2[k];
    double kH3Gain   = p->gen_kick_h3[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kSat      = p->gen_kick_sat[k];

    double lpNoise = 0.0;
    float r0 = noise_ma_white_tick(noise);
    float r1 = noise_ma_white_tick(noise);
    float r2 = noise_ma_white_tick(noise);
    double phase0 = 2.0 * M_PI * (double)r0;
    double phase1 = 2.0 * M_PI * (double)r1;
    double phase2 = 2.0 * M_PI * (double)r2;
    double f1 = f0 * 2.0;
    double f2 = f0 * 3.0;

    double lpOut = 0.0;
    double lpAlpha = 2.0 * M_PI * 600.0 / (double)sampleRate;
    if (lpAlpha > 1.0) lpAlpha = 1.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        float n = noise_ma_white_tick(noise);
        sp_onepole_mix(&lpNoise, 0.96, 0.04, (double)n);
        double noiseEnv = exp(-12.0 * tSec);
        double noiseThud = lpNoise * noiseEnv * kNoiseAmt;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
        double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
        double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;
        phase0 += step0; phase1 += step1; phase2 += step2;

        double s0 = osc_wave_shared(kWave, phase0);
        double s1 = osc_wave_shared(kWave, phase1);
        double s2 = osc_wave_shared(kWave, phase2);

        double env0 = exp(-kEnv0Rate * tSec);
        double env1 = exp(-kEnv1Rate * tSec);
        double env2 = exp(-10.0 * tSec);

        double tonal = s0 * env0 * 0.85 + s1 * env1 * kH2Gain + s2 * env2 * kH3Gain;
        double attackShape = 1.0 + kAttack * exp(-35.0 * tSec);
        tonal *= attackShape;

        double g = exp(-kFade * tNorm);
        double mixed = (tonal + noiseThud) * g;

        double levels = 128.0;
        mixed = floor(mixed * levels + 0.5) / levels;
        mixed = tanh(tanh(mixed * kSat) * 1.8);
        sp_onepole_tick(&lpOut, lpAlpha, mixed);
        mixed = lpOut;

        float y = (float)mixed * 0.95f;
        if (y > 1.0f) y = 1.0f;
        if (y < -1.0f) y = -1.0f;
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_deep(float *out, int sampleRate, int samples,
                          const modular_params *p, int k,
                          double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── deep (render_kick_deep_internal) ── */
    double kNoiseAmt = p->gen_kick_noise[k];
    double kClickAmt = p->gen_kick_click[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k];
    double kEnv1Rate = p->gen_kick_env1[k];
    double kH2Gain   = p->gen_kick_h2[k];
    double kH3Gain   = p->gen_kick_h3[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kSat      = p->gen_kick_sat[k];

    double lpNoise = 0.0;
    double phase0 = M_PI * 0.5;
    double phase1 = M_PI * 0.5;
    double f1 = f0 * 2.0;
    double f2 = f0 * 3.0;
    double phase2 = M_PI * 0.5;
    double lpClick = 0.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        float n = noise_ma_white_tick(noise);
        sp_onepole_mix(&lpNoise, 0.97, 0.03, (double)n);
        double noiseEnv = exp(-20.0 * tSec);
        double noiseThud = lpNoise * noiseEnv * kNoiseAmt;

        sp_onepole_mix(&lpClick, 0.85, 0.15, (double)n);
        double click = ((double)n - lpClick) * exp(-180.0 * tSec) * kClickAmt;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
        double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
        double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;
        phase0 += step0; phase1 += step1; phase2 += step2;

        double s0 = osc_wave_shared(kWave, phase0);
        double s1 = osc_wave_shared(kWave, phase1);
        double s2 = osc_wave_shared(kWave, phase2);

        double env0 = exp(-kEnv0Rate * tSec);
        double env1 = exp(-kEnv1Rate * tSec);
        double env2 = exp(-12.0 * tSec);

        double tonal = s0 * env0 * 0.95 + s1 * env1 * kH2Gain + s2 * env2 * kH3Gain;
        double attackShape = 1.0 + kAttack * exp(-50.0 * tSec);
        tonal *= attackShape;

        double g = exp(-kFade * tNorm);
        double mixed = (tonal + noiseThud + click) * g;

        float y = (float)(tanh(mixed * kSat) * 0.95);
        if (y > 1.0f) y = 1.0f;
        if (y < -1.0f) y = -1.0f;
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

static void gen_kick_punchy(float *out, int sampleRate, int samples,
                            const modular_params *p, int k,
                            double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── punchy (render_kick_punchy_internal) ── */
    double kClickAmt = p->gen_kick_click[k];
    double kPeRate   = p->gen_kick_pe_rate[k];
    double kPeAmt    = p->gen_kick_pe_amt[k];
    double kEnv0Rate = p->gen_kick_env0[k];
    double kEnv1Rate = p->gen_kick_env1[k];
    double kH2Gain   = p->gen_kick_h2[k];
    double kAttack   = p->gen_kick_attack[k];
    double kFade     = p->gen_kick_fade[k];
    double kSat      = p->gen_kick_sat[k];

    double lpClick = 0.0;
    float r0 = noise_ma_white_tick(noise);
    float r1 = noise_ma_white_tick(noise);
    double phase0 = 2.0 * M_PI * (double)r0;
    double phase1 = 2.0 * M_PI * (double)r1;
    double f1 = f0 * 2.0;

    for (int i = 0; i < samples; ++i) {
        double tSec  = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        float cn = noise_ma_white_tick(noise);
        double clickRaw = (double)cn;
        sp_onepole_mix(&lpClick, 0.80, 0.20, clickRaw);
        double hpClick = clickRaw - lpClick;
        double clickEnv = exp(-120.0 * tSec);
        double click = hpClick * clickEnv * kClickAmt;

        double pitchEnv = exp(-kPeRate * tSec);
        double freqMul  = 1.0 + kPeAmt * pitchEnv;

        double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
        double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
        phase0 += step0; phase1 += step1;

        double s0 = osc_wave_shared(kWave, phase0);
        double s1 = osc_wave_shared(kWave, phase1);

        double env0 = exp(-kEnv0Rate * tSec);
        double env1 = exp(-kEnv1Rate * tSec);

        double tonal = s0 * env0 * 0.85 + s1 * env1 * kH2Gain;
        double attackShape = 1.0 + kAttack * exp(-60.0 * tSec);
        tonal *= attackShape;

        double g = exp(-kFade * tNorm);
        double mixed = (tonal + click) * g;
        float y = softsat_shared((float)mixed * (float)kSat) * 1.3f;
        if (y > 1.0f) y = 1.0f;
        if (y < -1.0f) y = -1.0f;
        KICK_WRITE(y);
    }
#undef KICK_WRITE
}

/* A modal drumhead: a raised-cosine beater pulse (contact time sets which modes it reaches), stable complex one-pole
 * modes blending harmonic to measured membrane ratios, tension glide from modal energy, a resonant head on the SAME
 * pitch path (a detuned one warbles), a fixed shell bank; only fundamental + head are saturated (more intermodulates). */
#define KPHYS_MODES 9
#define KPHYS_SHELL 7
static void gen_kick_physical(float *out, int sampleRate, int samples,
                              const modular_params *p, int k,
                              double f0, int kWave, noise_ma_gen *noise, int assign) {
    (void)kWave;
    double peAmt    = p->gen_kick_pe_amt[k];
    double peRate   = p->gen_kick_pe_rate[k];
    double vel      = p->gen_kick_attack[k];
    double hard     = p->gen_kick_click[k];
    double env0     = p->gen_kick_env0[k];
    double env1     = p->gen_kick_env1[k];
    double h2       = p->gen_kick_h2[k];
    double h3       = p->gen_kick_h3[k];
    double skin     = p->gen_kick_h4[k];
    double skinDec  = p->gen_kick_mode_decay[k] * 4.0;
    double memb     = p->gen_kick_mode_detune[k] * 2.0;
    double resGain  = p->gen_kick_mode_gain[k];
    double drive    = p->gen_kick_sat[k];
    double airAmt   = p->gen_kick_noise[k];
    double fade     = p->gen_kick_fade[k];
    double shellAmt = p->gen_kick_reverb[k];
    double holdSec  = p->gen_kick_hold[k];
    double hissAmt  = p->gen_kick_hiss[k];
    if (hard < 0.0) hard = 0.0;
    if (hard > 1.0) hard = 1.0;
    if (memb < 0.0) memb = 0.0;
    if (memb > 1.0) memb = 1.0;
    if (vel < 0.05) vel = 0.05;
    if (samples <= 0) return;

    const double sr = (double)sampleRate;
    /* 0 the fundamental (saturated), 1-3 body modes harmonic→membrane, 4-8 the skin cluster. */
    static const double harm[4] = {1.0, 2.0, 2.98, 3.95};
    static const double membr[4] = {1.0, 1.72, 2.04, 2.42};
    double ratio[KPHYS_MODES] = {1.0, 0, 0, 0, 5.31, 6.63, 7.92, 9.41, 11.2};
    for (int m = 1; m < 4; ++m) ratio[m] = harm[m] + memb * (membr[m] - harm[m]);
    double gain[KPHYS_MODES] = {1.0, h2, h3, h3 * 0.5,
                                skin, skin * 0.8, skin * 0.65, skin * 0.5, skin * 0.4};
    double sigma[KPHYS_MODES] = {env0, env1, env1 * 1.3, env1 * 1.6,
                                 skinDec, skinDec * 1.15, skinDec * 1.3, skinDec * 1.5, skinDec * 1.7};
    double rho[KPHYS_MODES], rhoHold[KPHYS_MODES];
    for (int m = 0; m < KPHYS_MODES; ++m) {
        rho[m] = exp(-sigma[m] / sr);
        /* body modes (0-3) barely damped while held; the skin keeps its decay */
        rhoHold[m] = (m < 4) ? exp(-sigma[m] * 0.05 / sr) : rho[m];
    }
    int holdN = (int)(holdSec * sr);

    /* shell / room: fixed-pitch resonances (ratios to the SETTLED f0) */
    static const double shRatio[KPHYS_SHELL] = {1.38, 1.61, 2.24, 2.68, 3.22, 3.86, 4.52};
    static const double shGain[KPHYS_SHELL]  = {1.0, 0.9, 0.8, 0.75, 0.6, 0.5, 0.4};
    double shZr[KPHYS_SHELL] = {0}, shZi[KPHYS_SHELL] = {0};
    double shC[KPHYS_SHELL], shS[KPHYS_SHELL], shRho[KPHYS_SHELL];
    for (int m = 0; m < KPHYS_SHELL; ++m) {
        double th = 2.0 * M_PI * f0 * shRatio[m] / sr;
        if (th > M_PI * 0.95) th = M_PI * 0.95;
        shC[m] = cos(th); shS[m] = sin(th);
        shRho[m] = exp(-(9.0 + 1.5 * (double)m) / sr);
    }

    /* Beater force pulse: raised cosine of contact time Tc, unit area. */
    double tc = 0.007 * (1.0 - hard) + 0.0007 * hard;
    int nc = (int)(tc * sr);
    if (nc < 2) nc = 2;
    double pulseNorm = 2.0 / (double)nc;
    int slapN = (int)(0.003 * sr);
    double slapLP = 0.20 + 0.75 * hard; /* brighter slap from a harder beater */

    double zr[KPHYS_MODES] = {0}, zi[KPHYS_MODES] = {0};
    double sigR = env0 * 0.6;
    double rhoR = exp(-sigR / sr);
    double rhoRHold = exp(-sigR * 0.05 / sr);
    double rr = 0.0, ri = 0.0;
    double couple = sigR / sr;

    double kappa = peAmt * (peAmt + 2.0) * vel * vel;
    double eMax = 1e-30;

    float *low = (float *)malloc((size_t)samples * sizeof(float));
    float *hi  = (float *)malloc((size_t)samples * sizeof(float));
    if (!low || !hi) {
        free(low); free(hi);
        if (assign) for (int i = 0; i < samples; ++i) out[i] = 0.0f;
        return;
    }

    double lpS1 = 0.0, lpS2 = 0.0, lpA = 0.0, hpA = 0.0, lpR = 0.0;
    double lpH = 0.0, lpH2 = 0.0, amp0Max = 0.0;
    double lpV1 = 0.0, lpV2 = 0.0, lpV3 = 0.0, hpV = 0.0;
    double peakLow = 1e-12;
    int roomN = (int)(0.012 * sr);
    for (int i = 0; i < samples; ++i) {
        double t = (double)i / sr;
        double force = 0.0;
        if (i < nc) force = pulseNorm * 0.5 * (1.0 - cos(2.0 * M_PI * (double)i / (double)nc));

        /* normalized modal energy → tension → pitch (every mode together) */
        double e = 0.0;
        for (int m = 0; m < KPHYS_MODES; ++m) e += zr[m] * zr[m] + zi[m] * zi[m];
        if (e > eMax) eMax = e;
        double eh = (e / eMax) * exp(-peRate * t);
        if (i < nc) eh = 1.0; /* tension peaks while the beater is in contact */
        double mul = sqrt(1.0 + kappa * eh);

        int held = (i < holdN);
        for (int m = 0; m < KPHYS_MODES; ++m) {
            double rm = held ? rhoHold[m] : rho[m];
            double th = 2.0 * M_PI * f0 * ratio[m] * mul / sr;
            if (th > M_PI * 0.95) { zr[m] *= rm; zi[m] *= rm; continue; }
            double c = cos(th), s = sin(th);
            double nr = rm * (zr[m] * c - zi[m] * s) + gain[m] * force;
            double ni = rm * (zr[m] * s + zi[m] * c);
            zr[m] = nr; zi[m] = ni;
        }
        {
            double rmR = held ? rhoRHold : rhoR;
            double th = 2.0 * M_PI * f0 * mul / sr;
            double c = cos(th), s = sin(th);
            double nr = rmR * (rr * c - ri * s) + couple * zr[0];
            double ni = rmR * (rr * s + ri * c);
            rr = nr; ri = ni;
        }
        double lowS = zr[0] + resGain * rr;
        double hiS = zr[1] + zr[2] + zr[3];
        for (int m = 4; m < KPHYS_MODES; ++m) hiS += zr[m];

        /* beater slap: contact noise, ~3 ms, brighter + louder when harder */
        float wn = noise_ma_white_tick(noise);
        if (i < 4 * slapN) {
            double bp = sp_onepole_tick(&lpS1, slapLP, (double)wn);
            bp = bp - sp_onepole_tick(&lpS2, 0.10, bp);
            hiS += bp * exp(-(double)i / (double)slapN) * hard * 1.6;
        }
        /* shell / room: struck by the pulse + a short dark noise burst */
        float rn = noise_ma_white_tick(noise);
        if (shellAmt > 0.0) {
            double burst = (i < 4 * roomN) ? sp_onepole_tick(&lpR, 0.05, (double)rn) * exp(-(double)i / (double)roomN) : 0.0;
            double ex = force + burst * 0.02;
            double sh = 0.0;
            for (int m = 0; m < KPHYS_SHELL; ++m) {
                double nr = shRho[m] * (shZr[m] * shC[m] - shZi[m] * shS[m]) + shGain[m] * ex;
                double ni = shRho[m] * (shZr[m] * shS[m] + shZi[m] * shC[m]);
                shZr[m] = nr; shZi[m] = ni;
                sh += nr;
            }
            hiS += sh * shellAmt * 0.35;
        }
        /* On the fundamental's ENVELOPE, never its wave: no AM flutter. */
        double amp0 = sqrt(zr[0] * zr[0] + zi[0] * zi[0]);
        float an = noise_ma_white_tick(noise);
        double a = sp_onepole_tick(&lpA, 0.06, (double)an);
        a = a - sp_onepole_tick(&hpA, 0.006, a);
        hiS += a * amp0 * airAmt * 4.0;
        /* A recorded kick's body is about half noise in 25-120 Hz, a synth kick nearly all tone (the "digital" tell): steep
         * low-passed noise on the body envelope plus a faint slow floor above 1 kHz. Drawn only when on, so 0 is byte-identical. */
        if (hissAmt > 0.0) {
            if (amp0 > amp0Max) amp0Max = amp0;
            double envB = amp0 + resGain * sqrt(rr * rr + ri * ri);
            float vn = noise_ma_white_tick(noise);
            double v = sp_onepole_tick(&lpV1, 0.010, (double)vn);
            v = sp_onepole_tick(&lpV2, 0.010, v);
            v = sp_onepole_tick(&lpV3, 0.016, v);
            v = v - sp_onepole_tick(&hpV, 0.0036, v);
            hiS += v * envB * hissAmt * 90.0;
            double envH = pow(amp0 / (amp0Max + 1e-30), 0.35);
            float hn = noise_ma_white_tick(noise);
            double h = (double)hn - sp_onepole_tick(&lpH, 0.14, (double)hn);
            h = sp_onepole_tick(&lpH2, 0.30, h);
            hiS += h * envH * amp0Max * hissAmt * 0.010;
        }

        low[i] = (float)lowS;
        hi[i]  = (float)hiS;
        if (fabs(lowS) > peakLow) peakLow = fabs(lowS);
    }

    /* biased drive on the fundamental only, DC-blocked; the rest clean */
    double d = drive * (0.7 + 0.3 * vel);
    const double bias = 0.15;
    double tb = tanh(d * bias);
    double dn = (d > 1e-6) ? (tanh(d * (1.0 + bias)) - tb) : 1.0;
    double inv = 1.0 / peakLow;
    sp_dcblock dc = {0.0, 0.0};
    double dcR = 1.0 - 2.0 * M_PI * 12.0 / sr;
    double peak = 1e-12;
    int fadeN = (int)(0.008 * sr);
    for (int i = 0; i < samples; ++i) {
        double t = (double)i / sr;
        double x = (double)low[i] * inv;
        double y = (d > 1e-6) ? (tanh(d * (x + bias)) - tb) / dn : x;
        y = sp_dcblock_tick(&dc, y, dcR);
        y += (double)hi[i] * inv;
        if (fade > 0.0) y *= exp(-fade * t);
        int left = samples - 1 - i;
        if (left < fadeN) y *= 0.5 * (1.0 - cos(M_PI * (double)left / (double)fadeN));
        low[i] = (float)y;
    }
    /* A look-ahead limiter on the GAIN (no waveshaping, no intermodulation) holds the transient just above the body: a
     * recorded kick's low crest and its loudness at a fixed peak. Strength follows the drive. */
    {
        double ceil = 1.0 + 2.0 / (1.0 + drive);
        int la = (int)(0.0015 * sr);
        if (la < 1) la = 1;
        double relC = exp(-1.0 / (0.025 * sr));
        double atkC = exp(-1.0 / (0.0005 * sr));
        float *need = hi; /* reuse: hi[] is no longer needed */
        for (int i = 0; i < samples; ++i) {
            double a = fabs((double)low[i]);
            need[i] = (float)((a > ceil) ? ceil / a : 1.0);
        }
        double gcur = 1.0;
        for (int i = 0; i < samples; ++i) {
            double gmin = 1.0;
            int end = i + la < samples ? i + la : samples - 1;
            for (int j = i; j <= end; ++j) if ((double)need[j] < gmin) gmin = (double)need[j];
            gcur = (gmin < gcur) ? gmin + (gcur - gmin) * atkC : gmin + (gcur - gmin) * relC;
            double y = (double)low[i] * gcur;
            low[i] = (float)y;
            if (fabs(y) > peak) peak = fabs(y);
        }
    }
    double g = 0.98 / peak;
    for (int i = 0; i < samples; ++i) {
        float v = (float)((double)low[i] * g);
        if (assign) out[i] = v; else out[i] += v;
    }
    free(low);
    free(hi);
}
#undef KPHYS_SHELL
#undef KPHYS_MODES

static void modular_gen_slot_kick(float *out, int sampleRate, int samples,
                                  const modular_params *p, int k,
                                  double voice_freq, int assign) {
    /* Below 0.5 silences the voice: an assigning slot zeroes the buffer, an accumulating one adds nothing. */
    if (p->kick_enabled < 0.5f) {
        if (assign) {
            for (int i = 0; i < samples; ++i) out[i] = 0.0f;
        }
        return;
    }
    int variant = (int)lrint(p->gen_kick_variant[k]);
    int kWave   = (int)p->gen_wave[k];

    double f0 = (double)p->gen_freq[k];
    if ((int)lrint(p->gen_freq_mode[k]) == 0) f0 *= voice_freq;

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    if (variant == 1) {
        gen_kick_deep(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 2) {
        gen_kick_punchy(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 3) {
        gen_kick_lofi(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 4) {
        gen_kick_tight(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 5) {
        gen_kick_hybrid(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 6) {
        gen_kick_modal(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 7) {
        gen_kick_acoustic(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else if (variant == 8) {
        gen_kick_physical(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    } else {
        gen_kick_base(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    }
}

/* The source-6 tom: one parameterized loop, the variant (0 tom, 1 high, 2 low) choosing structural literals while the
 * knobs are read from the block. noise_ma seed 0, one seed draw then one per sample; the fundamental flows through
 * voice_freq. assign 1 ASSIGNS the first write so a -0.0 sample is not flipped. */
static void modular_gen_slot_tom(float *out, int sampleRate, int samples,
                                 const modular_params *p, int k,
                                 double voice_freq, int assign, double gain) {
    /* Appended as a FLOAT, not folded in: `(float)x * scale` and `(float)(x * scale * gain)` round differently, while ×1.0f
     * is exact, so unity gain stays byte-identical. */
    const float gf = (float)gain;
    int variant = (int)lrint(p->gen_tom_variant[k]);
    if (variant < 0 || variant > 2) variant = 0;
    int tWave = (int)p->gen_wave[k];

    /* Structural constants per variant, not user knobs. */
    static const double fStartMul[3]  = { 1.25, 1.30, 1.20 };
    static const double fEndMul[3]    = { 0.85, 0.88, 0.82 };
    static const double fundEnvMul[3] = { 1.15, 1.10, 1.20 };
    static const double envO1Rate[3]  = { 4.0,  5.0,  3.2  };
    static const double envO2Rate[3]  = { 5.5,  6.5,  4.5  };
    static const double resAmt[3]     = { 0.4,  0.5,  0.35 };
    static const double resRate[3]    = { 80.0, 90.0, 70.0 };
    static const double lpAlphaV[3]   = { 0.12, 0.14, 0.10 };
    static const double hpAlphaV[3]   = { 0.92, 0.90, 0.94 };
    static const double attackRate[3] = { 120.0, 140.0, 100.0 };
    static const double ambientRate[3]= { 6.0,  8.0,  4.0  };
    static const double globalFade[3] = { 0.05, 0.06, 0.04 };

    double tSweepRate = p->gen_tom_sweep[k];
    double tRingRate  = p->gen_tom_ring[k];
    double tO1Gain    = p->gen_tom_o1[k];
    double tO2Gain    = p->gen_tom_o2[k];
    double tStick     = p->gen_tom_stick[k];
    double tRoom      = p->gen_tom_room[k];

    double fundamentalHz = (double)p->gen_freq[k];
    if ((int)lrint(p->gen_freq_mode[k]) == 0) fundamentalHz *= voice_freq;

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    sp_osc oscFund, oscO1, oscO2;
    sp_osc_init(&oscFund, 0.0);
    sp_osc_init(&oscO1, 0.0);
    sp_osc_init(&oscO2, 0.0);

    double lpState1 = 0.0, lpState2 = 0.0;
    double hpState = 0.0;

    float seed = noise_ma_white_tick(&noise);
    double detune = 1.0 + 0.04 * (double)seed;

    double fBase = fundamentalHz * detune;
    double fStart = fBase * fStartMul[variant];
    double fEnd = fBase * fEndMul[variant];

    for (int i = 0; i < samples; ++i) {
        double tSec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        float n = noise_ma_white_tick(&noise);

        double freqFund = fEnd + (fStart - fEnd) * exp(-tSweepRate * tSec);

        double fund = sp_osc_tick(&oscFund, tWave, 2.0 * M_PI * freqFund / (double)sampleRate);
        double o1 = sp_osc_tick(&oscO1, tWave, 2.0 * M_PI * freqFund * 1.5 / (double)sampleRate);
        double o2 = sp_osc_tick(&oscO2, tWave, 2.0 * M_PI * freqFund * 2.1 / (double)sampleRate);

        double envFund = exp(-tRingRate * tSec);
        double envO1 = exp(-envO1Rate[variant] * tSec);
        double envO2 = exp(-envO2Rate[variant] * tSec);
        double tone = fund * envFund * fundEnvMul[variant] + o1 * envO1 * tO1Gain + o2 * envO2 * tO2Gain;

        double resonantBoost = 1.0 + resAmt[variant] * exp(-resRate[variant] * tSec);
        tone *= resonantBoost;

        double lpAlpha = lpAlphaV[variant];
        lpState1 = lpState1 * (1.0 - lpAlpha) + (double)n * lpAlpha;
        lpState2 = lpState2 * (1.0 - lpAlpha) + lpState1 * lpAlpha;
        double hpAlpha = hpAlphaV[variant];
        double midNoise = lpState2 - hpState;
        hpState = hpState * hpAlpha + lpState2 * (1.0 - hpAlpha);
        double attackEnv = exp(-attackRate[variant] * tSec);
        double attack = midNoise * attackEnv * tStick;

        double ambientEnv = exp(-ambientRate[variant] * tSec);
        double ambient = lpState1 * ambientEnv * tRoom;

        double global = 1.0 - globalFade[variant] * tNorm;
        if (global < 0.0) global = 0.0;

        float y = softsat_shared((float)((tone + attack + ambient) * global));
        if (assign) out[i] = y * gf;
        else        out[i] += y * gf;
    }
}

/* A filter designed at 0 Hz is NaN from the first sample and an RBJ biquad above Nyquist is unstable; Tune's range
 * reached both. 0.45·sr matches the cymbal plate's cap. */
static double snare_fc(double fc, int sampleRate) {
    double nyq = 0.45 * (double)sampleRate;
    if (!(fc >= 1.0)) return 1.0;
    return fc > nyq ? nyq : fc;
}

/* The source-7 voice, selected by gen_snare_variant (0 snare, 1 rimshot, 2 sidestick): each branch keeps its float-op
 * order, its own inline biquad form, noise topology (one seed draw, then one per sample) and 0.95·softsat. The
 * fundamental flows through voice_freq. */
static void gen_snare_rimshot(float *out, int sampleRate, int samples,
                              const modular_params *p, int k,
                              double fundamentalHz, int sWave,
                              noise_ma_gen *noise, int assign) {
#define SNARE_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── rimshot (render_snare_rimshot_internal) ── */
    double sTone2  = p->gen_snare_tone2[k];
    double sTune   = p->gen_snare_tune[k];
    double sToneD  = p->gen_snare_tone_d[k];
    double sNoiseD = p->gen_snare_noise_d[k];
    double sToneM  = p->gen_snare_tone_m[k];
    double sNoiseM = p->gen_snare_noise_m[k];
    double sAtk    = p->gen_snare_attack[k];

    float seed;
    seed = noise_ma_white_tick(noise);
    double detune = 1.0 + 0.02 * (double)seed;

    double f1 = fundamentalHz * detune;
    double f2 = sTone2 * detune;
    double f3 = 1700.0 * detune;
    double f4 = 2800.0 * detune;
    sp_osc osc1, osc2, osc3, osc4;
    sp_osc_init(&osc1, 0.0);
    sp_osc_init(&osc2, 0.0);
    sp_osc_init(&osc3, 0.0);
    sp_osc_init(&osc4, 0.0);

    double rc = 1.0 / (2.0 * M_PI * snare_fc(400.0 * sTune, sampleRate));
    double dt = 1.0 / (double)sampleRate;
    double hpAlpha = rc / (rc + dt);
    double hpPrev = 0.0, hpOut = 0.0;

    double lpAlpha = dt / (1.0 / (2.0 * M_PI * snare_fc(5000.0 * sTune, sampleRate)) + dt);
    double lpState = 0.0;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double p1 = sp_osc_tick(&osc1, sWave, 2.0 * M_PI * f1 / (double)sampleRate) * 1.0  * exp(-sToneD * sec);
        double p2 = sp_osc_tick(&osc2, sWave, 2.0 * M_PI * f2 / (double)sampleRate) * 0.95 * exp(-50.0 * sec);
        double p3 = sp_osc_tick(&osc3, sWave, 2.0 * M_PI * f3 / (double)sampleRate) * 0.8  * exp(-65.0 * sec);
        double p4 = sp_osc_tick(&osc4, sWave, 2.0 * M_PI * f4 / (double)sampleRate) * 0.5  * exp(-90.0 * sec);
        double tones = p1 + p2 + p3 + p4;

        float n = noise_ma_white_tick(noise);
        sp_onepole_tick(&lpState, lpAlpha, (double)n);
        double hpNoise = (double)n - lpState;
        double transient = hpNoise * sNoiseM * exp(-sNoiseD * sec);

        double attack = 1.0 + sAtk * exp(-1500.0 * sec);

        double raw = (tones * sToneM + transient) * attack;

        hpOut = hpAlpha * (hpOut + raw - hpPrev);
        hpPrev = raw;

        double driven = tanh(hpOut * 4.0);
        double global = 1.0 - 0.1 * tNorm;
        if (global < 0.0) global = 0.0;

        SNARE_WRITE(0.95f * softsat_shared((float)(driven * global)));
    }
#undef SNARE_WRITE
}

static void gen_snare_sidestick(float *out, int sampleRate, int samples,
                                const modular_params *p, int k,
                                double fundamentalHz, int sWave,
                                noise_ma_gen *noise, int assign) {
#define SNARE_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── sidestick (render_snare_sidestick_internal) ── */
    double sTone2  = p->gen_snare_tone2[k];
    double sTune   = p->gen_snare_tune[k];
    double sToneD  = p->gen_snare_tone_d[k];
    double sNoiseD = p->gen_snare_noise_d[k];
    double sToneM  = p->gen_snare_tone_m[k];
    double sNoiseM = p->gen_snare_noise_m[k];
    double sAtk    = p->gen_snare_attack[k];

    float seed;
    seed = noise_ma_white_tick(noise);
    double detune = 1.0 + 0.02 * (double)seed;

    double fWood = fundamentalHz * detune;
    double fRim = sTone2 * detune;
    sp_osc oscWood, oscRim;
    sp_osc_init(&oscWood, 0.0);
    sp_osc_init(&oscRim, 0.0);

    double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
    {
        double fc = snare_fc(1500.0 * sTune, sampleRate); double Q = 0.7;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
        double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
        b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0; a1_bp = a1/a0; a2_bp = a2/a0;
    }
    sp_biquad bq_bp = {0};
    bq_bp.b0 = b0_bp; bq_bp.b1 = b1_bp; bq_bp.b2 = b2_bp; bq_bp.a1 = a1_bp; bq_bp.a2 = a2_bp;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;

        double wood = sp_osc_tick(&oscWood, sWave, 2.0 * M_PI * fWood / (double)sampleRate) * sToneM * exp(-sToneD * sec);
        double rim = sp_osc_tick(&oscRim, sWave, 2.0 * M_PI * fRim / (double)sampleRate) * 0.45 * exp(-120.0 * sec);

        float n = noise_ma_white_tick(noise);
        double x = (double)n;
        double ybp = sp_biquad_tick(&bq_bp, x);
        double noise_out = ybp * sNoiseM * exp(-sNoiseD * sec);

        double attackBoost = 1.0 + sAtk * exp(-800.0 * sec);
        double mixed = (wood + rim + noise_out) * attackBoost;

        SNARE_WRITE(0.95f * softsat_shared((float)mixed));
    }
#undef SNARE_WRITE
}

static void gen_snare_main(float *out, int sampleRate, int samples,
                           const modular_params *p, int k,
                           double fundamentalHz, int sWave,
                           noise_ma_gen *noise, int assign) {
#define SNARE_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* A fat-bottom snare: lower 2nd tone, more tone and less noise, a fast body thump and a long wire ring. */
    double sTone2  = p->gen_snare_tone2[k];
    double sTune   = p->gen_snare_tune[k];
    double sToneD  = p->gen_snare_tone_d[k];
    double sNoiseD = p->gen_snare_noise_d[k];
    double sTailD  = p->gen_snare_tail_d[k];
    double sToneM  = p->gen_snare_tone_m[k];
    double sNoiseM = p->gen_snare_noise_m[k];
    double sWireM  = p->gen_snare_wire_m[k];
    double sAtk    = p->gen_snare_attack[k];

    float seed;
    seed = noise_ma_white_tick(noise);
    double detune = 1.0 + 0.02 * (double)seed;

    double fBody1 = fundamentalHz * detune;
    double fBody2 = sTone2 * detune;
    sp_osc osc1, osc2;
    sp_osc_init(&osc1, 0.0);
    sp_osc_init(&osc2, 0.0);

    double b0_lp, b1_lp, b2_lp, a1_lp, a2_lp;
    double b0_bp1, b1_bp1, b2_bp1, a1_bp1, a2_bp1;
    double b0_bp2, b1_bp2, b2_bp2, a1_bp2, a2_bp2;
    double b0_bp3, b1_bp3, b2_bp3, a1_bp3, a2_bp3;

    {
        double fc = snare_fc(350.0 * sTune, sampleRate);
        double Q = 0.7;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0);
        double alpha = sin(w0) / (2.0 * Q);
        double b0 = (1.0 - cosw) * 0.5;
        double b1 = 1.0 - cosw;
        double b2 = (1.0 - cosw) * 0.5;
        double a0 = 1.0 + alpha;
        double a1 = -2.0 * cosw;
        double a2 = 1.0 - alpha;
        b0_lp = b0 / a0; b1_lp = b1 / a0; b2_lp = b2 / a0;
        a1_lp = a1 / a0; a2_lp = a2 / a0;
    }
    {
        double fc = snare_fc(1800.0 * sTune, sampleRate);
        double Q = 1.0;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0);
        double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5;
        double b1 = 0.0;
        double b2 = -b0;
        double a0 = 1.0 + alpha;
        double a1 = -2.0 * cosw;
        double a2 = 1.0 - alpha;
        b0_bp1 = b0 / a0; b1_bp1 = b1 / a0; b2_bp1 = b2 / a0;
        a1_bp1 = a1 / a0; a2_bp1 = a2 / a0;
    }
    {
        double fc = snare_fc(3200.0 * sTune, sampleRate);
        double Q = 1.0;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0);
        double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5;
        double b1 = 0.0;
        double b2 = -b0;
        double a0 = 1.0 + alpha;
        double a1 = -2.0 * cosw;
        double a2 = 1.0 - alpha;
        b0_bp2 = b0 / a0; b1_bp2 = b1 / a0; b2_bp2 = b2 / a0;
        a1_bp2 = a1 / a0; a2_bp2 = a2 / a0;
    }
    {
        double fc = snare_fc(4500.0 * sTune, sampleRate);
        double Q = 1.0;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0);
        double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5;
        double b1 = 0.0;
        double b2 = -b0;
        double a0 = 1.0 + alpha;
        double a1 = -2.0 * cosw;
        double a2 = 1.0 - alpha;
        b0_bp3 = b0 / a0; b1_bp3 = b1 / a0; b2_bp3 = b2 / a0;
        a1_bp3 = a1 / a0; a2_bp3 = a2 / a0;
    }

    sp_biquad bq_lp = {0};
    bq_lp.b0 = b0_lp; bq_lp.b1 = b1_lp; bq_lp.b2 = b2_lp; bq_lp.a1 = a1_lp; bq_lp.a2 = a2_lp;
    sp_biquad bq_bp1 = {0};
    bq_bp1.b0 = b0_bp1; bq_bp1.b1 = b1_bp1; bq_bp1.b2 = b2_bp1; bq_bp1.a1 = a1_bp1; bq_bp1.a2 = a2_bp1;
    sp_biquad bq_bp2 = {0};
    bq_bp2.b0 = b0_bp2; bq_bp2.b1 = b1_bp2; bq_bp2.b2 = b2_bp2; bq_bp2.a1 = a1_bp2; bq_bp2.a2 = a2_bp2;
    sp_biquad bq_bp3 = {0};
    bq_bp3.b0 = b0_bp3; bq_bp3.b1 = b1_bp3; bq_bp3.b2 = b2_bp3; bq_bp3.a1 = a1_bp3; bq_bp3.a2 = a2_bp3;

    double lfoRate = 13.0 + 2.0 * (double)seed;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double f1Step = fBody1 - 40.0 * sec;
        double f2Step = fBody2 - 60.0 * sec;
        if (f1Step < 140.0) f1Step = 140.0;
        if (f2Step < 180.0) f2Step = 180.0;
        double bodyTone = sp_osc_tick(&osc1, sWave, 2.0 * M_PI * f1Step / (double)sampleRate)
                        + 0.6 * sp_osc_tick(&osc2, sWave, 2.0 * M_PI * f2Step / (double)sampleRate);
        double envBody = exp(-sec * sToneD);
        double body = bodyTone * envBody * 0.45;

        float n = noise_ma_white_tick(noise);
        double x = (double)n;

        double ylp = sp_biquad_tick(&bq_lp, x);
        double lowNoise = ylp;

        double ybp1 = sp_biquad_tick(&bq_bp1, x);

        double ybp2 = sp_biquad_tick(&bq_bp2, x);

        double ybp3 = sp_biquad_tick(&bq_bp3, x);

        double lfo = 1.0 + 0.10 * sin(2.0 * M_PI * lfoRate * sec);
        double wiresBand = (ybp1 * 0.9 + ybp2 * 0.6 + ybp3 * 0.2) * lfo;

        double envLow = exp(-sec * sNoiseD);
        double envHighFast = exp(-sec * 200.0);
        double envHighTail = exp(-sec * sTailD);

        double noisyBody = lowNoise * envLow * 1.1;
        double wires = wiresBand * (envHighFast * 0.9 + envHighTail * 0.45);

        double attackBoost = 1.0 + sAtk * exp(-sec * 350.0);
        wires *= attackBoost;

        double mixed = body * sToneM + noisyBody * sNoiseM + wires * sWireM;
        mixed *= (1.0 - 0.04 * tNorm);

        SNARE_WRITE(0.95f * softsat_shared((float)mixed));
    }
#undef SNARE_WRITE
}

/* Measured from the reference by two independent methods. Mode 2 comes from snare_tone2_freq so "Ring Pitch" is live. */
static const double kSnareAcousticRatio[4] = {2.31, 2.99, 3.72, 4.50};
/* Peak levels, relative to mode 1, from the same measurement. */
static const double kSnareAcousticGain[4]  = {0.29, 0.24, 0.29, 0.07};
/* The spread keeps the bank from reading as a tidy synth chord. */
static const double kSnareAcousticSpread[4] = {0.90, 1.08, 1.44, 1.42};

static void gen_snare_acoustic(float *out, int sampleRate, int samples,
                               const modular_params *p, int k,
                               double fundamentalHz, int sWave,
                               noise_ma_gen *noise, int assign) {
#define SNARE_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* An open acoustic snare: inharmonic head modes, a shell/air resonance that outlives the strike, and wires driven BY
     * the head. Mode 1 is TWO-STAGE (fast drop, then a long plateau): a single exponential is right for 150 ms and hollow
     * after, so do not "simplify" the slow term away. */
    double sTone2  = p->gen_snare_tone2[k];
    double sTune   = p->gen_snare_tune[k];
    double sToneD  = p->gen_snare_tone_d[k];
    double sNoiseD = p->gen_snare_noise_d[k];
    double sTailD  = p->gen_snare_tail_d[k];
    double sToneM  = p->gen_snare_tone_m[k];
    double sNoiseM = p->gen_snare_noise_m[k];
    double sWireM  = p->gen_snare_wire_m[k];
    double sAtk    = p->gen_snare_attack[k];

    /* The RATE is what matters: decayed at tail_decay the hum dies with the ring and the plateau disappears. Fixed: it
     * is the instrument's identity, not a knob. */
    const double kShellLevel = 0.025;
    const double kShellDecay = 1.2;
    /* A single exponential slow enough for the plateau would still ring when the buffer ends, so a high-order gate ends it. */
    const double kShellGateSec = 0.95;

    sp_osc osc1, osc2, oscN[4];
    sp_osc_init(&osc1, M_PI * 0.5);
    sp_osc_init(&osc2, M_PI * 0.5);
    for (int m = 0; m < 4; ++m) sp_osc_init(&oscN[m], M_PI * 0.5);

    /* Measured wire energy runs 2-16 kHz with the top octave ~5 dB down. */
    sp_biquad bq_w1 = {0}, bq_w2 = {0};
    {
        double fc = snare_fc(4000.0 * sTune, sampleRate), Q = 0.8;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0), alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5, a0 = 1.0 + alpha;
        bq_w1.b0 = b0 / a0; bq_w1.b1 = 0.0; bq_w1.b2 = -b0 / a0;
        bq_w1.a1 = (-2.0 * cosw) / a0; bq_w1.a2 = (1.0 - alpha) / a0;
    }
    {
        double fc = snare_fc(9000.0 * sTune, sampleRate), Q = 0.7;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0), alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5, a0 = 1.0 + alpha;
        bq_w2.b0 = b0 / a0; bq_w2.b1 = 0.0; bq_w2.b2 = -b0 / a0;
        bq_w2.a1 = (-2.0 * cosw) / a0; bq_w2.a2 = (1.0 - alpha) / a0;
    }

    double lpHi = 0.0, lpLo = 0.0, couple = 0.0;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;

        /* Struck at t=0, no swell: the natural transient. */
        double m1 = sp_osc_tick(&osc1, sWave, 2.0 * M_PI * fundamentalHz / (double)sampleRate);
        double m2 = sp_osc_tick(&osc2, sWave, 2.0 * M_PI * sTone2 / (double)sampleRate);

        /* mode 1: fast struck-head term + slow shell/air term (see above). */
        double shellT = sec / kShellGateSec;
        double shellT3 = shellT * shellT * shellT;
        double shellGate = exp(-shellT3 * shellT3); /* (t/T)^6 */
        double e1 = exp(-sToneD * sec) + kShellLevel * exp(-kShellDecay * sec) * shellGate;
        double thump = m1 * e1;

        /* mode 2 = the ring anchor, on its own knob. */
        double ring = 0.25 * m2 * exp(-sTailD * 1.44 * sec);
        for (int m = 0; m < 4; ++m) {
            double fm = fundamentalHz * kSnareAcousticRatio[m];
            double mn = sp_osc_tick(&oscN[m], sWave, 2.0 * M_PI * fm / (double)sampleRate);
            ring += kSnareAcousticGain[m] * mn * exp(-sTailD * kSnareAcousticSpread[m] * sec);
        }

        double head = thump * sToneM + ring * sNoiseM;

        /* Real wires are driven by the head, so the buzz breathes with the hit instead of a pasted envelope. */
        double wn = (double)noise_ma_white_tick(noise);
        double wireBand = sp_biquad_tick(&bq_w1, wn) * 0.7 + sp_biquad_tick(&bq_w2, wn) * 0.5;
        sp_onepole_tick(&couple, 0.08, fabs(head));
        double drive = 0.25 + 0.75 * (couple * 2.5 > 1.0 ? 1.0 : couple * 2.5);
        double wires = wireBand * exp(-sNoiseD * sec) * drive * sWireM;

        /* ── stick transient: band-limited burst, not full-bright white. ── */
        double cn = (double)noise_ma_white_tick(noise);
        double bpStick = sp_onepole_tick(&lpHi, 0.45, cn) - sp_onepole_tick(&lpLo, 0.08, cn);
        double stick = bpStick * sAtk * exp(-350.0 * sec);

        /* No saturation across inharmonic modes: tanh on them makes intermodulation roughness, a "creaky door". */
        double s = head + wires + stick;
        SNARE_WRITE((float)(tanh(s) * 0.95));
    }
#undef SNARE_WRITE
}

static void modular_gen_slot_snare(float *out, int sampleRate, int samples,
                                   const modular_params *p, int k,
                                   double voice_freq, int assign) {
    /* A disabled slot contributes nothing; an assigning one still zeroes out[] rather than leaving stale contents. */
    if (p->snare_enabled < 0.5f) {
        if (assign) {
            for (int i = 0; i < samples; ++i) out[i] = 0.0f;
        }
        return;
    }
    int variant = (int)lrint(p->gen_snare_variant[k]);
    if (variant < 0 || variant > 3) variant = 0;
    int sWave = (int)p->gen_wave[k];

    double fundamentalHz = (double)p->gen_freq[k];
    if ((int)lrint(p->gen_freq_mode[k]) == 0) fundamentalHz *= voice_freq;

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    if (variant == 1) {
        gen_snare_rimshot(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    } else if (variant == 2) {
        gen_snare_sidestick(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    } else if (variant == 3) {
        gen_snare_acoustic(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    } else {
        gen_snare_main(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    }
}

/* The source-8 clap: pure noise with a different draw topology (no seed draw), which is why it is its own source. A
 * band-pass at 1800 Hz, a 3-4-burst envelope and a room tail; softsat with NO 0.95 scale. No pitched body. */
static void modular_gen_slot_clap(float *out, int sampleRate, int samples,
                                  const modular_params *p, int k,
                                  double voice_freq, int assign, double gain) {
    /* Appended as a FLOAT, not folded in: `(float)x * scale` and `(float)(x * scale * gain)` round differently, while ×1.0f
     * is exact, so unity gain stays byte-identical. */
    const float gf = (float)gain;
    (void)voice_freq; /* clap is pure noise — no pitched body */
    double sTune   = p->gen_snare_tune[k];
    double sNoiseD = p->gen_snare_noise_d[k];
    double sTailD  = p->gen_snare_tail_d[k];
    double sNoiseM = p->gen_snare_noise_m[k];
    double sAtk    = p->gen_snare_attack[k];

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
    {
        double fc = snare_fc(1800.0 * sTune, sampleRate);
        double Q = 1.5;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0);
        double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5;
        double b1 = 0.0;
        double b2 = -b0;
        double a0 = 1.0 + alpha;
        double a1 = -2.0 * cosw;
        double a2 = 1.0 - alpha;
        b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0;
        a1_bp = a1/a0; a2_bp = a2/a0;
    }
    sp_biquad bq_bp = {0};
    bq_bp.b0 = b0_bp; bq_bp.b1 = b1_bp; bq_bp.b2 = b2_bp; bq_bp.a1 = a1_bp; bq_bp.a2 = a2_bp;

    for (int i = 0; i < samples; ++i) {
        double t = (double)i / (double)sampleRate;
        float n = noise_ma_white_tick(&noise);

        double ybp = sp_biquad_tick(&bq_bp, (double)n);

        double burst =
            exp(-sAtk * fabs(t - 0.000)) +
            exp(-sAtk * fabs(t - 0.015)) +
            exp(-sAtk * fabs(t - 0.030)) +
            0.7 * exp(-80.0 * fabs(t - 0.060));
        double env = exp(-sNoiseD * t);

        double roomTail = (double)n * exp(-sTailD * t) * sNoiseM;

        float y = softsat_shared((float)(ybp * burst * env + roomTail));
        if (assign) out[i] = y * gf;
        else        out[i] += y * gf;
    }
}

/* The source-9 metallic voice, selected by gen_cym_variant (0 hihat, 1 open hihat, 2 cowbell, 3 shaker, 4 ride, 5 crash):
 * pairs that share a loop shape keep distinct branches so each float-op order is exact. Partials are ABSOLUTE Hz ×
 * cym_tune: no pitched fundamental. assign 1 ASSIGNS the first write. */
static void gen_cymbal_open(float *out, int sampleRate, int samples,
                            const modular_params *p, int k,
                            noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
#define HH_PARTIALS 6
    /* ── open-hihat (render_open_hihat_internal) ── */
    double cyTune   = p->gen_cym_tune[k];
    double cyFast   = p->gen_cym_env_fast[k];
    double cyTail   = p->gen_cym_env_tail[k];
    double cyToneM  = p->gen_cym_tone_m[k];
    double cyNoiseM = p->gen_cym_noise_m[k];
    double cyNoiseD = p->gen_cym_noise_d[k];
    int    cyWave   = (int)p->gen_wave[k];

    static const double baseFreq[HH_PARTIALS] = {4100.0, 5400.0, 6700.0, 8300.0, 9900.0, 11800.0};
    sp_osc osc[HH_PARTIALS];
    double freq[HH_PARTIALS];
    double gain[HH_PARTIALS];

    for (int kk = 0; kk < HH_PARTIALS; ++kk) {
        float s = noise_ma_white_tick(noise);
        double det = 1.0 + 0.015 * (double)s;
        freq[kk] = (baseFreq[kk] * cyTune) * det;
        sp_osc_init(&osc[kk], 2.0 * M_PI * (double)s);
        gain[kk] = (kk < 3) ? 1.0 / (double)HH_PARTIALS : 0.9 / (double)HH_PARTIALS;
    }

    double lpCluster = 0.0;
    double lpNoise = 0.0;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double cluster = 0.0;
        for (int kk = 0; kk < HH_PARTIALS; ++kk) {
            double sq = sp_osc_tick(&osc[kk], cyWave, 2.0 * M_PI * freq[kk] / (double)sampleRate);
            double lfo = 1.0 + 0.12 * sin(2.0 * M_PI * (9.0 + 3.0 * kk) * sec);
            cluster += sq * gain[kk] * lfo;
        }

        sp_onepole_mix(&lpCluster, 0.92, 0.08, cluster);
        double hpCluster = cluster - lpCluster;

        float wn = noise_ma_white_tick(noise);
        double xNoise = (double)wn;
        sp_onepole_mix(&lpNoise, 0.94, 0.06, xNoise);
        double hpNoise = xNoise - lpNoise;

        double envClusterFast = exp(-sec * cyFast);
        double envClusterTail = exp(-sec * cyTail);
        double envCluster = envClusterFast * 0.7 + envClusterTail * 0.8;

        double envNoiseFast = exp(-sec * 80.0);
        double envNoiseTail = exp(-sec * cyNoiseD);
        double envNoise = envNoiseFast * 0.7 + envNoiseTail * 1.1;

        double y = hpCluster * envCluster * cyToneM + hpNoise * envNoise * cyNoiseM;
        y *= exp(-1.5 * tNorm);

        CYM_WRITE(0.9f * softsat_shared((float)(y * 1.05)));
    }
#undef HH_PARTIALS
#undef CYM_WRITE
}

static void gen_cymbal_cowbell(float *out, int sampleRate, int samples,
                               const modular_params *p, int k,
                               noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── cowbell (render_cowbell_internal) ── */
    float seed = noise_ma_white_tick(noise);
    double detune = 1.0 + 0.01 * (double)seed;

    double cyTune   = p->gen_cym_tune[k];
    double cyFast   = p->gen_cym_env_fast[k];
    double cyTail   = p->gen_cym_env_tail[k];
    double cyToneM  = p->gen_cym_tone_m[k];
    double cyNoiseM = p->gen_cym_noise_m[k];
    double cyNoiseD = p->gen_cym_noise_d[k];
    int    cyWave   = (int)p->gen_wave[k];

    const double freqs[4] = {
        (640.0 * cyTune) * detune,
        (920.0 * cyTune) * detune,
        (1300.0 * cyTune) * detune,
        (1900.0 * cyTune) * detune,
    };
    const double gains[4] = {1.0, 0.85, 0.6, 0.4};
    sp_osc osc[4];
    for (int kk = 0; kk < 4; ++kk) sp_osc_init(&osc[kk], 0.0);

    double lpNoise = 0.0;

    double b0_cb, b1_cb, b2_cb, a1_cb, a2_cb;
    {
        double fc = 1200.0;
        double Q = 2.0;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0);
        double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5;
        double b1 = 0.0;
        double b2 = -b0;
        double a0 = 1.0 + alpha;
        double a1 = -2.0 * cosw;
        double a2 = 1.0 - alpha;
        b0_cb = b0/a0; b1_cb = b1/a0; b2_cb = b2/a0;
        a1_cb = a1/a0; a2_cb = a2/a0;
    }
    sp_biquad bq_cb = {0};
    bq_cb.b0 = b0_cb; bq_cb.b1 = b1_cb; bq_cb.b2 = b2_cb; bq_cb.a1 = a1_cb; bq_cb.a2 = a2_cb;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;
        float n = noise_ma_white_tick(noise);

        double tone = 0.0;
        for (int kk = 0; kk < 4; ++kk) {
            tone += sp_osc_tick(&osc[kk], cyWave, 2.0 * M_PI * freqs[kk] / (double)sampleRate) * gains[kk];
        }
        double envTone = exp(-sec * cyTail);

        sp_onepole_mix(&lpNoise, 0.9, 0.1, n);
        double ycb = sp_biquad_tick(&bq_cb, (double)n);
        double impactEnv = exp(-sec * cyFast);
        double impact = ycb * impactEnv * cyNoiseM;
        double hpMetal = (double)n - lpNoise;
        double metalEnv = exp(-sec * cyNoiseD);
        double metal = hpMetal * metalEnv * 0.30;

        double global = 1.0 - 0.06 * tNorm;
        if (global < 0.0) global = 0.0;

        double mixed = (tone * envTone * cyToneM + impact + metal) * global;
        CYM_WRITE(0.95f * softsat_shared((float)mixed));
    }
#undef CYM_WRITE
}

static void gen_cymbal_shaker(float *out, int sampleRate, int samples,
                              const modular_params *p, int k,
                              noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── shaker (render_shaker_internal) ── */
    double cyTune   = p->gen_cym_tune[k];
    double cyFast   = p->gen_cym_env_fast[k];
    double cyTail   = p->gen_cym_env_tail[k];
    double cyToneM  = p->gen_cym_tone_m[k];
    double cyNoiseM = p->gen_cym_noise_m[k];

    float seed = noise_ma_white_tick(noise);
    double timeVar = 0.001 * (double)seed; /* ±1ms variation */

    double burstOff[4] = { 0.000, 0.004, 0.009, 0.015 };
    const double burstAmp[4] = { 1.0, 0.85, 0.7, 0.5 };
    for (int kk = 0; kk < 4; ++kk) {
        burstOff[kk] += timeVar;
    }

    double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
    {
        double fc = 8000.0 * cyTune; double Q = 0.7;
        double w0 = 2.0 * M_PI * fc / (double)sampleRate;
        double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
        double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
        double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
        b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0; a1_bp = a1/a0; a2_bp = a2/a0;
    }
    sp_biquad bq_bp = {0};
    bq_bp.b0 = b0_bp; bq_bp.b1 = b1_bp; bq_bp.b2 = b2_bp; bq_bp.a1 = a1_bp; bq_bp.a2 = a2_bp;

    double hpState = 0.0;
    double hpAlpha = 0.85;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;

        float n = noise_ma_white_tick(noise);
        double x = (double)n;

        hpState = hpState * hpAlpha + x * (1.0 - hpAlpha);
        double hp = x - hpState;

        double ybp = sp_biquad_tick(&bq_bp, x);

        double burst = 0.0;
        for (int kk = 0; kk < 4; ++kk) {
            burst += burstAmp[kk] * exp(-cyFast * fabs(sec - burstOff[kk]));
        }

        double overall = exp(-cyTail * sec);
        double mixed = (hp * cyToneM + ybp * cyNoiseM) * burst * overall;

        CYM_WRITE(0.95f * softsat_shared((float)mixed));
    }
#undef CYM_WRITE
}

static void gen_cymbal_crash(float *out, int sampleRate, int samples,
                             const modular_params *p, int k,
                             noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── crash (render_crash_internal) ── */
    const int CRASH_PARTIALS = 8;
    double cyTune   = p->gen_cym_tune[k];
    double cyFast   = p->gen_cym_env_fast[k];
    double cyTail   = p->gen_cym_env_tail[k];
    double cyToneM  = p->gen_cym_tone_m[k];
    double cyNoiseM = p->gen_cym_noise_m[k];
    double cyNoiseD = p->gen_cym_noise_d[k];
    int    cyWave   = (int)p->gen_wave[k];

    static const double baseFreq[8] = {
        2000.0, 2800.0, 3700.0, 4800.0, 6200.0, 7800.0, 9500.0, 11500.0
    };
    static const double baseGain[8] = {
        0.6, 0.9, 1.0, 1.0, 0.85, 0.65, 0.45, 0.3
    };

    sp_osc osc[8];
    double freq[8];
    double gain[8];

    for (int kk = 0; kk < CRASH_PARTIALS; ++kk) {
        float s = noise_ma_white_tick(noise);
        double det = 1.0 + 0.02 * (double)s;
        freq[kk] = (baseFreq[kk] * cyTune) * det;
        sp_osc_init(&osc[kk], 2.0 * M_PI * (double)s);
        gain[kk] = baseGain[kk] / (double)CRASH_PARTIALS;
    }

    double bellPhase = 0.0;
    float bs = noise_ma_white_tick(noise);
    double bellFreq = 2500.0 * (1.0 + 0.01 * (double)bs);

    double lpCluster = 0.0;
    double lpNoise = 0.0;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double cluster = 0.0;
        for (int kk = 0; kk < CRASH_PARTIALS; ++kk) {
            cluster += sp_osc_tick(&osc[kk], cyWave, 2.0 * M_PI * freq[kk] / (double)sampleRate) * gain[kk];
        }

        sp_onepole_mix(&lpCluster, 0.93, 0.07, cluster);
        double hpCluster = cluster - lpCluster;

        bellPhase += 2.0 * M_PI * bellFreq / (double)sampleRate;
        double bellEnv = exp(-4.0 * sec);
        double bell = sin(bellPhase) * bellEnv * 0.18;

        double envFast = exp(-cyFast * sec);
        double envTail = exp(-cyTail * sec);
        double env = envFast * 0.5 + envTail * 0.9;

        float wn = noise_ma_white_tick(noise);
        double xNoise = (double)wn;
        sp_onepole_mix(&lpNoise, 0.93, 0.07, xNoise);
        double hpNoise = xNoise - lpNoise;
        double noiseEnv = exp(-cyNoiseD * sec);

        double lfo = 1.0 + 0.06 * sin(2.0 * M_PI * 5.0 * sec);

        double y = (hpCluster * env * cyToneM + bell) * lfo + hpNoise * noiseEnv * cyNoiseM;
        y *= exp(-1.5 * tNorm);

        CYM_WRITE(0.9f * softsat_shared((float)y));
    }
#undef CYM_WRITE
}

/* A struck plate is damped modes, no noise: Mathews–Smith phasors, uniform in [lo, hi] with scatter, amplitudes a
 * Rayleigh draw under the strike envelope (uneven, so dense without rough), each a beating doublet (the shimmer).
 * rate(f) = tail·(f/1k)^tilt + fast·(f/corner)^6; peak-normalized so knobs move COLOUR, not loudness. */
_Static_assert(MODULAR_KICKEXTRA_SLOTS == MODULAR_KICK_SLOTS && MODULAR_KICKMODE_SLOTS == MODULAR_KICK_SLOTS &&
               MODULAR_KICKREVERB_SLOTS == MODULAR_KICK_SLOTS && MODULAR_KICKPHYS_SLOTS == MODULAR_KICK_SLOTS,
               "kick blocks must share one slot capacity");
_Static_assert(MODULAR_CYMMODAL_SLOTS == MODULAR_CYM_SLOTS, "cymbal blocks must share one slot capacity");
_Static_assert(MODULAR_KS_SLOTS <= MODULAR_BASS_SLOTS, "the KS voice reads gen_out_scale (bass block)");
#define CYM_MODAL_MAX_MODES 400

static double cym_uniform01(noise_ma_gen *noise) {
    double u = 0.5 * ((double)noise_ma_white_tick(noise) + 1.0);
    if (u < 1e-9) u = 1e-9;
    if (u > 1.0) u = 1.0;
    return u;
}

static void gen_cymbal_modal(float *out, int sampleRate, int samples,
                             const modular_params *p, int k, int variant,
                             noise_ma_gen *noise, int assign) {
    if (samples <= 0 || sampleRate <= 0) return;
    /* 0 modes is no plate: silence. */
    int nModes = (int)lrint(p->gen_cym_modes[k]);
    if (nModes > CYM_MODAL_MAX_MODES) nModes = CYM_MODAL_MAX_MODES;
    if (nModes < 1) return;

    double sr     = (double)sampleRate;
    double nyq    = 0.45 * sr;
    double tune   = fmax(p->gen_cym_tune[k], 0.0);
    double lo     = fmax(p->gen_cym_mode_lo[k], 0.0) * tune;
    double hi     = fmax(p->gen_cym_mode_hi[k], 0.0) * tune;
    double tail   = fmax(p->gen_cym_env_tail[k], 0.0);
    double fast   = fmax(p->gen_cym_env_fast[k], 0.0);
    double tilt   = fmax(p->gen_cym_decay_tilt[k], 0.0);
    double corner = fmax(p->gen_cym_hf_corner[k], 0.0) * tune;
    double beat   = fmax(p->gen_cym_beat[k], 0.0);
    double toneF  = fmax(p->gen_cym_tone_f[k], 0.0) * tune;
    double toneW  = fmax(p->gen_cym_tone_w[k], 0.0);
    double bodyF  = fmax(p->gen_cym_body_f[k], 0.0) * tune;
    double bodyW  = fmax(p->gen_cym_body_w[k], 0.0);
    double body   = fmax(p->gen_cym_body[k], 0.0);
    double sparkF = fmax(p->gen_cym_spark_f[k], 0.0) * tune;
    double sparkW = fmax(p->gen_cym_spark_w[k], 0.0);
    double spark  = fmax(p->gen_cym_spark[k], 0.0);
    double wash   = fmax(p->gen_cym_wash[k], 0.0);
    double attack = fmax(p->gen_cym_attack[k], 0.0);
    double level  = p->gen_cym_tone_m[k];
    double sizzM  = p->gen_cym_noise_m[k];
    double sizzD  = fmax(p->gen_cym_noise_d[k], 0.0);
    double rattle = fmax(p->gen_cym_rattle[k], 0.0);
    double rRate  = fmax(p->gen_cym_rattle_rate[k], 0.0);
    double rDecay = fmax(p->gen_cym_rattle_decay[k], 0.0);
    double driveDelay = fmax(p->gen_cym_drive_delay[k], 0.0);
    double rHold      = fmax(p->gen_cym_rattle_hold[k], 0.0);
    /* The Variant is the MOUNTING: a suspended ride/crash has nothing to rattle against, a hi-hat pair rattles, an open
     * hi-hat rattles longer and damps less. So the selector is never a dead control on the plate. */
    if (variant == 4 || variant == 5) {
        rattle = 0.0;
    } else if (variant == 1) {
        rDecay *= 5.0;
        tail   *= 0.3;
        fast   *= 0.3;
    }
    if (hi > nyq) hi = nyq;
    if (hi <= lo) hi = lo * 2.0;

    float *bank = (float *)calloc((size_t)samples, sizeof(float));
    if (!bank) return;

    for (int m = 0; m < nModes; ++m) {
        /* Fixed draw order per mode: scatter, Rayleigh, sign, rate jitter, doublet offset, doublet level, two phases. */
        double scatter = (double)noise_ma_white_tick(noise);
        double ray     = sqrt(-2.0 * log(cym_uniform01(noise)));
        double sign    = noise_ma_white_tick(noise) < 0.0f ? -1.0 : 1.0;
        double jitter  = 1.0 + 0.15 * (double)noise_ma_white_tick(noise);
        double bOff    = 0.3 + 0.7 * cym_uniform01(noise);
        double bLvl    = 0.3 + 0.7 * cym_uniform01(noise);
        double ph0     = 2.0 * M_PI * cym_uniform01(noise);
        double ph1     = 2.0 * M_PI * cym_uniform01(noise);

        double f = lo + (hi - lo) * (((double)m + 0.5 + 0.5 * scatter) / (double)nModes);
        if (f <= 20.0 || f >= nyq) continue;

        double o;
        double env;
        /* A centre or width of 0 would make 0/0: such a hump contributes nothing. */
        env = 0.0;
        if (toneW > 0.0 && toneF > 0.0) { o = log2(f / toneF);  env  = exp(-0.5 * (o / toneW) * (o / toneW)); }
        if (body > 0.0 && bodyW > 0.0 && bodyF > 0.0)    { o = log2(f / bodyF);  env += body  * exp(-0.5 * (o / bodyW) * (o / bodyW)); }
        if (spark > 0.0 && sparkW > 0.0 && sparkF > 0.0) { o = log2(f / sparkF); env += spark * exp(-0.5 * (o / sparkW) * (o / sparkW)); }
        env += wash;
        double amp = env * ray * sign;

        double rate = tail * pow(f / 1000.0, tilt);
        if (fast > 0.0) rate += fast * pow(f / corner, 6.0);
        rate *= jitter;
        if (rate < 0.05) rate = 0.05;

        int nOsc = beat > 0.0 ? 2 : 1;
        double norm = 1.0 / (1.0 + (nOsc == 2 ? bLvl : 0.0));
        for (int d = 0; d < nOsc; ++d) {
            double fd = d == 0 ? f : f * (1.0 + beat * bOff);
            if (fd >= nyq) continue;
            double a = amp * norm * (d == 0 ? 1.0 : bLvl);
            if (a == 0.0) continue;
            /* Render only until the mode falls 140 dB below its start. */
            int nLive = (int)(log(1e7) / rate * sr) + 1;
            if (nLive > samples) nLive = samples;
            double decay = exp(-rate / sr);
            double w = 2.0 * M_PI * fd / sr;
            double cr = decay * cos(w), ci = decay * sin(w);
            double ph = d == 0 ? ph0 : ph1;
            double re = a * cos(ph), im = a * sin(ph);
            for (int i = 0; i < nLive; ++i) {
                bank[i] += (float)im;
                double nr = re * cr - im * ci;
                im = re * ci + im * cr;
                re = nr;
            }
        }
    }

    /* Re-strikes the linear plate with a decaying Poisson train: out = bank + Σ a_k·bank[n−t_k]. Drawn after every mode, so
     * no rattle consumes exactly the same noise. A zero decay time would make exp(-t/0) NaN. */
    if (rattle > 0.0 && rDecay > 0.0) {
        float *hit = (float *)malloc((size_t)samples * sizeof(float));
        if (hit) {
            memcpy(hit, bank, (size_t)samples * sizeof(float));
            /* A DRIVEN plate (snare wires) is never struck: only the collisions excite it, the train measured from its own start. */
            if (driveDelay > 0.0) memset(bank, 0, (size_t)samples * sizeof(float));
            /* The RATE decays on its own long constant, so the train stays dense while it fades; each convolution is cut 120 dB
             * down (inaudible), or thousands of events would cost full length. */
            double rateTau = rHold > 0.0 ? rHold : rDecay;
            int maxEvents = rHold > 0.0 ? 65536 : 4096;
            int hitLen = samples;
            if (rHold > 0.0) {
                float hp = 0.0f;
                for (int i = 0; i < samples; ++i) { float a = fabsf(hit[i]); if (a > hp) hp = a; }
                float thr = hp * 1e-6f;
                hitLen = 1;
                for (int i = samples - 1; i > 0; --i) { if (fabsf(hit[i]) > thr) { hitLen = i + 1; break; } }
            }
            double t = 0.0;
            for (int ev = 0; ev < maxEvents; ++ev) {
                double lam = rRate * exp(-t / rateTau);
                if (lam < 5.0) break;
                if (rHold > 0.0 && t / rDecay > 9.2) break;   /* amplitude below ~-80 dB */
                t += -log(cym_uniform01(noise)) / lam;   /* exponential gap */
                double atT = (t + driveDelay) * sr;
                if (!(atT >= 0.0) || atT >= (double)samples) break; /* also stops on NaN */
                int at = (int)atT;
                double a = rattle * exp(-t / rDecay) * (0.2 + 0.8 * cym_uniform01(noise));
                if (noise_ma_white_tick(noise) < 0.0f) a = -a;
                int end = at + hitLen < samples ? at + hitLen : samples;
                for (int i = at; i < end; ++i) bank[i] += (float)(a * (double)hit[i - at]);
            }
            free(hit);
        }
    }

    /* Stick-contact onset + end fade, then peak-normalize. */
    int nAtk = (int)(attack * 6.0 * sr);
    if (nAtk > samples) nAtk = samples;
    for (int i = 0; i < nAtk; ++i) bank[i] *= (float)(1.0 - exp(-((double)i / sr) / attack));
    int nFade = samples / 10;
    for (int i = 0; i < nFade; ++i) {
        double x = (double)i / (double)nFade;             /* 0 → 1 across the fade */
        bank[samples - 1 - i] *= (float)(0.5 - 0.5 * cos(M_PI * x));
    }
    float peak = 0.0f;
    for (int i = 0; i < samples; ++i) {
        float a = fabsf(bank[i]);
        if (a > peak) peak = a;
    }
    double gain = peak > 0.0f ? 0.9 * level / (double)peak : 0.0;

    /* Gated by a follower on the plate's own amplitude: rivets buzz only while the plate moves. */
    double hpA = exp(-2.0 * M_PI * 3000.0 / sr);
    double lpA = exp(-2.0 * M_PI * 9000.0 / sr);
    double follow = exp(-1.0 / (0.01 * sr));
    double lpHP = 0.0, lpLP = 0.0, envF = 0.0;
    for (int i = 0; i < samples; ++i) {
        double y = (double)bank[i] * gain;
        if (sizzM > 0.0) {
            double x = (double)noise_ma_white_tick(noise);
            lpHP = hpA * lpHP + (1.0 - hpA) * x;
            double bp = x - lpHP;
            lpLP = lpA * lpLP + (1.0 - lpA) * bp;
            double a = fabs(y);
            envF = a > envF ? a : follow * envF + (1.0 - follow) * a;
            y += lpLP * envF * sizzM * exp(-sizzD * (double)i / sr);
        }
        if (assign) out[i] = (float)y; else out[i] += (float)y;
    }
    free(bank);
}

static void modular_gen_slot_cymbal(float *out, int sampleRate, int samples,
                                    const modular_params *p, int k,
                                    double voice_freq, int assign) {
    (void)voice_freq; /* cymbal partials are absolute Hz × cym_tune */
    int variant = (int)lrint(p->gen_cym_variant[k]);
    if (variant < 0 || variant > 5) variant = 0;

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    /* Hi-hat and ride ARE the plate; open hi-hat and crash run it once Plate Modes ≥ 1; cowbell and shaker are not plates. */
    int plate_on = p->gen_cym_modes[k] >= 1.0;
    if (variant == 0 || variant == 4 || (plate_on && (variant == 1 || variant == 5))) {
        gen_cymbal_modal(out, sampleRate, samples, p, k, variant, &noise, assign);
        return;
    }

    if (variant == 1) {
        gen_cymbal_open(out, sampleRate, samples, p, k, &noise, assign);
    } else if (variant == 2) {
        gen_cymbal_cowbell(out, sampleRate, samples, p, k, &noise, assign);
    } else if (variant == 3) {
        gen_cymbal_shaker(out, sampleRate, samples, p, k, &noise, assign);
    } else {
        gen_cymbal_crash(out, sampleRate, samples, p, k, &noise, assign);
    }
}

/* The source-10 FM voice: the variant fixes the preset STRUCTURE (op amplitudes, ADSR, carriers, routing) and the knobs
 * are read from the block. fm_render zeroes its own output, so it renders into scratch then assigns or adds. */
static void modular_gen_slot_fm(float *out, int sampleRate, int samples,
                                const modular_params *p, int k,
                                double voice_freq, int assign) {
    (void)voice_freq; /* FM base freq is an absolute knob (gen_fm_base), not the
                       * derived modular voice freq. */
    int variant = (int)lrint(p->gen_fm_variant[k]);

    fm_preset pr;
    memset(&pr, 0, sizeof(pr));

    /* Structure only; every knob is read from the block. */

    switch (variant) {
    case 1: /* PRESET_FM_BELL */
        pr.num_ops = 2;
        /* op0 carrier */
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.7f;
        pr.ops[0].attack_sec = 0.001f; pr.ops[0].sustain_level = 0.0f;
        pr.ops[0].release_sec = 0.3f; pr.ops[0].is_carrier = 1;
        /* op1 modulator (inharmonic) */
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 0.6f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.0f;
        pr.ops[1].release_sec = 0.2f; pr.ops[1].is_carrier = 0;
        break;
    case 2: /* PRESET_FM_LEAD */
        pr.num_ops = 3;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.8f;
        pr.ops[0].attack_sec = 0.003f; pr.ops[0].sustain_level = 0.5f;
        pr.ops[0].release_sec = 0.15f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 0.9f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.3f;
        pr.ops[1].release_sec = 0.1f; pr.ops[1].is_carrier = 0;
        pr.ops[2].freq_offset = 0.0f; pr.ops[2].amplitude = 0.5f;
        pr.ops[2].attack_sec = 0.001f; pr.ops[2].sustain_level = 0.1f;
        pr.ops[2].release_sec = 0.05f; pr.ops[2].is_carrier = 0;
        break;
    case 3: /* PRESET_FM_EPIANO */
        pr.num_ops = 3;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.7f;
        pr.ops[0].attack_sec = 0.002f; pr.ops[0].sustain_level = 0.3f;
        pr.ops[0].release_sec = 0.4f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 7.0f; pr.ops[1].amplitude = 0.6f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.1f;
        pr.ops[1].release_sec = 0.2f; pr.ops[1].is_carrier = 0;
        pr.ops[2].freq_offset = 0.5f; pr.ops[2].amplitude = 0.30f;
        pr.ops[2].attack_sec = 0.001f; pr.ops[2].sustain_level = 0.05f;
        pr.ops[2].release_sec = 0.3f; pr.ops[2].is_carrier = 1; /* independent carrier */
        break;
    case 4: /* PRESET_FM_PLUCK */
        pr.num_ops = 2;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.85f;
        pr.ops[0].attack_sec = 0.001f; pr.ops[0].sustain_level = 0.0f;
        pr.ops[0].release_sec = 0.05f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 1.0f;
        pr.ops[1].attack_sec = 0.0005f; pr.ops[1].sustain_level = 0.0f;
        pr.ops[1].release_sec = 0.02f; pr.ops[1].is_carrier = 0;
        break;
    default: /* PRESET_FM_BASS (variant 0) */
        pr.num_ops = 2;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.9f;
        pr.ops[0].attack_sec = 0.005f; pr.ops[0].sustain_level = 0.6f;
        pr.ops[0].release_sec = 0.2f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 0.8f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.2f;
        pr.ops[1].release_sec = 0.1f; pr.ops[1].is_carrier = 0;
        break;
    }

    /* Exact doubles: GCC -O2 elided some float casts and kept others, so a narrowed result depended on the optimizer. */
    pr.base_freq        = p->gen_fm_base[k];
    pr.pitch_env_amount = p->gen_fm_pe_amt[k];
    pr.pitch_env_decay  = p->gen_fm_pe_decay[k];
    pr.wave             = (int)p->gen_wave[k];

    pr.ops[0].freq_ratio = p->gen_fm_r1[k];
    pr.ops[1].freq_ratio = p->gen_fm_r2[k];
    pr.ops[2].freq_ratio = p->gen_fm_r3[k];
    pr.ops[3].freq_ratio = p->gen_fm_r4[k];

    pr.ops[0].decay_sec = p->gen_fm_dec1[k];
    pr.ops[1].decay_sec = p->gen_fm_dec2[k];
    pr.ops[2].decay_sec = p->gen_fm_dec3[k];
    pr.ops[3].decay_sec = p->gen_fm_dec4[k];

    /* The routing is fixed by the variant, so the exact edge is set. */
    double depth[4];
    depth[0] = p->gen_fm_d1[k];
    depth[1] = p->gen_fm_d2[k];
    depth[2] = p->gen_fm_d3[k];
    depth[3] = p->gen_fm_d4[k];
    /* op1 → op0 (every preset routes the first modulator into the carrier). */
    pr.mod_matrix[1][0] = depth[1];
    if (variant == 2) {
        /* lead: op2 → op1 */
        pr.mod_matrix[2][1] = depth[2];
    }
    /* bell/bass/pluck: op1→op0 only. epiano: op1→op0 only (op2 independent). */

    /* fm_render zeroes its buffer, and a copy keeps a -0.0 sample exact. */
    float *scratch = (float *)malloc((size_t)samples * sizeof(float));
    if (!scratch) return;
    fm_render(&pr, scratch, sampleRate, samples);
    if (assign) {
        for (int i = 0; i < samples; i++) out[i] = scratch[i];
    } else {
        for (int i = 0; i < samples; i++) out[i] += scratch[i];
    }
    free(scratch);
}

void modular_gen_bank_render(float *out, int sampleRate, int samples,
                             const modular_params *p, double voice_freq,
                             unsigned int seed, int osc_wrote) {
    int active = 0;
    for (int k = 0; k < MODULAR_GEN_SLOTS; k++) {
        if ((int)lrint(p->gen_source[k]) > 0) { active = 1; break; }
    }
    if (!active) return;

    /* With no osc write, the first producing slot ASSIGNS so a -0.0 sample is not flipped; later slots add in slot order. */
    int wrote = osc_wrote;

    /* The same formula as the OSC stage; inactive leaves the plain path untouched. */
    int   gb_lfo_on     = p->lfo_enabled >= 0.5f;
    double gb_lfo_rate   = p->lfo_rate;
    double gb_lfo_depth  = p->lfo_depth;
    int   gb_lfo_target = (int)lrint(p->lfo_target);
    double gb_lfo_delay  = p->lfo_delay;
    int   gb_vib_on     = gb_lfo_on && gb_lfo_target == 1
                          && gb_lfo_depth > 0.0f && gb_lfo_rate > 0.0f;

    /* Rendered ONCE so slots sharing it consume the same draws in a fixed order: prelude, then per-sample groups. */
    int draws = (int)lrint(p->noise_draws);
    if (draws < 1) draws = 1;
    int prelude = (int)lrint(p->noise_prelude);
    if (prelude < 0) prelude = 0;
    float *bus = NULL;
    int need_bus = 0;
    for (int k = 0; k < MODULAR_GEN_SLOTS; k++) {
        int src = (int)lrint(p->gen_source[k]);
        int pm = (int)lrint(p->gen_phase_mode[k]);
        if (src == 2 || (src > 0 && pm == 2)) { need_bus = 1; break; }
    }
    if (need_bus) {
        size_t total = (size_t)prelude + (size_t)samples * (size_t)draws;
        bus = (float *)malloc(total * sizeof(float));
        if (!bus) return;
        noise_ma_white_fill(bus, (int)total, (int)seed);
    }

    for (int k = 0; k < MODULAR_GEN_SLOTS; k++) {
        int src = (int)lrint(p->gen_source[k]);
        if (src <= 0) continue;

        /* With an osc write this is always 0, so the accumulate path is unchanged. */
        int assign = !wrote;

        double gain = (double)p->gen_gain[k];
        double fr = (double)p->gen_env_fast_rate[k];
        double tr = (double)p->gen_env_tail_rate[k];
        double fm = (double)p->gen_env_fast_mix[k];
        double tm = (double)p->gen_env_tail_mix[k];
        gen_slot_filter filt;
        gen_slot_filter_init(&filt, p, k, sampleRate);

        /* 0 is silent, like any other gain. */

        if (src == 1) { /* wavetable oscillator */
            double f = (double)p->gen_freq[k];
            if ((int)lrint(p->gen_freq_mode[k]) == 0) f *= voice_freq;
            if (f < 0.0) f = 0.0;
            /* Above Nyquist pushes phase_inc past the table length and breaks wt_osc_tick's single-step wrap; ratio mode can reach
             * absurd values. A no-op in band. */
            double nyq = 0.5 * (double)sampleRate;
            if (f > nyq * 0.99) f = nyq * 0.99;
            const wavetable_t *wt = modular_shared_table_for((int)lrint(p->gen_wave[k]));
            wt_osc_t osc;
            wt_osc_init(&osc, wt, f, sampleRate);
            int pm = (int)lrint(p->gen_phase_mode[k]);
            if (pm == 1) {
                wt_osc_set_phase(&osc, (double)p->gen_phase[k] / (2.0 * M_PI));
            } else if (pm == 2 && bus) {
                int pi = (int)lrint(p->gen_phase[k]);
                if (pi >= 0 && pi < prelude) {
                    wt_osc_set_phase(&osc, (double)bus[pi]); /* draw in [0,1) (or slightly negative post-wrap; wt wraps) */
                }
            }
            /* Phase-8E unison: gated on nv>=2 so nv==1 is the exact original loop. */
            int unison_nv = (int)lrint(p->unison_voices);
            if (unison_nv < 1) unison_nv = 1;
            if (unison_nv > 7) unison_nv = 7;
            if (unison_nv >= 2) {
                double udet = (double)p->unison_detune;
                double umix = (double)p->unison_mix;
                double udrift_rate  = (double)p->unison_drift_rate;
                double udrift_depth = (double)p->unison_drift_depth;
                int ns = unison_nv - 1;
                wt_osc_t side[6];
                double base_cents_gb[6];
                for (int v = 0; v < ns; v++) {
                    double spread = -1.0 + 2.0 * (double)v / (double)(ns > 1 ? ns - 1 : 1);
                    base_cents_gb[v] = udet * spread;
                    double fv = f * pow(2.0, base_cents_gb[v] / 1200.0);
                    if (fv > nyq * 0.99) fv = nyq * 0.99;
                    wt_osc_init(&side[v], wt, fv, sampleRate);
                    wt_osc_set_phase(&side[v], (double)(v + 1) / (double)unison_nv);
                }
                double norm = 1.0 / sqrt((double)unison_nv);
                int drift_on_gb = (udrift_rate > 0.0 && udrift_depth > 0.0);
                double drift_rate_v_gb[6], drift_phase_v_gb[6];
                if (drift_on_gb) {
                    for (int v = 0; v < ns; v++) {
                        drift_rate_v_gb[v]  = udrift_rate * (1.0 + 0.13 * (double)v);
                        drift_phase_v_gb[v] = 2.0 * M_PI * (double)(v + 1) / (double)unison_nv;
                    }
                }
                /* Vibrato and drift are slow, so their sin/pow are evaluated every 32 samples and held: a per-sample evaluation
                 * dominated a 7-voice additive patch (and cost seconds on emscripten's software libm) for an inaudible difference. */
                const int GB_CTRL_STRIDE = 32;
                double vib_mult = 1.0;
                /* fm·decF^i + tm·decT^i equals the per-sample exp() form; two exp() at setup replace 2·samples of them. */
                double gb_decF = exp(-fr / (double)sampleRate);
                double gb_decT = exp(-tr / (double)sampleRate);
                double gb_ef = fm;
                double gb_et = tm;
                for (int i = 0; i < samples; i++) {
                    double t = (double)i / (double)sampleRate;
                    int ctrl = ((i % GB_CTRL_STRIDE) == 0);
                    /* Centre and every side voice, as in the OSC stage's unison. */
                    if (gb_vib_on && ctrl) {
                        double ramp = (gb_lfo_delay > 0.0f)
                                      ? fmin(t / (double)gb_lfo_delay, 1.0) : 1.0;
                        double semis = (double)gb_lfo_depth * ramp
                                       * sin(2.0 * M_PI * (double)gb_lfo_rate * t);
                        vib_mult = pow(2.0, semis / 12.0);
                        /* Center voice: set freq at control rate with vibrato. */
                        double fc = f * vib_mult;
                        if (fc > nyq * 0.99) fc = nyq * 0.99;
                        wt_osc_set_freq(&osc, fc, sampleRate);
                    }
                    if (drift_on_gb && ctrl) {
                        for (int v = 0; v < ns; v++) {
                            double drift_cents = udrift_depth
                                * sin(2.0 * M_PI * drift_rate_v_gb[v] * t + drift_phase_v_gb[v]);
                            double total_cents = base_cents_gb[v] + drift_cents;
                            double fv = f * pow(2.0, total_cents / 1200.0) * vib_mult;
                            if (fv > nyq * 0.99) fv = nyq * 0.99;
                            wt_osc_set_freq(&side[v], fv, sampleRate);
                        }
                    } else if (gb_vib_on && ctrl) {
                        /* No drift but vibrato active: update side voices with vibrato. */
                        for (int v = 0; v < ns; v++) {
                            double fv = f * pow(2.0, base_cents_gb[v] / 1200.0) * vib_mult;
                            if (fv > nyq * 0.99) fv = nyq * 0.99;
                            wt_osc_set_freq(&side[v], fv, sampleRate);
                        }
                    }
                    double center = (double)wt_osc_tick(&osc);
                    double ensemble = center;
                    for (int v = 0; v < ns; v++) ensemble += (double)wt_osc_tick(&side[v]);
                    ensemble *= norm;
                    double s = (1.0 - umix) * center + umix * ensemble;
                    s = gen_slot_filter_tick(&filt, s);
                    double env = gb_ef + gb_et;
                    gb_ef *= gb_decF;
                    gb_et *= gb_decT;
                    if (assign) out[i] = (float)(s * env * gain);
                    else        out[i] += (float)(s * env * gain);
                }
            } else if (gb_vib_on) {
                /* Kept EXACT per sample: the control-rate approximation drifts audibly on strong vibrato. Only the envelope is optimised,
                 * which is float32-transparent. */
                double gb_decF = exp(-fr / (double)sampleRate);
                double gb_decT = exp(-tr / (double)sampleRate);
                double gb_ef = fm, gb_et = tm;
                for (int i = 0; i < samples; i++) {
                    double t = (double)i / (double)sampleRate;
                    double ramp = (gb_lfo_delay > 0.0f)
                                  ? fmin(t / (double)gb_lfo_delay, 1.0) : 1.0;
                    double semis = (double)gb_lfo_depth * ramp
                                   * sin(2.0 * M_PI * (double)gb_lfo_rate * t);
                    double mult = pow(2.0, semis / 12.0);
                    wt_osc_set_freq(&osc, f * mult, sampleRate);
                    double s = (double)wt_osc_tick(&osc);
                    s = gen_slot_filter_tick(&filt, s);
                    double env = gb_ef + gb_et;
                    gb_ef *= gb_decF;
                    gb_et *= gb_decT;
                    if (assign) out[i] = (float)(s * env * gain);
                    else        out[i] += (float)(s * env * gain);
                }
            } else {
                /* The default single-voice case: the incremental envelope's ~1e-11 drift rounds away at the float32 cast, so it is
                 * byte-identical and much cheaper. */
                double gb_decF = exp(-fr / (double)sampleRate);
                double gb_decT = exp(-tr / (double)sampleRate);
                double gb_ef = fm, gb_et = tm;
                for (int i = 0; i < samples; i++) {
                    double s = (double)wt_osc_tick(&osc);
                    s = gen_slot_filter_tick(&filt, s);
                    double env = gb_ef + gb_et;
                    gb_ef *= gb_decF;
                    gb_et *= gb_decT;
                    if (assign) out[i] = (float)(s * env * gain);
                    else        out[i] += (float)(s * env * gain);
                }
            }
            wrote = 1;
        } else if (src == 2 && bus) { /* noise tap */
            int off = (int)lrint(p->gen_noise_offset[k]);
            if (off < 0) off = 0;
            if (off >= draws) off = draws - 1;
            /* The same incremental envelope as the wavetable branch. */
            double gb_decF = exp(-fr / (double)sampleRate);
            double gb_decT = exp(-tr / (double)sampleRate);
            double gb_ef = fm, gb_et = tm;
            for (int i = 0; i < samples; i++) {
                double s = (double)bus[prelude + i * draws + off];
                s = gen_slot_filter_tick(&filt, s);
                double env = gb_ef + gb_et;
                gb_ef *= gb_decF;
                gb_et *= gb_decT;
                if (assign) out[i] = (float)(s * env * gain);
                else        out[i] += (float)(s * env * gain);
            }
            wrote = 1;
        } else if (src == 3) { /* Karplus-Strong string voice (bass-guitar transplant) */
            if (k < MODULAR_KS_SLOTS) modular_gen_slot_ks(out, sampleRate, samples, p, k, voice_freq, assign, gain);
            wrote = 1;
        } else if (src == 4) { /* analytic dual-osc voice (sub-bass transplant) */
            if (k < MODULAR_BASS_SLOTS) modular_gen_slot_analytic(out, sampleRate, samples, p, k, voice_freq, assign, gain);
            wrote = 1;
        } else if (src == 5) { /* harmonic-bank kick voice (kick-family transplant) */
            if (k < MODULAR_KICK_SLOTS) modular_gen_slot_kick(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 6) { /* 808-style tom voice (tom-family transplant) */
            if (k < MODULAR_TOM_SLOTS) modular_gen_slot_tom(out, sampleRate, samples, p, k, voice_freq, assign, gain);
            wrote = 1;
        } else if (src == 7) { /* snare-ish voice (snare/rimshot/sidestick transplant) */
            if (k < MODULAR_SNARE_SLOTS) modular_gen_slot_snare(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 8) { /* clap voice (clap-family transplant) */
            if (k < MODULAR_SNARE_SLOTS) modular_gen_slot_clap(out, sampleRate, samples, p, k, voice_freq, assign, gain);
            wrote = 1;
        } else if (src == 9) { /* metallic cymbal voice (hihat/open/cowbell/shaker/ride/crash) */
            if (k < MODULAR_CYM_SLOTS) modular_gen_slot_cymbal(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 10) { /* 4-op FM-family voice (bass/bell/lead/epiano/pluck) */
            if (k < MODULAR_FM_SLOTS) modular_gen_slot_fm(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        }
    }
    free(bus);
}

/* Csound Appendix D tables [voice][vowel][formant]: soprano/alto/tenor/bass × a/e/i/o/u; amplitudes in dB re F1,
 * bandwidths at -6 dB (Q = Fc/BW). */
static const float form_hz[4][5][5] = {
    {{800,1150,2900,3900,4950},{350,2000,2800,3600,4950},{270,2140,2950,3900,4950},
     {450,800,2830,3800,4950},{325,700,2700,3800,4950}},   /* soprano */
    {{800,1150,2800,3500,4950},{400,1600,2700,3300,4950},{350,1700,2700,3700,4950},
     {450,800,2830,3500,4950},{325,700,2530,3500,4950}},   /* alto */
    {{650,1080,2650,2900,3250},{400,1700,2600,3200,3580},{290,1870,2800,3250,3540},
     {400,800,2600,2800,3000},{350,600,2700,2900,3300}},   /* tenor */
    {{600,1040,2250,2450,2750},{400,1620,2400,2800,3100},{250,1750,2600,3050,3340},
     {400,750,2400,2600,2900},{350,600,2400,2675,2950}},   /* bass */
};
static const float form_db[4][5][5] = {
    {{0,-6,-32,-20,-50},{0,-20,-15,-40,-56},{0,-12,-26,-26,-44},{0,-11,-22,-22,-50},{0,-16,-35,-40,-60}},
    {{0,-4,-20,-36,-60},{0,-24,-30,-35,-60},{0,-20,-30,-36,-60},{0,-9,-16,-28,-55},{0,-12,-30,-40,-64}},
    {{0,-6,-7,-8,-22},{0,-14,-12,-14,-20},{0,-15,-18,-20,-30},{0,-10,-12,-12,-26},{0,-20,-17,-14,-26}},
    {{0,-7,-9,-9,-20},{0,-12,-9,-12,-18},{0,-30,-16,-22,-28},{0,-11,-21,-20,-40},{0,-20,-32,-28,-36}},
};
static const float form_bw[4][5][5] = {
    {{80,90,120,130,140},{60,100,120,150,200},{60,90,100,120,120},{40,80,100,120,120},{50,60,170,180,200}},
    {{80,90,120,130,140},{60,80,120,150,200},{50,100,120,150,200},{70,80,100,130,135},{50,60,170,180,200}},
    {{80,90,120,130,140},{70,80,100,120,120},{40,90,100,120,120},{70,80,100,130,135},{40,60,100,120,120}},
    {{60,70,110,120,130},{40,80,100,120,120},{60,90,100,120,120},{40,80,100,120,120},{40,80,100,120,120}},
};
/* Singer's-formant center per voice type (Sundberg). */
static const float form_sing_hz[4] = {3092.0f, 3000.0f, 2750.0f, 2400.0f};

/* init ≠ 0 starts fresh; 0 swaps coefficients keeping state, the block-rate morph the filter env uses. */
static void formant_bank_tune(mod_biquad *bank, float *g, int voice_type,
                              double v, double shift, int sampleRate, int init) {
    if (v < 0.0f) v = 0.0f; else if (v > 4.0f) v = 4.0f;
    int   i0 = (int)v; if (i0 > 3) i0 = 3;
    double fr = v - (double)i0;
    for (int k = 0; k < 5; k++) {
        double hz = ((1.0f - fr) * form_hz[voice_type][i0][k] + fr * form_hz[voice_type][i0 + 1][k])
                  * (double)shift;
        double db = (1.0f - fr) * form_db[voice_type][i0][k] + fr * form_db[voice_type][i0 + 1][k];
        double bw = ((1.0f - fr) * form_bw[voice_type][i0][k] + fr * form_bw[voice_type][i0 + 1][k])
                  * (double)shift;
        double nyq = 0.49 * (double)sampleRate;
        if (hz > nyq) hz = nyq;
        double q = hz / bw; if (q < 0.5) q = 0.5;
        g[k] = (float)pow(10.0, db / 20.0);
        if (init) mod_biquad_set(&bank[k], 2 /* band-pass */, hz, q, sampleRate);
        else      mod_biquad_set_coeffs(&bank[k], 2, hz, q, sampleRate);
    }
}

void modular_formant_process(float *out, int sampleRate, int samples,
                             int voice_type, double vowel, double mix,
                             double shift, double sing,
                             double morph_rate, double morph_to, double dry) {
    if (mix <= 0.0f || samples <= 0) return;
    if (voice_type < 0) voice_type = 0; else if (voice_type > 3) voice_type = 3;
    if (shift < 0.01) shift = 0.01; /* structural: a non-positive formant shift has no meaning */
    mod_biquad bank[5];
    float g[5];
    formant_bank_tune(bank, g, voice_type, vowel, shift, sampleRate, 1);
    mod_biquad singbq;
    if (sing > 0.0f) {
        double shz = (double)form_sing_hz[voice_type] * (double)shift;
        double nyq = 0.49 * (double)sampleRate;
        if (shz > nyq) shz = nyq;
        mod_biquad_set(&singbq, 2, shz, 10.0, sampleRate);
    }
    int morph_on = morph_rate > 0.0f;
    /* The harmonics should move THROUGH the formants, so the dry drops as mix grows (exact bypass at 0); dry scales the
     * whole blend (0 = wet only). */
    double dryAmt = (1.0 - 0.85 * fmin((double)mix, 1.0)) * (double)dry;
    for (int i = 0; i < samples; i++) {
        if (morph_on && (i & 63) == 0) {
            double t = (double)i / (double)sampleRate;
            double m = 0.5 - 0.5 * cos(2.0 * M_PI * (double)morph_rate * t);
            float v = (float)((1.0 - m) * (double)vowel + m * (double)morph_to);
            formant_bank_tune(bank, g, voice_type, v, shift, sampleRate, 0);
        }
        double x = (double)out[i];
        double wet = 0.0;
        for (int k = 0; k < 5; k++)
            wet += (double)mod_biquad_tick_f(&bank[k], (float)x) * (double)g[k];
        if (sing > 0.0f)
            wet += (double)mod_biquad_tick_f(&singbq, (float)x) * (double)sing * 0.7;
        out[i] = (float)(x * dryAmt + wet * (double)mix);
    }
}
