#ifndef FMSYNTH_H
#define FMSYNTH_H

#define FM_MAX_OPS 4

/* A single FM operator. The engine routes operator outputs through a
 * mod_matrix; carriers contribute to the mix, modulators only feed others. */
typedef struct {
    double freq_ratio;     /* frequency = base_freq * freq_ratio + freq_offset */
    double freq_offset;    /* Hz, for inharmonic metallic sounds              */
    double amplitude;      /* 0.0–1.0                                         */
    double attack_sec;     /* ADSR attack time                                */
    double decay_sec;      /* ADSR decay time                                 */
    double sustain_level;  /* ADSR sustain level (0.0–1.0)                    */
    double release_sec;    /* ADSR release time                               */
    int   is_carrier;     /* 1 = output to speaker, 0 = modulator only       */
} fm_operator;

typedef struct {
    int   num_ops;
    double base_freq;                              /* Hz                       */
    double mod_matrix[FM_MAX_OPS][FM_MAX_OPS];     /* [src][dst] = mod depth   */
    fm_operator ops[FM_MAX_OPS];
    double pitch_env_amount;                        /* semitones of pitch sweep */
    double pitch_env_decay;                         /* decay rate (seconds)     */
    int   wave;  /* operator wavetable: 0=Sine 1=Saw 2=Square 3=Triangle.
                  * 0 keeps the historical sine table (bit-identical);
                  * static presets and build_fm_preset zero-init it. */
} fm_preset;

/* Core wavetable-based FM render. Shared with the modular voice engine
 * (src/c/modular.c osc_type==4 FM stage + src/c/modular_stages.c source==10
 * FM-family voice), which assemble a preset from live params. This is the
 * single compiled FM core every FM sound flows through. */
void fm_render(const fm_preset *p, float *out, int sampleRate, int samples);

#endif
