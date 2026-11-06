// voice_tempo_independence.browser.test.js
//
// A note sounds the same at every tempo and on both platforms (user decision
// 2026-10-27: "the same circuit sounds exactly the same at any tempo in both
// platforms. This is paramount."). A voice is its document's render.seconds
// long — never a number of beats — and the browser post-processes it exactly
// like desktop (voice_post.js ↔ doc_voice_native.go: voice FX, gate tail,
// normalise × amplitude, end fade). Three checks, every shipped instrument:
//
//   1. TEMPO: the voice the PLAYBACK path renders (ensureRenderedSample) is
//      the same sample for sample at 60 and 240 BPM (each tempo renders fresh).
//   2. BRIDGE: what Go pushed (setInstrumentBlock) is the desktop document —
//      the same block, seconds, amplitude, gate tail and voice FX calls.
//   3. PLATFORMS: the browser renders that block into the desktop's played
//      voice (cmd/export_audio --voices, rendered fresh from this tree):
//      exactly as many frames, and every sample within the float-rounding seam
//      of the C engine (1e-4, the xplat_audio_compare contract — the C is the
//      same; only the compiler and libm differ, ~1e-7 typically).
//
// COVERED-BY-GO: TestVoiceIsTheSameAtEveryTempo / TestTempoChangeReRendersNothing
// (internal/audio, real build) for desktop. JS-owned here: the render worker,
// voice_post.js and the Go→JS document push.

import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import childProcess from "child_process";
import { setupFullWasm } from "./real_input_test_helpers.js";
import { resolveGoBinary, waitForAudioReady } from "./browser_test_helpers.js";

const repoRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");

const MAX_SAMPLE_DIFF = 1e-4;

function nativeVoices(sr) {
  const env = { ...process.env, CGO_ENABLED: "1" };
  delete env.GOOS; // export_audio is a native host tool (cgo)
  delete env.GOARCH;
  const r = childProcess.spawnSync(resolveGoBinary(), ["run", "./cmd/export_audio", "--voices", String(sr)], {
    cwd: path.resolve(repoRoot, "src/go"),
    env,
    encoding: "utf8",
    maxBuffer: 1 << 30,
  });
  if (r.status !== 0) {
    throw new Error(`cmd/export_audio --voices failed (status ${r.status}): ${(r.stderr || String(r.error || "")).slice(-2000)}`);
  }
  return JSON.parse(r.stdout);
}

// The startup demo carries its own documents for a few instruments; for those
// the page holds the demo's version, not the shipped one (bridge check only).
function demoInstrumentIDs() {
  const demo = JSON.parse(fs.readFileSync(path.resolve(repoRoot, "src/go/internal/assets/startup_demo.json"), "utf8"));
  return new Set(Object.keys(demo.instrument_docs || {}));
}

