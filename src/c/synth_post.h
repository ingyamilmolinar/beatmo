#ifndef SYNTH_POST_H
#define SYNTH_POST_H

/* osc_wave + softsat are the generator / saturation primitives the modular
 * engine's analytic voice sources share (the function bodies are the single
 * compiled copy in drums.c). 0=Sine 1=Saw 2=Square 3=Triangle. Phase in radians
 * (saw/triangle wrap via floor). See drums.c for the byte-parity contract. */
double osc_wave_shared(int wave, double phase);
float  softsat_shared(float x);

#endif
