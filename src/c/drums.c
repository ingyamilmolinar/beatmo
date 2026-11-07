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

/* The single compiled copy of the saturation and generator primitives (declared in synth_post.h). */
float softsat_shared(float x) {
  // gentle tanh saturation
  return (float)tanh(x * 1.5);
}

/* 0=Sine 1=Saw 2=Square 3=Triangle, phase in radians (saw/triangle wrap via floor). Sine and Square are the exact
 * expressions the engines used as literals, so an explicit default renders bit-identically. */
double osc_wave_shared(int wave, double phase) {
  switch (wave) {
  case 1: { /* saw */
    double t = phase * (1.0 / (2.0 * M_PI));
    t -= floor(t);
    return 2.0 * t - 1.0;
  }
  case 2: /* square — mirrors the hi-hat cluster expression exactly */
    return (sin(phase) >= 0.0) ? 1.0 : -1.0;
  case 3: { /* triangle */
    double t = phase * (1.0 / (2.0 * M_PI));
    t -= floor(t);
    return (t < 0.5) ? (4.0 * t - 1.0) : (3.0 - 4.0 * t);
  }
  default: /* sine — mirrors the engines' literal sin(phase) */
    return sin(phase);
  }
}

// Mono f32 at the file's own rate when targetSampleRate is 0, which is how Beatmo calls it: the rate conversion is
// Go's, shared with the browser. Returns the frame count or a negative ma_result.
int load_audio(const char *path, int targetSampleRate, float **buffer, int *sampleRate) {
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

#include "synth_post.h"


const char *result_description(int code) {
  return ma_result_description((ma_result)code);
}
