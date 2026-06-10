#include "adsr.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

EXPORT void adsr_init(adsr_t *env, int sr,
                       float attack, float decay, float sustain, float release,
                       int exponential) {
    env->sr            = sr;
    env->attack_sec    = attack;
    env->decay_sec     = decay;
    env->sustain_level = sustain;
    env->release_sec   = release;
    env->exponential   = exponential;
    env->stage         = ADSR_IDLE;
    env->value         = 0;
    env->rate          = 0;
    env->target        = 0;
    env->exp_coeff     = 0;
}

EXPORT void adsr_trigger(adsr_t *env) {
    env->stage = ADSR_ATTACK;
    float attack_samples = env->attack_sec * (float)env->sr;
    if (env->exponential) {
        /* Exponential attack: aim ABOVE 1.0 so the asymptotic curve actually
         * crosses the value>=1.0 threshold and transitions to DECAY. Aiming at
         * exactly 1.0 would approach it asymptotically and never transition,
         * leaving the envelope stuck in ATTACK forever. The attack-stage tick
         * clamps value to 1.0 on transition. */
        env->target = 1.2f;
        env->rate = (attack_samples > 0) ? 3.0f / attack_samples : 1.0f;
    } else {
        /* Linear attack: fixed increment per sample */
        env->target = 1.0f;
        env->rate = (attack_samples > 0) ? (1.0f - env->value) / attack_samples : 1.0f;
    }
}

EXPORT void adsr_release(adsr_t *env) {
    if (env->stage == ADSR_IDLE) return;
    env->stage = ADSR_RELEASE;
    env->target = 0;
    float release_samples = env->release_sec * (float)env->sr;
    if (env->exponential) {
        env->rate = (release_samples > 0) ? 3.0f / release_samples : 1.0f;
    } else {
        env->rate = (release_samples > 0) ? -env->value / release_samples : -1.0f;
    }
}

EXPORT void adsr_reset(adsr_t *env) {
    env->stage = ADSR_IDLE;
    env->value = 0;
    env->rate  = 0;
}

EXPORT void adsr_process(adsr_t *env, float *out, int samples) {
    for (int i = 0; i < samples; i++) {
        out[i] = adsr_tick(env);
    }
}
