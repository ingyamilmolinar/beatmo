#ifndef SYNTH_POST_H
#define SYNTH_POST_H

/* The single compiled copy is in drums.c. 0=Sine 1=Saw 2=Square 3=Triangle; phase in radians. */
double osc_wave_shared(int wave, double phase);
float  softsat_shared(float x);

#endif
