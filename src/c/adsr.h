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
    double attack_sec;
    double decay_sec;
    double sustain_level;  /* 0-1 */
    double release_sec;
    int   exponential;    /* 0=linear, 1=exponential curves */
    int   sr;

    /* Internal state */
    adsr_stage_t stage;
    double value;          /* current envelope value 0-1 */
    double rate;           /* per-sample rate for current stage */
    double target;         /* target value for current stage */
    double exp_coeff;      /* exponential coefficient for current stage */
} adsr_t;

/* Initialize ADSR with parameters. Does not trigger. */
void adsr_init(adsr_t *env, int sr,
               double attack, double decay, double sustain, double release,
               int exponential);

/* Trigger note-on: start attack from current value (for legato). */
void adsr_trigger(adsr_t *env);

/* Trigger note-off: start release from current value. */
void adsr_release(adsr_t *env);

/* Single-sample tick. */
static inline double adsr_tick(adsr_t *env) {
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
                double decay_samples = env->decay_sec * (double)env->sr;
                env->rate = (decay_samples > 0) ? 1.0f / decay_samples : 1.0f;
                env->target = env->sustain_level;
            } else {
                double decay_samples = env->decay_sec * (double)env->sr;
                env->rate = (decay_samples > 0) ? (env->sustain_level - 1.0f) / decay_samples : 0;
            }
        }
        return env->value;

    case ADSR_DECAY:
        if (env->exponential) {
            env->value += env->rate * (env->target - env->value);
            /* Check if close enough to sustain */
            double diff = env->value - env->sustain_level;
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
