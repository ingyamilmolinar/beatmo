#include <math.h>

#define MA_NO_DEVICE_IO
#define MA_NO_THREADING
#include "miniaudio.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

EXPORT void render_snare(float* out, int sampleRate, int samples) {
    ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
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

EXPORT void render_kick(float* out, int sampleRate, int samples) {
    ma_noise_config nc = ma_noise_config_init(ma_format_f32, 1, ma_noise_type_white, 0, 1.0);
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
