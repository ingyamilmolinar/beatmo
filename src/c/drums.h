#ifndef DRUMS_H
#define DRUMS_H

int load_audio(const char *path, int targetSampleRate, float **buffer, int *outSampleRate);
const char *result_description(int code);

#endif
