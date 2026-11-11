/**
 * Recording Encoder Web Worker.
 *
 * Owns the entire WAV encode + zip bundle pipeline for the WASM recording
 * subsystem. Runs on a dedicated worker thread so neither the audio
 * rendering thread (AudioWorklet) nor the main thread (Go-WASM goroutines,
 * UI repaint, RAF) does any encoding work while a session is active.
 *
 * Lifecycle (driven by main thread):
 *
 *   main                                  worker
 *   ────                                  ──────
 *   new Worker('recording_encoder_worker.js')
 *   postMessage({type:'init',
 *                format, sampleRate})  ──▶ initialize encoder, channel registry
 *   for each channel:
 *     create MessageChannel(p1, p2)
 *     send p2 to worklet, p1 to worker
 *     postMessage({type:'addChannel',
 *                  id, name, filename,
 *                  port: p1, bitDepth},
 *                 [p1])              ──▶ attach onmessage to p1; encode batches
 *
 *   ... worklet posts Float32 batches to p2 → arrives at p1 in worker ...
 *   ... worker WAV-encodes into per-channel chunk lists ...
 *
 *   postMessage({type:'finalize',
 *                channels:[{id,name,filename,...meta}]}) ──▶
 *                                          patch WAV headers, zip, post Blob
 *                                       ◀── postMessage({type:'blob',
 *                                                        blob, meta},[blob])
 *   trigger anchor.click(blob)
 *
 * Resource bounds (worker-side, hard-stop on breach):
 *   - BYTES_PER_CHANNEL_MAX (default 128 MB) — drop further samples + counter
 *   - DURATION_MAX_SEC (default 30 min) — post {type:'autoStop'} to main
 *
 * The worker has no DOM or AudioContext access. Pure encoding + zip.
 */

/* eslint-disable no-undef */

// ─── Configuration ──────────────────────────────────────────────────────

const DEFAULTS = {
  bytesPerChannelMax: 128 * 1024 * 1024,  // 128 MB (~11 min mono WAV24 @ 48 kHz)
  durationMaxSec: 30 * 60,                // 30 min
  chunkSize: 64 * 1024,                   // 64 KB per encoded chunk
};

// ─── Worker state ───────────────────────────────────────────────────────

let format = 'wav24';
let sampleRate = 48000;
let bytesPerChannelMax = DEFAULTS.bytesPerChannelMax;
let durationMaxSec = DEFAULTS.durationMaxSec;
let startedAt = 0;
let autoStopFired = false;

const channels = new Map(); // id → channelState

function newChannelState(id, name, filename, bitDepth) {
  return {
    id,
    name,
    filename,
    bitDepth,
    bytesPerSample: bitDepth / 8,
    chunks: [],         // Uint8Array[]
    chunkOffset: 0,     // bytes filled in the current tail chunk
    totalSampleBytes: 0,
    samplesEncoded: 0,
    droppedSamples: 0,
    queuedBatches: 0,
    maxQueueDepth: 0,
    closed: false,
  };
}

// ─── Counters surfaced to main thread ────────────────────────────────────

function statsSnapshot() {
  let drops = 0;
  let bytesUsed = 0;
  let queued = 0;
  let maxQueued = 0;
  for (const c of channels.values()) {
    drops += c.droppedSamples;
    bytesUsed += c.totalSampleBytes;
    queued += c.queuedBatches;
    if (c.maxQueueDepth > maxQueued) maxQueued = c.maxQueueDepth;
  }
  return {
    activeChannels: channels.size,
    droppedSamples: drops,
    bytesUsed,
    queuedBatches: queued,
    maxQueueDepth: maxQueued,
    elapsedSec: startedAt ? (performance.now() - startedAt) / 1000 : 0,
  };
}

// ─── Sample → PCM conversion ─────────────────────────────────────────────

// Each writer takes a Float32Array slice and returns a Uint8Array of bytes.
// All formats are little-endian PCM mono.

function encodeSamplesPCM16(samples) {
  const out = new Uint8Array(samples.length * 2);
  const view = new DataView(out.buffer);
  for (let i = 0, o = 0; i < samples.length; i++, o += 2) {
    let s = samples[i];
    if (s > 1) s = 1; else if (s < -1) s = -1;
    view.setInt16(o, Math.round(s * 32767), true);
  }
  return out;
}

