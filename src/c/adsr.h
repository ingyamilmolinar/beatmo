#ifndef ADSR_H
#define ADSR_H

/* ================================================================
 * ADSR envelope generator with linear and exponential curves.
 *
 * State-machine design: more efficient than time-based lookup for
 * real-time processing. Supports block rendering.
 * ================================================================ */

typedef enum {
    ADSR_IDLE,
    ADSR_ATTACK,
    ADSR_DECAY,
    ADSR_SUSTAIN,
    ADSR_RELEASE
} adsr_stage_t;

typedef struct {
    float attack_sec;
    float decay_sec;
    float sustain_level;  /* 0-1 */
    float release_sec;
    int   exponential;    /* 0=linear, 1=exponential curves */
    int   sr;

    /* Internal state */
    adsr_stage_t stage;
    float value;          /* current envelope value 0-1 */
    float rate;           /* per-sample rate for current stage */
    float target;         /* target value for current stage */
    float exp_coeff;      /* exponential coefficient for current stage */
} adsr_t;

/* Initialize ADSR with parameters. Does not trigger. */
void adsr_init(adsr_t *env, int sr,
               float attack, float decay, float sustain, float release,
               int exponential);

/* Trigger note-on: start attack from current value (for legato). */
void adsr_trigger(adsr_t *env);

/* Trigger note-off: start release from current value. */
void adsr_release(adsr_t *env);

/* Reset to idle state (value=0). */
void adsr_reset(adsr_t *env);

/* Process a block of envelope values. */
void adsr_process(adsr_t *env, float *out, int samples);

/* Single-sample tick. */
static inline float adsr_tick(adsr_t *env) {
    switch (env->stage) {
    case ADSR_IDLE:
        return 0;

    case ADSR_ATTACK:
        if (env->exponential) {
            env->value += env->rate * (env->target - env->value);
        } else {
            env->value += env->rate;
        }
        if (env->value >= 1.0f) {
            env->value = 1.0f;
            env->stage = ADSR_DECAY;
            /* Compute decay rate */
            if (env->exponential) {
                float decay_samples = env->decay_sec * (float)env->sr;
                env->rate = (decay_samples > 0) ? 1.0f / decay_samples : 1.0f;
                env->target = env->sustain_level;
            } else {
                float decay_samples = env->decay_sec * (float)env->sr;
                env->rate = (decay_samples > 0) ? (env->sustain_level - 1.0f) / decay_samples : 0;
            }
        }
        return env->value;

    case ADSR_DECAY:
        if (env->exponential) {
            env->value += env->rate * (env->target - env->value);
            /* Check if close enough to sustain */
            float diff = env->value - env->sustain_level;
            if (diff < 0) diff = -diff;
            if (diff < 0.001f) {
                env->value = env->sustain_level;
                env->stage = ADSR_SUSTAIN;
            }
        } else {
            env->value += env->rate;
            if ((env->rate <= 0 && env->value <= env->sustain_level) ||
                (env->rate >= 0 && env->value >= env->sustain_level)) {
                env->value = env->sustain_level;
                env->stage = ADSR_SUSTAIN;
            }
        }
        return env->value;

    case ADSR_SUSTAIN:
        return env->sustain_level;

    case ADSR_RELEASE:
        if (env->exponential) {
            env->value += env->rate * (0 - env->value);
        } else {
            env->value += env->rate;
        }
        if (env->value <= 0.001f) {
            env->value = 0;
            env->stage = ADSR_IDLE;
        }
        return env->value;
    }
    return 0;
}

#endif /* ADSR_H */
