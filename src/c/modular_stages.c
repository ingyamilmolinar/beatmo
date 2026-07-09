#include <math.h>
#include <stdlib.h>
#include <string.h>
#include "modular_stages.h"
#include "wavetable.h"
#include "noise.h"
#include "synth_post.h"          /* osc_wave_shared (analytic source==4) + kp_get */
#include "synth_dsp_primitives.h" /* sp_onepole / sp_osc / sp_pitch_glide / sp_reverb3 */
#include "fmsynth.h"             /* fm_preset + fm_render (source==10 FM-family voice) */

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

/* Re-declared from modular.c (kept static there); the slot oscillator MUST
 * go through the exact same wavetable objects + tick loop so a slot sine is
 * byte-identical to the legacy osc stage (TestGenSlotSineMatchesLegacyOscExactly). */
const wavetable_t *modular_shared_table_for(int wave);

/* ── RBJ biquad — moved here from modular.c (de-static'd, bodies IDENTICAL to
 * the originals so the modular FILTER stage keeps its exact bytes). ── */

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
    f->type = (int)lrintf(p->gen_filt_type[k]);
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

/* modular_gen_slot_analytic: the source==4 analytic dual-osc voice — the
 * render_sub_bass_internal per-sample loop transplanted VERBATIM (drums.c
 * ~1986-2022), with every hardcoded literal replaced by a per-slot param read
 * through kp_get so the NaN sentinel reproduces the exact legacy double literal
 * at default (byte-identity, plan Task-1 / spec §2). Float-op order, double
 * precision, the `if (phase > 2π) phase -= 2π` normalization form, and the
 * `(float)sat * out_scale` cast positions all match the source exactly.
 *
 * Base freq: gen_freq[k] × voice_freq in ratio mode (gen_freq_mode==0), else
 * absolute gen_freq[k]. Both phase accumulators start at gen_phase (radians,
 * kp_get fallback M_PI*0.5 — float32 cannot carry the exact double π/2, so the
 * binding passes NaN and the literal fallback supplies the precise value).
 *
 * assign: when 1 the first sample-write ASSIGNS (out[i] = v) — matching the
 * legacy renderer's direct `out[i] = ...` — instead of accumulating, so a -0.0
 * sample is not flipped to +0.0 by a preceding zeroed buffer. */
static void modular_gen_slot_analytic(float *out, int sampleRate, int samples,
                                      const modular_params *p, int k,
                                      double voice_freq, int assign) {
    double bHarm    = kp_get(p->gen_harm_mix[k], 0.08);
    double bPitchE  = kp_get(p->gen_pitch_env_amt[k], 0.15);
    double bPitchER = kp_get(p->gen_pitch_env_rate[k], 40.0);
    double bAttack  = kp_get(p->gen_atk_amt[k], 0.2);
    double bAttackR = kp_get(p->gen_atk_rate[k], 60.0);
    double bEnvRate = kp_get(p->gen_env_fast_rate[k], 2.0);
    double satK     = kp_get(p->gen_sat_k[k], 1.2);
    int    bWave    = (int)kp_get(p->gen_wave[k], 0.0);

    double baseFreq = (double)p->gen_freq[k];
    if ((int)lrintf(p->gen_freq_mode[k]) == 0) baseFreq *= voice_freq;

    double phase  = kp_get(p->gen_phase[k], M_PI * 0.5);
    double phase2 = phase;
    double harmonicMix = bHarm;

    /* out scale read as a FLOAT (legacy `(float)sat * 0.95f` — float×float). */
    float outScale = (float)kp_get(p->gen_out_scale[k], 0.95);

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

        if (assign) out[i] = (float)sat * outScale;
        else        out[i] += (float)sat * outScale;
    }
}

/* modular_gen_slot_ks: the source==3 Karplus-Strong plucked-string voice — the
 * render_bass_guitar_internal per-sample loop transplanted VERBATIM (drums.c
 * ~1865-1968). Every hardcoded literal is replaced by a per-slot param read
 * through kp_get so the NaN sentinel reproduces the exact legacy double literal
 * at default (byte-identity, plan Task-2 / spec §2).
 *
 * KS owns a PRIVATE delay line + its own noise stream — it does NOT consume the
 * rectangular noise bus (the legacy draw order is control-flow-dependent: a
 * detune-seed draw, then delayLen pluck-LP draws, then per-sample a stretch-
 * decision draw plus a conditional attack-window draw). The stream is the
 * miniaudio-compatible noise_ma white stream seeded 0 (→4321), identical to the
 * legacy ma_noise seed-0 stream consumed by render_bass_guitar_internal
 * (TestNoiseMaParity locks the bit-equality).
 *
 * Base freq (the KS fundamental): gen_freq[k] × voice_freq in ratio mode
 * (gen_freq_mode==0), else absolute gen_freq[k] — the binding passes ratio 1 so
 * the fundamental flows through voice_freq (= the resolved bass_fund Hz).
 *
 * assign: when 1 the first sample-write ASSIGNS (out[i] = v); but KS always
 * writes via softsat into out[i] directly (the legacy renderer does
 * `out[i] = softsat(...)`), so assign and accumulate paths differ only in the
 * += vs =. Matching the legacy direct-assign avoids flipping a -0.0 sample. */
static void modular_gen_slot_ks(float *out, int sampleRate, int samples,
                                const modular_params *p, int k,
                                double voice_freq, int assign) {
    /* Family knobs — NaN keeps the original literals (bit-identical). */
    double bSustain = kp_get(p->gen_ks_sustain[k], 0.996);
    double bPluck   = kp_get(p->gen_ks_pluck[k], 0.35);
    double bBlow    = kp_get(p->gen_ks_blow[k], 0.0); /* 0 = plucked (legacy); >0 = blown tube / wind */
    double bAttack  = kp_get(p->gen_atk_amt[k], 0.25);
    double bAttackR = kp_get(p->gen_atk_rate[k], 400.0);
    double bEnvRate = kp_get(p->gen_env_fast_rate[k], 1.8);
    /* KS-specific fallback 0.85: legacy bass-guitar ends with
     * softsat((float)mixed * 0.85f) — distinct from the analytic voice's
     * 0.95 trailing scale. Same shared field, per-source fallback. */
    float  outScale = (float)kp_get(p->gen_out_scale[k], 0.85);

    /* Karplus-Strong delay buffer size for the base frequency. */
    double baseFreq = (double)p->gen_freq[k];
    if ((int)lrintf(p->gen_freq_mode[k]) == 0) baseFreq *= voice_freq;
    int delayLen = (int)((double)sampleRate / baseFreq + 0.5);
    if (delayLen < 2) delayLen = 2;
    if (delayLen > 4096) delayLen = 4096;

    /* Local buffer (legacy uses a 4096 float stack buffer, zeroed in full). */
    float delayBuf[4096];
    for (int i = 0; i < 4096; i++) delayBuf[i] = 0.0f;

    /* Per-hit variation seed draw (legacy: ma_noise seed 0 ⇒ 4321). The first
     * draw is the detune seed (consumed but unused for audio); reproduce it so
     * the subsequent stream offset matches the legacy renderer exactly. */
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

    /* ── Blown-flute mode (source==3 with gen_ks_blow>0) ──────────────────────
     * STK / Perry Cook jet-driven flute (single-bore simplification). A real
     * flute is a TUBE: breath pressure drives a tuned bore delay through a CUBIC
     * jet nonlinearity (x^3 - x, clipped). The cubic's saturation SELF-LIMITS the
     * oscillation to a stable limit cycle — this is what bounds the amplitude
     * (the earlier linear comb built up without bound). A one-pole lowpass in the
     * reflection path is the open-tube radiation loss (sets brightness); a DC
     * blocker removes the asymmetric jet DC. Continuous breath noise = the air
     * turbulence, so the tone is pitched, breathy and never exactly repeats.
     *   gen_ks_blow    = breath pressure (drive / loudness),
     *   gen_ks_sustain = bore reflection gain (resonance vs damping),
     *   gen_ks_pluck   = reflection-filter lowpass alpha (air color).
     * The recipe amp ADSR shapes the note. Gated on bBlow>0 so every plucked KS
     * instrument is byte-identical (bBlow defaults to 0). */
    if (bBlow > 0.0) {
        /* Resonant bore = a tuned delay with a lowpass in the feedback (the comb
         * that pitches the breath noise). High feedback gives a clear pitch but a
         * LINEAR comb builds up without bound; a tanh saturator on the recirculated
         * signal turns it into a stable, self-limiting LIMIT CYCLE (loop gain falls
         * as amplitude grows) — the bounded "jet" behaviour, without a second delay
         * line. Continuous breath noise keeps it alive and breathy. */
        /* Regeneration: a blown tube self-oscillates only when the loop gain
         * exceeds unity at small signal. gen_ks_sustain is in [0,1] (and >1 would
         * blow up the linear pluck path), so scale it up by 1.05 HERE — the tanh
         * keeps the resulting limit cycle bounded. ks_sustain ~0.96-0.99 → loop
         * gain ~1.01-1.04 → a sustained, stable tone; lower → it decays (softer). */
        double fb      = decayFactor * 1.05; /* gen_ks_sustain → blown loop gain (>1 = self-oscillating) */
        double lpAlpha = bPluck;             /* reflection lowpass alpha (tube air color) */
        if (lpAlpha <= 0.0) lpAlpha = 0.5;
        double lpState = 0.0;         /* reflection lowpass state */
        /* Breath = SMOOTH airy turbulence, NOT white hiss. A raw white-noise drive
         * makes the tone harsh/"industrial"; a 2-pole lowpass turns it into soft air
         * so the tube sings smoothly. (This LP is on the excitation, not the bore
         * round-trip, so it does not affect pitch.) */
        const double breathAlpha = 0.10;
        double bA = 0.0, bB = 0.0;    /* 2-pole breath lowpass state */
        /* High-pass JUST BELOW the fundamental (0.65×f0): the loop's broadband
         * floor leaks a low-frequency rumble ("old-recording" background noise);
         * a tube has no musical content below f0, so removing it cleans the noise
         * while leaving the (treble) tone intact. Pitch-relative → safe at any note.
         * 2-pole one-pole-cascade HP, pole R = exp(-2π·fc/sr). */
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
            if (assign) out[i] = v;
            else        out[i] += v;
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
        if (assign) out[i] = v;
        else        out[i] += v;
    }
}