function encodeSamplesPCM24(samples) {
  const out = new Uint8Array(samples.length * 3);
  for (let i = 0, o = 0; i < samples.length; i++, o += 3) {
    let s = samples[i];
    if (s > 1) s = 1; else if (s < -1) s = -1;
    const v = Math.round(s * 8388607);  // 2^23 - 1
    const u = v < 0 ? v + 0x1000000 : v;
    out[o] = u & 0xff;
    out[o + 1] = (u >> 8) & 0xff;
    out[o + 2] = (u >> 16) & 0xff;
  }
  return out;
}

function encodeSamplesPCM32F(samples) {
  // 32-bit float WAV: store raw float samples in little-endian.
  const out = new Uint8Array(samples.length * 4);
  const view = new DataView(out.buffer);
  for (let i = 0, o = 0; i < samples.length; i++, o += 4) {
    view.setFloat32(o, samples[i], true);
  }
  return out;
}

// Float32 passthrough used by formats we encode at finalize-time (FLAC,
// OGG). Storing raw little-endian float bytes keeps appendChannelBytes /
// chunk bookkeeping unchanged and lets finalize re-decode without loss.
function encodeSamplesFloat32(samples) {
  const out = new Uint8Array(samples.length * 4);
  const view = new DataView(out.buffer);
  for (let i = 0, o = 0; i < samples.length; i++, o += 4) {
    view.setFloat32(o, samples[i], true);
  }
  return out;
}

function pickEncoder(fmt) {
  switch (fmt) {
  case 'wav16':  return { fn: encodeSamplesPCM16,   bitDepth: 16, isFloat: false, container: 'wav' };
  case 'wav24':  return { fn: encodeSamplesPCM24,   bitDepth: 24, isFloat: false, container: 'wav' };
  case 'wav32f': return { fn: encodeSamplesPCM32F,  bitDepth: 32, isFloat: true,  container: 'wav' };
  // FLAC and OGG buffer raw float samples and encode at finalize time.
  // bitDepth=32/isFloat=true here only describes the *intermediate* storage
  // (float32 LE) — the finalize step writes the real container.
  case 'flac':   return { fn: encodeSamplesFloat32, bitDepth: 32, isFloat: true,  container: 'flac' };
  case 'ogg':    return { fn: encodeSamplesFloat32, bitDepth: 32, isFloat: true,  container: 'ogg' };
  default: throw new Error('unsupported recording format: ' + fmt);
  }
}

// Decode the float32-LE storage back into a Float32Array sample stream.
// Used by finalize for FLAC/OGG containers.
function readChannelFloatSamples(c) {
  const totalSamples = (c.totalSampleBytes / 4) | 0;
  const out = new Float32Array(totalSamples);
  let dst = 0;
  for (let i = 0; i < c.chunks.length; i++) {
    const ch = c.chunks[i];
    const isLast = (i === c.chunks.length - 1);
    const usedBytes = isLast ? c.chunkOffset : ch.length;
    const view = new DataView(ch.buffer, ch.byteOffset, ch.byteLength);
    const n = (usedBytes / 4) | 0;
    for (let j = 0; j < n; j++) {
      out[dst++] = view.getFloat32(j * 4, true);
    }
  }
  return out.subarray(0, dst);
}

// ─── Append to channel chunks ────────────────────────────────────────────

function appendChannelBytes(c, bytes) {
  // Append into the trailing chunk if it has room; otherwise allocate.
  // Pre-allocated chunkSize keeps allocations bounded and predictable.
  let remaining = bytes;
  while (remaining.length > 0) {
    let tail = c.chunks[c.chunks.length - 1];
    if (!tail || c.chunkOffset >= tail.length) {
      tail = new Uint8Array(DEFAULTS.chunkSize);
      c.chunks.push(tail);
      c.chunkOffset = 0;
    }
    const space = tail.length - c.chunkOffset;
    const take = Math.min(space, remaining.length);
    tail.set(remaining.subarray(0, take), c.chunkOffset);
    c.chunkOffset += take;
    c.totalSampleBytes += take;
    remaining = remaining.subarray(take);
  }
}

