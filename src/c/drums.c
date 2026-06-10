#include <math.h>
#include <stdlib.h>

#define MA_NO_DEVICE_IO
#define MA_NO_THREADING
#include "miniaudio.h"
#include "synth_params.h"

#ifdef __EMSCRIPTEN__
#include <emscripten.h>
#define EXPORT EMSCRIPTEN_KEEPALIVE
#else
#define EXPORT
#endif

/* softsat_shared / osc_wave_shared: the single compiled copy of the legacy
 * saturation + generator-waveform primitives, declared in synth_post.h so the
 * modular engine's analytic voice sources call the IDENTICAL code (byte parity
 * by construction). The thin `softsat` / `osc_wave` aliases below preserve the
 * historical call sites in this file unchanged. */
float softsat_shared(float x) {
  // gentle tanh saturation
  return (float)tanh(x * 1.5);
}
static inline float softsat(float x) { return softsat_shared(x); }

/* osc_wave: discrete generator waveform for the family engines' core
 * oscillators (the <family>_wave knob). 0=Sine 1=Saw 2=Square 3=Triangle.
 * Phase is in radians (possibly unbounded — saw/triangle wrap via floor).
 *
 * BYTE-PARITY CONTRACT: the Sine case is the exact `sin(phase)` expression
 * the engines used as literals, and the Square case is the exact
 * `(sin(phase) >= 0) ? 1 : -1` expression from the hi-hat cluster — so an
 * explicit selection of the engine's default waveform (the browser path,
 * which does not elide defaults) renders bit-identically to the NaN
 * fallback path. */
double osc_wave_shared(int wave, double phase) {
  switch (wave) {
  case 1: { /* saw */
    double t = phase * (1.0 / (2.0 * M_PI));
    t -= floor(t);
    return 2.0 * t - 1.0;
  }
  case 2: /* square — mirrors the hi-hat cluster expression exactly */
    return (sin(phase) >= 0.0) ? 1.0 : -1.0;
  case 3: { /* triangle */
    double t = phase * (1.0 / (2.0 * M_PI));
    t -= floor(t);
    return (t < 0.5) ? (4.0 * t - 1.0) : (3.0 - 4.0 * t);
  }
  default: /* sine — mirrors the engines' literal sin(phase) */
    return sin(phase);
  }
}
static inline double osc_wave(int wave, double phase) {
  return osc_wave_shared(wave, phase);
}

/* Thread-local scratch buffer for pitch-shift resampling. Replaces the
 * per-call malloc/free pair that used to appear at five sites in this
 * file (apply_post_params + render_snare_p + render_cowbell_p; the
 * render_kick_p / render_tom_p / render_cowbell_p sites moved to the modular
 * engine in the Phase-3..6 cutovers, leaving only apply_post_params here). The
 * buffer grows lazily and lives for the lifetime
 * of the thread. _Thread_local makes the multi-thread story explicit:
 * each goroutine that crosses into CGo gets its own scratch. Under
 * Emscripten single-thread the storage class degrades to a plain static.
 *
 * Why never freed: in production the audio render thread reuses the
 * same scratch on every cache-miss render — freeing on each call is what
 * we are removing. Cache misses are rare relative to lifetime, and the
 * scratch is bounded by the longest single render (one drum hit). */
static _Thread_local float *g_pitch_scratch = NULL;
static _Thread_local size_t  g_pitch_scratch_cap = 0;

static float *pitch_scratch(size_t want) {
  if (want > g_pitch_scratch_cap) {
    float *grown = (float *)realloc(g_pitch_scratch, want * sizeof(float));
    if (!grown) return NULL;
    g_pitch_scratch = grown;
    g_pitch_scratch_cap = want;
  }
  return g_pitch_scratch;
}

/* render_snare_internal / render_snare DELETED — the snare family migrated to the
 * modular engine (Phase-5); modular_gen_slot_snare variant 0 (source==7) is the
 * verbatim transplant. osc_wave / softsat remain shared and unchanged. */

/* render_kick_internal / render_kick deleted — the base kick migrated to the
 * modular engine (Phase-3); modular_gen_slot_kick variant 0 is the verbatim
 * transplant. osc_wave / softsat remain shared and unchanged. */


/* render_hihat* / render_open_hihat* / render_cowbell* / render_shaker* /
 * render_ride* / render_crash* (internal + EXPORT wrappers + the _p variants
 * below) DELETED — the cymbal family migrated to the modular engine (Phase-6);
 * modular_gen_slot_cymbal (source==9) is the verbatim transplant, selected by
 * gen_cym_variant (0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash). The
 * curated cymbal knobs travel as gen_cym_* gen-slot columns; the shared POST stage
 * reproduces each legacy _p wrapper's decay→brightness→drive (or pitch→decay)
 * post-processing (PostOrder=0). osc_wave / softsat remain shared and unchanged. */

/* render_tom_internal / render_tom / render_tom_high_internal / render_tom_high
 * / render_tom_low_internal / render_tom_low DELETED — the tom family migrated
 * to the modular engine (Phase-4); modular_gen_slot_tom (source==6) is the
 * verbatim transplant, selected by gen_tom_variant (0=tom 1=high 2=low). */

