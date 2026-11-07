/** The samples one note puts into the mix, built exactly as desktop's mixer builds them, then played at rate 1 through one
 * gain: WebAudio's resampler is never used (it differs per engine). Double arithmetic over the float32 render, rounded
 * once when stored: the only seam (≤ 1 ulp). */

/** The first floor(len × dur) samples, the last `fade` (sr × gateFadeMs / 1000, at most half, at least 1) ramped to zero.
 * null when nothing is gated. */
export function gateLength(len, dur, sr, gateFadeMs) {
  if (!(dur > 0 && dur < 1)) return null;
  const limit = Math.trunc(len * dur);
  if (limit <= 0) return null;
  let fade = Math.trunc((sr * gateFadeMs) / 1000);
  if (fade > Math.trunc(limit / 2)) fade = Math.trunc(limit / 2);
  if (fade < 1) fade = 1;
  return { limit, fade };
}

/** gate from gateLength or null; step = 2^(pitch/12)/dur for drums, 1 melodic (output k reads src at k × step, silent
 * past its end); fadeIn = frames of the linear anti-pop ramp. */
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