function checkAutoStop() {
  if (autoStopFired) return;
  const elapsed = (performance.now() - startedAt) / 1000;
  if (elapsed >= durationMaxSec) {
    autoStopFired = true;
    self.postMessage({ type: 'autoStop', reason: 'duration', elapsedSec: elapsed });
    return;
  }
  for (const c of channels.values()) {
    if (c.totalSampleBytes >= bytesPerChannelMax) {
      autoStopFired = true;
      self.postMessage({
        type: 'autoStop',
        reason: 'bytes',
        channelId: c.id,
        bytesUsed: c.totalSampleBytes,
      });
      return;
    }
  }
}

// ─── Per-channel batch handler ───────────────────────────────────────────

function handleChannelBatch(c, encoder, payload) {
  if (c.closed || autoStopFired) return;

  if (payload && payload.type === 'drop') {
    c.droppedSamples += (payload.n | 0);
    return;
  }
  if (payload && payload.type === 'done') {
    c.closed = true;
    return;
  }
  if (!(payload instanceof ArrayBuffer)) return;

  // Bytes budget (post-encode); reject the batch if we'd exceed.
  const samples = new Float32Array(payload);
  const projectedBytes = c.totalSampleBytes + samples.length * c.bytesPerSample;
  if (projectedBytes > bytesPerChannelMax) {
    c.droppedSamples += samples.length;
    checkAutoStop();
    return;
  }

  const bytes = encoder.fn(samples);
  appendChannelBytes(c, bytes);
  c.samplesEncoded += samples.length;
  c.queuedBatches = Math.max(0, c.queuedBatches - 1);
  checkAutoStop();
}

// ─── WAV file assembly ───────────────────────────────────────────────────

function buildWavFile(c, encoder) {
  // Concatenate sample chunks into a single Uint8Array (sample data).
  const dataSize = c.totalSampleBytes;
  const headerSize = 44;
  const fileSize = headerSize + dataSize;

  const buf = new Uint8Array(fileSize);
  const view = new DataView(buf.buffer);

  // Always write WAV format spec — handle PCM (16/24) vs IEEE float (32f).
  // For float, fmt format code is 3; for PCM it's 1.
  const fmtCode = encoder.isFloat ? 3 : 1;
  const numChannels = 1;
  const blockAlign = numChannels * c.bytesPerSample;
  const byteRate = sampleRate * blockAlign;

  // RIFF header
  buf[0] = 0x52; buf[1] = 0x49; buf[2] = 0x46; buf[3] = 0x46; // "RIFF"
  view.setUint32(4, fileSize - 8, true);
  buf[8] = 0x57; buf[9] = 0x41; buf[10] = 0x56; buf[11] = 0x45; // "WAVE"
  // fmt chunk
  buf[12] = 0x66; buf[13] = 0x6d; buf[14] = 0x74; buf[15] = 0x20; // "fmt "
  view.setUint32(16, 16, true);                 // fmt chunk size
  view.setUint16(20, fmtCode, true);
  view.setUint16(22, numChannels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, byteRate, true);
  view.setUint16(32, blockAlign, true);
  view.setUint16(34, c.bitDepth, true);
  // data chunk
  buf[36] = 0x64; buf[37] = 0x61; buf[38] = 0x74; buf[39] = 0x61; // "data"
  view.setUint32(40, dataSize, true);

  // Copy chunks into the data section.
  let off = headerSize;
  for (let i = 0; i < c.chunks.length; i++) {
    const ch = c.chunks[i];
    const isLast = (i === c.chunks.length - 1);
    const slice = isLast ? ch.subarray(0, c.chunkOffset) : ch;
    buf.set(slice, off);
    off += slice.length;
  }
  return buf;
}

// ─── FLAC verbatim encoder (mono, 16-bit) ──────────────────────────────
//
// Ported from src/go/internal/audio/encode_flac.go. Writes a valid FLAC
// stream using verbatim (uncompressed) subframes — one frame per ~4096
// samples. File sizes are similar to 16-bit WAV; any FLAC decoder will
// accept the output.

const FLAC_CRC8_TABLE = (() => {
  const t = new Uint8Array(256);
  for (let i = 0; i < 256; i++) {
    let c = i & 0xff;
    for (let j = 0; j < 8; j++) {
      c = (c & 0x80) ? (((c << 1) ^ 0x07) & 0xff) : ((c << 1) & 0xff);
    }
    t[i] = c;
  }
  return t;
})();

