#ifndef SYNTH_PARAMS_H
#define SYNTH_PARAMS_H

#include <math.h>

/* Shared synthesis parameter struct for parameterized instrument rendering.
 * All fields have sensible defaults when zero-initialized.
 * Pass NULL to any _p() function to use the original hardcoded defaults.
 *
 * Synth-tab redesign (2026-05-16) dropped the `attack` and `color` fields
 * because no C renderer reads them — they were silent no-ops in every
 * shipped recipe. If a future recipe needs either, re-add the field to
 * this struct AND wire the read in the recipe's _p() function AND extend
 * recipeWiredParams in synth_recipe_wired.go (the UI surfaces only the
 * wired-knob set per recipe). */
typedef struct {
    float pitch;       /* Semitone offset from default (0 = default) */
    float decay;       /* Decay multiplier (1.0 = default, 0.5 = half, 2.0 = double) */
    float tone;        /* Tone/brightness: -1 = dark, 0 = default, 1 = bright */
    float drive;       /* Saturation/overdrive amount (0 = none, 1 = heavy) */
    float body;        /* Body/resonance emphasis (0 = default, 1 = max) */
    float brightness;  /* High-frequency content: 0 = default, 1 = max shimmer */
    float fundamental; /* Recipe-specific core-synthesis override (Hz).
                        * Identity = 0 = "use the recipe's built-in default".
                        * Phase-3 addition; only drum-kick reads it today. */
} synth_params;

/* Helper: read a param or return default if params is NULL. */
static inline float sp_pitch(const synth_params *p)       { return p ? p->pitch       : 0.0f; }
static inline float sp_decay(const synth_params *p)       { return p ? p->decay       : 1.0f; }
static inline float sp_tone(const synth_params *p)        { return p ? p->tone        : 0.0f; }
static inline float sp_drive(const synth_params *p)       { return p ? p->drive       : 0.0f; }
static inline float sp_body(const synth_params *p)        { return p ? p->body        : 0.0f; }
static inline float sp_brightness(const synth_params *p)  { return p ? p->brightness  : 0.0f; }
static inline float sp_fundamental(const synth_params *p) { return p ? p->fundamental : 0.0f; }

/* Apply pitch offset: multiply a base frequency by 2^(semitones/12). */
static inline double sp_freq(double baseFreq, const synth_params *p) {
    float st = sp_pitch(p);
    if (st == 0.0f) return baseFreq;
    return baseFreq * pow(2.0, (double)st / 12.0);
}

/* Apply decay multiplier to a base envelope time constant. */
static inline double sp_env_decay(double baseSec, const synth_params *p) {
    float d = sp_decay(p);
    if (d <= 0.0f) d = 0.01f; /* clamp to avoid infinite decay */
    return baseSec * (double)d;
}

/* Apply tone to a filter cutoff: shift by ±1 octave. */
static inline double sp_filter_cutoff(double baseCutoff, const synth_params *p) {
    float t = sp_tone(p);
    return baseCutoff * pow(2.0, (double)t);
}

/* Apply drive as tanh saturation. */
static inline float sp_saturate(float x, const synth_params *p) {
    float d = sp_drive(p);
    if (d <= 0.0f) return x;
    float driven = x * (1.0f + d * 3.0f);
    return (float)tanh((double)driven) / (float)tanh(1.0 + (double)d * 3.0);
}

/* ── Family param blocks (native-deprecation migration) ──────────────────────
 *
 * Each native engine family gets a wide param block embedding the generic
 * synth_params as `base` (offset 0) plus curated family fields. Family
 * fields use the NaN sentinel: NaN = "unset", the renderer falls back to
 * its hardcoded double literal, so a NULL block / all-NaN block renders
 * bit-identically to the original engine BY CONSTRUCTION. 0 is NOT the
 * sentinel — several constants have a legitimate 0.
 *
 * The Go side elides values that exactly equal the recipe's ParamDef
 * default (sends NaN instead), so an unedited instrument always takes the
 * exact-double fallback path; user edits arrive float32-quantized, which
 * both platforms do identically.
 *
 * Field order is mirrored by the *ParamSchema slices in
 * src/go/internal/audio/synth_param_schema.go and the generated JS ABI —
 * append only, never reorder. */

/* kp_get: family-field read. NaN → exact hardcoded double literal. */
static inline double kp_get(float v, double fallback) {
    return (v != v) ? fallback : (double)v;
}

/* kick_params DELETED — the kick family (render_kick + deep/punchy/lofi/tight)
 * migrated to the unified modular engine (Phase-3 modular-synth-unification).
 * The curated knobs (h2/h3/h4 gains, env0/env1 rates, pitch-env amount/rate,
 * click, noise, wave) now live on the modular gen-slot block (gen_kick_*),
 * read by modular_gen_slot_kick (source==5). The fundamental travels via
 * modular_params.voice_freq_hz. */

/* snare_params DELETED — the snare family (snare / rimshot / sidestick / clap)
 * migrated to the modular engine (Phase-5). Its curated knobs now travel as
 * gen_snare_* gen-slot columns in modular_params (src/c/modular.h); the
 * source==7 voice (snare/rimshot/sidestick) and source==8 voice (clap) read them
 * via kp_get. No separate snare_params ABI block remains. */

/* cymbal_params DELETED — the cymbal family (hihat / open_hihat / cowbell /
 * shaker / ride / crash) migrated to the unified modular engine (Phase-6). Its
 * curated knobs (tune/env_fast/env_tail/tone_mix/noise_mix/noise_decay/wave) now
 * live on the modular gen-slot block (gen_cym_* + gen_wave), read by
 * modular_gen_slot_cymbal (source==9). No separate cymbal_params ABI block
 * remains. */

/* bass_params DELETED — the bass family (bass-guitar Karplus-Strong + sub-bass
 * 808) migrated to the unified modular engine (Phase-2 modular-synth-
 * unification). Its knobs now travel in the wide modular_params block via the
 * Go-side bassRecipeToModular / bassGuitarRecipeToModular binding. */

/* tom_params DELETED — the tom family (render_tom / tom_high / tom_low) migrated
 * to the modular engine (Phase-4 modular-synth-unification). The curated tom
 * knobs (sweep/ring rates, o1/o2 gains, stick, room) now travel in the wide
 * modular_params block (gen_tom_* gen-slot columns, source==6) via the Go-side
 * tomRecipeToModular binding. */

#endif
