#ifndef DRUMS_H
#define DRUMS_H

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
int load_audio(const char *path, float **buffer, int *sampleRate);
int load_wav(const char *path, float **buffer, int *sampleRate);
const char *result_description(int code);

#endif