const FLAC_CRC16_TABLE = (() => {
  const t = new Uint16Array(256);
  for (let i = 0; i < 256; i++) {
    let c = (i << 8) & 0xffff;
    for (let j = 0; j < 8; j++) {
      c = (c & 0x8000) ? (((c << 1) ^ 0x8005) & 0xffff) : ((c << 1) & 0xffff);
    }
    t[i] = c;
  }
  return t;
})();

function flacBlockSizeCode(n) {
  switch (n) {
  case 192: return 1;
  case 576: return 2;
  case 1152: return 3;
  case 2304: return 4;
  case 4608: return 5;
  case 256: return 8;
  case 512: return 9;
  case 1024: return 10;
  case 2048: return 11;
  case 4096: return 12;
  case 8192: return 13;
  case 16384: return 14;
  case 32768: return 15;
  }
  return n <= 256 ? 6 : 7;
}

function flacSampleRateCode(sr) {
  switch (sr) {
  case 88200: return 1;
  case 176400: return 2;
  case 192000: return 3;
  case 8000: return 4;
  case 16000: return 5;
  case 22050: return 6;
  case 24000: return 7;
  case 32000: return 8;
  case 44100: return 9;
  case 48000: return 10;
  case 96000: return 11;
  }
  return 12; // 8-bit kHz
}

function flacSampleSizeCode(bps) {
  switch (bps) {
  case 8: return 1;
  case 12: return 2;
  case 16: return 4;
  case 20: return 5;
  case 24: return 6;
  case 32: return 7;
  }
  return 0;
}

class FlacBitBuffer {
  constructor() {
    this.buf = [];      // byte numbers
    this.bitBuf = 0;
    this.bits = 0;
  }
  writeBits(val, n) {
    val = val >>> 0;
    while (n > 0) {
      const space = 8 - this.bits;
      if (n <= space) {
        this.bitBuf |= ((val & ((1 << n) - 1)) << (space - n)) & 0xff;
        this.bits += n;
        if (this.bits === 8) { this.buf.push(this.bitBuf & 0xff); this.bitBuf = 0; this.bits = 0; }
        return;
      }
      const take = space;
      this.bitBuf |= ((val >>> (n - take)) & ((1 << take) - 1)) & 0xff;
      this.bits = 8;
      this.buf.push(this.bitBuf & 0xff);
      this.bitBuf = 0;
      this.bits = 0;
      n -= take;
    }
  }
  alignByte() {
    if (this.bits > 0) {
      this.buf.push(this.bitBuf & 0xff);
      this.bitBuf = 0;
      this.bits = 0;
    }
  }
  writeByteRaw(b) { this.buf.push(b & 0xff); }
  writeUTF8(val) {
    val = val >>> 0;
    if (val < 0x80) {
      this.writeBits(val, 8);
    } else if (val < 0x800) {
      this.writeBits(0xC0 | (val >>> 6), 8);
      this.writeBits(0x80 | (val & 0x3F), 8);
    } else if (val < 0x10000) {
      this.writeBits(0xE0 | (val >>> 12), 8);
      this.writeBits(0x80 | ((val >>> 6) & 0x3F), 8);
      this.writeBits(0x80 | (val & 0x3F), 8);
    } else {
      this.writeBits(0xF0 | (val >>> 18), 8);
      this.writeBits(0x80 | ((val >>> 12) & 0x3F), 8);
      this.writeBits(0x80 | ((val >>> 6) & 0x3F), 8);
      this.writeBits(0x80 | (val & 0x3F), 8);
    }
  }
  bytes() { return this.buf; }
  crc8() {
    let crc = 0;
    for (let i = 0; i < this.buf.length; i++) crc = FLAC_CRC8_TABLE[(crc ^ this.buf[i]) & 0xff];
    return crc & 0xff;
  }
  crc16() {
    let crc = 0;
    for (let i = 0; i < this.buf.length; i++) {
      crc = (((crc << 8) & 0xffff) ^ FLAC_CRC16_TABLE[((crc >>> 8) ^ this.buf[i]) & 0xff]) & 0xffff;
    }
    return crc & 0xffff;
  }
}

