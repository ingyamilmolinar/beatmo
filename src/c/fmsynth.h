#ifndef FMSYNTH_H
#define FMSYNTH_H

#define FM_MAX_OPS 4

/* Carriers contribute to the mix; modulators only feed others through the mod_matrix. */
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

/* The single compiled FM core every FM sound flows through (the modular FM stage and the FM-family voice). */
void fm_render(const fm_preset *p, float *out, int sampleRate, int samples);

#endif
