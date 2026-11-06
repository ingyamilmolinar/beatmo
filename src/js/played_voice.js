/**
 * played_voice.js — the samples one note puts into the mix, built exactly as
 * desktop's mixer builds them (internal/audio engine_play.go playedVoice →
 * voice_gate.go → engine_resample.go → voice_antipop.go fade-in), so a note is
 * the same samples on both platforms. The browser then plays this buffer at
 * playbackRate 1 through one gain = headroom × the note's volume (desktop:
 * scaledVoice × mixHeadroom). WebAudio's own resampler is never used: its
 * interpolation differs per engine (Firefox is a sinc filter).
 *
 * Every step is double arithmetic over the float32 render, rounded to float32
 * once when stored — desktop keeps the double until the channel's float32
 * stage, which is the only seam (≤ 1 ulp). xplat_audio_stages.browser.test.js
 * pins it per sample against desktop's mixer.
 */

/**
 * The gate of a melodic note shortened by its Duration (dur < 1): the first
 * floor(len × dur) samples, the last `fade` of them ramped linearly to zero
 * (fade = sr × gateFadeMs / 1000, at most half the note, at least 1) —
 * gateVoice. Returns null when nothing is gated.
 */
export function gateLength(len, dur, sr, gateFadeMs) {
  if (!(dur > 0 && dur < 1)) return null;
  const limit = Math.trunc(len * dur);
  if (limit <= 0) return null;
  let fade = Math.trunc((sr * gateFadeMs) / 1000);
  if (fade > Math.trunc(limit / 2)) fade = Math.trunc(limit / 2);
  if (fade < 1) fade = 1;
  return { limit, fade };
}

/**
 * playedVoiceSamples builds the played voice of a rendered buffer `src`
 * (Float32Array) as a Float32Array:
 *   - gate:  { limit, fade } from gateLength, or null;
 *   - step:  the resample rate (drums: 2^(pitch/12) / dur; melodic: 1) —
 *            output k is src linearly interpolated at k × step, silent past
 *            its end, for every k with k × step < its length;
 *   - fadeIn: frames of the linear anti-pop fade-in (factor k / fadeIn).
 */
export function playedVoiceSamples(src, gate, step, fadeIn) {
  const n0 = gate ? Math.min(gate.limit, src.length) : src.length;
  const sample = (i) => {
    if (i >= n0) return 0;
    let s = src[i];
    if (gate) {
      const left = gate.limit - i;
      if (left <= gate.fade) s *= left / gate.fade;
    }
    return s;
  };
  const st = step > 0 ? step : 1;
  let outN = 0;
  if (n0 > 0) {
    outN = Math.ceil(n0 / st);
    while (outN > 0 && (outN - 1) * st >= n0) outN--;
    while (outN * st < n0) outN++;
  }
  const out = new Float32Array(outN);
  for (let k = 0; k < outN; k++) {
    const pos = k * st;
    const i0 = Math.trunc(pos);
    const s0 = sample(i0);
    const s1 = sample(i0 + 1);
    let v = s0 + (s1 - s0) * (pos - i0);
    if (k < fadeIn) v *= k / fadeIn;
    out[k] = v;
  }
  return out;
}