/* modular_gen_slot_kick: the source==5 harmonic-bank kick voice. The five
 * legacy kick renderers (render_kick_internal + deep/punchy/lofi/tight) are
 * transplanted VERBATIM here, selected by gen_kick_variant[k]:
 *   0 = base    (render_kick_internal,        drums.c ~277-386)
 *   1 = deep    (render_kick_deep_internal,   drums.c ~1159-1239)
 *   2 = punchy  (render_kick_punchy_internal, drums.c ~1245-1321)
 *   3 = lofi    (render_kick_lofi_internal,   drums.c ~1327-1428)
 *   4 = tight   (render_kick_tight_internal,  drums.c ~1434-1559)
 *
 * Each branch keeps its EXACT float-op order, noise-draw count/topology, and
 * final shaping. The CURATED kick knobs (h2/h3/h4 gains, env0/env1 rates,
 * pitch-env amount/rate, click/noise amounts, wave) are read via kp_get so the
 * NaN sentinel reproduces the legacy double literal at default; the structural
 * per-variant constants (filter coeffs, fixed harmonic env rates, attack/fade
 * rates, output scales) stay as literals exactly like the legacy functions —
 * they are not user knobs. The noise stream is noise_ma seeded 0 (→4321),
 * bit-identical to the legacy ma_noise seed-0 stream (TestNoiseMaParity), drawn
 * in the same order as the legacy loop.
 *
 * Base freq (the kick fundamental): gen_freq[k] × voice_freq in ratio mode, so
 * the binding passes ratio 1 and the fundamental flows through voice_freq (=
 * the resolved kick_fund Hz, clamped [30,200] in the binding).
 *
 * assign: when 1 the first sample-write ASSIGNS (out[i]=v) — matching the legacy
 * renderer's direct out[i]= — so a -0.0 sample is not flipped to +0.0. */