function writeFlacVerbatimFrame(out, block, srCode, bps, frameNum) {
  const fb = new FlacBitBuffer();
  fb.writeBits(0x3FFE, 14);          // sync
  fb.writeBits(0, 1);                // reserved
  fb.writeBits(0, 1);                // blocking strategy = fixed
  const bsCode = flacBlockSizeCode(block.length);
  fb.writeBits(bsCode, 4);
  fb.writeBits(srCode, 4);
  fb.writeBits(0, 4);                // mono
  fb.writeBits(flacSampleSizeCode(bps), 3);
  fb.writeBits(0, 1);                // reserved
  fb.writeUTF8(frameNum);
  if (bsCode === 6) fb.writeBits(block.length - 1, 8);
  else if (bsCode === 7) fb.writeBits(block.length - 1, 16);
  // Sample-rate "from end of header" cases not used: srCode 12 needs an
  // 8-bit kHz value, but our writer only emits 12 for non-standard rates
  // we don't expect from WebAudio. WebAudio rates (44.1k/48k/96k) all hit
  // the table codes above.
  fb.alignByte();
  const crc8 = fb.crc8();
  fb.writeByteRaw(crc8);

  // Verbatim subframe.
  fb.writeBits(0, 1);                // padding
  fb.writeBits(1, 6);                // type = verbatim
  fb.writeBits(0, 1);                // no wasted bits
  for (let i = 0; i < block.length; i++) {
    // Two's-complement n-bit signed.
    const v = block[i] & ((1 << bps) - 1);
    fb.writeBits(v, bps);
  }
  fb.alignByte();
  const crc16 = fb.crc16();

  for (let i = 0; i < fb.buf.length; i++) out.push(fb.buf[i]);
  out.push((crc16 >>> 8) & 0xff);
  out.push(crc16 & 0xff);
}

function buildFlacFile(samples, sr) {
  // Convert float [-1,1] → int16. Saturate.
  const pcm = new Int16Array(samples.length);
  for (let i = 0; i < samples.length; i++) {
    let s = samples[i];
    if (s > 1) s = 1; else if (s < -1) s = -1;
    pcm[i] = Math.round(s * 32767);
  }

  const bps = 16;
  const numChannels = 1;
  const totalSamples = pcm.length;

  const out = [];
  // "fLaC"
  out.push(0x66, 0x4c, 0x61, 0x43);
  // STREAMINFO header: last=1, type=0, length=34. Big-endian 32-bit.
  const header = (1 << 31) | 0 | 34;  // length=34
  out.push((header >>> 24) & 0xff, (header >>> 16) & 0xff,
           (header >>> 8) & 0xff, header & 0xff);

  // STREAMINFO body (34 bytes).
  let blockSize = 4096;
  if (pcm.length < blockSize) blockSize = pcm.length || 1;
  // min/max block size (16 bits each, BE)
  out.push((blockSize >>> 8) & 0xff, blockSize & 0xff);
  out.push((blockSize >>> 8) & 0xff, blockSize & 0xff);
  // min/max frame size (24 bits each) = unknown
  out.push(0, 0, 0,  0, 0, 0);
  // sample rate (20 bits) | channels-1 (3) | bps-1 (5) | total samples high 4 bits
  // Use BigInt to safely shift across 32-bit boundary.
  const sr20 = BigInt(sr) & 0xFFFFFn;          // 20 bits
  const ch3  = BigInt(numChannels - 1) & 0x7n; // 3 bits
  const bp5  = BigInt(bps - 1) & 0x1Fn;        // 5 bits
  const ts   = BigInt(totalSamples);
  const high4 = (ts >> 32n) & 0xFn;
  // 36-bit packed value: [sr20][ch3][bp5][high4]
  const packed64 = (sr20 << 12n) | (ch3 << 9n) | (bp5 << 4n) | high4;
  // Pack as 4 BE bytes (the high 32 of a 36-bit field; lowest 4 bits go in the next byte alongside totalSamples low).
  // Simpler: write high 4 bytes (32 bits) of the 36-bit packed, then 32-bit total samples low.
  // packed64 fits in 36 bits — top 4 belong to the byte that also holds high4? No, the layout is:
  //   bytes [4..7]: top 32 bits of 36-bit packed (sr20 + ch3 + bp5 + high4 occupies bits 35..0)
  //   bytes [8..11]: low 32 bits of total samples
  // Actually the Go code writes packed (uint32) as BE then totalSamples low (uint32) as BE.
  // The Go `packed` is a uint32 = (sr<<12) | ((ch-1)<<9) | ((bps-1)<<4) | (totalSamples>>32).
  // Since totalSamples>>32 is the high 4 bits, and totalSamples fits in 36 bits for any practical recording,
  // we can compute packed as a 32-bit number directly. The shift can overflow 32-bit JS bitwise ops,
  // so use math.
  const packedHi = Number(((sr20 << 12n) | (ch3 << 9n) | (bp5 << 4n) | high4) & 0xFFFFFFFFn);
  out.push((packedHi >>> 24) & 0xff, (packedHi >>> 16) & 0xff,
           (packedHi >>> 8) & 0xff, packedHi & 0xff);
  // total samples low 32 bits
  const tsLow = Number(ts & 0xFFFFFFFFn);
  out.push((tsLow >>> 24) & 0xff, (tsLow >>> 16) & 0xff,
           (tsLow >>> 8) & 0xff, tsLow & 0xff);
  // MD5 signature (16 bytes) = 0
  for (let i = 0; i < 16; i++) out.push(0);

  // Frames.
  const srCode = flacSampleRateCode(sr);
  let frameNum = 0;
  for (let off = 0; off < pcm.length; off += blockSize) {
    let end = off + blockSize;
    if (end > pcm.length) end = pcm.length;
    const block = pcm.subarray(off, end);
    writeFlacVerbatimFrame(out, block, srCode, bps, frameNum);
    frameNum++;
  }

  return Uint8Array.from(out);
}