async function main() {
  const { page, cleanup } = await setupFullWasm({ logLevel: "INFO" });
  const failures = [];
  try {
    await page.waitForFunction(
      () =>
        typeof window.renderDocPCM === "function" &&
        typeof window.__instrumentBlockForTest === "function" &&
        typeof window.__testCaptureSynthRender === "function" &&
        typeof window.captureInstrumentPCM === "function" &&
        typeof window.setBPM === "function"
    );
    await waitForAudioReady(page);

    // The page's render rate (no AudioContext yet → audio.js's default).
    const sr = await page.evaluate(async () => {
      const b = new Float64Array(window.__instrumentBlockForTest("kick").block);
      return (await window.renderDocPCM("kick", b, 0.1, 1, 0, [])).sr;
    });
    const ref = nativeVoices(sr);
    const demo = demoInstrumentIDs();
    console.log(`[test] ${ref.voices.length} desktop voices at ${sr} Hz`);

    let tempoChecked = 0;
    let bridgeChecked = 0;
    let exactVoices = 0;
    let worstDiff = 0;
    let worstID = "";
    for (const v of ref.voices) {
      const res = await page.evaluate(
        async ({ v, sr, checkBridge }) => {
          const out = { id: v.id };
          const info = window.__instrumentBlockForTest(v.id);
          // 2. BRIDGE
          if (checkBridge) {
            if (!info) {
              out.bridge = "no block pushed";
            } else {
              const sorted = (p) => Object.keys(p || {}).sort().map((k) => [k, p[k]]);
              const calls = (fx) => JSON.stringify((fx || []).map((e) => [e.type, sorted(e.params)]));
              const same = (a, b) => calls(a) === calls(b);
              const blockDiff = Array.from(info.block).findIndex((x, i) => x !== v.block[i]);
              if (blockDiff >= 0) out.bridge = `block[${blockDiff}] ${info.block[blockDiff]}, desktop ${v.block[blockDiff]}`;
              else if (info.seconds !== v.seconds || info.amp !== v.amp || (info.gateTail || 0) !== v.gateTail) {
                out.bridge = `render settings ${JSON.stringify([info.seconds, info.amp, info.gateTail])}, desktop ${JSON.stringify([v.seconds, v.amp, v.gateTail])}`;
              } else if (!same(info.fx || [], v.fx)) {
                out.bridge = `voice FX ${JSON.stringify(info.fx)}, desktop ${JSON.stringify(v.fx)}`;
              } else {
                out.bridgeOK = true;
              }
            }
          }
          // 1. TEMPO — the playback path, re-rendered fresh at each tempo.
          if (info) {
            const played = [];
            for (const bpm of [60, 240]) {
              window.setBPM(bpm);
              const rec = await window.__testCaptureSynthRender(v.id);
              const pcm = rec && window.captureInstrumentPCM(v.id, false);
              if (!pcm) { out.tempo = `no playback render at ${bpm} BPM`; break; }
              played.push(new Float32Array(pcm.bytes.buffer.slice(pcm.bytes.byteOffset, pcm.bytes.byteOffset + pcm.bytes.byteLength)));
            }
            window.setBPM(120);
            if (!out.tempo && played.length === 2) {
              if (played[0].length !== played[1].length) out.tempo = `${played[0].length} frames at 60 BPM, ${played[1].length} at 240`;
              else {
                const i = played[0].findIndex((x, k) => x !== played[1][k]);
                if (i >= 0) out.tempo = `sample ${i}: ${played[0][i]} at 60 BPM, ${played[1][i]} at 240`;
                else out.tempoOK = true;
              }
            }
          }
          // 3. PLATFORMS — the desktop document, rendered here.
          const bin = atob(v.pcm);
          const bytes = new Uint8Array(bin.length);
          for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
          const want = new Float32Array(bytes.buffer);
          const got = await window.renderDocPCM(v.id, Float64Array.from(v.block), v.seconds, v.amp, v.gateTail, v.fx);
          if (got.sr !== sr) { out.platform = `rendered at ${got.sr} Hz, want ${sr}`; return out; }
          const g = new Float32Array(got.bytes.buffer, got.bytes.byteOffset, got.length);
          if (g.length !== want.length) { out.platform = `${g.length} frames, desktop ${want.length}`; return out; }
          let maxDiff = 0, at = -1, exact = 0, peak = 0;
          for (let i = 0; i < g.length; i++) {
            const d = Math.abs(g[i] - want[i]);
            if (d === 0) exact++;
            if (d > maxDiff) { maxDiff = d; at = i; }
            peak = Math.max(peak, Math.abs(want[i]));
          }
          out.maxDiff = maxDiff; out.at = at; out.exact = exact; out.n = g.length; out.peak = peak;
          return out;
        },
        { v, sr, checkBridge: !demo.has(v.id) }
      );
      if (res.bridge) failures.push(`${v.id} bridge: ${res.bridge}`);
      if (res.bridgeOK) bridgeChecked++;
      if (res.tempo) failures.push(`${v.id} tempo: ${res.tempo}`);
      if (res.tempoOK) tempoChecked++;
      if (res.platform) { failures.push(`${v.id} platforms: ${res.platform}`); continue; }
      if (!(res.peak > 0)) failures.push(`${v.id}: the desktop voice is silent — the comparison would be vacuous`);
      if (res.maxDiff > MAX_SAMPLE_DIFF) failures.push(`${v.id} platforms: sample ${res.at} differs by ${res.maxDiff.toExponential(2)} (> ${MAX_SAMPLE_DIFF})`);
      if (res.exact === res.n) exactVoices++;
      if (res.maxDiff > worstDiff) { worstDiff = res.maxDiff; worstID = v.id; }
    }
    console.log(`[test] tempo: ${tempoChecked} voices identical at 60 and 240 BPM`);
    console.log(`[test] bridge: ${bridgeChecked} pushed documents equal the desktop's`);
    console.log(`[test] platforms: ${ref.voices.length} voices, ${exactVoices} bit-identical, worst |Δ| ${worstDiff.toExponential(2)} (${worstID})`);
    if (tempoChecked < ref.voices.length - demo.size) failures.push(`tempo checked only ${tempoChecked} voices`);
    if (bridgeChecked < ref.voices.length - demo.size) failures.push(`bridge checked only ${bridgeChecked} voices`);
  } finally {
    await cleanup();
  }
  if (failures.length) {
    console.error("FAIL voice_tempo_independence:\n  " + failures.join("\n  "));
    process.exit(1);
  }
  console.log("PASS voice_tempo_independence");
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
