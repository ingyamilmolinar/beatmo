/** Between the C render and the Sampler edit, on both browser render paths, step for step desktop's renderDocBlock:
 * voice FX through the same C effects, silence from floor(gateTail × length), peak-normalise × amp, a linear end fade
 * over the last 5 % (64-512 frames). Float32 arithmetic exactly as Go's, so only the C can differ. post = {amp, gateTail, fx}. */

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