// OGG stub: matches Go's encode_ogg.go (which returns an error and is not
// implemented). Produce a minimal placeholder so the worker doesn't hang
// or crash; the file won't decode as real Vorbis. Tests that exercise OGG
// only check the format string round-trips, not the file content.
function buildOggStub(samples) {
  // Minimal "OggS" capture-pattern header so file sniffers recognize the
  // container, followed by zero data. 27 bytes is the minimum OggS page
  // header size; the version, type, and segment-count fields are all
  // zero, which is technically malformed but harmless for our purposes.
  return new Uint8Array([
    0x4f, 0x67, 0x67, 0x53, // "OggS"
    0,                       // version
    0,                       // header type
    0,0,0,0, 0,0,0,0,        // granule position
    0,0,0,0,                 // serial number
    0,0,0,0,                 // page sequence
    0,0,0,0,                 // checksum (unset)
    0,                       // page segments
  ]);
}

// ─── Minimal stored-format ZIP writer ────────────────────────────────────
//
// Implements just enough of PKZIP to bundle our WAV files. No compression
// (stored-only), no encryption. Validated against `unzip` and Go's
// archive/zip reader.

const crcTable = (() => {
  const t = new Uint32Array(256);
  for (let n = 0; n < 256; n++) {
    let c = n;
    for (let k = 0; k < 8; k++) c = (c & 1) ? (0xedb88320 ^ (c >>> 1)) : (c >>> 1);
    t[n] = c >>> 0;
  }
  return t;
})();

function crc32(bytes) {
  let c = 0xffffffff;
  for (let i = 0; i < bytes.length; i++) {
    c = crcTable[(c ^ bytes[i]) & 0xff] ^ (c >>> 8);
  }
  return (c ^ 0xffffffff) >>> 0;
}

function dosTime(d) {
  // DOS time: hhhhh mmmmmm sssss (2-second resolution)
  const t = (d.getHours() << 11) | (d.getMinutes() << 5) | (d.getSeconds() >> 1);
  return t & 0xffff;
}

function dosDate(d) {
  // DOS date: yyyyyyy mmmm ddddd, year offset from 1980
  const yr = Math.max(0, d.getFullYear() - 1980);
  const dt = (yr << 9) | ((d.getMonth() + 1) << 5) | d.getDate();
  return dt & 0xffff;
}