static void gen_kick_acoustic(float *out, int sampleRate, int samples,
                              const modular_params *p, int k,
                              double f0, int kWave, noise_ma_gen *noise, int assign) {
#define KICK_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── acoustic modal (inharmonic modal bank + beater excitation) ──
     * A REAL recorded kick's timbre = coupled INHARMONIC membrane/shell modes
     * STRUCK by a beater — not a pure sine + one band-passed click (which is
     * why the "tight" variant, however tuned, reads synthetic: its MFCC /
     * spectral SHAPE is wrong). Body = a small bank of inharmonic partials
     * (ratios stretched by mode_detune), each STRUCK at t=0 (no swell → the
     * sharp natural transient / high crest) with its OWN decay (fundamental
     * slow, higher modes fast — the frequency-dependent damping of a real
     * head). A bright HP beater CLICK supplies the impact brightness; a
     * broadband, envelope-shaped, spectrally-tilted NOISE supplies the
     * recorded-air texture (raises spectral flatness the way a real recording
     * does); gentle saturation glues the layers into one event. Reuses every
     * kick knob (NO new ABI): mode_detune = inharmonic stretch, mode_gain =
     * high-mode level, mode_decay = high-mode damping. */
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.25);
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.60);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 60.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 0.20);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 12.0); /* fundamental decay /s */
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 28.0); /* low-mid mode decay /s */
    double kH2       = kp_get(p->gen_kick_h2[k], 0.50);   /* mode-2 gain */
    double kH3       = kp_get(p->gen_kick_h3[k], 0.35);   /* mode-3 gain */
    double kH4       = kp_get(p->gen_kick_h4[k], 0.20);   /* mode-4 gain */
    double kSat      = kp_get(p->gen_kick_sat[k], 1.4);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.6);
    double kFade     = kp_get(p->gen_kick_fade[k], 5.0);
    double kStretch  = kp_get(p->gen_kick_mode_detune[k], 0.15); /* inharmonic stretch */
    double kModeGain = kp_get(p->gen_kick_mode_gain[k], 1.0);    /* high-mode level */
    double kHiDecay  = kp_get(p->gen_kick_mode_decay[k], 55.0);  /* high-mode damping /s */
    double kReverb   = kp_get(p->gen_kick_reverb[k], 0.0);       /* room/echo tail amount */

    /* Composed from synth_dsp_primitives.h: a 4-osc INHARMONIC modal bank, a
     * band-pass beater click + a low-mid texture (both one-pole primitives),
     * a pitch glide, and a dense feedback reverb TAIL. Byte-identical to the
     * former inline loop (TestVariant7RefactorByteIdentity). */
    sp_reverb3 rev;
    sp_reverb3_init(&rev, sampleRate); /* zeroed tank; dry kicks just don't read it */

    /* INHARMONIC partial ratios (a real membrane's modes are non-integer,
     * clustering higher; extra stretch via mode_detune). */
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
        /* saturate the BODY ONLY (glue the modes into one tone) — the click +
         * texture are added AFTER, un-compressed, so the sharp transient
         * survives (a real kick's crest is ~8; compressing everything flattens
         * it toward ~4). */
        double driven = tanh(body * (1.0 + kSat)) * 0.6;

        /* BAND-PASS beater click (~500 Hz–2.5 kHz felt beater), two one-pole
         * LPs subtracted → a felt-beater band, NOT full-bright white. */
        double cn = (double)noise_ma_white_tick(noise);
        double bpClick = sp_onepole_tick(&lpHi, 0.30, cn) - sp_onepole_tick(&lpLo, 0.05, cn);
        double click = bpClick * kClickAmt * (1.0 + kAttack) * exp(-220.0 * tSec);

        /* LP-limited (~2 kHz) recorded-air texture, attack-weighted — keeps the
         * body tonal (low flatness), unlike full white (hissy/synthetic). */
        double nz = (double)noise_ma_white_tick(noise);
        double texture = sp_onepole_tick(&nzLo, 0.28, nz) * kNoiseAmt * exp(-45.0 * tSec);

        double g = exp(-kFade * tNorm);
        double s = (driven + click + texture) * g;

        /* reverb/echo TAIL — the recorded-kick "space". Skipped when dry. */
        if (kReverb > 0.0) {
            s += sp_reverb3_tick(&rev, s, kReverb);
        }

        /* final gentle soft-limit only (does NOT compress the body a second
         * time) → the click peak stays sharp. */
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
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.40);
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.15);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 65.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 0.05);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 7.5);
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 12.0);
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.35);
    double kH3Gain   = kp_get(p->gen_kick_h3[k], 0.15);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.3);
    double kFade     = kp_get(p->gen_kick_fade[k], 4.5);
    double kSat      = kp_get(p->gen_kick_sat[k], 0.45);

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
    /* ── modal (coupled two-mode drumhead) ──
     * A physically-motivated ORGANIC kick. Two pitch-swept modes:
     *   BATTER  — struck, heavily damped, fast swell → the initial weighty thump;
     *   RESONANT— detuned below the batter (f0·(1-detune)), LESS damped, SLOW
     *             (delayed) swell → a late "bloom". Summed, the two modes BEAT
     *             at Δf = f0·detune, so a plateau-then-bloom amplitude envelope
     *             emerges from a REAL mechanism (energy sloshing batter→resonant
     *             head), not a hand-drawn multi-segment envelope. That is what
     *             lets it reproduce the organic complexity without overfitting.
     * A drumhead-tension pitch glide, modest 2nd/3rd harmonics for punch, a low
     * beater thud, and a DECOUPLED low-mid grit texture (additive, envelope-
     * scaled — never run through the waveshaper, the anti-"spit" rule from
     * variant 5) sit on top. Gentle tanh soft-limit, no hard clamp.
     * Reuses the kick knobs; adds three modal knobs (mode_detune/gain/decay). */
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.12);
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.30);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 45.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 1.6);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 11.0); /* batter decay /s */
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 20.0); /* harmonic decay /s */
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.30);
    double kH3Gain   = kp_get(p->gen_kick_h3[k], 0.04);
    double kDrive    = kp_get(p->gen_kick_sat[k], 0.8);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.5);
    double kFade     = kp_get(p->gen_kick_fade[k], 5.0);
    double kDetune   = kp_get(p->gen_kick_mode_detune[k], 0.11);
    double kModeGain = kp_get(p->gen_kick_mode_gain[k], 0.8);
    double kModeDec  = kp_get(p->gen_kick_mode_decay[k], 4.0);

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
        /* resonant: SLOW swell (~40 ms, energy sloshes in) × LONG decay → the
         * delayed bloom; beats against the batter at Δf = f0·detune. */
        double resoEnv = exp(-kModeDec * tSec) * (1.0 - exp(-25.0 * tSec)) * kModeGain;
        /* harmonics ride the batter's SWELL too (they come from the struck
         * head's nonlinearity) — without the swell they sound at full level at
         * t=0 while the fundamental is still ramping, so the onset is all
         * high-pitch 2f0/3f0 (energy lands in 120-250 not 60-120, and the peak
         * jumps to t=0). Sharing the swell keeps the fundamental dominant. */
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

        /* DECOUPLED low-mid grit texture (HP ~120 → steep 2-pole LP ~500 Hz),
         * attack-only, added on top (not through the waveshaper). */
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
    /* ── hybrid/punchy (layered DSP, for punchy/raw kicks) ──
     * Three DECOUPLED layers — decoupling is what keeps it SMOOTH, not spitty:
     *   (1) a pitch-swept harmonic BODY, smoothly WAVESHAPED for raw harmonics
     *       (deterministic → no flutter);
     *   (2) a separate COLORED-NOISE TEXTURE — band-passed, given the body's
     *       smooth ENVELOPE (not its oscillating wave) and ADDED on top, NOT
     *       run through the distortion. Fusing noise into the waveshaper and
     *       multiplying it by the body WAVE amplitude-modulates it at ~2x the
     *       pitch and the tanh amplifies its peaks → sputtering "spit"; an
     *       additive, envelope-scaled texture is a steady "air" instead;
     *   (3) a short bright CLICK = the attack snap + crest/punch.
     * Reuses the kick knobs: sat=distortion drive, noise=texture, click=attack. */
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.12);
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.40);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 90.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 1.5);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 7.0);
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 12.0);
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.40);
    double kH3Gain   = kp_get(p->gen_kick_h3[k], 0.20);
    double kDrive    = kp_get(p->gen_kick_sat[k], 2.0);
    double kAttack   = kp_get(p->gen_kick_attack[k], 1.5);
    double kFade     = kp_get(p->gen_kick_fade[k], 6.0);

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

        /* body amp env = a fast attack RAMP (~15 ms) × exp decay → the body
         * SWELLS to a peak then decays (the reference peaks ~30 ms). A pure
         * exp decay peaks at t=0 and feels "weak/instant"; the swell is the
         * weighty THUMP. */
        double decayEnv = exp(-kEnv0Rate * tSec);
        double atkRamp  = 1.0 - exp(-90.0 * tSec);
        double bodyEnv  = decayEnv * atkRamp;
        double harmEnv = exp(-kEnv1Rate * tSec);
        double body = sp_osc_tick(&osc0, kWave, 2.0 * M_PI * f0 * freqMul / (double)sampleRate)
                    + kH2Gain * harmEnv * sp_osc_tick(&osc1, kWave, 2.0 * M_PI * f1 * freqMul / (double)sampleRate)
                    + kH3Gain * harmEnv * sp_osc_tick(&osc2, kWave, 2.0 * M_PI * f2 * freqMul / (double)sampleRate);
        body *= bodyEnv;

        /* (1) smooth raw body: waveshape the BODY ALONE (no noise inside →
         *     deterministic, no flutter), re-apply bodyEnv so it decays. */
        double distorted = tanh(body * (1.0 + kDrive * 3.0)) * bodyEnv * 0.45;

        /* (2) LOW-MID GRIT TEXTURE: HP (~120 Hz) then a STEEP 2-pole LP
         *     (~500 Hz) → a low-mid "grit/thickness", NOT high air. "Airy/spitty"
         *     is high-frequency noise (>1 kHz); the organic body character is
         *     low-mid noise. This adds attack thwack + organic thickness with NO
         *     high hiss. Attack-only (fast decay) so the tail stays dry. */
        float nz = noise_ma_white_tick(noise);
        sp_onepole_tick(&nzLo, 0.016, (double)nz);   /* HP ~120 Hz (drop sub rumble) */
        double nzHP = (double)nz - nzLo;
        sp_onepole_tick(&nzBand, 0.065, nzHP);       /* 1-pole LP ~500 Hz */
        sp_onepole_tick(&nzBand2, 0.065, nzBand);    /* 2nd pole → steep, kills >800 Hz (no air) */
        /* Attack-only fast decay (gone ~50 ms) → dry tail. */
        double airEnv = exp(-75.0 * tSec);
        double texture = nzBand2 * kNoiseAmt * airEnv * 3.5;

        /* (3) LOW THUD transient = the thump's punch + crest. A heavily
         * LOW-passed noise burst (~130 Hz) is a low "thud", NOT a high tick:
         * an HP/bright click reads as "tss"/spit and pulls the perceived pitch
         * up. Fast decay (~3 ms) so it's a punch, not a tail. */
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
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.18);
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.35);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 30.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 0.10);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 5.5);
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 9.0);
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.40);
    double kH3Gain   = kp_get(p->gen_kick_h3[k], 0.20);
    double kH4Gain   = kp_get(p->gen_kick_h4[k], 0.12);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.3);
    double kFade     = kp_get(p->gen_kick_fade[k], 4.0);
    double kSat      = kp_get(p->gen_kick_sat[k], 0.55);

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
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.25);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 20.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 0.08);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 4.5);
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 7.0);
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.40);
    double kH3Gain   = kp_get(p->gen_kick_h3[k], 0.20);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.2);
    double kFade     = kp_get(p->gen_kick_fade[k], 3.0);
    double kSat      = kp_get(p->gen_kick_sat[k], 1.5);

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
    double kNoiseAmt = kp_get(p->gen_kick_noise[k], 0.10);
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.20);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 15.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 0.15);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 3.5);
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 7.0);
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.15);
    double kH3Gain   = kp_get(p->gen_kick_h3[k], 0.10);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.15);
    double kFade     = kp_get(p->gen_kick_fade[k], 3.0);
    double kSat      = kp_get(p->gen_kick_sat[k], 1.2);

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
    double kClickAmt = kp_get(p->gen_kick_click[k], 0.45);
    double kPeRate   = kp_get(p->gen_kick_pe_rate[k], 55.0);
    double kPeAmt    = kp_get(p->gen_kick_pe_amt[k], 0.25);
    double kEnv0Rate = kp_get(p->gen_kick_env0[k], 8.0);
    double kEnv1Rate = kp_get(p->gen_kick_env1[k], 14.0);
    double kH2Gain   = kp_get(p->gen_kick_h2[k], 0.40);
    double kAttack   = kp_get(p->gen_kick_attack[k], 0.5);
    double kFade     = kp_get(p->gen_kick_fade[k], 5.0);
    double kSat      = kp_get(p->gen_kick_sat[k], 0.8);

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

static void modular_gen_slot_kick(float *out, int sampleRate, int samples,
                                  const modular_params *p, int k,
                                  double voice_freq, int assign) {
    /* KICK stage enable pill (Phase-10): <0.5 silences the voice. assign=1 means
     * this slot is the primary writer, so zero the buffer; assign=0 accumulates,
     * so add nothing. Every real source==5 consumer sets kick_enabled=1. */
    if (p->kick_enabled < 0.5f) {
        if (assign) {
            for (int i = 0; i < samples; ++i) out[i] = 0.0f;
        }
        return;
    }
    int variant = (int)lrintf(p->gen_kick_variant[k]);
    int kWave   = (int)kp_get(p->gen_wave[k], 0.0);

    double f0 = (double)p->gen_freq[k];
    if ((int)lrintf(p->gen_freq_mode[k]) == 0) f0 *= voice_freq;

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
    } else {
        gen_kick_base(out, sampleRate, samples, p, k, f0, kWave, &noise, assign);
    }
}