/* render_clap_internal / render_clap DELETED — the clap migrated to the modular
 * engine (Phase-5); modular_gen_slot_clap (source==8) is the verbatim transplant
 * (pure noise, separate source from the snare-ish trio). */

/* render_snare_rimshot_internal / render_snare_rimshot DELETED — migrated to the
 * modular engine (Phase-5); modular_gen_slot_snare variant 1 (source==7). */

/* render_snare_sidestick_internal / render_snare_sidestick DELETED — migrated to
 * the modular engine (Phase-5); modular_gen_slot_snare variant 2 (source==7). */

/* render_kick_deep / punchy / lofi / tight (internal + EXPORT wrappers) deleted
 * — migrated to the modular engine (Phase-3); modular_gen_slot_kick variants
 * 1..4 are the verbatim transplants. */

// Decode an audio file (WAV/MP3/FLAC/OGG where enabled) into mono f32 at the
// engine sample rate. Returns frame count or a negative ma_result code.
EXPORT int load_audio(const char *path, int targetSampleRate, float **buffer, int *sampleRate) {
  const ma_uint32 targetRate = (ma_uint32)targetSampleRate;
  ma_decoder_config cfg = ma_decoder_config_init(ma_format_f32, 1, targetRate);
  ma_decoder dec;
  ma_result res = ma_decoder_init_file(path, &cfg, &dec);
  if (res != MA_SUCCESS) {
    return res;
  }
  ma_uint64 frames;
  res = ma_decoder_get_length_in_pcm_frames(&dec, &frames);
  if (res != MA_SUCCESS) {
    ma_decoder_uninit(&dec);
    return res;
  }
  float *data = (float *)malloc(frames * sizeof(float));
  if (data == NULL) {
    ma_decoder_uninit(&dec);
    return MA_OUT_OF_MEMORY;
  }
  res = ma_decoder_read_pcm_frames(&dec, data, frames, NULL);
  if (res != MA_SUCCESS) {
    free(data);
    ma_decoder_uninit(&dec);
    return res;
  }
  *buffer = data;
  if (sampleRate != NULL)
    *sampleRate = dec.outputSampleRate;
  ma_decoder_uninit(&dec);
  return (int)frames;
}

// Bass family (bass-guitar Karplus-Strong + sub-bass 808) migrated to the
// unified modular engine (Phase-2 modular-synth-unification). The bespoke
// render_bass_guitar* / render_sub_bass* C paths are DELETED; the modular
// engine's source==3 (KS) and source==4 (analytic dual-osc) gen-slot voices
// (modular_stages.c) reproduce them byte-identically. osc_wave_shared /
// softsat_shared / apply_post_params remain shared and unchanged.

/* ==== Parameterized render variants ====
 * Each _p() function accepts an optional synth_params pointer.
 * When params is NULL, behavior is identical to the original function.
 * Params modify pitch, decay, tone, attack, drive, body, brightness.
 *
 * The 6 original _p() variants (snare/kick/hihat/clap/tom/cowbell) were
 * hand-written with bespoke per-knob handling. The 14 new variants added
 * in the SynthRecipe Phase 2 rollout share post-processing via the
 * apply_post_params helper below; each declares which knobs are wired up
 * via a post_config flag set, plus its own decay time constant.
 */

#include "synth_post.h"

/* apply_post_params is the shared parameter-driven post-processor used by
 * every Phase-2 _p() variant. Each enabled flag toggles one transform; the
 * exact formula matches the hand-written 6 _p() variants above so existing
 * audio output is bit-identical when only the same knobs are wired up. */