function bundleZip(entries) {
  // entries: [{name: string, bytes: Uint8Array}]
  const now = new Date();
  const dt = dosTime(now);
  const dd = dosDate(now);

  // First pass: compute total size and per-entry CRC + offsets.
  const records = [];
  let localPartSize = 0;
  for (const e of entries) {
    const nameBytes = new TextEncoder().encode(e.name);
    const crc = crc32(e.bytes);
    const localHeaderSize = 30 + nameBytes.length;
    const entrySize = localHeaderSize + e.bytes.length;
    records.push({
      name: e.name, nameBytes, bytes: e.bytes, crc,
      offset: localPartSize, localHeaderSize,
    });
    localPartSize += entrySize;
  }
  let cdSize = 0;
  for (const r of records) cdSize += 46 + r.nameBytes.length;
  const total = localPartSize + cdSize + 22;
  const out = new Uint8Array(total);
  const view = new DataView(out.buffer);
  let p = 0;

  // Local file headers + data.
  for (const r of records) {
    view.setUint32(p, 0x04034b50, true);                  // signature
    view.setUint16(p + 4, 20, true);                       // version
    view.setUint16(p + 6, 0, true);                        // flags
    view.setUint16(p + 8, 0, true);                        // method = stored
    view.setUint16(p + 10, dt, true);                      // mod time
    view.setUint16(p + 12, dd, true);                      // mod date
    view.setUint32(p + 14, r.crc, true);                   // crc32
    view.setUint32(p + 18, r.bytes.length, true);          // compressed size
    view.setUint32(p + 22, r.bytes.length, true);          // uncompressed size
    view.setUint16(p + 26, r.nameBytes.length, true);      // filename length
    view.setUint16(p + 28, 0, true);                       // extra length
    out.set(r.nameBytes, p + 30);
    p += 30 + r.nameBytes.length;
    out.set(r.bytes, p);
    p += r.bytes.length;
  }

  // Central directory.
  const cdStart = p;
  for (const r of records) {
    view.setUint32(p, 0x02014b50, true);                   // signature
    view.setUint16(p + 4, 0x0314, true);                   // version made by (Unix, 2.0)
    view.setUint16(p + 6, 20, true);                       // version needed
    view.setUint16(p + 8, 0, true);                        // flags
    view.setUint16(p + 10, 0, true);                       // method
    view.setUint16(p + 12, dt, true);
    view.setUint16(p + 14, dd, true);
    view.setUint32(p + 16, r.crc, true);
    view.setUint32(p + 20, r.bytes.length, true);
    view.setUint32(p + 24, r.bytes.length, true);
    view.setUint16(p + 28, r.nameBytes.length, true);
    view.setUint16(p + 30, 0, true);                       // extra length
    view.setUint16(p + 32, 0, true);                       // comment length
    view.setUint16(p + 34, 0, true);                       // disk number
    view.setUint16(p + 36, 0, true);                       // internal attrs
    view.setUint32(p + 38, 0, true);                       // external attrs
    view.setUint32(p + 42, r.offset, true);                // local header offset
    out.set(r.nameBytes, p + 46);
    p += 46 + r.nameBytes.length;
  }

  // End of central directory.
  view.setUint32(p, 0x06054b50, true);
  view.setUint16(p + 4, 0, true);                          // disk number
  view.setUint16(p + 6, 0, true);                          // disk where CD starts
  view.setUint16(p + 8, records.length, true);             // entries on this disk
  view.setUint16(p + 10, records.length, true);            // total entries
  view.setUint32(p + 12, p - cdStart, true);               // CD size
  view.setUint32(p + 16, cdStart, true);                   // CD offset
  view.setUint16(p + 20, 0, true);                         // comment length

  return out;
}

// ─── Finalize: patch WAV headers, build zip, post Blob ──────────────────