/* modular_gen_slot_tom: the source==6 808-style tom voice. The three legacy tom
 * renderers (render_tom_internal + render_tom_high_internal +
 * render_tom_low_internal, drums.c) share ONE loop shape and differ ONLY in
 * literal sets, so this is a SINGLE parameterized loop (not three branches): the
 * gen_tom_variant[k] discriminator (0=tom 1=high 2=low) selects a per-variant
 * literal set for the structural constants, while the CURATED knobs
 * (sweep/ring rates, o1/o2 gains, stick, room) are read via kp_get so the NaN
 * sentinel reproduces the exact per-variant legacy double literal at default.
 *
 * Noise stream: noise_ma seeded 0 (→4321), bit-identical to the legacy ma_noise
 * seed-0 stream (TestNoiseMaParity), drawn in the same order as the legacy loop
 * (1 seed draw, then 1 per-sample draw). Base freq = gen_freq[k] × voice_freq in
 * ratio mode, so the binding passes ratio 1 and the fundamental flows through
 * voice_freq (= the resolved tom_fund Hz, clamped [40,400] in the binding).
 *
 * assign: when 1 the first sample-write ASSIGNS (out[i]=expr) — matching the
 * legacy renderer's direct out[i]= — so a -0.0 sample is not flipped to +0.0. */
static void modular_gen_slot_tom(float *out, int sampleRate, int samples,
                                 const modular_params *p, int k,
                                 double voice_freq, int assign) {
    int variant = (int)lrintf(p->gen_tom_variant[k]);
    if (variant < 0 || variant > 2) variant = 0;
    int tWave = (int)kp_get(p->gen_wave[k], 0.0);

    /* Per-variant STRUCTURAL literal tables, indexed [0]=tom [1]=high [2]=low,
     * transplanted VERBATIM from render_tom_internal / _high_internal /
     * _low_internal. These are NOT user knobs (the legacy functions hardcode
     * them); they are selected by the variant only. */
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
    /* Per-variant CURATED-knob fallbacks (the legacy kp_get literals). */
    static const double sweepDef[3]   = { 18.0, 22.0, 14.0 };
    static const double ringDef[3]    = { 2.8,  3.5,  2.2  };
    static const double o1Def[3]      = { 0.5,  0.55, 0.45 };
    static const double o2Def[3]      = { 0.25, 0.28, 0.22 };
    static const double stickDef[3]   = { 0.35, 0.38, 0.32 };
    static const double roomDef[3]    = { 0.08, 0.06, 0.10 };

    double tSweepRate = kp_get(p->gen_tom_sweep[k], sweepDef[variant]);
    double tRingRate  = kp_get(p->gen_tom_ring[k],  ringDef[variant]);
    double tO1Gain    = kp_get(p->gen_tom_o1[k],    o1Def[variant]);
    double tO2Gain    = kp_get(p->gen_tom_o2[k],    o2Def[variant]);
    double tStick     = kp_get(p->gen_tom_stick[k], stickDef[variant]);
    double tRoom      = kp_get(p->gen_tom_room[k],  roomDef[variant]);

    double fundamentalHz = (double)p->gen_freq[k];
    if ((int)lrintf(p->gen_freq_mode[k]) == 0) fundamentalHz *= voice_freq;

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
        if (assign) out[i] = y;
        else        out[i] += y;
    }
}

/* modular_gen_slot_snare: the source==7 snare-ish voice. The three legacy
 * snare-family renderers are transplanted VERBATIM here, selected by
 * gen_snare_variant[k]:
 *   0 = snare     (render_snare_internal,           drums.c ~87-266)
 *   1 = rimshot   (render_snare_rimshot_internal,   drums.c ~614-692)
 *   2 = sidestick (render_snare_sidestick_internal, drums.c ~698-759)
 *
 * Each branch keeps its EXACT float-op order, RBJ-biquad inline coefficient
 * computation (the legacy snare uses its OWN inline biquad form, NOT mod_biquad —
 * transplanted verbatim for byte-identity), noise-draw topology (1 seed draw then
 * 1 white draw per sample), and final shaping (0.95f * softsat). The CURATED
 * snare knobs (tone2/tune/tone-decay/noise-decay/tail-decay/tone-mix/noise-mix/
 * wire-mix/attack, wave) are read via kp_get so the NaN sentinel reproduces the
 * exact per-variant legacy double literal at default; the structural per-variant
 * constants (fixed band centers/Qs, fixed partial ratios/decays, HP cutoffs, fade
 * slopes) stay as literals exactly like the legacy functions. The noise stream is
 * noise_ma seeded 0 (→4321), bit-identical to the legacy ma_noise seed-0 stream
 * (TestNoiseMaParity), drawn in the same order as the legacy loop.
 *
 * Primary tone freq (the snare fundamental): gen_freq[k] × voice_freq in ratio
 * mode, so the binding passes ratio 1 and the fundamental flows through voice_freq
 * (= the resolved snare_fund Hz, clamped [80,2000] in the binding). */
