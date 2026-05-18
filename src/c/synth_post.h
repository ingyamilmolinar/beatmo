#ifndef SYNTH_POST_H
#define SYNTH_POST_H

#include "synth_params.h"

/* post_config flags which of the 8 generic knobs are wired to a parameterized
 * renderer's post-processing stage. decay_rate is the per-instrument time
 * constant that scales the decay envelope (snare=8, kick=6, hihat=20, ...).
 * Phase-2 _p() variants in drums.c and fmsynth.c all share this struct so
 * the synth-recipe Render path is bit-identical across instrument families. */
typedef struct {
  double decay_rate;
  int    pitch;
  int    decay;
  int    drive;
  int    tone;
  int    body;
  int    brightness;
} post_config;

/* apply_post_params is the single source of truth for synth_params-driven
 * post-processing across every Phase-2 parameterized renderer. The base
 * renderer fills out[]; this helper applies the enabled transforms in
 * the order: pitch -> decay -> body -> brightness -> tone -> drive. The
 * 6 hand-written Phase-1 _p() variants (snare_p, kick_p, hihat_p, clap_p,
 * tom_p, cowbell_p) predate this helper and keep their bespoke order; the
 * test suite checks bit-equality at default params for both groups. */
void apply_post_params(float *out, int sampleRate, int samples,
                       const synth_params *params, const post_config *cfg);

#endif
