/**
 * Stored-ZIP + WAV decode + RMS — factored from recording_lifecycle.browser.test.js
 * so the agent harness can verify captured audio is non-silent (measure_audio tool).
 * Assert-free: throws Error on malformed input.
 */

/** Parse a stored-only PKZIP (as produced by recording_encoder_worker.js). → { name: Uint8Array }. */
export function parseZip(buf) {
  const u8 = new Uint8Array(buf);
  const dv = new DataView(u8.buffer, u8.byteOffset, u8.byteLength);
  let eocd = -1;
  for (let i = u8.length - 22; i >= 0; i--) {
    if (dv.getUint32(i, true) === 0x06054b50) { eocd = i; break; }
  }
  if (eocd < 0) throw new Error("ZIP: EOCD not found");
  const cdEntries = dv.getUint16(eocd + 10, true);
  const cdOff = dv.getUint32(eocd + 16, true);
  const out = {};
  let p = cdOff;
  for (let i = 0; i < cdEntries; i++) {
    if (dv.getUint32(p, true) !== 0x02014b50) throw new Error("ZIP: bad central dir signature");
    const uncompSize = dv.getUint32(p + 24, true);
    const nameLen = dv.getUint16(p + 28, true);
    const extraLen = dv.getUint16(p + 30, true);
    const commentLen = dv.getUint16(p + 32, true);
    const localOff = dv.getUint32(p + 42, true);
    const name = new TextDecoder().decode(u8.subarray(p + 46, p + 46 + nameLen));
    p += 46 + nameLen + extraLen + commentLen;
    if (dv.getUint32(localOff, true) !== 0x04034b50) throw new Error("ZIP: bad local header signature");
    const lNameLen = dv.getUint16(localOff + 26, true);
    const lExtraLen = dv.getUint16(localOff + 28, true);
    const dataStart = localOff + 30 + lNameLen + lExtraLen;
    out[name] = u8.subarray(dataStart, dataStart + uncompSize);
  }
  return out;
}

/** Decode a PCM16/24 or IEEE-float32 WAV. → { sampleRate, channels, bitDepth, fmtCode, samples:Float32Array }. */
export function decodeWav(buf) {
  const dv = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  if (dv.getUint32(0, false) !== 0x52494646) throw new Error("WAV: missing RIFF");
  if (dv.getUint32(8, false) !== 0x57415645) throw new Error("WAV: missing WAVE");
  const fmtCode = dv.getUint16(20, true);
  const channels = dv.getUint16(22, true);
  const sampleRate = dv.getUint32(24, true);
  const bitDepth = dv.getUint16(34, true);
  let p = 36;
  while (p < buf.length) {
    const id = dv.getUint32(p, false);
    const size = dv.getUint32(p + 4, true);
    p += 8;
    if (id === 0x64617461) { // "data"
      return { sampleRate, channels, bitDepth, fmtCode, samples: decodePCM(buf, p, size, fmtCode, bitDepth) };
    }
    p += size;
  }
  throw new Error("WAV: no data chunk");
}

function decodePCM(buf, off, size, fmtCode, bitDepth) {
  const dv = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  if (fmtCode === 3 && bitDepth === 32) {
    const n = size / 4, out = new Float32Array(n);
    for (let i = 0; i < n; i++) out[i] = dv.getFloat32(off + i * 4, true);
    return out;
  }
  if (fmtCode === 1 && bitDepth === 16) {
    const n = size / 2, out = new Float32Array(n);
    for (let i = 0; i < n; i++) out[i] = dv.getInt16(off + i * 2, true) / 32768;
    return out;
  }
  if (fmtCode === 1 && bitDepth === 24) {
    const n = size / 3, out = new Float32Array(n);
    for (let i = 0; i < n; i++) {
      let v = buf[off + i * 3] | (buf[off + i * 3 + 1] << 8) | (buf[off + i * 3 + 2] << 16);
      if (v & 0x800000) v -= 0x1000000;
      out[i] = v / 8388608;
    }
    return out;
  }
  throw new Error(`WAV: unsupported format code=${fmtCode} bits=${bitDepth}`);
}

/** Root-mean-square of a sample buffer. */
export function rms(samples) {
  if (!samples || samples.length === 0) return 0;
  let s = 0;
  for (let i = 0; i < samples.length; i++) s += samples[i] * samples[i];
  return Math.sqrt(s / samples.length);
}
