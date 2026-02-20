#ifndef DRUMS_H
#define DRUMS_H

#include "synth_params.h"

void render_snare(float *out, int sampleRate, int samples);
void render_kick(float *out, int sampleRate, int samples);
void render_hihat(float *out, int sampleRate, int samples);
void render_open_hihat(float *out, int sampleRate, int samples);
void render_tom(float *out, int sampleRate, int samples);
void render_tom_high(float *out, int sampleRate, int samples);
void render_tom_low(float *out, int sampleRate, int samples);
void render_clap(float *out, int sampleRate, int samples);
void render_cowbell(float *out, int sampleRate, int samples);
void render_bass_guitar(float *out, int sampleRate, int samples);
void render_sub_bass(float *out, int sampleRate, int samples);
void render_snare_rimshot(float *out, int sampleRate, int samples);
void render_snare_sidestick(float *out, int sampleRate, int samples);
void render_kick_deep(float *out, int sampleRate, int samples);
void render_kick_punchy(float *out, int sampleRate, int samples);
void render_kick_lofi(float *out, int sampleRate, int samples);
void render_kick_tight(float *out, int sampleRate, int samples);
void render_shaker(float *out, int sampleRate, int samples);
void render_ride(float *out, int sampleRate, int samples);
void render_crash(float *out, int sampleRate, int samples);
int load_audio(const char *path, int targetSampleRate, float **buffer, int *outSampleRate);
int load_wav(const char *path, int targetSampleRate, float **buffer, int *outSampleRate);
const char *result_description(int code);

/* Parameterized render variants. Pass NULL params for default behavior. */
void render_snare_p(float *out, int sampleRate, int samples, const synth_params *params);
void render_kick_p(float *out, int sampleRate, int samples, const synth_params *params);
void render_hihat_p(float *out, int sampleRate, int samples, const synth_params *params);
void render_clap_p(float *out, int sampleRate, int samples, const synth_params *params);
void render_tom_p(float *out, int sampleRate, int samples, const synth_params *params);
void render_cowbell_p(float *out, int sampleRate, int samples, const synth_params *params);

#endif
