#include <math.h>
#include <stdlib.h>

#define MA_NO_DEVICE_IO
#define MA_NO_THREADING
#include "miniaudio.h"

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

EXPORT void render_snare(float *out, int sampleRate, int samples) {
  // Body-forward snare with mellow wires (less brittle highs).
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }

  double phase = 0.0;
  // Two lowpasses to form a mid band via difference (fast - slow)
  double lpFast = 0.0, lpSlow = 0.0, lpOut = 0.0;
  // Slight per-hit tone variation
  float seed; ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double toneVar = 1.0 + 0.03 * (double)seed;
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float wn; ma_noise_read_pcm_frames(&noise, &wn, 1, NULL);
    // Mid-band noise around ~2–4 kHz by subtracting slow LP from fast LP
    lpFast = lpFast * 0.75 + wn * 0.25; // faster cutoff
    lpSlow = lpSlow * 0.95 + wn * 0.05; // slower cutoff
    double band = (lpFast - lpSlow);
    // Additional gentle lowpass to tame top-end
    lpOut = lpOut * 0.8 + band * 0.2;

    // Body tone ~170Hz gently falling, with moderate decay
    double freq = (170.0 * toneVar) - 50.0 * t;
    if (freq < 100.0) freq = 100.0;
    phase += 2.0 * M_PI * freq / (double)sampleRate;
    double body = sin(phase) * exp(-3.2 * t);

    // Noise envelope: quick attack into mellow tail
    double envN = 0.55 * exp(-18.0 * t) + 0.45 * exp(-5.0 * t);
    // Global HF tilt: progressively darken tail
    double dark = 1.0 - 0.6 * t; if (dark < 0.2) dark = 0.2;
    double wires = lpOut * envN * dark;

    double mixed = body * 0.55 + wires * 0.70;
    // Gentle saturation and slight output trim
    out[i] = 0.9f * softsat((float)mixed);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_kick(float *out, int sampleRate, int samples) {
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  double phase = 0.0;
  float seed;
  ma_noise_read_pcm_frames(&noise, &seed, 1, NULL);
  double randmul = 1.0 + 0.02 * (double)seed; // slight per-hit variation
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    // Exponential pitch drop for punch
    double f0 = 190.0 * randmul;
    double f1 = 42.0;
    double freq = f1 + (f0 - f1) * exp(-10.0 * t);
    phase += 2.0 * M_PI * freq / (double)sampleRate;
    double env = exp(-5.5 * t);
    double tone = sin(phase) * env;
    // short click
    double click = (n) * exp(-60.0 * t);
    // gentle saturation
    out[i] = softsat((float)(tone + 0.4 * click));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_hihat(float *out, int sampleRate, int samples) {
  // Closed hat: mid-focused, darker decay (less brittle highs).
  ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise; if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) { return; }
  // Mid-band via fast/slow LP difference; final output lowpass to tame treble.
  double lpF = 0.0, lpS = 0.0, lpFinal = 0.0;
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float wn; ma_noise_read_pcm_frames(&noise, &wn, 1, NULL);
    lpF = lpF * 0.65 + wn * 0.35; // relatively fast
    lpS = lpS * 0.90 + wn * 0.10; // relatively slow
    double band = lpF - lpS;
    // Darken progressively over the decay
    double env = exp(-36.0 * t);
    double dark = 1.0 - 0.7 * t; if (dark < 0.25) dark = 0.25;
    // Output lowpass
    lpFinal = lpFinal * 0.7 + band * 0.3;
    double y = lpFinal * env * dark;
    out[i] = 0.85f * softsat((float)y);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_tom(float *out, int sampleRate, int samples) {
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  double phase = 0.0;
  double lp = 0.0;
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    double freq = 220.0 - 120.0 * t;
    phase += 2.0 * M_PI * freq / (double)sampleRate;
    double tone = sin(phase) * exp(-3.2 * t);
    lp = lp * 0.85 + n * 0.15;
    double env = exp(-6.0 * t);
    out[i] = softsat((float)(tone + lp * 0.18 * env));
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
    double burst = exp(-120.0 * fabs(t - 0.00)) + exp(-120.0 * fabs(t - 0.022)) +
                   exp(-120.0 * fabs(t - 0.047));
    double env = exp(-7.0 * t);
    out[i] = softsat((float)(n * burst * env));
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT int load_wav(const char *path, float **buffer, int *sampleRate) {
  ma_decoder_config cfg = ma_decoder_config_init(ma_format_f32, 1, 0);
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

EXPORT const char *result_description(int code) {
  return ma_result_description((ma_result)code);
}
