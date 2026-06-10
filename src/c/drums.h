#ifndef DRUMS_H
#define DRUMS_H

#include "synth_params.h"

/* snare family migrated to the modular engine (Phase-5): no bespoke
 * render_snare / render_snare_rimshot / render_snare_sidestick / render_clap
 * declarations. */
/* kick family migrated to the modular engine (Phase-3): no bespoke
 * render_kick / render_kick_deep / render_kick_punchy / render_kick_lofi /
 * render_kick_tight declarations. */
/* cymbal family migrated to the modular engine (Phase-6): no bespoke
 * render_hihat / render_open_hihat / render_cowbell / render_shaker /
 * render_ride / render_crash declarations. */
/* tom family migrated to the modular engine (Phase-4): no bespoke
 * render_tom / render_tom_high / render_tom_low declarations. */
/* bass-guitar / sub-bass migrated to the modular engine (Phase-2): no bespoke
 * render_bass_guitar / render_sub_bass declarations. */
int load_audio(const char *path, int targetSampleRate, float **buffer, int *outSampleRate);
const char *result_description(int code);

/* Parameterized render variants. Pass NULL params for default behavior. */
/* render_snare_p / render_clap_p deleted — snare/clap render via render_modular_p
 * (Phase-5). */
/* render_kick_p deleted — kick renders via render_modular_p (Phase-3). */
/* render_hihat_p / render_open_hihat_p / render_cowbell_p / render_shaker_p /
 * render_ride_p / render_crash_p deleted — cymbal renders via render_modular_p
 * (Phase-6). */
/* render_tom_p / render_tom_high_p / render_tom_low_p deleted — tom renders via
 * render_modular_p (Phase-4). */
/* render_bass_guitar_p / render_sub_bass_p deleted — bass renders via
 * render_modular_p (Phase-2 modular-synth-unification). */
/* render_snare_rimshot_p / render_snare_sidestick_p deleted — snare family
 * renders via render_modular_p (Phase-5). */
/* render_kick_deep_p / render_kick_punchy_p / render_kick_lofi_p /
 * render_kick_tight_p deleted — kick renders via render_modular_p (Phase-3). */

#endif
