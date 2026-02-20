#ifndef FMSYNTH_H
#define FMSYNTH_H

void render_fm_bass(float *out, int sampleRate, int samples);
void render_fm_bell(float *out, int sampleRate, int samples);
void render_fm_lead(float *out, int sampleRate, int samples);
void render_fm_epiano(float *out, int sampleRate, int samples);
void render_fm_pluck(float *out, int sampleRate, int samples);

#endif
