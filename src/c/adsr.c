#include "adsr.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

void adsr_init(adsr_t *env, int sr,
                       double attack, double decay, double sustain, double release,
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

void adsr_trigger(adsr_t *env) {
    env->stage = ADSR_ATTACK;
    double attack_samples = env->attack_sec * (double)env->sr;
    if (env->exponential) {
        /* Aims ABOVE 1.0: an exponential aimed exactly at 1.0 approaches it forever and never leaves ATTACK. The attack
         * tick clamps to 1.0 on the transition. */
        env->target = 1.2f;
        env->rate = (attack_samples > 0) ? 3.0f / attack_samples : 1.0f;
    } else {
        /* Linear attack: fixed increment per sample */
        env->target = 1.0f;
        env->rate = (attack_samples > 0) ? (1.0f - env->value) / attack_samples : 1.0f;
    }
}

void adsr_release(adsr_t *env) {
    if (env->stage == ADSR_IDLE) return;
    env->stage = ADSR_RELEASE;
    env->target = 0;
    double release_samples = env->release_sec * (double)env->sr;
    if (env->exponential) {
        env->rate = (release_samples > 0) ? 3.0f / release_samples : 1.0f;
    } else {
        env->rate = (release_samples > 0) ? -env->value / release_samples : -1.0f;
    }
}