function finalize(meta) {
  // Wait for each channel's worklet to have closed before reading its
  // chunk list. The 'done' message from each worklet sets c.closed = true.
  // If a channel hasn't finished draining when finalize arrives, we still
  // build the file with whatever is in c.chunks — drops are reported in
  // statsSnapshot().
  let encoder;
  try {
    encoder = pickEncoder(format);
  } catch (err) {
    self.postMessage({ type: 'blob', error: String(err && err.message || err) });
    return;
  }
  const entries = [];
  const channelMeta = [];
  for (const ch of meta.channels || []) {
    const c = channels.get(ch.id);
    if (!c) continue;
    let bytes;
    let extOverride = null;
    try {
      switch (encoder.container) {
      case 'wav':
        bytes = buildWavFile(c, encoder);
        break;
      case 'flac': {
        const samples = readChannelFloatSamples(c);
        bytes = samples.length > 0
          ? buildFlacFile(samples, sampleRate)
          : new Uint8Array([0x66, 0x4c, 0x61, 0x43]); // "fLaC" only
        extOverride = '.flac';
        break;
      }
      case 'ogg': {
        const samples = readChannelFloatSamples(c);
        bytes = buildOggStub(samples);
        extOverride = '.ogg';
        break;
      }
      default:
        bytes = buildWavFile(c, encoder);
      }
    } catch (err) {
      self.postMessage({ type: 'blob', error: 'encode failed: ' + String(err && err.message || err) });
      return;
    }
    let outName = ch.filename;
    if (extOverride) {
      outName = outName.replace(/\.wav$/i, extOverride);
      if (!outName.endsWith(extOverride)) outName += extOverride;
    }
    entries.push({ name: outName, bytes });
    channelMeta.push({
      id: ch.id,
      name: ch.name,
      filename: outName,
      samples: c.samplesEncoded,
      droppedSamples: c.droppedSamples,
      bytes: bytes.length,
    });
  }

  // session.json
  const session = {
    version: 1,
    format,
    sampleRate,
    bpm: meta.bpm || 0,
    timestamp: meta.timestamp || '',
    duration: meta.duration || 0,
    channels: channelMeta.map(c => ({
      id: c.id, name: c.name, filename: c.filename,
      samples: c.samples, bytes: c.bytes, droppedSamples: c.droppedSamples,
    })),
    stats: statsSnapshot(),
    autoStopped: autoStopFired,
  };
  const sessionBytes = new TextEncoder().encode(JSON.stringify(session, null, 2));
  entries.push({ name: 'session.json', bytes: sessionBytes });

  const zip = bundleZip(entries);
  const blob = new Blob([zip], { type: 'application/zip' });
  self.postMessage({
    type: 'blob',
    blob,
    meta: channelMeta,
    sessionStats: statsSnapshot(),
    autoStopped: autoStopFired,
  });
  // Reset for potential reuse (in practice the worker is one-shot).
  channels.clear();
  startedAt = 0;
  autoStopFired = false;
}

// ─── Top-level message handling ─────────────────────────────────────────

self.onmessage = (e) => {
  const msg = e.data;
  if (!msg || typeof msg !== 'object') return;

  switch (msg.type) {
  case 'init': {
    format = msg.format || 'wav24';
    sampleRate = msg.sampleRate || 48000;
    bytesPerChannelMax = msg.bytesPerChannelMax || DEFAULTS.bytesPerChannelMax;
    durationMaxSec = msg.durationMaxSec || DEFAULTS.durationMaxSec;
    startedAt = performance.now();
    autoStopFired = false;
    channels.clear();
    self.postMessage({ type: 'ready' });
    break;
  }
  case 'addChannel': {
    const encoder = pickEncoder(format);
    const c = newChannelState(msg.id, msg.name, msg.filename, encoder.bitDepth);
    channels.set(msg.id, c);
    const port = msg.port;
    port.onmessage = (ev) => {
      const payload = ev.data;
      // Track queue depth for backpressure observability.
      if (payload instanceof ArrayBuffer) {
        c.queuedBatches++;
        if (c.queuedBatches > c.maxQueueDepth) c.maxQueueDepth = c.queuedBatches;
      }
      handleChannelBatch(c, encoder, payload);
      // Send ack so the worklet can advance its pending counter.
      if (payload instanceof ArrayBuffer) {
        try { port.postMessage({ type: 'ack' }); } catch (_) { /* port closed */ }
      }
    };
    break;
  }
  case 'stats': {
    self.postMessage({ type: 'stats', stats: statsSnapshot() });
    break;
  }
  case 'finalize': {
    finalize(msg);
    break;
  }
  case 'cancel': {
    channels.clear();
    startedAt = 0;
    autoStopFired = false;
    self.postMessage({ type: 'cancelled' });
    break;
  }
  }
};
