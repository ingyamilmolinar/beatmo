#include <math.h>
#include <stdlib.h>

#define MA_NO_DEVICE_IO
#define MA_NO_THREADING
#include "miniaudio.h"
#include "synth_params.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

static inline float softsat(float x) {
  // gentle tanh saturation
  return (float)tanh(x * 1.5);
}

#define HH_PARTIALS 6

EXPORT void render_snare(float *out, int sampleRate, int samples) {
  // Snare: acoustic-inspired design with subtle pitched body and layered
  // band-limited noise (head + wires).
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  // Per-hit detune for a bit of natural variation.
  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.02 * (double)seed;

  // Body: two sines roughly around low snare fundamentals. Keep them subtle
  // so the snare does not turn into a synth-bass tone, but allow a hint of
  // pitch for rock punch.
  double fBody1 = 200.0 * detune;  // Slightly higher fundamental
  double fBody2 = 330.0 * detune;  // More separation from tom range
  double phase1 = 0.0, phase2 = 0.0;

  // Biquad coefficients for low and mid/high noise bands (RBJ cookbook).
  double b0_lp, b1_lp, b2_lp, a1_lp, a2_lp;
  double b0_bp1, b1_bp1, b2_bp1, a1_bp1, a2_bp1;
  double b0_bp2, b1_bp2, b2_bp2, a1_bp2, a2_bp2;
  double b0_bp3, b1_bp3, b2_bp3, a1_bp3, a2_bp3;

  // Low band (head noise) ~350 Hz, Q ~0.7
  {
    double fc = 350.0;
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
  // Mid band ~1.8 kHz, Q ~1.0
  {
    double fc = 1800.0;
    double Q = 1.0;
    double w0 = 2.0 * M_PI * fc / (double)sampleRate;
    double cosw = cos(w0);
    double alpha = sin(w0) / (2.0 * Q);
    double b0 = sin(w0) * 0.5; // band-pass (constant skirt gain, peak gain = Q)
    double b1 = 0.0;
    double b2 = -b0;
    double a0 = 1.0 + alpha;
    double a1 = -2.0 * cosw;
    double a2 = 1.0 - alpha;
    b0_bp1 = b0 / a0; b1_bp1 = b1 / a0; b2_bp1 = b2 / a0;
    a1_bp1 = a1 / a0; a2_bp1 = a2 / a0;
  }
  // Higher mid band ~3.2 kHz, Q ~1.0
  {
    double fc = 3200.0;
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
  // Extra high band ~4.5 kHz, Q ~1.0 (very subtle, for crispness).
  {
    double fc = 4500.0;
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

  // Filter state.
  double x1_lp = 0.0, x2_lp = 0.0, y1_lp = 0.0, y2_lp = 0.0;
  double x1_bp1 = 0.0, x2_bp1 = 0.0, y1_bp1 = 0.0, y2_bp1 = 0.0;
  double x1_bp2 = 0.0, x2_bp2 = 0.0, y1_bp2 = 0.0, y2_bp2 = 0.0;
  double x1_bp3 = 0.0, x2_bp3 = 0.0, y1_bp3 = 0.0, y2_bp3 = 0.0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Body oscillators: subtle underlay for weight and punch. Apply a very
    // slight downward pitch drift over time so the hit feels less static.
    double f1Step = fBody1 - 40.0 * sec;
    double f2Step = fBody2 - 60.0 * sec;
    if (f1Step < 140.0) f1Step = 140.0;
    if (f2Step < 180.0) f2Step = 180.0;
    phase1 += 2.0 * M_PI * f1Step / (double)sampleRate;
    phase2 += 2.0 * M_PI * f2Step / (double)sampleRate;
    double bodyTone = sin(phase1) + 0.6 * sin(phase2);
    // Fast decay: body provides punch but fades quickly so noise dominates.
    // This prevents the snare from sounding like a tom.
    double envBody = exp(-sec * 28.0);
    double body = bodyTone * envBody * 0.45;

    // Single white-noise sample for this frame.
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    double x = (double)n;

    // Low band (head noise).
    double ylp = b0_lp * x + b1_lp * x1_lp + b2_lp * x2_lp - a1_lp * y1_lp - a2_lp * y2_lp;
    x2_lp = x1_lp; x1_lp = x;
    y2_lp = y1_lp; y1_lp = ylp;
    double lowNoise = ylp;

    // Mid bands (snare wires) at ~1.8 kHz and ~3.2 kHz.
    double ybp1 = b0_bp1 * x + b1_bp1 * x1_bp1 + b2_bp1 * x2_bp1 - a1_bp1 * y1_bp1 - a2_bp1 * y2_bp1;
    x2_bp1 = x1_bp1; x1_bp1 = x;
    y2_bp1 = y1_bp1; y1_bp1 = ybp1;

    double ybp2 = b0_bp2 * x + b1_bp2 * x1_bp2 + b2_bp2 * x2_bp2 - a1_bp2 * y1_bp2 - a2_bp2 * y2_bp2;
    x2_bp2 = x1_bp2; x1_bp2 = x;
    y2_bp2 = y1_bp2; y1_bp2 = ybp2;

    double ybp3 = b0_bp3 * x + b1_bp3 * x1_bp3 + b2_bp3 * x2_bp3 - a1_bp3 * y1_bp3 - a2_bp3 * y2_bp3;
    x2_bp3 = x1_bp3; x1_bp3 = x;
    y2_bp3 = y1_bp3; y1_bp3 = ybp3;

    // Slight time-varying modulation of wires for organic complexity.
    double lfo = 1.0 + 0.10 * sin(2.0 * M_PI * 13.0 * sec);
    double wiresBand = (ybp1 * 0.9 + ybp2 * 0.6 + ybp3 * 0.2) * lfo;

    // Envelopes: head noise provides sustained thud/weight; wires with a strong
    // crack and longer ring for character. Noise tail outlasts body for snare feel.
    double envLow = exp(-sec * 12.0);          // head provides sustained thud
    double envHighFast = exp(-sec * 200.0);    // very fast crack
    double envHighTail = exp(-sec * 18.0);     // wires ring longer for character

    double noisyBody = lowNoise * envLow * 1.1;
    double wires = wiresBand * (envHighFast * 0.9 + envHighTail * 0.45);

    // Extra front-end aggression for the first few ms to make the snare
    // feel raw and punchy, but keep it less clappy.
    double attackBoost = 1.0 + 0.3 * exp(-sec * 350.0);
    wires *= attackBoost;

    // Final mix: noise-led snare with supportive body. Wires and head noise
    // dominate while body provides initial punch without tom-like tonal dominance.
    double mixed = body * 0.40 + noisyBody * 1.1 + wires * 0.9;
    mixed *= (1.0 - 0.04 * tNorm);

    out[i] = 0.95f * softsat((float)mixed);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_kick(float *out, int sampleRate, int samples) {
  // Kick: low thump with a beater click for presence on small speakers.
  // The click gives audibility even when low-end is rolled off by hardware.

  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  double lpNoise = 0.0;
  double lpClick = 0.0;

  // Slight per-hit variation via random starting phases.
  float r0, r1, r2, r3;
  ma_noise_read_pcm_frames(&noise, &r0, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r1, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r2, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r3, 1, NULL);
  double phase0 = 2.0 * M_PI * (double)r0;
  double phase1 = 2.0 * M_PI * (double)r1;
  double phase2 = 2.0 * M_PI * (double)r2;
  double phase3 = 2.0 * M_PI * (double)r3;

  // Fundamental and three harmonics. Upper harmonics provide audibility
  // on speakers that can't reproduce 55 Hz.
  double f0 = 55.0;       // deep fundamental
  double f1 = f0 * 2.0;   // 110 Hz — body
  double f2 = f0 * 3.0;   // 165 Hz — low-mid presence
  double f3 = f0 * 4.0;   // 220 Hz — audible on all speakers

  for (int i = 0; i < samples; ++i) {
    double tSec  = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Dark, low-passed noise "thud" for rawness.
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lpNoise = lpNoise * 0.97 + (double)n * 0.03;
    double noiseEnv = exp(-16.0 * tSec);
    double noiseThud = lpNoise * noiseEnv * 0.18;

    // Beater click: HP-filtered noise burst, very short (~2-3ms).
    // Gives the kick a sharp transient audible on any speaker.
    float cn;
    ma_noise_read_pcm_frames(&noise, &cn, 1, NULL);
    double clickRaw = (double)cn;
    lpClick = lpClick * 0.85 + clickRaw * 0.15;
    double hpClick = clickRaw - lpClick;
    double clickEnv = exp(-120.0 * tSec);
    double click = hpClick * clickEnv * 0.35;

    // Gentle, short pitch envelope for natural "punch".
    double pitchEnv = exp(-30.0 * tSec);
    double freqMul  = 1.0 + 0.10 * pitchEnv;

    double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
    double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
    double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;
    double step3 = 2.0 * M_PI * f3 * freqMul / (double)sampleRate;

    phase0 += step0;
    phase1 += step1;
    phase2 += step2;
    phase3 += step3;

    double s0 = sin(phase0);
    double s1 = sin(phase1);
    double s2 = sin(phase2);
    double s3 = sin(phase3);

    // Short, punchy envelopes: higher harmonics decay faster.
    double env0 = exp(-5.5  * tSec); // fundamental
    double env1 = exp(-9.0  * tSec); // body
    double env2 = exp(-12.0 * tSec); // low-mid
    double env3 = exp(-18.0 * tSec); // mid presence (short)

    // Balance: fundamental dominates, upper harmonics add presence.
    double tonal =
        s0 * env0 * 0.85 +
        s1 * env1 * 0.40 +
        s2 * env2 * 0.20 +
        s3 * env3 * 0.12;

    // Front-loaded attack boost.
    double attackShape = 1.0 + 0.3 * exp(-40.0 * tSec);
    tonal *= attackShape;

    // Global fade.
    double g = exp(-4.0 * tNorm);

    double mixed = (tonal + noiseThud + click) * g;
    float y = softsat((float)mixed * 0.55f) * 1.2f;
    if (y > 1.0f) y = 1.0f;
    if (y < -1.0f) y = -1.0f;
    out[i] = y;
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_hihat(float *out, int sampleRate, int samples) {
  // Closed hat: 808-style cluster of inharmonic square oscillators plus
  // high-passed noise, shaped into a short metallic tick.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  static const double baseFreq[HH_PARTIALS] = {4100.0, 5400.0, 6700.0, 8300.0, 9900.0, 11800.0};
  double phase[HH_PARTIALS] = {0, 0, 0, 0, 0, 0};
  double freq[HH_PARTIALS];
  double gain[HH_PARTIALS];

  // Per-hit detune and initial phase from noise.
  for (int k = 0; k < HH_PARTIALS; ++k) {
    float s;
    ma_noise_read_pcm_frames(&noise, &s, 1, NULL);
    double det = 1.0 + 0.02 * (double)s;
    freq[k] = baseFreq[k] * det;
    phase[k] = 2.0 * M_PI * (double)s;
    // Slightly lower gain for highest partials.
    gain[k] = (k < 3) ? 1.0 / (double)HH_PARTIALS : 0.75 / (double)HH_PARTIALS;
  }

  // High-pass states for oscillator cluster and noise.
  double lpCluster = 0.0;
  double lpNoise = 0.0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Oscillator cluster: sum of inharmonic square waves.
    double cluster = 0.0;
    for (int k = 0; k < HH_PARTIALS; ++k) {
      phase[k] += 2.0 * M_PI * freq[k] / (double)sampleRate;
      double s = sin(phase[k]);
      double sq = (s >= 0.0) ? 1.0 : -1.0;
      // Tiny slow wobble to avoid static ringing.
      double lfo = 1.0 + 0.10 * sin(2.0 * M_PI * (10.0 + 4.0 * k) * sec);
      cluster += sq * gain[k] * lfo;
    }

    // High-pass the cluster to keep it bright and remove DC/low hum.
    lpCluster = lpCluster * 0.9 + cluster * 0.1;
    double hpCluster = cluster - lpCluster;

    // High-passed white noise for additional sizzle.
    float wn;
    ma_noise_read_pcm_frames(&noise, &wn, 1, NULL);
    double xNoise = (double)wn;
    lpNoise = lpNoise * 0.92 + xNoise * 0.08;
    double hpNoise = xNoise - lpNoise;

    // Fast attack with a longer metallic tail for more body.
    double envFast = exp(-sec * 180.0);
    double envTail = exp(-sec * 35.0);
    double env = envFast * 0.8 + envTail * 0.6;

    // Slightly shorter envelope for the noise component so it does not wash out.
    double noiseEnv = exp(-sec * 100.0);

    double y = hpCluster * env * 0.85 + hpNoise * noiseEnv * 0.45;
    // Keep some crispness but avoid overly aggressive damping.
    y *= (1.0 - 0.15 * tNorm);

    out[i] = 0.9f * softsat((float)y);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_open_hihat(float *out, int sampleRate, int samples) {
  // Open hat: longer, sizzly variant built from the same inharmonic cluster
  // plus a more prominent noise layer. Aimed at an organic house-style open
  // hat with a clear, sustained top end.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  static const double baseFreq[HH_PARTIALS] = {4100.0, 5400.0, 6700.0, 8300.0, 9900.0, 11800.0};
  double phase[HH_PARTIALS] = {0, 0, 0, 0, 0, 0};
  double freq[HH_PARTIALS];
  double gain[HH_PARTIALS];

  // Per-hit detune and initial phase from noise.
  for (int k = 0; k < HH_PARTIALS; ++k) {
    float s;
    ma_noise_read_pcm_frames(&noise, &s, 1, NULL);
    double det = 1.0 + 0.015 * (double)s;
    freq[k] = baseFreq[k] * det;
    phase[k] = 2.0 * M_PI * (double)s;
    // Slightly flatter gain profile so upper partials contribute more to the
    // sizzly ring.
    gain[k] = (k < 3) ? 1.0 / (double)HH_PARTIALS : 0.9 / (double)HH_PARTIALS;
  }

  double lpCluster = 0.0;
  double lpNoise = 0.0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Oscillator cluster: inharmonic squares with a bit of wobble for
    // organic variation.
    double cluster = 0.0;
    for (int k = 0; k < HH_PARTIALS; ++k) {
      phase[k] += 2.0 * M_PI * freq[k] / (double)sampleRate;
      double s = sin(phase[k]);
      double sq = (s >= 0.0) ? 1.0 : -1.0;
      double lfo = 1.0 + 0.12 * sin(2.0 * M_PI * (9.0 + 3.0 * k) * sec);
      cluster += sq * gain[k] * lfo;
    }

    // High-pass the cluster; keep some low-mid but emphasize the top.
    lpCluster = lpCluster * 0.92 + cluster * 0.08;
    double hpCluster = cluster - lpCluster;

    // High-passed white noise for extra sizzle. Use a slower envelope than
    // the closed hat so the noise carries much of the long tail.
    float wn;
    ma_noise_read_pcm_frames(&noise, &wn, 1, NULL);
    double xNoise = (double)wn;
    lpNoise = lpNoise * 0.94 + xNoise * 0.06;
    double hpNoise = xNoise - lpNoise;

    // Envelopes: longer decay for both cluster and noise to approximate an
    // open hat. Noise envelope dominates slightly for a sizzly feel.
    double envClusterFast = exp(-sec * 120.0);
    double envClusterTail = exp(-sec * 22.0);
    double envCluster = envClusterFast * 0.7 + envClusterTail * 0.8;

    double envNoiseFast = exp(-sec * 80.0);
    double envNoiseTail = exp(-sec * 18.0);
    double envNoise = envNoiseFast * 0.7 + envNoiseTail * 1.1;

    double y = hpCluster * envCluster * 0.7 + hpNoise * envNoise * 0.9;
    // Very mild additional damping over the buffer to avoid an overlong tail.
    y *= exp(-1.5 * tNorm);

    out[i] = 0.9f * softsat((float)(y * 1.05));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_tom(float *out, int sampleRate, int samples) {
  // Tom: 808-style organic drum with resonant attack, warm body decay, and
  // subtle room ambience. Fast pitch sweep for punch, bandpass-like stick
  // noise, and ambient tail for organic feel.
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }

  double phaseFund = 0.0;
  double phaseO1 = 0.0;
  double phaseO2 = 0.0;

  // Two-pole lowpass state for bandpass-like stick noise.
  double lpState1 = 0.0, lpState2 = 0.0;
  // Highpass state for mid-band extraction.
  double hpState = 0.0;

  // Slight per-hit tuning variation.
  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.04 * (double)seed;

  // Base tom pitch in a low-mid range with a modest downward sweep.
  double fBase = 150.0 * detune;
  double fStart = fBase * 1.25;
  double fEnd = fBase * 0.85;

  for (int i = 0; i < samples; ++i) {
    double tSec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);

    // Pitch envelope: fast 808-style sweep for punch. The note is audible
    // but quickly decays toward a stable shell resonance.
    double freqFund = fEnd + (fStart - fEnd) * exp(-18.0 * tSec);

    phaseFund += 2.0 * M_PI * freqFund / (double)sampleRate;
    phaseO1 += 2.0 * M_PI * freqFund * 1.5 / (double)sampleRate;
    phaseO2 += 2.0 * M_PI * freqFund * 2.1 / (double)sampleRate;

    double fund = sin(phaseFund);
    double o1 = sin(phaseO1);
    double o2 = sin(phaseO2);

    // Longer decay for more shell ring. Fundamental dominates; overtones
    // decay a bit faster so the tail stays smooth and warm.
    double envFund = exp(-2.8 * tSec);
    double envO1 = exp(-4.0 * tSec);
    double envO2 = exp(-5.5 * tSec);
    double tone = fund * envFund * 1.15 + o1 * envO1 * 0.5 + o2 * envO2 * 0.25;

    // Resonant attack boost: bright transient that fades to warm body.
    double resonantBoost = 1.0 + 0.4 * exp(-80.0 * tSec);
    tone *= resonantBoost;

    // Mid-band focused stick impact using bandpass-like filtering.
    // Two-pole lowpass for smoother stick sound (~400-500 Hz equivalent).
    double lpAlpha = 0.12;
    lpState1 = lpState1 * (1.0 - lpAlpha) + (double)n * lpAlpha;
    lpState2 = lpState2 * (1.0 - lpAlpha) + lpState1 * lpAlpha;
    // Highpass to remove mud and extract mid-band.
    double hpAlpha = 0.92;
    double midNoise = lpState2 - hpState;
    hpState = hpState * hpAlpha + lpState2 * (1.0 - hpAlpha);
    // Stick envelope: longer than before for more presence.
    double attackEnv = exp(-120.0 * tSec);
    double attack = midNoise * attackEnv * 0.35;

    // Subtle pink-ish noise for room ambience (very quiet, extends tail).
    double ambientEnv = exp(-6.0 * tSec);
    double ambient = lpState1 * ambientEnv * 0.08;

    // Global fade so the very end of the buffer is quiet, avoiding a
    // truncation "crack" while preserving a longer, musical ring.
    double global = 1.0 - 0.05 * tNorm;
    if (global < 0.0) global = 0.0;

    out[i] = softsat((float)((tone + attack + ambient) * global));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_tom_high(float *out, int sampleRate, int samples) {
  // High tom (~170 Hz base): punchy, shorter decay, brighter attack.
  // Based on 808 high tom range (165-220 Hz, ~100ms decay).
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }

  double phaseFund = 0.0;
  double phaseO1 = 0.0;
  double phaseO2 = 0.0;

  double lpState1 = 0.0, lpState2 = 0.0;
  double hpState = 0.0;

  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.04 * (double)seed;

  // Higher base pitch for high tom.
  double fBase = 170.0 * detune;
  double fStart = fBase * 1.3;  // Slightly wider sweep for brightness.
  double fEnd = fBase * 0.88;

  for (int i = 0; i < samples; ++i) {
    double tSec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);

    // Very fast pitch sweep for punchy attack.
    double freqFund = fEnd + (fStart - fEnd) * exp(-22.0 * tSec);

    phaseFund += 2.0 * M_PI * freqFund / (double)sampleRate;
    phaseO1 += 2.0 * M_PI * freqFund * 1.5 / (double)sampleRate;
    phaseO2 += 2.0 * M_PI * freqFund * 2.1 / (double)sampleRate;

    double fund = sin(phaseFund);
    double o1 = sin(phaseO1);
    double o2 = sin(phaseO2);

    // Faster decay for tight, punchy high tom.
    double envFund = exp(-3.5 * tSec);
    double envO1 = exp(-5.0 * tSec);
    double envO2 = exp(-6.5 * tSec);
    double tone = fund * envFund * 1.1 + o1 * envO1 * 0.55 + o2 * envO2 * 0.28;

    // Stronger resonant attack for high tom brightness.
    double resonantBoost = 1.0 + 0.5 * exp(-90.0 * tSec);
    tone *= resonantBoost;

    // Mid-band stick impact.
    double lpAlpha = 0.14;  // Slightly brighter for high tom.
    lpState1 = lpState1 * (1.0 - lpAlpha) + (double)n * lpAlpha;
    lpState2 = lpState2 * (1.0 - lpAlpha) + lpState1 * lpAlpha;
    double hpAlpha = 0.90;
    double midNoise = lpState2 - hpState;
    hpState = hpState * hpAlpha + lpState2 * (1.0 - hpAlpha);
    double attackEnv = exp(-140.0 * tSec);
    double attack = midNoise * attackEnv * 0.38;

    // Shorter ambient tail for high tom.
    double ambientEnv = exp(-8.0 * tSec);
    double ambient = lpState1 * ambientEnv * 0.06;

    double global = 1.0 - 0.06 * tNorm;
    if (global < 0.0) global = 0.0;

    out[i] = softsat((float)((tone + attack + ambient) * global));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_tom_low(float *out, int sampleRate, int samples) {
  // Low tom (~90 Hz base): deep, longer decay, warm body.
  // Based on 808 low tom range (80-100 Hz, ~200ms decay).
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }

  double phaseFund = 0.0;
  double phaseO1 = 0.0;
  double phaseO2 = 0.0;

  double lpState1 = 0.0, lpState2 = 0.0;
  double hpState = 0.0;

  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.04 * (double)seed;

  // Lower base pitch for low tom (deeper thud).
  double fBase = 90.0 * detune;
  double fStart = fBase * 1.2;
  double fEnd = fBase * 0.82;

  for (int i = 0; i < samples; ++i) {
    double tSec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);

    // Slightly slower pitch sweep for deeper, warmer feel.
    double freqFund = fEnd + (fStart - fEnd) * exp(-14.0 * tSec);

    phaseFund += 2.0 * M_PI * freqFund / (double)sampleRate;
    phaseO1 += 2.0 * M_PI * freqFund * 1.5 / (double)sampleRate;
    phaseO2 += 2.0 * M_PI * freqFund * 2.1 / (double)sampleRate;

    double fund = sin(phaseFund);
    double o1 = sin(phaseO1);
    double o2 = sin(phaseO2);

    // Longer decay for deep, resonant low tom.
    double envFund = exp(-2.2 * tSec);
    double envO1 = exp(-3.2 * tSec);
    double envO2 = exp(-4.5 * tSec);
    double tone = fund * envFund * 1.2 + o1 * envO1 * 0.45 + o2 * envO2 * 0.22;

    // Moderate resonant attack (less bright than high tom).
    double resonantBoost = 1.0 + 0.35 * exp(-70.0 * tSec);
    tone *= resonantBoost;

    // Mid-band stick impact (slightly darker for low tom).
    double lpAlpha = 0.10;
    lpState1 = lpState1 * (1.0 - lpAlpha) + (double)n * lpAlpha;
    lpState2 = lpState2 * (1.0 - lpAlpha) + lpState1 * lpAlpha;
    double hpAlpha = 0.94;
    double midNoise = lpState2 - hpState;
    hpState = hpState * hpAlpha + lpState2 * (1.0 - hpAlpha);
    double attackEnv = exp(-100.0 * tSec);
    double attack = midNoise * attackEnv * 0.32;

    // Longer ambient tail for depth and room feel.
    double ambientEnv = exp(-4.0 * tSec);
    double ambient = lpState1 * ambientEnv * 0.10;

    double global = 1.0 - 0.04 * tNorm;
    if (global < 0.0) global = 0.0;

    out[i] = softsat((float)((tone + attack + ambient) * global));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_clap(float *out, int sampleRate, int samples) {
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)sampleRate;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    // Clustered bursts to approximate palms plus a short room tail.
    double burst =
        exp(-110.0 * fabs(t - 0.000)) +
        exp(-110.0 * fabs(t - 0.020)) +
        exp(-110.0 * fabs(t - 0.040)) +
        0.7 * exp(-80.0 * fabs(t - 0.075));
    double env = exp(-7.0 * t);
    out[i] = softsat((float)(n * burst * env));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_cowbell(float *out, int sampleRate, int samples) {
  // Cowbell: salsa-style metallic cowbell built from several inharmonic
  // partials plus a short noise impact.
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.01 * (double)seed;

  // Base partial frequencies (roughly cowbell-ish region).
  const double freqs[4] = {
      640.0 * detune,
      920.0 * detune,
      1300.0 * detune,
      1900.0 * detune,
  };
  const double gains[4] = {1.0, 0.85, 0.6, 0.4};
  double phase[4] = {0, 0, 0, 0};

  double lpNoise = 0.0;
  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);

    // Body: sum of a few inharmonic partials that ring with moderate decay.
    double tone = 0.0;
    for (int k = 0; k < 4; ++k) {
      phase[k] += 2.0 * M_PI * freqs[k] / (double)sampleRate;
      tone += sin(phase[k]) * gains[k];
    }
    // Use a relatively slow decay so the cowbell rings; rely on the pattern
    // (not voice truncation) to stop the sound.
    double envTone = exp(-sec * 9.0);

    // Short, mid-focused noise impact at the front and a faint metallic rasp.
    lpNoise = lpNoise * 0.9 + n * 0.1;
    double impactEnv = exp(-sec * 260.0);
    double impact = (double)lpNoise * impactEnv * 0.55;
    // Subtle high-passed noise tail for extra metal, kept low to avoid hiss.
    double hpMetal = (double)n - lpNoise;
    double metalEnv = exp(-sec * 60.0);
    double metal = hpMetal * metalEnv * 0.25;

    // Gentle additional fade so we don't abruptly cut a very loud tail.
    double global = 1.0 - 0.06 * tNorm;
    if (global < 0.0) global = 0.0;

    double mixed = (tone * envTone + impact + metal) * global;
    out[i] = 0.95f * softsat((float)mixed);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_snare_rimshot(float *out, int sampleRate, int samples) {
  // Rimshot: 808-inspired inharmonic resonator with heavy saturation.
  // 4 damped sinusoids at inharmonic ratios + loud HP noise transient
  // → high-pass (~400 Hz) → hard tanh drive → softsat for raw metallic crunch.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.02 * (double)seed;

  // 4 inharmonic partials — ratios 1 : 2.1 : 3.4 : 5.6 (fixed, no drift).
  double f1 = 500.0 * detune;
  double f2 = 1050.0 * detune;
  double f3 = 1700.0 * detune;
  double f4 = 2800.0 * detune;
  double phase1 = 0.0, phase2 = 0.0, phase3 = 0.0, phase4 = 0.0;

  // 1st-order high-pass (~400 Hz) to strip warmth, keep only metallic ring.
  double rc = 1.0 / (2.0 * M_PI * 400.0);
  double dt = 1.0 / (double)sampleRate;
  double hpAlpha = rc / (rc + dt);
  double hpPrev = 0.0, hpOut = 0.0;

  // 1st-order LP coefficient for transient noise HP (white - LP = HP).
  double lpAlpha = dt / (1.0 / (2.0 * M_PI * 5000.0) + dt);
  double lpState = 0.0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // --- 4 inharmonic damped sinusoids ---
    phase1 += 2.0 * M_PI * f1 / (double)sampleRate;
    phase2 += 2.0 * M_PI * f2 / (double)sampleRate;
    phase3 += 2.0 * M_PI * f3 / (double)sampleRate;
    phase4 += 2.0 * M_PI * f4 / (double)sampleRate;
    double p1 = sin(phase1) * 1.0  * exp(-40.0 * sec);
    double p2 = sin(phase2) * 0.95 * exp(-50.0 * sec);
    double p3 = sin(phase3) * 0.8  * exp(-65.0 * sec);
    double p4 = sin(phase4) * 0.5  * exp(-90.0 * sec);
    double tones = p1 + p2 + p3 + p4;

    // --- HP noise transient (louder crack, ~5ms) ---
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lpState += lpAlpha * ((double)n - lpState);
    double hpNoise = (double)n - lpState; // white - LP = HP
    double transient = hpNoise * 0.7 * exp(-200.0 * sec);

    // --- Sharp attack spike (~1ms) for initial hit impact ---
    double attack = 1.0 + 2.0 * exp(-1500.0 * sec);

    double raw = (tones + transient) * attack;

    // --- 1st-order high-pass (~400 Hz) ---
    hpOut = hpAlpha * (hpOut + raw - hpPrev);
    hpPrev = raw;

    // --- Hard tanh drive + softsat for raw metallic crunch ---
    double driven = tanh(hpOut * 4.0);
    double global = 1.0 - 0.1 * tNorm;
    if (global < 0.0) global = 0.0;

    out[i] = 0.95f * softsat((float)(driven * global));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_snare_sidestick(float *out, int sampleRate, int samples) {
  // Cross-stick: dry, woody click — purely transient, no pitch movement.
  // 2 fixed-frequency inharmonic damped sines + low-Q bandpass noise.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.02 * (double)seed;

  // 2 inharmonic tones at ratio 1 : 2.4 (fixed frequencies, no sweep).
  double fWood = 500.0 * detune;
  double fRim = 1200.0 * detune;
  double phaseWood = 0.0, phaseRim = 0.0;

  // Low-Q bandpass at 1500 Hz for woody mid character (Q=0.7, not ringy).
  double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
  {
    double fc = 1500.0; double Q = 0.7;
    double w0 = 2.0 * M_PI * fc / (double)sampleRate;
    double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
    double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
    double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
    b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0; a1_bp = a1/a0; a2_bp = a2/a0;
  }
  double x1_bp = 0, x2_bp = 0, y1_bp = 0, y2_bp = 0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;

    // Fixed-frequency damped sines.
    phaseWood += 2.0 * M_PI * fWood / (double)sampleRate;
    phaseRim += 2.0 * M_PI * fRim / (double)sampleRate;
    double wood = sin(phaseWood) * 0.5 * exp(-100.0 * sec);
    double rim = sin(phaseRim) * 0.45 * exp(-120.0 * sec);

    // Low-Q bandpass noise.
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    double x = (double)n;
    double ybp = b0_bp*x + b1_bp*x1_bp + b2_bp*x2_bp - a1_bp*y1_bp - a2_bp*y2_bp;
    x2_bp = x1_bp; x1_bp = x; y2_bp = y1_bp; y1_bp = ybp;
    double noise_out = ybp * 0.5 * exp(-150.0 * sec);

    double attackBoost = 1.0 + 1.0 * exp(-800.0 * sec);
    double mixed = (wood + rim + noise_out) * attackBoost;

    out[i] = 0.95f * softsat((float)mixed);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_kick_deep(float *out, int sampleRate, int samples) {
  // 808-style deep sub kick: lower, boomier, longer sustain.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }
  double lpNoise = 0.0;

  // Start phase at Pi/2 for maximum punch on first sample.
  double phase0 = M_PI * 0.5;
  double phase1 = M_PI * 0.5;

  double f0 = 42.0;       // deep fundamental (lower than standard kick's 55)
  double f1 = f0 * 2.0;   // subtle 2nd harmonic

  for (int i = 0; i < samples; ++i) {
    double tSec  = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lpNoise = lpNoise * 0.97 + (double)n * 0.03;
    double noiseEnv = exp(-20.0 * tSec);
    double noiseThud = lpNoise * noiseEnv * 0.10;

    // Wider pitch sweep, slower decay than standard kick.
    double pitchEnv = exp(-15.0 * tSec);
    double freqMul  = 1.0 + 0.15 * pitchEnv;

    double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
    double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;

    phase0 += step0;
    phase1 += step1;

    double s0 = sin(phase0);
    double s1 = sin(phase1);

    // Much longer decay for the sub tail.
    double env0 = exp(-3.5 * tSec);
    double env1 = exp(-7.0 * tSec);

    double tonal = s0 * env0 * 0.95 + s1 * env1 * 0.15;

    // Gentler attack boost.
    double attackShape = 1.0 + 0.15 * exp(-50.0 * tSec);
    tonal *= attackShape;

    double g = exp(-3.0 * tNorm);
    double mixed = (tonal + noiseThud) * g;

    // Clean sub with mild warmth.
    float y = (float)(tanh(mixed * 1.2) * 0.95);
    if (y > 1.0f) y = 1.0f;
    if (y < -1.0f) y = -1.0f;
    out[i] = y;
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_kick_punchy(float *out, int sampleRate, int samples) {
  // Kick-1 "Punchy/909-style": short, snappy, cuts through a mix.
  // Higher fundamental (62 Hz), aggressive pitch sweep, loud beater click,
  // no noise thud, stronger saturation. Clean electronic character.
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }
  double lpClick = 0.0;

  float r0, r1;
  ma_noise_read_pcm_frames(&noise, &r0, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r1, 1, NULL);
  double phase0 = 2.0 * M_PI * (double)r0;
  double phase1 = 2.0 * M_PI * (double)r1;

  double f0 = 62.0;       // higher fundamental for presence
  double f1 = f0 * 2.0;   // 124 Hz — body

  for (int i = 0; i < samples; ++i) {
    double tSec  = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Beater click: brighter HP filter, louder.
    float cn;
    ma_noise_read_pcm_frames(&noise, &cn, 1, NULL);
    double clickRaw = (double)cn;
    lpClick = lpClick * 0.80 + clickRaw * 0.20;
    double hpClick = clickRaw - lpClick;
    double clickEnv = exp(-120.0 * tSec);
    double click = hpClick * clickEnv * 0.45;

    // Aggressive pitch envelope: +25% sweep, fast decay.
    double pitchEnv = exp(-55.0 * tSec);
    double freqMul  = 1.0 + 0.25 * pitchEnv;

    double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
    double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;

    phase0 += step0;
    phase1 += step1;

    double s0 = sin(phase0);
    double s1 = sin(phase1);

    // Fast body decay: only 2 harmonics for clean electronic character.
    double env0 = exp(-8.0  * tSec);
    double env1 = exp(-14.0 * tSec);

    double tonal = s0 * env0 * 0.85 + s1 * env1 * 0.40;

    // Stronger attack boost.
    double attackShape = 1.0 + 0.5 * exp(-60.0 * tSec);
    tonal *= attackShape;

    // Faster global fade.
    double g = exp(-5.0 * tNorm);

    double mixed = (tonal + click) * g;
    // More aggressive saturation for density.
    float y = softsat((float)mixed * 0.8f) * 1.3f;
    if (y > 1.0f) y = 1.0f;
    if (y < -1.0f) y = -1.0f;
    out[i] = y;
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_kick_lofi(float *out, int sampleRate, int samples) {
  // Kick-2 "Lo-fi/Warm/Vintage": warm, gritty, thick.
  // Lower fundamental (50 Hz), gentle pitch sweep, no beater click,
  // thick noise thud, bit reduction, double saturation, built-in LP darkening.
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }
  double lpNoise = 0.0;

  float r0, r1, r2;
  ma_noise_read_pcm_frames(&noise, &r0, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r1, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r2, 1, NULL);
  double phase0 = 2.0 * M_PI * (double)r0;
  double phase1 = 2.0 * M_PI * (double)r1;
  double phase2 = 2.0 * M_PI * (double)r2;

  double f0 = 50.0;       // lower, more sub-focused
  double f1 = f0 * 2.0;   // 100 Hz
  double f2 = f0 * 3.0;   // 150 Hz

  // One-pole LP state for final darkening (~600 Hz).
  double lpOut = 0.0;
  double lpAlpha = 2.0 * M_PI * 600.0 / (double)sampleRate;
  if (lpAlpha > 1.0) lpAlpha = 1.0;

  for (int i = 0; i < samples; ++i) {
    double tSec  = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Thick noise thud: less filtered, louder.
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lpNoise = lpNoise * 0.96 + (double)n * 0.04;
    double noiseEnv = exp(-12.0 * tSec);
    double noiseThud = lpNoise * noiseEnv * 0.25;

    // Gentle pitch envelope: +8% sweep, slow decay.
    double pitchEnv = exp(-20.0 * tSec);
    double freqMul  = 1.0 + 0.08 * pitchEnv;

    double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
    double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
    double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;

    phase0 += step0;
    phase1 += step1;
    phase2 += step2;

    double s0 = sin(phase0);
    double s1 = sin(phase1);
    double s2 = sin(phase2);

    // Slower body decay for fullness.
    double env0 = exp(-4.5  * tSec);
    double env1 = exp(-7.0  * tSec);
    double env2 = exp(-10.0 * tSec);

    double tonal =
        s0 * env0 * 0.85 +
        s1 * env1 * 0.40 +
        s2 * env2 * 0.20;

    // Mild attack boost.
    double attackShape = 1.0 + 0.2 * exp(-35.0 * tSec);
    tonal *= attackShape;

    // Slower global fade (more sustain).
    double g = exp(-3.0 * tNorm);

    double mixed = (tonal + noiseThud) * g;

    // Built-in bit reduction: quantize to ~128 levels (7-bit) for grit.
    double levels = 128.0;
    mixed = floor(mixed * levels + 0.5) / levels;

    // Double saturation for harmonic warmth.
    mixed = tanh(tanh(mixed * 1.5) * 1.8);

    // Built-in one-pole LP to darken the sound (~600 Hz).
    lpOut += lpAlpha * (mixed - lpOut);
    mixed = lpOut;

    float y = (float)mixed * 0.95f;
    if (y > 1.0f) y = 1.0f;
    if (y < -1.0f) y = -1.0f;
    out[i] = y;
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_kick_tight(float *out, int sampleRate, int samples) {
  // Kick-tight "Acoustic/Studio": natural, controlled, defined.
  // Medium fundamental (58 Hz), minimal pitch sweep, bandpass beater transient,
  // brief room thump, built-in gate, minimal saturation.
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  float r0, r1, r2;
  ma_noise_read_pcm_frames(&noise, &r0, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r1, 1, NULL);
  ma_noise_read_pcm_frames(&noise, &r2, 1, NULL);
  double phase0 = 2.0 * M_PI * (double)r0;
  double phase1 = 2.0 * M_PI * (double)r1;
  double phase2 = 2.0 * M_PI * (double)r2;

  double f0 = 58.0;       // between deep and punchy
  double f1 = f0 * 2.0;   // 116 Hz
  double f2 = f0 * 3.0;   // 174 Hz

  // RBJ biquad bandpass at 2.5 kHz (Q=0.7) for natural beater transient.
  double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
  {
    double fc = 2500.0; double Q = 0.7;
    double w0 = 2.0 * M_PI * fc / (double)sampleRate;
    double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
    double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
    double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
    b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0; a1_bp = a1/a0; a2_bp = a2/a0;
  }
  double x1_bp = 0, x2_bp = 0, y1_bp = 0, y2_bp = 0;

  // RBJ biquad bandpass at 200 Hz (Q=0.5) for brief room thump.
  double b0_rm, b1_rm, b2_rm, a1_rm, a2_rm;
  {
    double fc = 200.0; double Q = 0.5;
    double w0 = 2.0 * M_PI * fc / (double)sampleRate;
    double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
    double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
    double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
    b0_rm = b0/a0; b1_rm = b1/a0; b2_rm = b2/a0; a1_rm = a1/a0; a2_rm = a2/a0;
  }
  double x1_rm = 0, x2_rm = 0, y1_rm = 0, y2_rm = 0;

  for (int i = 0; i < samples; ++i) {
    double tSec  = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    double x = (double)n;

    // Beater transient: bandpass noise at 2.5 kHz for natural "stick on head" sound.
    double ybp = b0_bp*x + b1_bp*x1_bp + b2_bp*x2_bp - a1_bp*y1_bp - a2_bp*y2_bp;
    x2_bp = x1_bp; x1_bp = x; y2_bp = y1_bp; y1_bp = ybp;
    double beaterEnv = exp(-150.0 * tSec);
    double beater = ybp * beaterEnv * 0.40;

    // Brief room thump: bandpass noise at 200 Hz, very fast decay.
    double yrm = b0_rm*x + b1_rm*x1_rm + b2_rm*x2_rm - a1_rm*y1_rm - a2_rm*y2_rm;
    x2_rm = x1_rm; x1_rm = x; y2_rm = y1_rm; y1_rm = yrm;
    double roomEnv = exp(-80.0 * tSec);
    double room = yrm * roomEnv * 0.15;

    // Minimal pitch envelope: +5%, ultra-fast decay.
    double pitchEnv = exp(-65.0 * tSec);
    double freqMul  = 1.0 + 0.05 * pitchEnv;

    double step0 = 2.0 * M_PI * f0 * freqMul / (double)sampleRate;
    double step1 = 2.0 * M_PI * f1 * freqMul / (double)sampleRate;
    double step2 = 2.0 * M_PI * f2 * freqMul / (double)sampleRate;

    phase0 += step0;
    phase1 += step1;
    phase2 += step2;

    double s0 = sin(phase0);
    double s1 = sin(phase1);
    double s2 = sin(phase2);

    // Fast body decay for tight, controlled sound.
    double env0 = exp(-7.5  * tSec);
    double env1 = exp(-12.0 * tSec);
    double env2 = exp(-17.0 * tSec);

    double tonal =
        s0 * env0 * 0.85 +
        s1 * env1 * 0.35 +
        s2 * env2 * 0.15;

    // Moderate attack boost.
    double attackShape = 1.0 + 0.3 * exp(-50.0 * tSec);
    tonal *= attackShape;

    // Built-in gate: envelope goes to 0 after ~45% of buffer.
    double gate = 1.0;
    if (tNorm > 0.45) {
      gate = exp(-12.0 * (tNorm - 0.45));
    }

    // Global fade.
    double g = exp(-4.5 * tNorm);

    double mixed = (tonal + beater + room) * g * gate;
    // Minimal saturation — clean, natural.
    float y = softsat((float)mixed * 0.45f) * 1.1f;
    if (y > 1.0f) y = 1.0f;
    if (y < -1.0f) y = -1.0f;
    out[i] = y;
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_shaker(float *out, int sampleRate, int samples) {
  // Shaker: noise-based with grain-like micro-bursts for rhythmic texture.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  // Per-hit timing variation seed.
  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double timeVar = 0.001 * (double)seed; // ±1ms variation

  // Staggered burst offsets (seconds).
  double burstOff[4] = { 0.000, 0.004, 0.009, 0.015 };
  const double burstAmp[4] = { 1.0, 0.85, 0.7, 0.5 };
  for (int k = 0; k < 4; ++k) {
    burstOff[k] += timeVar;
  }

  // Bandpass biquad at 8000 Hz, Q=0.7 for shimmer emphasis.
  double b0_bp, b1_bp, b2_bp, a1_bp, a2_bp;
  {
    double fc = 8000.0; double Q = 0.7;
    double w0 = 2.0 * M_PI * fc / (double)sampleRate;
    double cosw = cos(w0); double alpha = sin(w0) / (2.0 * Q);
    double b0 = sin(w0) * 0.5; double b1 = 0.0; double b2 = -b0;
    double a0 = 1.0 + alpha; double a1 = -2.0 * cosw; double a2 = 1.0 - alpha;
    b0_bp = b0/a0; b1_bp = b1/a0; b2_bp = b2/a0; a1_bp = a1/a0; a2_bp = a2/a0;
  }
  double x1 = 0, x2 = 0, y1 = 0, y2 = 0;

  // One-pole HP state for brightness (~5 kHz).
  double hpState = 0.0;
  double hpAlpha = 0.85;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;

    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    double x = (double)n;

    // High-pass the noise for crispness.
    hpState = hpState * hpAlpha + x * (1.0 - hpAlpha);
    double hp = x - hpState;

    // Bandpass for shimmer.
    double ybp = b0_bp*x + b1_bp*x1 + b2_bp*x2 - a1_bp*y1 - a2_bp*y2;
    x2 = x1; x1 = x; y2 = y1; y1 = ybp;

    // Sum micro-burst envelopes.
    double burst = 0.0;
    for (int k = 0; k < 4; ++k) {
      burst += burstAmp[k] * exp(-200.0 * fabs(sec - burstOff[k]));
    }

    double overall = exp(-25.0 * sec);
    double mixed = (hp * 0.6 + ybp * 0.5) * burst * overall;

    out[i] = 0.95f * softsat((float)mixed);
  }
  ma_noise_uninit(&noise, NULL);
}

#define RIDE_PARTIALS 10

EXPORT void render_ride(float *out, int sampleRate, int samples) {
  // Ride cymbal: bright metallic ring with bell-like tone.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  static const double baseFreq[RIDE_PARTIALS] = {
    3100.0, 3800.0, 4700.0, 5900.0, 7100.0,
    8400.0, 9800.0, 11500.0, 13200.0, 15000.0
  };
  static const double baseGain[RIDE_PARTIALS] = {
    1.0, 1.0, 0.9, 0.8, 0.7, 0.6, 0.5, 0.45, 0.4, 0.35
  };

  double phase[RIDE_PARTIALS];
  double freq[RIDE_PARTIALS];
  double gain[RIDE_PARTIALS];

  for (int k = 0; k < RIDE_PARTIALS; ++k) {
    float s;
    ma_noise_read_pcm_frames(&noise, &s, 1, NULL);
    double det = 1.0 + 0.015 * (double)s;
    freq[k] = baseFreq[k] * det;
    phase[k] = 2.0 * M_PI * (double)s;
    gain[k] = baseGain[k] / (double)RIDE_PARTIALS;
  }

  // Bell component: extra sine at ~3000 Hz with slower decay.
  double bellPhase = 0.0;
  float bs;
  ma_noise_read_pcm_frames(&noise, &bs, 1, NULL);
  double bellFreq = 3000.0 * (1.0 + 0.01 * (double)bs);

  // HP state for cluster.
  double lpCluster = 0.0;
  double lpNoise = 0.0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Sine partial cluster (softer than hi-hat's squares).
    double cluster = 0.0;
    for (int k = 0; k < RIDE_PARTIALS; ++k) {
      phase[k] += 2.0 * M_PI * freq[k] / (double)sampleRate;
      cluster += sin(phase[k]) * gain[k];
    }

    // HP the cluster.
    lpCluster = lpCluster * 0.92 + cluster * 0.08;
    double hpCluster = cluster - lpCluster;

    // Bell ping with slow decay.
    bellPhase += 2.0 * M_PI * bellFreq / (double)sampleRate;
    double bellEnv = exp(-6.0 * sec);
    double bell = sin(bellPhase) * bellEnv * 0.15;

    // Cluster envelope: fast initial + long tail.
    double envFast = exp(-40.0 * sec);
    double envTail = exp(-8.0 * sec);
    double env = envFast * 0.5 + envTail * 0.8;

    // HP white noise for wash.
    float wn;
    ma_noise_read_pcm_frames(&noise, &wn, 1, NULL);
    double xNoise = (double)wn;
    lpNoise = lpNoise * 0.92 + xNoise * 0.08;
    double hpNoise = xNoise - lpNoise;
    double noiseEnv = exp(-12.0 * sec);

    // LFO for organic shimmer.
    double lfo = 1.0 + 0.06 * sin(2.0 * M_PI * 7.0 * sec);

    double y = (hpCluster * env + bell) * lfo + hpNoise * noiseEnv * 0.3;
    y *= exp(-2.0 * tNorm);

    out[i] = 0.9f * softsat((float)y);
  }
  ma_noise_uninit(&noise, NULL);
}

#define CRASH_PARTIALS 8

EXPORT void render_crash(float *out, int sampleRate, int samples) {
  // Crash cymbal: darker, washy cousin of the ride. Tonal bell anchor with
  // tapered partials and restrained noise wash for a musical, non-harsh sound.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  // Darker frequencies than ride (ride starts at 3100).
  static const double baseFreq[CRASH_PARTIALS] = {
    2000.0, 2800.0, 3700.0, 4800.0, 6200.0, 7800.0, 9500.0, 11500.0
  };
  // Tapered gain: emphasize mid partials (3-6 kHz), reduce lowest and highest.
  static const double baseGain[CRASH_PARTIALS] = {
    0.6, 0.9, 1.0, 1.0, 0.85, 0.65, 0.45, 0.3
  };

  double phase[CRASH_PARTIALS];
  double freq[CRASH_PARTIALS];
  double gain[CRASH_PARTIALS];

  for (int k = 0; k < CRASH_PARTIALS; ++k) {
    float s;
    ma_noise_read_pcm_frames(&noise, &s, 1, NULL);
    double det = 1.0 + 0.02 * (double)s;
    freq[k] = baseFreq[k] * det;
    phase[k] = 2.0 * M_PI * (double)s;
    gain[k] = baseGain[k] / (double)CRASH_PARTIALS;
  }

  // Bell component at ~2500 Hz (darker than ride's 3000 Hz bell).
  double bellPhase = 0.0;
  float bs;
  ma_noise_read_pcm_frames(&noise, &bs, 1, NULL);
  double bellFreq = 2500.0 * (1.0 + 0.01 * (double)bs);

  double lpCluster = 0.0;
  double lpNoise = 0.0;

  for (int i = 0; i < samples; ++i) {
    double sec = (double)i / (double)sampleRate;
    double tNorm = (double)i / (double)samples;

    // Sine partial cluster.
    double cluster = 0.0;
    for (int k = 0; k < CRASH_PARTIALS; ++k) {
      phase[k] += 2.0 * M_PI * freq[k] / (double)sampleRate;
      cluster += sin(phase[k]) * gain[k];
    }

    // HP the cluster.
    lpCluster = lpCluster * 0.93 + cluster * 0.07;
    double hpCluster = cluster - lpCluster;

    // Bell ping with moderate decay (slower than ride for longer ring).
    bellPhase += 2.0 * M_PI * bellFreq / (double)sampleRate;
    double bellEnv = exp(-4.0 * sec);
    double bell = sin(bellPhase) * bellEnv * 0.18;

    // Long cluster decay: crash rings longer than ride.
    double envFast = exp(-10.0 * sec);
    double envTail = exp(-3.0 * sec);
    double env = envFast * 0.5 + envTail * 0.9;

    // HP white noise for wash — restrained, not dominant.
    float wn;
    ma_noise_read_pcm_frames(&noise, &wn, 1, NULL);
    double xNoise = (double)wn;
    lpNoise = lpNoise * 0.93 + xNoise * 0.07;
    double hpNoise = xNoise - lpNoise;
    double noiseEnv = exp(-8.0 * sec);

    // LFO for organic shimmer.
    double lfo = 1.0 + 0.06 * sin(2.0 * M_PI * 5.0 * sec);

    double y = (hpCluster * env + bell) * lfo + hpNoise * noiseEnv * 0.35;
    y *= exp(-1.5 * tNorm);

    out[i] = 0.9f * softsat((float)y);
  }
  ma_noise_uninit(&noise, NULL);
}

// Decode an audio file (WAV/MP3/FLAC/OGG where enabled) into mono f32 at the
// engine sample rate. Returns frame count or a negative ma_result code.
EXPORT int load_audio(const char *path, int targetSampleRate, float **buffer, int *sampleRate) {
  const ma_uint32 targetRate = (ma_uint32)targetSampleRate;
  ma_decoder_config cfg = ma_decoder_config_init(ma_format_f32, 1, targetRate);
  ma_decoder dec;
  ma_result res = ma_decoder_init_file(path, &cfg, &dec);
  if (res != MA_SUCCESS) {
    return res;
  }
  ma_uint64 frames;
  res = ma_decoder_get_length_in_pcm_frames(&dec, &frames);
  if (res != MA_SUCCESS) {
    ma_decoder_uninit(&dec);
    return res;
  }
  float *data = (float *)malloc(frames * sizeof(float));
  if (data == NULL) {
    ma_decoder_uninit(&dec);
    return MA_OUT_OF_MEMORY;
  }
  res = ma_decoder_read_pcm_frames(&dec, data, frames, NULL);
  if (res != MA_SUCCESS) {
    free(data);
    ma_decoder_uninit(&dec);
    return res;
  }
  *buffer = data;
  if (sampleRate != NULL)
    *sampleRate = dec.outputSampleRate;
  ma_decoder_uninit(&dec);
  return (int)frames;
}

// Backward-compat shim retained for existing exports.
EXPORT int load_wav(const char *path, int targetSampleRate, float **buffer, int *sampleRate) {
  return load_audio(path, targetSampleRate, buffer, sampleRate);
}

// Bass Guitar: Karplus-Strong plucked string synthesis with finger pluck transient.
// Base pitch ~55 Hz (A1), pitchable via playback rate.
EXPORT void render_bass_guitar(float *out, int sampleRate, int samples) {
  // Karplus-Strong delay buffer size for base frequency ~55 Hz.
  double baseFreq = 55.0;
  int delayLen = (int)((double)sampleRate / baseFreq + 0.5);
  if (delayLen < 2) delayLen = 2;
  if (delayLen > 4096) delayLen = 4096;

  // Use local buffer to prevent cross-call contamination that causes echo.
  // Previously a static buffer only cleared delayLen elements, leaving stale
  // data that caused reverb/echo artifacts.
  float delayBuf[4096];
  for (int i = 0; i < 4096; i++) delayBuf[i] = 0.0f;

  // Initialize with noise generator for randomness.
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    for (int i = 0; i < samples; i++) out[i] = 0.0f;
    return;
  }

  // Per-hit variation seed.
  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double detune = 1.0 + 0.005 * (double)seed; // Slight organic variation
  (void)detune; // Applied through playback rate in Go/JS

  // Fill delay buffer with filtered noise (simulates pluck position).
  // Use a simple lowpass on the noise for a rounder pluck.
  double lpState = 0.0;
  double lpAlpha = 0.35; // Fairly soft pluck
  for (int i = 0; i < delayLen; i++) {
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lpState = lpState * (1.0 - lpAlpha) + (double)n * lpAlpha;
    delayBuf[i] = (float)lpState * 0.9f;
  }

  // Decay factor: controls how long the string rings.
  // Higher = longer sustain. ~0.996 gives a nice bass guitar decay.
  double decayFactor = 0.996;

  // Stretch factor for slight pitch stability (Karplus-Strong extension).
  double stretchProb = 0.5; // Probability of delaying the filter step

  int readPtr = 0;
  double prev = delayBuf[0];

  // Attack transient: short noise burst for finger/pick sound.
  int attackSamples = (int)(0.008 * (double)sampleRate); // ~8ms
  if (attackSamples > samples) attackSamples = samples;

  for (int i = 0; i < samples; i++) {
    double tSec = (double)i / (double)sampleRate;

    // Read current sample from delay line.
    double current = (double)delayBuf[readPtr];

    // Karplus-Strong loop filter: average with previous sample.
    // Apply stretch factor via probabilistic delay.
    double filtered;
    float rnd;
    ma_noise_read_pcm_frames(&noise, &rnd, 1, NULL);
    if ((double)rnd > stretchProb) {
      // Standard filter: y[n] = 0.5 * (y[n] + y[n-1])
      filtered = 0.5 * (current + prev) * decayFactor;
    } else {
      // Stretch: don't filter this sample (preserves high frequencies slightly longer)
      filtered = current * decayFactor;
    }
    prev = current;

    // Write filtered value back to delay line.
    delayBuf[readPtr] = (float)filtered;
    readPtr = (readPtr + 1) % delayLen;

    // Add finger pluck transient at the start.
    double attack = 0.0;
    if (i < attackSamples) {
      float atkNoise;
      ma_noise_read_pcm_frames(&noise, &atkNoise, 1, NULL);
      // Bandpass-like: simple lowpass on noise for finger thump.
      double atkEnv = exp(-tSec * 400.0); // Very fast decay
      attack = (double)atkNoise * atkEnv * 0.25;
    }

    // Global amplitude envelope to ensure clean tail-off.
    double envGlobal = exp(-tSec * 1.8);

    double mixed = (current + attack) * envGlobal;

    // Gentle saturation for warmth.
    out[i] = softsat((float)mixed * 0.85f);
  }

  ma_noise_uninit(&noise, NULL);
}

// Sub Bass: 808-style deep sine bass with pitch envelope for punch.
// Base pitch ~45 Hz, starts at +Pi/2 phase for maximum attack.
EXPORT void render_sub_bass(float *out, int sampleRate, int samples) {
  // Base frequency: deep sub territory (~45 Hz, between E1 and A1).
  double baseFreq = 45.0;

  // Start phase at +Pi/2 so sine starts at maximum amplitude (punchy attack).
  double phase = M_PI * 0.5;

  // Optional: add very quiet 2nd harmonic for presence on small speakers.
  double phase2 = M_PI * 0.5;
  double harmonicMix = 0.08; // Very subtle

  for (int i = 0; i < samples; i++) {
    double tSec = (double)i / (double)sampleRate;

    // Pitch envelope: starts higher, quickly settles to base frequency.
    // This gives the 808-style "punch" at the attack.
    double pitchEnv = exp(-40.0 * tSec);
    double freq = baseFreq * (1.0 + 0.15 * pitchEnv);

    // Phase increment for fundamental.
    double phaseInc = 2.0 * M_PI * freq / (double)sampleRate;
    phase += phaseInc;
    if (phase > 2.0 * M_PI) phase -= 2.0 * M_PI;

    // Fundamental sine.
    double fund = sin(phase);

    // 2nd harmonic (octave up) for speaker presence.
    phase2 += 2.0 * phaseInc; // Double frequency
    if (phase2 > 2.0 * M_PI) phase2 -= 2.0 * M_PI;
    double harm = sin(phase2);

    // Amplitude envelope: very slow decay for sustained sub.
    // The 808 sub is famous for its long, clean sustain.
    double ampEnv = exp(-2.0 * tSec);

    // Slight attack boost for extra punch in first few ms.
    double attackBoost = 1.0 + 0.2 * exp(-60.0 * tSec);

    // Mix fundamental and harmonic.
    double mixed = (fund + harm * harmonicMix) * ampEnv * attackBoost;

    // Very gentle saturation (lower drive than drums) to round peaks.
    // Using tanh with lower multiplier than softsat for cleaner sub.
    double sat = tanh(mixed * 1.2);

    out[i] = (float)sat * 0.95f;
  }
}

/* ==== Parameterized render variants ====
 * Each _p() function accepts an optional synth_params pointer.
 * When params is NULL, behavior is identical to the original function.
 * Params modify pitch, decay, tone, attack, drive, body, brightness. */

EXPORT void render_snare_p(float *out, int sampleRate, int samples,
                           const synth_params *params) {
  /* Render default snare first. */
  render_snare(out, sampleRate, samples);
  if (!params) return;

  /* Post-process: apply pitch shift as playback rate change (re-index). */
  float pitchSt = sp_pitch(params);
  if (pitchSt != 0.0f) {
    double rate = pow(2.0, (double)pitchSt / 12.0);
    /* Simple nearest-neighbor resample in-place via temp buffer. */
    float *tmp = (float *)malloc((size_t)samples * sizeof(float));
    if (tmp) {
      for (int i = 0; i < samples; i++) tmp[i] = out[i];
      for (int i = 0; i < samples; i++) {
        double srcIdx = (double)i * rate;
        int idx = (int)srcIdx;
        if (idx >= samples) { out[i] = 0.0f; continue; }
        int idx1 = idx + 1 < samples ? idx + 1 : idx;
        double frac = srcIdx - (double)idx;
        out[i] = (float)((1.0 - frac) * (double)tmp[idx] + frac * (double)tmp[idx1]);
      }
      free(tmp);
    }
  }

  /* Apply decay: shorten or lengthen the envelope. */
  float decayMul = sp_decay(params);
  if (decayMul != 1.0f) {
    for (int i = 0; i < samples; i++) {
      double t = (double)i / (double)sampleRate;
      double env = exp(-t * 8.0 * (1.0 / (double)decayMul - 1.0));
      out[i] *= (float)env;
    }
  }

  /* Apply drive (saturation). */
  if (sp_drive(params) > 0.0f) {
    for (int i = 0; i < samples; i++) {
      out[i] = sp_saturate(out[i], params);
    }
  }

  /* Apply tone (brightness): simple one-pole LP/HP based on sign. */
  float tone = sp_tone(params);
  if (tone != 0.0f) {
    double cutoff = tone > 0 ? 8000.0 + tone * 8000.0 : 2000.0 + (1.0 + tone) * 6000.0;
    double rc = 1.0 / (2.0 * M_PI * cutoff);
    double dt = 1.0 / (double)sampleRate;
    if (tone < 0) {
      /* Low-pass for darker sound. */
      double alpha = dt / (rc + dt);
      double prev = 0;
      for (int i = 0; i < samples; i++) {
        prev += alpha * ((double)out[i] - prev);
        out[i] = (float)prev;
      }
    }
  }
}

EXPORT void render_kick_p(float *out, int sampleRate, int samples,
                          const synth_params *params) {
  render_kick(out, sampleRate, samples);
  if (!params) return;

  /* Pitch: re-index for pitch shift. */
  float pitchSt = sp_pitch(params);
  if (pitchSt != 0.0f) {
    double rate = pow(2.0, (double)pitchSt / 12.0);
    float *tmp = (float *)malloc((size_t)samples * sizeof(float));
    if (tmp) {
      for (int i = 0; i < samples; i++) tmp[i] = out[i];
      for (int i = 0; i < samples; i++) {
        double srcIdx = (double)i * rate;
        int idx = (int)srcIdx;
        if (idx >= samples) { out[i] = 0.0f; continue; }
        int idx1 = idx + 1 < samples ? idx + 1 : idx;
        double frac = srcIdx - (double)idx;
        out[i] = (float)((1.0 - frac) * (double)tmp[idx] + frac * (double)tmp[idx1]);
      }
      free(tmp);
    }
  }

  /* Decay. */
  float decayMul = sp_decay(params);
  if (decayMul != 1.0f) {
    for (int i = 0; i < samples; i++) {
      double t = (double)i / (double)sampleRate;
      double env = exp(-t * 6.0 * (1.0 / (double)decayMul - 1.0));
      out[i] *= (float)env;
    }
  }

  /* Drive. */
  if (sp_drive(params) > 0.0f) {
    for (int i = 0; i < samples; i++) {
      out[i] = sp_saturate(out[i], params);
    }
  }

  /* Body: low-frequency resonance boost via one-pole LP mix. */
  float body = sp_body(params);
  if (body > 0.0f) {
    double cutoff = 120.0;
    double rc = 1.0 / (2.0 * M_PI * cutoff);
    double dt = 1.0 / (double)sampleRate;
    double alpha = dt / (rc + dt);
    double prev = 0;
    for (int i = 0; i < samples; i++) {
      prev += alpha * ((double)out[i] - prev);
      out[i] = out[i] * (1.0f - body * 0.5f) + (float)prev * body;
    }
  }
}

EXPORT void render_hihat_p(float *out, int sampleRate, int samples,
                           const synth_params *params) {
  render_hihat(out, sampleRate, samples);
  if (!params) return;

  /* Decay. */
  float decayMul = sp_decay(params);
  if (decayMul != 1.0f) {
    for (int i = 0; i < samples; i++) {
      double t = (double)i / (double)sampleRate;
      double env = exp(-t * 20.0 * (1.0 / (double)decayMul - 1.0));
      out[i] *= (float)env;
    }
  }

  /* Brightness: HP filter to emphasize shimmer. */
  float brightness = sp_brightness(params);
  if (brightness > 0.0f) {
    double cutoff = 4000.0 + brightness * 8000.0;
    double rc = 1.0 / (2.0 * M_PI * cutoff);
    double dt = 1.0 / (double)sampleRate;
    double alpha = rc / (rc + dt);
    double prevIn = 0, prevOut = 0;
    for (int i = 0; i < samples; i++) {
      double x = (double)out[i];
      double y = alpha * (prevOut + x - prevIn);
      prevIn = x; prevOut = y;
      out[i] = (float)(out[i] * (1.0 - brightness * 0.5) + y * brightness * 0.5);
    }
  }

  /* Drive. */
  if (sp_drive(params) > 0.0f) {
    for (int i = 0; i < samples; i++) {
      out[i] = sp_saturate(out[i], params);
    }
  }
}

EXPORT void render_clap_p(float *out, int sampleRate, int samples,
                          const synth_params *params) {
  render_clap(out, sampleRate, samples);
  if (!params) return;

  float decayMul = sp_decay(params);
  if (decayMul != 1.0f) {
    for (int i = 0; i < samples; i++) {
      double t = (double)i / (double)sampleRate;
      double env = exp(-t * 10.0 * (1.0 / (double)decayMul - 1.0));
      out[i] *= (float)env;
    }
  }

  if (sp_drive(params) > 0.0f) {
    for (int i = 0; i < samples; i++) {
      out[i] = sp_saturate(out[i], params);
    }
  }
}

EXPORT void render_tom_p(float *out, int sampleRate, int samples,
                         const synth_params *params) {
  render_tom(out, sampleRate, samples);
  if (!params) return;

  /* Pitch shift. */
  float pitchSt = sp_pitch(params);
  if (pitchSt != 0.0f) {
    double rate = pow(2.0, (double)pitchSt / 12.0);
    float *tmp = (float *)malloc((size_t)samples * sizeof(float));
    if (tmp) {
      for (int i = 0; i < samples; i++) tmp[i] = out[i];
      for (int i = 0; i < samples; i++) {
        double srcIdx = (double)i * rate;
        int idx = (int)srcIdx;
        if (idx >= samples) { out[i] = 0.0f; continue; }
        int idx1 = idx + 1 < samples ? idx + 1 : idx;
        double frac = srcIdx - (double)idx;
        out[i] = (float)((1.0 - frac) * (double)tmp[idx] + frac * (double)tmp[idx1]);
      }
      free(tmp);
    }
  }

  float decayMul = sp_decay(params);
  if (decayMul != 1.0f) {
    for (int i = 0; i < samples; i++) {
      double t = (double)i / (double)sampleRate;
      double env = exp(-t * 8.0 * (1.0 / (double)decayMul - 1.0));
      out[i] *= (float)env;
    }
  }

  if (sp_drive(params) > 0.0f) {
    for (int i = 0; i < samples; i++) {
      out[i] = sp_saturate(out[i], params);
    }
  }
}

EXPORT void render_cowbell_p(float *out, int sampleRate, int samples,
                             const synth_params *params) {
  render_cowbell(out, sampleRate, samples);
  if (!params) return;

  float pitchSt = sp_pitch(params);
  if (pitchSt != 0.0f) {
    double rate = pow(2.0, (double)pitchSt / 12.0);
    float *tmp = (float *)malloc((size_t)samples * sizeof(float));
    if (tmp) {
      for (int i = 0; i < samples; i++) tmp[i] = out[i];
      for (int i = 0; i < samples; i++) {
        double srcIdx = (double)i * rate;
        int idx = (int)srcIdx;
        if (idx >= samples) { out[i] = 0.0f; continue; }
        int idx1 = idx + 1 < samples ? idx + 1 : idx;
        double frac = srcIdx - (double)idx;
        out[i] = (float)((1.0 - frac) * (double)tmp[idx] + frac * (double)tmp[idx1]);
      }
      free(tmp);
    }
  }

  float decayMul = sp_decay(params);
  if (decayMul != 1.0f) {
    for (int i = 0; i < samples; i++) {
      double t = (double)i / (double)sampleRate;
      double env = exp(-t * 12.0 * (1.0 / (double)decayMul - 1.0));
      out[i] *= (float)env;
    }
  }
}

EXPORT const char *result_description(int code) {
  return ma_result_description((ma_result)code);
}