void apply_post_params(float *out, int sampleRate, int samples,
                       const synth_params *params,
                       const post_config *cfg) {
  if (!params || !cfg) return;

  if (cfg->pitch) {
    float pitchSt = sp_pitch(params);
    if (pitchSt != 0.0f) {
      double rate = pow(2.0, (double)pitchSt / 12.0);
      float *tmp = pitch_scratch((size_t)samples);
      if (tmp) {
        for (int i = 0; i < samples; i++) tmp[i] = out[i];
        for (int i = 0; i < samples; i++) {
          double srcIdx = (double)i * rate;
          int idx = (int)srcIdx;
          if (idx >= samples) { out[i] = 0.0f; continue; }
          int idx1 = idx + 1 < samples ? idx + 1 : idx;
          double frac = srcIdx - (double)idx;
          out[i] = (float)((1.0 - frac) * (double)tmp[idx] + frac * (double)tmp[idx1]);
        }
      }
    }
  }

  if (cfg->decay) {
    float decayMul = sp_decay(params);
    /* Clamp to a strictly-positive epsilon. Without the guard, decayMul
     * = 0 (a real value the user can dial in from the slider's leftmost
     * position) makes 1/decayMul = +Inf, then at t=0 the expression
     * `0 * 8.0 * +Inf` evaluates to NaN, exp(NaN) = NaN, and the NaN
     * propagates through the channel's EQ BiquadFilter chain — leaving
     * the chain permanently corrupted with NaN coefficients. The user
     * hears "audio stops" + 22+ BiquadFilterNode warnings.
     * 0.01 matches the sp_env_decay() clamp used by the unparameterized
     * renderers in this file. */
    if (decayMul < 0.01f) decayMul = 0.01f;
    if (decayMul != 1.0f) {
      for (int i = 0; i < samples; i++) {
        double t = (double)i / (double)sampleRate;
        double env = exp(-t * cfg->decay_rate * (1.0 / (double)decayMul - 1.0));
        out[i] *= (float)env;
      }
    }
  }

  if (cfg->body) {
    float body = sp_body(params);
    if (body > 0.0f) {
      double cutoff = 120.0;
      double rc = 1.0 / (2.0 * M_PI * cutoff);
      double dt = 1.0 / (double)sampleRate;
      double alpha = dt / (rc + dt);
      double prev = 0;
      for (int i = 0; i < samples; i++) {
        prev += alpha * ((double)out[i] - prev);
        out[i] = out[i] * (1.0f - body * 0.5f) + (float)prev * body;
      }
    }
  }

  if (cfg->brightness) {
    float brightness = sp_brightness(params);
    if (brightness > 0.0f) {
      double cutoff = 4000.0 + brightness * 8000.0;
      double rc = 1.0 / (2.0 * M_PI * cutoff);
      double dt = 1.0 / (double)sampleRate;
      double alpha = rc / (rc + dt);
      double prevIn = 0, prevOut = 0;
      for (int i = 0; i < samples; i++) {
        double x = (double)out[i];
        double y = alpha * (prevOut + x - prevIn);
        prevIn = x; prevOut = y;
        out[i] = (float)(out[i] * (1.0 - brightness * 0.5) + y * brightness * 0.5);
      }
    }
  }

  if (cfg->tone) {
    float tone = sp_tone(params);
    if (tone < 0.0f) {
      double cutoff = 2000.0 + (1.0 + tone) * 6000.0;
      double rc = 1.0 / (2.0 * M_PI * cutoff);
      double dt = 1.0 / (double)sampleRate;
      double alpha = dt / (rc + dt);
      double prev = 0;
      for (int i = 0; i < samples; i++) {
        prev += alpha * ((double)out[i] - prev);
        out[i] = (float)prev;
      }
    }
  }

  if (cfg->drive) {
    if (sp_drive(params) > 0.0f) {
      for (int i = 0; i < samples; i++) {
        out[i] = sp_saturate(out[i], params);
      }
    }
  }
}


/* snare_fund + render_snare_p DELETED — the snare family migrated to render_modular_p
 * (Phase-5). The [80,2000] fundamental clamp now lives in snareFundamental
 * (snare_modular_push.go); the bespoke inline post (pitch → decay(rate 8) → drive
 * → tone) is reproduced by the shared modular POST stage with post_order=2 (the
 * legacy drive-before-tone op order). */

/* render_kick_p deleted — the base kick migrated to render_modular_p (Phase-3).
 * The POST stage (pitch/decay/body/drive) is the shared modular post; the
 * base-kick drive-before-body order is modular post_order=1. */

/* render_hihat_p / render_open_hihat_p / render_cowbell_p / render_shaker_p /
 * render_ride_p / render_crash_p DELETED — the cymbal family migrated to
 * render_modular_p (Phase-6). The bespoke inline / apply_post_params post chains
 * (hihat: decay(rate 20)→brightness→drive; open-hihat: {decay_rate 20, decay,
 * brightness, drive}; cowbell: pitch→decay(rate 12); shaker: {decay_rate 15,
 * decay, brightness, drive}; ride: {decay_rate 12, …}; crash: {decay_rate 4, …})
 * are reproduced by the shared modular POST stage (PostOrder=0) wired per
 * variant in cymbalPostConfigFor (cymbal_modular_push.go). */

/* render_clap_p DELETED — clap renders via render_modular_p (Phase-5); the bespoke
 * inline post (decay(rate 10) → drive) is the shared modular POST stage
 * {decay_rate=10, gates {decay,drive}}. */

/* tom_fund / render_tom_p / render_tom_high_p / render_tom_low_p DELETED — tom
 * renders via render_modular_p (Phase-4); the [40,400] fundamental clamp now lives
 * in tomFundamental (tom_modular_push.go) and the bespoke inline post (pitch ->
 * decay(rate 8) -> drive) is reproduced by the shared modular POST stage
 * (decay_rate=8, gates {pitch,decay,drive}). */

/* render_snare_rimshot_p / render_snare_sidestick_p DELETED — migrated to
 * render_modular_p (Phase-5); the apply_post_params {decay_rate=8, pitch, decay,
 * drive, tone} config is the shared modular POST stage (post_order=0). */

/* kick_fund + render_kick_deep_p / punchy_p / lofi_p / tight_p deleted — the
 * kick family migrated to render_modular_p (Phase-3). The fundamental clamp
 * ([30,200]) lives in the Go binding (kickFundamental); the POST stage
 * {decay_rate=6, pitch,decay,body,drive} is the shared modular post. */

EXPORT const char *result_description(int code) {
  return ma_result_description((ma_result)code);
}
