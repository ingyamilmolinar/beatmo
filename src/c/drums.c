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

EXPORT void render_snare(float *out, int sampleRate, int samples) {
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  double lp = 0.0, phase = 0.0;
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lp = lp * 0.7 + n * 0.3;
    double hp = n - lp;
    double env = exp(-6.0 * t);
    double noiseVal = (hp * 0.5 + lp * 0.5) * env;
    double freq = 200.0 - 60.0 * t;
    phase += 2.0 * M_PI * freq / (double)sampleRate;
    double tone = sin(phase) * exp(-4.0 * t);
    out[i] = (float)(noiseVal * 0.7 + tone * 0.3);
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
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    double freq = 150.0 - 100.0 * t;
    phase += 2.0 * M_PI * freq / (double)sampleRate;
    double env = exp(-5.0 * t);
    double tone = sin(phase) * env;
    double attack = n * exp(-40.0 * t);
    out[i] = (float)(tone + attack);
  }
  ma_noise_uninit(&noise, NULL);
}

EXPORT void render_hihat(float *out, int sampleRate, int samples) {
  ma_noise_config nc =
      ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
  ma_noise noise;
  if (ma_noise_init(&nc, NULL, &noise) != MA_SUCCESS) {
    return;
  }
  double lp = 0.0;
  for (int i = 0; i < samples; ++i) {
    double t = (double)i / (double)samples;
    float n;
    ma_noise_read_pcm_frames(&noise, &n, 1, NULL);
    lp = lp * 0.95 + n * 0.05;
    double hp = n - lp;
    double env = exp(-40.0 * t);
    out[i] = (float)(hp * env);
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
    double freq = 300.0 - 200.0 * t;
    phase += 2.0 * M_PI * freq / (double)sampleRate;
    double tone = sin(phase) * exp(-3.0 * t);
    lp = lp * 0.8 + n * 0.2;
    double env = exp(-6.0 * t);
    out[i] = (float)(tone + lp * 0.2 * env);
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
    double burst = exp(-100.0 * fabs(t - 0.00)) + exp(-100.0 * fabs(t - 0.02)) +
                   exp(-100.0 * fabs(t - 0.04));
    double env = exp(-6.0 * t);
    out[i] = (float)(n * burst * env);
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
