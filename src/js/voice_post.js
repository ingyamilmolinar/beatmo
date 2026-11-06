/**
 * voice_post.js — what happens to a synth voice between the C render and the
 * Sampler edit, on both browser render paths: the render worker
 * (synth_render_worker.js) and the main-thread fallback (audio.js
 * renderVoiceSync). It is desktop's renderDocBlock + normalizeAndScaleAmp
 * (internal/audio/doc_voice_native.go, export.go) step for step:
 *
 *   1. the document's voice FX, in order, each through the SAME C effect
 *      desktop runs (fx_host.c ifx_create / ifx_run, from drums.single.js —
 *      dsp_host.js), with the complete parameter set Go pushed,
 *   2. the gate tail: silence from floor(gateTail × length) on,
 *   3. peak-normalise, × amplitude, then a linear fade over the last 5 %
 *      (64…512 frames) so a voice never ends on a click.
 *
 * Step 3 is float32 arithmetic exactly like Go's: a Float32Array element times
 * a float32-valued number is an exact double product rounded once on store,
 * and Math.fround of a double quotient/difference of float32 values equals the
 * float32 operation. So the post-processing itself is bit-identical across
 * platforms; only the C (libm, compiler) can differ, by float rounding.
 *
 * post = { amp, gateTail, fx: [{ type, params }] } (setInstrumentBlock).
 */

import { applyEffectsToBuffer } from "./dsp_host.js";

export function applyVoicePost(m, data, sr, post) {
  if (!post) return data;
  if (Array.isArray(post.fx) && post.fx.length > 0) applyEffectsToBuffer(m, data, sr, post.fx);
  const gate = post.gateTail;
  if (gate > 0 && gate < 1) data.fill(0, Math.trunc(data.length * gate));
  let peak = 0;
  for (let i = 0; i < data.length; i++) {
    const v = Math.abs(data[i]);
    if (v > peak) peak = v;
  }
  if (peak > 0) {
    const inv = Math.fround(1 / peak);
    for (let i = 0; i < data.length; i++) data[i] *= inv;
  }
  const amp = Math.fround(Number.isFinite(post.amp) ? post.amp : 0.6);
  for (let i = 0; i < data.length; i++) data[i] *= amp;
  const n = data.length;
  if (n > 0) {
    let fade = Math.trunc(n / 20);
    if (fade < 64) fade = 64;
    if (fade > 512) fade = 512;
    if (fade > n) fade = n;
    const start = n - fade;
    for (let i = 0; i < fade; i++) {
      data[start + i] *= Math.fround(1 - Math.fround(i / fade));
    }
  }
  return data;
}
