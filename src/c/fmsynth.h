#ifndef FMSYNTH_H
#define FMSYNTH_H

#include "synth_params.h"

#define FM_MAX_OPS 4

/* A single FM operator. The engine routes operator outputs through a
 * mod_matrix; carriers contribute to the mix, modulators only feed others. */
typedef struct {
    float freq_ratio;     /* frequency = base_freq * freq_ratio + freq_offset */
    float freq_offset;    /* Hz, for inharmonic metallic sounds              */
    float amplitude;      /* 0.0–1.0                                         */
    float attack_sec;     /* ADSR attack time                                */
    float decay_sec;      /* ADSR decay time                                 */
    float sustain_level;  /* ADSR sustain level (0.0–1.0)                    */
    float release_sec;    /* ADSR release time                               */
    int   is_carrier;     /* 1 = output to speaker, 0 = modulator only       */
} fm_operator;

typedef struct {
    int   num_ops;
    float base_freq;                              /* Hz                       */
    float mod_matrix[FM_MAX_OPS][FM_MAX_OPS];     /* [src][dst] = mod depth   */
    fm_operator ops[FM_MAX_OPS];
    float pitch_env_amount;                        /* semitones of pitch sweep */
    float pitch_env_decay;                         /* decay rate (seconds)     */
    int   wave;  /* operator wavetable: 0=Sine 1=Saw 2=Square 3=Triangle.
                  * 0 keeps the historical sine table (bit-identical);
                  * static presets and build_fm_preset zero-init it. */
} fm_preset;

/* Core wavetable-based FM render. Shared with the modular voice engine
 * (src/c/modular.c osc_type==4 FM stage + src/c/modular_stages.c source==10
 * FM-family voice), which assemble a preset from live params. This is the
 * single compiled FM core every FM sound flows through.
 *
 * The render_fm_* / render_fm_*_p wrappers, the static PRESET_FM_* tables,
 * fm_apply_params, and the fm_params ABI block were DELETED in the Phase-7
 * FM-family migration (the LAST legacy family). FM presets now render through
 * the unified modular engine (modular_gen_slot_fm, source==10); FM knobs travel
 * in the wide modular_params block (gen_fm_* columns), not a separate fm_params
 * block. fm_render + the fm_preset/fm_operator types below REMAIN — they are the
 * shared core. */
void fm_render(const fm_preset *p, float *out, int sampleRate, int samples);

#endif
