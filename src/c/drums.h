#ifndef DRUMS_H
#define DRUMS_H

void render_snare(float *out, int sampleRate, int samples);
void render_kick(float *out, int sampleRate, int samples);
void render_hihat(float *out, int sampleRate, int samples);
void render_tom(float *out, int sampleRate, int samples);
void render_clap(float *out, int sampleRate, int samples);
int load_wav(const char *path, float **buffer, int *sampleRate);
const char *result_description(int code);

#endif