static void gen_snare_rimshot(float *out, int sampleRate, int samples,
                              const modular_params *p, int k,
                              double fundamentalHz, int sWave,
                              noise_ma_gen *noise, int assign) {
#define SNARE_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── rimshot (render_snare_rimshot_internal) ── */
    double sTone2  = kp_get(p->gen_snare_tone2[k], 1050.0);
    double sTune   = kp_get(p->gen_snare_tune[k], 1.0);
    double sToneD  = kp_get(p->gen_snare_tone_d[k], 40.0);
    double sNoiseD = kp_get(p->gen_snare_noise_d[k], 200.0);
    double sToneM  = kp_get(p->gen_snare_tone_m[k], 1.0);
    double sNoiseM = kp_get(p->gen_snare_noise_m[k], 0.7);
    double sAtk    = kp_get(p->gen_snare_attack[k], 2.0);

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

    double rc = 1.0 / (2.0 * M_PI * (400.0 * sTune));
    double dt = 1.0 / (double)sampleRate;
    double hpAlpha = rc / (rc + dt);
    double hpPrev = 0.0, hpOut = 0.0;

    double lpAlpha = dt / (1.0 / (2.0 * M_PI * (5000.0 * sTune)) + dt);
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
    double sTone2  = kp_get(p->gen_snare_tone2[k], 1200.0);
    double sTune   = kp_get(p->gen_snare_tune[k], 1.0);
    double sToneD  = kp_get(p->gen_snare_tone_d[k], 100.0);
    double sNoiseD = kp_get(p->gen_snare_noise_d[k], 150.0);
    double sToneM  = kp_get(p->gen_snare_tone_m[k], 0.5);
    double sNoiseM = kp_get(p->gen_snare_noise_m[k], 0.5);
    double sAtk    = kp_get(p->gen_snare_attack[k], 1.0);

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
        double fc = 1500.0 * sTune; double Q = 0.7;
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
    /* ── snare (render_snare_internal) — FAT-BOTTOM tuning (matches
     * 579499__yenus__fat-snare-bottom-ramon): lower 2nd tone, MORE tone +
     * LESS noise, a FAST body thump (tone decay) + a LONG wire ring (slow
     * noise/tail decay), darker noise tune. Defaults kept in sync with the
     * drum-snare recipe spec in synth_recipe_wired.go. */
    double sTone2  = kp_get(p->gen_snare_tone2[k], 280.0);
    double sTune   = kp_get(p->gen_snare_tune[k], 0.55);
    double sToneD  = kp_get(p->gen_snare_tone_d[k], 46.0);
    double sNoiseD = kp_get(p->gen_snare_noise_d[k], 7.0);
    double sTailD  = kp_get(p->gen_snare_tail_d[k], 10.0);
    double sToneM  = kp_get(p->gen_snare_tone_m[k], 1.12);
    double sNoiseM = kp_get(p->gen_snare_noise_m[k], 0.56);
    double sWireM  = kp_get(p->gen_snare_wire_m[k], 0.8);
    double sAtk    = kp_get(p->gen_snare_attack[k], 0.5);

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
        double fc = 350.0 * sTune;
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
        double fc = 1800.0 * sTune;
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
        double fc = 3200.0 * sTune;
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
        double fc = 4500.0 * sTune;
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

static void modular_gen_slot_snare(float *out, int sampleRate, int samples,
                                   const modular_params *p, int k,
                                   double voice_freq, int assign) {
    int variant = (int)lrintf(p->gen_snare_variant[k]);
    if (variant < 0 || variant > 2) variant = 0;
    int sWave = (int)kp_get(p->gen_wave[k], 0.0);

    double fundamentalHz = (double)p->gen_freq[k];
    if ((int)lrintf(p->gen_freq_mode[k]) == 0) fundamentalHz *= voice_freq;

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    if (variant == 1) {
        gen_snare_rimshot(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    } else if (variant == 2) {
        gen_snare_sidestick(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    } else {
        gen_snare_main(out, sampleRate, samples, p, k, fundamentalHz, sWave, &noise, assign);
    }
}

/* modular_gen_slot_clap: the source==8 clap voice (render_clap_internal,
 * drums.c ~451-511). PURE noise — a different draw topology from the snare-ish
 * source (NO seed draw; the FIRST draw is per-sample), which is why clap is a
 * SEPARATE source rather than a fourth snare branch. Transplanted VERBATIM:
 * one RBJ band-pass at 1800 Hz Q1.5 (inline legacy biquad form), a 3-4-burst
 * envelope (|t-offset| exp shapes), and a room tail. Final shaping is softsat()
 * with NO 0.95 scale (the legacy clap omits it). The curated clap knobs
 * (tune/noise-decay/tail-decay/noise-mix/attack) are read via kp_get.
 *
 * The clap does NOT use the primary tone freq (no pitched body), so voice_freq is
 * unused here. */
static void modular_gen_slot_clap(float *out, int sampleRate, int samples,
                                  const modular_params *p, int k,
                                  double voice_freq, int assign) {
    (void)voice_freq; /* clap is pure noise — no pitched body */
    double sTune   = kp_get(p->gen_snare_tune[k], 1.0);
    double sNoiseD = kp_get(p->gen_snare_noise_d[k], 7.0);
    double sTailD  = kp_get(p->gen_snare_tail_d[k], 4.0);
    double sNoiseM = kp_get(p->gen_snare_noise_m[k], 0.15);
    double sAtk    = kp_get(p->gen_snare_attack[k], 110.0);

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
    {
        double fc = 1800.0 * sTune;
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
        if (assign) out[i] = y;
        else        out[i] += y;
    }
}

/* modular_gen_slot_cymbal: the source==9 metallic family voice. The six legacy
 * metallic renderers (render_hihat_internal + render_open_hihat_internal +
 * render_cowbell_internal + render_shaker_internal + render_ride_internal +
 * render_crash_internal, drums.c) are transplanted VERBATIM here, selected by
 * gen_cym_variant[k]:
 *   0 = hihat       (render_hihat_internal)
 *   1 = open-hihat  (render_open_hihat_internal)
 *   2 = cowbell     (render_cowbell_internal)
 *   3 = shaker      (render_shaker_internal)
 *   4 = ride        (render_ride_internal)
 *   5 = crash       (render_crash_internal)
 *
 * The hihat/open-hihat pair and the ride/crash pair share one loop shape each
 * (differing only in literal sets), but are kept as distinct branches with their
 * own literal constants so every legacy float-op order is preserved exactly. The
 * CURATED cymbal knobs (tune/env_fast/env_tail/tone_mix/noise_mix/noise_decay,
 * plus cym_wave via gen_wave) are read via kp_get so the NaN sentinel reproduces
 * the exact per-variant legacy double literal at default. The noise stream is
 * noise_ma seeded 0 (→4321), bit-identical to the legacy ma_noise seed-0 stream
 * (TestNoiseMaParity), drawn in the SAME order as the legacy loop.
 *
 * The cymbal partial tables are ABSOLUTE Hz × cym_tune (NOT voice_freq-derived),
 * so voice_freq is unused — the cymbals do not carry a pitched fundamental.
 *
 * assign: when 1 the first sample-write ASSIGNS (out[i]=expr) — matching the
 * legacy renderer's direct out[i]= — so a -0.0 sample is not flipped to +0.0. */
static void gen_cymbal_hihat(float *out, int sampleRate, int samples,
                             const modular_params *p, int k,
                             noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
#define HH_PARTIALS 6
    /* ── hihat (render_hihat_internal) ── */
    double cyTune   = kp_get(p->gen_cym_tune[k], 1.0);
    double cyFast   = kp_get(p->gen_cym_env_fast[k], 180.0);
    double cyTail   = kp_get(p->gen_cym_env_tail[k], 35.0);
    double cyToneM  = kp_get(p->gen_cym_tone_m[k], 0.85);
    double cyNoiseM = kp_get(p->gen_cym_noise_m[k], 0.45);
    double cyNoiseD = kp_get(p->gen_cym_noise_d[k], 100.0);
    int    cyWave   = (int)kp_get(p->gen_wave[k], 2.0);

    static const double baseFreq[HH_PARTIALS] = {4100.0, 5400.0, 6700.0, 8300.0, 9900.0, 11800.0};
    sp_osc osc[HH_PARTIALS];
    double freq[HH_PARTIALS];
    double gain[HH_PARTIALS];

    for (int kk = 0; kk < HH_PARTIALS; ++kk) {
        float s = noise_ma_white_tick(noise);
        double det = 1.0 + 0.02 * (double)s;
        freq[kk] = (baseFreq[kk] * cyTune) * det;
        sp_osc_init(&osc[kk], 2.0 * M_PI * (double)s);
        gain[kk] = (kk < 3) ? 1.0 / (double)HH_PARTIALS : 0.75 / (double)HH_PARTIALS;
    }

    double lpCluster = 0.0;
    double lpNoise = 0.0;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double cluster = 0.0;
        for (int kk = 0; kk < HH_PARTIALS; ++kk) {
            double sq = sp_osc_tick(&osc[kk], cyWave, 2.0 * M_PI * freq[kk] / (double)sampleRate);
            double lfo = 1.0 + 0.10 * sin(2.0 * M_PI * (10.0 + 4.0 * kk) * sec);
            cluster += sq * gain[kk] * lfo;
        }

        sp_onepole_mix(&lpCluster, 0.9, 0.1, cluster);
        double hpCluster = cluster - lpCluster;

        float wn = noise_ma_white_tick(noise);
        double xNoise = (double)wn;
        sp_onepole_mix(&lpNoise, 0.92, 0.08, xNoise);
        double hpNoise = xNoise - lpNoise;

        double envFast = exp(-sec * cyFast);
        double envTail = exp(-sec * cyTail);
        double env = envFast * 0.8 + envTail * 0.6;

        double noiseEnv = exp(-sec * cyNoiseD);

        double y = hpCluster * env * cyToneM + hpNoise * noiseEnv * cyNoiseM;
        y *= (1.0 - 0.15 * tNorm);

        CYM_WRITE(0.9f * softsat_shared((float)y));
    }
#undef HH_PARTIALS
#undef CYM_WRITE
}

static void gen_cymbal_open(float *out, int sampleRate, int samples,
                            const modular_params *p, int k,
                            noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
#define HH_PARTIALS 6
    /* ── open-hihat (render_open_hihat_internal) ── */
    double cyTune   = kp_get(p->gen_cym_tune[k], 1.0);
    double cyFast   = kp_get(p->gen_cym_env_fast[k], 120.0);
    double cyTail   = kp_get(p->gen_cym_env_tail[k], 22.0);
    double cyToneM  = kp_get(p->gen_cym_tone_m[k], 0.7);
    double cyNoiseM = kp_get(p->gen_cym_noise_m[k], 0.9);
    double cyNoiseD = kp_get(p->gen_cym_noise_d[k], 18.0);
    int    cyWave   = (int)kp_get(p->gen_wave[k], 2.0);

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

    double cyTune   = kp_get(p->gen_cym_tune[k], 1.0);
    double cyFast   = kp_get(p->gen_cym_env_fast[k], 260.0);
    double cyTail   = kp_get(p->gen_cym_env_tail[k], 9.0);
    double cyToneM  = kp_get(p->gen_cym_tone_m[k], 1.0);
    double cyNoiseM = kp_get(p->gen_cym_noise_m[k], 0.55);
    double cyNoiseD = kp_get(p->gen_cym_noise_d[k], 60.0);
    int    cyWave   = (int)kp_get(p->gen_wave[k], 0.0);

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
    double cyTune   = kp_get(p->gen_cym_tune[k], 1.0);
    double cyFast   = kp_get(p->gen_cym_env_fast[k], 200.0);
    double cyTail   = kp_get(p->gen_cym_env_tail[k], 25.0);
    double cyToneM  = kp_get(p->gen_cym_tone_m[k], 0.6);
    double cyNoiseM = kp_get(p->gen_cym_noise_m[k], 0.5);

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

static void gen_cymbal_ride(float *out, int sampleRate, int samples,
                            const modular_params *p, int k,
                            noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── ride (render_ride_internal) ── */
    const int RIDE_PARTIALS = 10;
    double cyTune   = kp_get(p->gen_cym_tune[k], 1.0);
    double cyFast   = kp_get(p->gen_cym_env_fast[k], 40.0);
    double cyTail   = kp_get(p->gen_cym_env_tail[k], 8.0);
    double cyToneM  = kp_get(p->gen_cym_tone_m[k], 1.0);
    double cyNoiseM = kp_get(p->gen_cym_noise_m[k], 0.3);
    double cyNoiseD = kp_get(p->gen_cym_noise_d[k], 12.0);
    int    cyWave   = (int)kp_get(p->gen_wave[k], 0.0);

    static const double baseFreq[10] = {
        3100.0, 3800.0, 4700.0, 5900.0, 7100.0,
        8400.0, 9800.0, 11500.0, 13200.0, 15000.0
    };
    static const double baseGain[10] = {
        1.0, 1.0, 0.9, 0.8, 0.7, 0.6, 0.5, 0.45, 0.4, 0.35
    };

    sp_osc osc[10];
    double freq[10];
    double gain[10];

    for (int kk = 0; kk < RIDE_PARTIALS; ++kk) {
        float s = noise_ma_white_tick(noise);
        double det = 1.0 + 0.015 * (double)s;
        freq[kk] = (baseFreq[kk] * cyTune) * det;
        sp_osc_init(&osc[kk], 2.0 * M_PI * (double)s);
        gain[kk] = baseGain[kk] / (double)RIDE_PARTIALS;
    }

    double bellPhase = 0.0;
    float bs = noise_ma_white_tick(noise);
    double bellFreq = 3000.0 * (1.0 + 0.01 * (double)bs);

    double lpCluster = 0.0;
    double lpNoise = 0.0;

    for (int i = 0; i < samples; ++i) {
        double sec = (double)i / (double)sampleRate;
        double tNorm = (double)i / (double)samples;

        double cluster = 0.0;
        for (int kk = 0; kk < RIDE_PARTIALS; ++kk) {
            cluster += sp_osc_tick(&osc[kk], cyWave, 2.0 * M_PI * freq[kk] / (double)sampleRate) * gain[kk];
        }

        sp_onepole_mix(&lpCluster, 0.92, 0.08, cluster);
        double hpCluster = cluster - lpCluster;

        bellPhase += 2.0 * M_PI * bellFreq / (double)sampleRate;
        double bellEnv = exp(-6.0 * sec);
        double bell = sin(bellPhase) * bellEnv * 0.15;

        double envFast = exp(-cyFast * sec);
        double envTail = exp(-cyTail * sec);
        double env = envFast * 0.5 + envTail * 0.8;

        float wn = noise_ma_white_tick(noise);
        double xNoise = (double)wn;
        sp_onepole_mix(&lpNoise, 0.92, 0.08, xNoise);
        double hpNoise = xNoise - lpNoise;
        double noiseEnv = exp(-cyNoiseD * sec);

        double lfo = 1.0 + 0.06 * sin(2.0 * M_PI * 7.0 * sec);

        double y = (hpCluster * env * cyToneM + bell) * lfo + hpNoise * noiseEnv * cyNoiseM;
        y *= exp(-2.0 * tNorm);

        CYM_WRITE(0.9f * softsat_shared((float)y));
    }
#undef CYM_WRITE
}

static void gen_cymbal_crash(float *out, int sampleRate, int samples,
                             const modular_params *p, int k,
                             noise_ma_gen *noise, int assign) {
#define CYM_WRITE(expr) do { if (assign) out[i] = (expr); else out[i] += (expr); } while (0)
    /* ── crash (render_crash_internal) ── */
    const int CRASH_PARTIALS = 8;
    double cyTune   = kp_get(p->gen_cym_tune[k], 1.0);
    double cyFast   = kp_get(p->gen_cym_env_fast[k], 10.0);
    double cyTail   = kp_get(p->gen_cym_env_tail[k], 3.0);
    double cyToneM  = kp_get(p->gen_cym_tone_m[k], 1.0);
    double cyNoiseM = kp_get(p->gen_cym_noise_m[k], 0.35);
    double cyNoiseD = kp_get(p->gen_cym_noise_d[k], 8.0);
    int    cyWave   = (int)kp_get(p->gen_wave[k], 0.0);

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

static void modular_gen_slot_cymbal(float *out, int sampleRate, int samples,
                                    const modular_params *p, int k,
                                    double voice_freq, int assign) {
    (void)voice_freq; /* cymbal partials are absolute Hz × cym_tune */
    int variant = (int)lrintf(p->gen_cym_variant[k]);
    if (variant < 0 || variant > 5) variant = 0;

    noise_ma_gen noise;
    noise_ma_init(&noise, 0);

    if (variant == 0) {
        gen_cymbal_hihat(out, sampleRate, samples, p, k, &noise, assign);
    } else if (variant == 1) {
        gen_cymbal_open(out, sampleRate, samples, p, k, &noise, assign);
    } else if (variant == 2) {
        gen_cymbal_cowbell(out, sampleRate, samples, p, k, &noise, assign);
    } else if (variant == 3) {
        gen_cymbal_shaker(out, sampleRate, samples, p, k, &noise, assign);
    } else if (variant == 4) {
        gen_cymbal_ride(out, sampleRate, samples, p, k, &noise, assign);
    } else {
        gen_cymbal_crash(out, sampleRate, samples, p, k, &noise, assign);
    }
}

/* modular_gen_slot_fm: the source==10 4-operator FM-family voice. Builds the
 * per-variant fm_preset (0=bass 1=bell 2=lead 3=epiano 4=pluck) with the
 * STRUCTURAL constants taken VERBATIM from the legacy PRESET_FM_* tables
 * (fmsynth.c) — op amplitudes, freq offsets, ADSR attack/sustain/release,
 * carrier flags, num_ops, mod_matrix ROUTING — and overlays the CURATED knobs
 * (base_freq, pitch-env amount/decay, per-op ratio/depth/decay, wave) via
 * kp_get so the NaN sentinel reproduces the exact legacy double literal at
 * default. This is the SAME overlay the deleted fm_apply_params performed: the
 * legacy _p wrappers copied PRESET_FM_X, overlaid the non-NaN fm_params fields,
 * then called fm_render. Here the per-variant builder bakes the preset literals
 * AND the kp_get curated-knob defaults, so a NaN (unedited) knob lands on the
 * exact preset literal — byte-identical to the legacy NULL/all-NaN overlay.
 *
 * The depth knobs map to each preset's outgoing mod_matrix edge exactly as the
 * legacy fm_apply_params did (op i's first nonzero outgoing edge); the routing
 * topology is fixed per variant. gen_fm_wave reuses gen_wave (NaN→0=sine, the
 * preset.wave default).
 *
 * fm_render memset-zeros its own output, so it CANNOT write directly into the
 * shared out[] (it would erase the legacy osc / prior slots). The voice renders
 * into a private scratch buffer then ASSIGN/accumulates into out[]: when assign
 * (first writer, osc off) it copies (preserving a legacy -0.0 sample exactly as
 * the legacy renderer's direct fm_render(out) would), otherwise it adds. The
 * shared POST stage that follows (post_enabled, PostOrder=0) calls the SAME
 * apply_post_params the legacy _p wrappers used. */
static void modular_gen_slot_fm(float *out, int sampleRate, int samples,
                                const modular_params *p, int k,
                                double voice_freq, int assign) {
    (void)voice_freq; /* FM base freq is an absolute knob (gen_fm_base), not the
                       * derived modular voice freq. */
    int variant = (int)lrintf(p->gen_fm_variant[k]);

    fm_preset pr;
    memset(&pr, 0, sizeof(pr));

    /* Curated knobs, read via kp_get with the per-variant legacy literal as the
     * NaN fallback (the byte-parity contract: an unedited knob == preset
     * literal). The per-op ratio/depth/decay arrays carry the legacy literals. */
    double r1, r2, r3, r4, d1, d2, d3, d4, dec1, dec2, dec3, dec4;
    double baseLit, peAmtLit, peDecayLit;

    switch (variant) {
    case 1: /* PRESET_FM_BELL */
        pr.num_ops = 2;
        baseLit = 440.0; peAmtLit = 0.0; peDecayLit = 0.0;
        /* op0 carrier */
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.7f;
        pr.ops[0].attack_sec = 0.001f; pr.ops[0].sustain_level = 0.0f;
        pr.ops[0].release_sec = 0.3f; pr.ops[0].is_carrier = 1;
        /* op1 modulator (inharmonic) */
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 0.6f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.0f;
        pr.ops[1].release_sec = 0.2f; pr.ops[1].is_carrier = 0;
        r1 = 1.0; r2 = 3.5; r3 = 1.0; r4 = 1.0;
        dec1 = 1.5; dec2 = 1.2; dec3 = 0.0; dec4 = 0.0;
        d1 = 0.0; d2 = 3.0; d3 = 0.0; d4 = 0.0; /* op1 → op0 depth 3 */
        break;
    case 2: /* PRESET_FM_LEAD */
        pr.num_ops = 3;
        baseLit = 220.0; peAmtLit = 1.5; peDecayLit = 0.04;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.8f;
        pr.ops[0].attack_sec = 0.003f; pr.ops[0].sustain_level = 0.5f;
        pr.ops[0].release_sec = 0.15f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 0.9f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.3f;
        pr.ops[1].release_sec = 0.1f; pr.ops[1].is_carrier = 0;
        pr.ops[2].freq_offset = 0.0f; pr.ops[2].amplitude = 0.5f;
        pr.ops[2].attack_sec = 0.001f; pr.ops[2].sustain_level = 0.1f;
        pr.ops[2].release_sec = 0.05f; pr.ops[2].is_carrier = 0;
        r1 = 1.0; r2 = 2.0; r3 = 3.0; r4 = 1.0;
        dec1 = 0.2; dec2 = 0.12; dec3 = 0.08; dec4 = 0.0;
        d1 = 0.0; d2 = 3.5; d3 = 2.0; d4 = 0.0; /* op1→op0 (3.5), op2→op1 (2.0) */
        break;
    case 3: /* PRESET_FM_EPIANO */
        pr.num_ops = 3;
        baseLit = 220.0; peAmtLit = 0.0; peDecayLit = 0.0;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.7f;
        pr.ops[0].attack_sec = 0.002f; pr.ops[0].sustain_level = 0.3f;
        pr.ops[0].release_sec = 0.4f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 7.0f; pr.ops[1].amplitude = 0.6f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.1f;
        pr.ops[1].release_sec = 0.2f; pr.ops[1].is_carrier = 0;
        pr.ops[2].freq_offset = 0.5f; pr.ops[2].amplitude = 0.30f;
        pr.ops[2].attack_sec = 0.001f; pr.ops[2].sustain_level = 0.05f;
        pr.ops[2].release_sec = 0.3f; pr.ops[2].is_carrier = 1; /* independent carrier */
        r1 = 1.0; r2 = 1.0; r3 = 2.0; r4 = 1.0;
        dec1 = 0.8; dec2 = 0.2; dec3 = 0.5; dec4 = 0.0;
        d1 = 0.0; d2 = 2.2; d3 = 0.0; d4 = 0.0; /* op1 → op0 depth 2.2; op2 independent */
        break;
    case 4: /* PRESET_FM_PLUCK */
        pr.num_ops = 2;
        baseLit = 196.0; peAmtLit = 2.0; peDecayLit = 0.03;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.85f;
        pr.ops[0].attack_sec = 0.001f; pr.ops[0].sustain_level = 0.0f;
        pr.ops[0].release_sec = 0.05f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 1.0f;
        pr.ops[1].attack_sec = 0.0005f; pr.ops[1].sustain_level = 0.0f;
        pr.ops[1].release_sec = 0.02f; pr.ops[1].is_carrier = 0;
        r1 = 1.0; r2 = 2.0; r3 = 1.0; r4 = 1.0;
        dec1 = 0.2; dec2 = 0.04; dec3 = 0.0; dec4 = 0.0;
        d1 = 0.0; d2 = 4.0; d3 = 0.0; d4 = 0.0; /* op1 → op0 depth 4 */
        break;
    default: /* PRESET_FM_BASS (variant 0) */
        pr.num_ops = 2;
        baseLit = 55.0; peAmtLit = 3.0; peDecayLit = 0.06;
        pr.ops[0].freq_offset = 0.0f; pr.ops[0].amplitude = 0.9f;
        pr.ops[0].attack_sec = 0.005f; pr.ops[0].sustain_level = 0.6f;
        pr.ops[0].release_sec = 0.2f; pr.ops[0].is_carrier = 1;
        pr.ops[1].freq_offset = 0.0f; pr.ops[1].amplitude = 0.8f;
        pr.ops[1].attack_sec = 0.001f; pr.ops[1].sustain_level = 0.2f;
        pr.ops[1].release_sec = 0.1f; pr.ops[1].is_carrier = 0;
        r1 = 1.0; r2 = 1.0; r3 = 1.0; r4 = 1.0;
        dec1 = 0.3; dec2 = 0.15; dec3 = 0.0; dec4 = 0.0;
        d1 = 0.0; d2 = 2.5; d3 = 0.0; d4 = 0.0; /* op1 → op0 depth 2.5 */
        break;
    }

    /* Overlay curated knobs (kp_get NaN → the variant literal above). */
    pr.base_freq        = (float)kp_get(p->gen_fm_base[k], baseLit);
    pr.pitch_env_amount = (float)kp_get(p->gen_fm_pe_amt[k], peAmtLit);
    pr.pitch_env_decay  = (float)kp_get(p->gen_fm_pe_decay[k], peDecayLit);
    pr.wave             = (int)kp_get(p->gen_wave[k], 0.0);

    pr.ops[0].freq_ratio = (float)kp_get(p->gen_fm_r1[k], r1);
    pr.ops[1].freq_ratio = (float)kp_get(p->gen_fm_r2[k], r2);
    pr.ops[2].freq_ratio = (float)kp_get(p->gen_fm_r3[k], r3);
    pr.ops[3].freq_ratio = (float)kp_get(p->gen_fm_r4[k], r4);

    pr.ops[0].decay_sec = (float)kp_get(p->gen_fm_dec1[k], dec1);
    pr.ops[1].decay_sec = (float)kp_get(p->gen_fm_dec2[k], dec2);
    pr.ops[2].decay_sec = (float)kp_get(p->gen_fm_dec3[k], dec3);
    pr.ops[3].decay_sec = (float)kp_get(p->gen_fm_dec4[k], dec4);

    /* mod_matrix routing: each variant's outgoing edge for op i carries depth
     * d(i+1). The legacy fm_apply_params replaced op i's FIRST nonzero outgoing
     * edge; here the routing is fixed by the variant so we set the exact edge. */
    double depth[4] = {d1, d2, d3, d4};
    depth[0] = kp_get(p->gen_fm_d1[k], d1);
    depth[1] = kp_get(p->gen_fm_d2[k], d2);
    depth[2] = kp_get(p->gen_fm_d3[k], d3);
    depth[3] = kp_get(p->gen_fm_d4[k], d4);
    /* op1 → op0 (every preset routes the first modulator into the carrier). */
    pr.mod_matrix[1][0] = (float)depth[1];
    if (variant == 2) {
        /* lead: op2 → op1 */
        pr.mod_matrix[2][1] = (float)depth[2];
    }
    /* bell/bass/pluck: op1→op0 only. epiano: op1→op0 only (op2 independent). */

    /* Render into a private scratch buffer (fm_render memset-zeros it), then
     * assign/accumulate into out[] preserving the legacy -0.0 sample. */
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
        if ((int)lrintf(p->gen_source[k]) > 0) { active = 1; break; }
    }
    if (!active) return;

    /* First-active-slot-assign: when the legacy osc stage wrote nothing, the
     * first slot that produces output ASSIGNS into out[] (rather than adding to
     * the zeroed buffer) so a -0.0 sample is not flipped to +0.0. Subsequent
     * slots accumulate in slot-index order (spec §2 mix order). */
    int wrote = osc_wrote;

    /* ── Pitch-vibrato gate (mirrors modular.c's vib_on). When active, the
     * wavetable-osc gen slots apply a per-sample 2^(semis/12) multiplier to
     * their slot frequency, using the SAME formula as the OSC stage. When
     * inactive, the existing code path runs unchanged (byte-identity). ── */
    int   gb_lfo_on     = p->lfo_enabled >= 0.5f;
    float gb_lfo_rate   = p->lfo_rate;
    float gb_lfo_depth  = p->lfo_depth;
    int   gb_lfo_target = (int)lrintf(p->lfo_target);
    float gb_lfo_delay  = p->lfo_delay;
    int   gb_vib_on     = gb_lfo_on && gb_lfo_target == 1
                          && gb_lfo_depth > 0.0f && gb_lfo_rate > 0.0f;

    /* ── Noise bus: rendered ONCE so slots sharing the stream consume the
     * same draws in the same order as a legacy interleaved loop (spec §2
     * shared-noise-draws rule). Layout: prelude draws first, then
     * draws-per-sample groups. ── */
    int draws = (int)lrintf(p->noise_draws);
    if (draws < 1) draws = 1;
    int prelude = (int)lrintf(p->noise_prelude);
    if (prelude < 0) prelude = 0;
    float *bus = NULL;
    int need_bus = 0;
    for (int k = 0; k < MODULAR_GEN_SLOTS; k++) {
        int src = (int)lrintf(p->gen_source[k]);
        int pm = (int)lrintf(p->gen_phase_mode[k]);
        if (src == 2 || (src > 0 && pm == 2)) { need_bus = 1; break; }
    }
    if (need_bus) {
        size_t total = (size_t)prelude + (size_t)samples * (size_t)draws;
        bus = (float *)malloc(total * sizeof(float));
        if (!bus) return;
        noise_ma_white_fill(bus, (int)total, (int)seed);
    }

    for (int k = 0; k < MODULAR_GEN_SLOTS; k++) {
        int src = (int)lrintf(p->gen_source[k]);
        if (src <= 0) continue;

        /* assign: this slot ASSIGNS into out[] (first writer when osc wrote
         * nothing); otherwise it accumulates. When osc_wrote was 1 this is
         * always 0, so the pre-Phase-2 `out[i] += ...` path is byte-unchanged. */
        int assign = !wrote;

        double gain = (double)p->gen_gain[k];
        double fr = (double)p->gen_env_fast_rate[k];
        double tr = (double)p->gen_env_tail_rate[k];
        double fm = (double)p->gen_env_fast_mix[k];
        double tm = (double)p->gen_env_tail_mix[k];
        gen_slot_filter filt;
        gen_slot_filter_init(&filt, p, k, sampleRate);

        if (src == 1) { /* wavetable oscillator */
            double f = (double)p->gen_freq[k];
            if ((int)lrintf(p->gen_freq_mode[k]) == 0) f *= voice_freq;
            if (f < 0.0) f = 0.0;
            /* Clamp to just under Nyquist: a freq above SR/2 has no meaningful
             * sample representation and would push phase_inc past the table
             * length, breaking wt_osc_tick's single-step phase wrap. ratio mode
             * (freq × voice_freq) can otherwise reach absurd values.
             * NOTE: deliberate deviation from the Phase-1 plan's reference code
             * (crash guard found via TestRenderModularP_EveryParamMutatesInContext);
             * a no-op for every in-band frequency, so byte-identity holds —
             * verified by the full golden gate. */
            double nyq = 0.5 * (double)sampleRate;
            if (f > nyq * 0.99) f = nyq * 0.99;
            const wavetable_t *wt = modular_shared_table_for((int)lrintf(p->gen_wave[k]));
            wt_osc_t osc;
            wt_osc_init(&osc, wt, f, sampleRate);
            int pm = (int)lrintf(p->gen_phase_mode[k]);
            if (pm == 1) {
                wt_osc_set_phase(&osc, (double)p->gen_phase[k] / (2.0 * M_PI));
            } else if (pm == 2 && bus) {
                int pi = (int)lrintf(p->gen_phase[k]);
                if (pi >= 0 && pi < prelude) {
                    wt_osc_set_phase(&osc, (double)bus[pi]); /* draw in [0,1) (or slightly negative post-wrap; wt wraps) */
                }
            }
            /* Phase-8E unison: gated on nv>=2 so nv==1 is the exact original loop. */
            int unison_nv = (int)lrintf(p->unison_voices);
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
                /* Control-rate pitch modulation. Vibrato (gb_lfo_rate) and unison
                 * drift (udrift_rate) are slow (~3 Hz); evaluating their sin/pow
                 * every sample cost ~(1+ns) sin + (1+ns) pow per sample per slot —
                 * the dominant cost of a 7-voice additive patch and, on emscripten's
                 * software libm, the reason organ-church rendered ~420 ms in WASM.
                 * Recompute the modulated frequencies every GB_CTRL_STRIDE samples
                 * (0.67 ms @ 48 kHz) and HOLD the wt_osc frequency between updates;
                 * the oscillators still tick every sample. At 0.67 ms the 3 Hz
                 * modulation moves ~0.006% between updates — inaudible — for ~30x
                 * fewer transcendentals. (Native/WASM share this C, so xplat parity
                 * is preserved; drums/FM don't reach this branch so their goldens
                 * are unaffected.) */
                const int GB_CTRL_STRIDE = 32;
                double vib_mult = 1.0;
                /* Envelope via incremental multiply instead of a per-sample
                 * exp(): fm*exp(-fr*t)+tm*exp(-tr*t) == fm*decF^i + tm*decT^i
                 * with decF=exp(-fr/sr). Two exp() at setup replace 2*samples
                 * exp() calls per slot — the remaining hot-loop transcendental
                 * cost after control-rate. Mathematically identical; float drift
                 * over the buffer is negligible and native/WASM share the code
                 * (xplat parity preserved). */
                double gb_decF = exp(-fr / (double)sampleRate);
                double gb_decT = exp(-tr / (double)sampleRate);
                double gb_ef = fm;
                double gb_et = tm;
                for (int i = 0; i < samples; i++) {
                    double t = (double)i / (double)sampleRate;
                    int ctrl = ((i % GB_CTRL_STRIDE) == 0);
                    /* Pitch-vibrato multiplier: same formula as OSC stage. Applied
                     * to the center voice AND every side voice (vibrato on top of
                     * unison detune/drift), matching OSC-stage unison behaviour. */
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
                /* Per-sample pitch-vibrato: same formula as modular.c OSC stage.
                 * Only runs when vibrato is active; inactive path below is unchanged
                 * (byte-identity when gb_vib_on == 0). */
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
                    double env = fm * exp(-fr * t) + tm * exp(-tr * t);
                    if (assign) out[i] = (float)(s * env * gain);
                    else        out[i] += (float)(s * env * gain);
                }
            } else {
                for (int i = 0; i < samples; i++) {
                    double t = (double)i / (double)sampleRate;
                    double s = (double)wt_osc_tick(&osc);
                    s = gen_slot_filter_tick(&filt, s);
                    double env = fm * exp(-fr * t) + tm * exp(-tr * t);
                    if (assign) out[i] = (float)(s * env * gain);
                    else        out[i] += (float)(s * env * gain);
                }
            }
            wrote = 1;
        } else if (src == 2 && bus) { /* noise tap */
            int off = (int)lrintf(p->gen_noise_offset[k]);
            if (off < 0) off = 0;
            if (off >= draws) off = draws - 1;
            for (int i = 0; i < samples; i++) {
                double t = (double)i / (double)sampleRate;
                double s = (double)bus[prelude + i * draws + off];
                s = gen_slot_filter_tick(&filt, s);
                double env = fm * exp(-fr * t) + tm * exp(-tr * t);
                if (assign) out[i] = (float)(s * env * gain);
                else        out[i] += (float)(s * env * gain);
            }
            wrote = 1;
        } else if (src == 3) { /* Karplus-Strong string voice (bass-guitar transplant) */
            modular_gen_slot_ks(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 4) { /* analytic dual-osc voice (sub-bass transplant) */
            modular_gen_slot_analytic(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 5) { /* harmonic-bank kick voice (kick-family transplant) */
            modular_gen_slot_kick(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 6) { /* 808-style tom voice (tom-family transplant) */
            modular_gen_slot_tom(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 7) { /* snare-ish voice (snare/rimshot/sidestick transplant) */
            modular_gen_slot_snare(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 8) { /* clap voice (clap-family transplant) */
            modular_gen_slot_clap(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 9) { /* metallic cymbal voice (hihat/open/cowbell/shaker/ride/crash) */
            modular_gen_slot_cymbal(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        } else if (src == 10) { /* 4-op FM-family voice (bass/bell/lead/epiano/pluck) */
            modular_gen_slot_fm(out, sampleRate, samples, p, k, voice_freq, assign);
            wrote = 1;
        }
    }
    free(bus);
}

/* ── Phase-15 FORMANT stage ────────────────────────────────────────────────
 * Csound Appendix D formant tables: [voice][vowel][formant] for voices
 * soprano/alto/tenor/bass and vowels a/e/i/o/u. Amplitudes are dB relative
 * to F1; bandwidths are -6 dB widths (Q = Fc/BW). */
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

/* Interpolate the 5-band (Fc, linear gain, Q) set at a continuous vowel
 * position v in [0,4], scaled by shift. init!=0 uses mod_biquad_set (fresh
 * state); init==0 uses mod_biquad_set_coeffs (state-preserving — the same
 * block-rate morph pattern as the filter-env sweep in modular.c). */
static void formant_bank_tune(mod_biquad *bank, float *g, int voice_type,
                              float v, float shift, int sampleRate, int init) {
    if (v < 0.0f) v = 0.0f; else if (v > 4.0f) v = 4.0f;
    int   i0 = (int)v; if (i0 > 3) i0 = 3;
    float fr = v - (float)i0;
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
                             int voice_type, float vowel, float mix,
                             float shift, float sing,
                             float morph_rate, float morph_to, float dry) {
    if (mix <= 0.0f || samples <= 0) return;
    if (voice_type < 0) voice_type = 0; else if (voice_type > 3) voice_type = 3;
    if (shift <= 0.0f) shift = 1.0f;
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
    /* Suppress the dry as mix grows (same shaping rationale as the body
     * resonator's dryAmt — the harmonics should move THROUGH the formants,
     * not sit under them). dryAmt(0)=1 → exact bypass at mix 0. Phase-16:
     * `dry` further scales the whole dry-blend amount (1.0 = identity,
     * pre-Phase-16 behavior; 0 = the wet-only formant signal, no dry bleed). */
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
