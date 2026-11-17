/**
 * Unit tests for the WAV decode + RMS used by the measure_audio audio gate.
 * Run: node --test src/js/llm_test/recording_decode.test.mjs
 */
import { test } from "node:test";
import assert from "node:assert/strict";
import { decodeWav, rms } from "./recording_decode.js";

// Build a minimal IEEE-float32 mono WAV (fmt code 3, 32-bit) from samples.
function floatWav(samples, sampleRate = 48000) {
  const dataBytes = samples.length * 4;
  const buf = Buffer.alloc(44 + dataBytes);
  buf.write("RIFF", 0, "ascii");
  buf.writeUInt32LE(36 + dataBytes, 4);
  buf.write("WAVE", 8, "ascii");
  buf.write("fmt ", 12, "ascii");
  buf.writeUInt32LE(16, 16);
  buf.writeUInt16LE(3, 20); // IEEE float
  buf.writeUInt16LE(1, 22); // mono
  buf.writeUInt32LE(sampleRate, 24);
  buf.writeUInt32LE(sampleRate * 4, 28);
  buf.writeUInt16LE(4, 32);
  buf.writeUInt16LE(32, 34); // bit depth
  buf.write("data", 36, "ascii");
  buf.writeUInt32LE(dataBytes, 40);
  for (let i = 0; i < samples.length; i++) buf.writeFloatLE(samples[i], 44 + i * 4);
  return buf;
}

test("rms: silence is 0, full-scale sine ≈ 0.707", () => {
  assert.equal(rms(new Float32Array(1000)), 0);
  const n = 4800;
  const sine = new Float32Array(n);
  for (let i = 0; i < n; i++) sine[i] = Math.sin((2 * Math.PI * 100 * i) / 48000);
  assert.ok(Math.abs(rms(sine) - 0.7071) < 0.01, `sine rms ${rms(sine)}`);
});

test("decodeWav: round-trips float32 PCM + computes RMS above the silence floor", () => {
  const n = 4800;
  const samples = new Float32Array(n);
  for (let i = 0; i < n; i++) samples[i] = 0.5 * Math.sin((2 * Math.PI * 220 * i) / 48000);
  const wav = floatWav(samples);
  const dec = decodeWav(wav);
  assert.equal(dec.sampleRate, 48000);
  assert.equal(dec.channels, 1);
  assert.equal(dec.bitDepth, 32);
  assert.equal(dec.samples.length, n);
  const r = rms(dec.samples);
  assert.ok(r > 0.001, `RMS ${r} should exceed silence floor`);
  assert.ok(Math.abs(r - 0.3536) < 0.01, `RMS ${r} ~ 0.5/sqrt(2)`);
});

test("decodeWav: rejects non-RIFF input", () => {
  assert.throws(() => decodeWav(Buffer.from("not a wav file at all....")), /RIFF|WAVE/);
});
