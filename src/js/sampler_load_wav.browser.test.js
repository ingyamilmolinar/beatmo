// sampler_load_wav.browser.test.js
//
// Phase 2 boundary: the browser "Load WAV" path. decodeWavToPCM(url) is the
// one Go↔WebAudio seam the Go-WASM Sampler tab calls to turn a WAV file into
// mono PCM it can draw + edit (sampler_load_wasm.go → samplerLoadWAVImpl).
// This test owns only that boundary: a real WAV decoded through
// AudioContext.decodeAudioData must yield non-empty float32 PCM at the
// context sample rate, and those exact bytes must round-trip back through
// registerSamplePCM into an audible AudioBuffer.
//
// The Go-side consumption (bytes → []float32, resample-to-engine-rate) is
// pure Go, covered by internal/audio/sample_pcm_notjs.go's DecodeWAVToPCM
// path and sampler_panel_zone_test.go's TestSamplerLoadWAVPopulatesRaw.
//
// COVERED-BY-GO:
//   internal/audio/sample_edit_test.go            (resample math)
//   internal/ui/sampler_panel_zone_test.go        (Load WAV → raw wiring)
//
// Reuses the playtest.wasm harness so audio.js loads in its normal Go-WASM
// context; decode + register + play are driven from page.evaluate.

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(__dirname, "../go");

const GO = resolveGoBinary();
if (!shouldSkipWasmBuild("playtest.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "playtest.wasm"), "./internal/audio/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  if (req.url === "/play.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('playtest.wasm'), go.importObject).then(r => go.run(r.instance));
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith(".wasm") ? "application/wasm" : "application/javascript";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();

// Capture raw output samples off the WebAudio graph.
await page.addInitScript(() => {
  const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 20000;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      window.__samples = [];
      sp.addEventListener("audioprocess", (e) => {
        const data = e.inputBuffer.getChannelData(0);
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) window.__done = true;
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

await page.goto(`http://localhost:${port}/play.html`);
await page.waitForFunction(
  () => typeof window.decodeWavToPCM === "function" &&
        typeof window.registerSamplePCM === "function" &&
        typeof window.playSound === "function",
);
await page.evaluate(() => document.dispatchEvent(new Event("mousedown")));

// Build a 16-bit PCM WAV of a 440 Hz sine, decode it via the real
// decodeAudioData boundary, then round-trip the decoded PCM back through
// registerSamplePCM and play it.
const result = await page.evaluate(async () => {
  const sr = 44100;
  const n = 8000;
  // --- assemble a minimal mono 16-bit PCM WAV ---
  const bytesPerSample = 2;
  const dataLen = n * bytesPerSample;
  const buf = new ArrayBuffer(44 + dataLen);
  const dv = new DataView(buf);
  const writeStr = (off, s) => { for (let i = 0; i < s.length; i++) dv.setUint8(off + i, s.charCodeAt(i)); };
  writeStr(0, "RIFF");
  dv.setUint32(4, 36 + dataLen, true);
  writeStr(8, "WAVE");
  writeStr(12, "fmt ");
  dv.setUint32(16, 16, true);          // fmt chunk size
  dv.setUint16(20, 1, true);           // PCM
  dv.setUint16(22, 1, true);           // mono
  dv.setUint32(24, sr, true);
  dv.setUint32(28, sr * bytesPerSample, true); // byte rate
  dv.setUint16(32, bytesPerSample, true);      // block align
  dv.setUint16(34, 16, true);          // bits per sample
  writeStr(36, "data");
  dv.setUint32(40, dataLen, true);
  for (let i = 0; i < n; i++) {
    const v = 0.5 * Math.sin((2 * Math.PI * 440 * i) / sr);
    dv.setInt16(44 + i * bytesPerSample, Math.max(-1, Math.min(1, v)) * 0x7fff, true);
  }
  const url = URL.createObjectURL(new Blob([buf], { type: "audio/wav" }));

  // --- the boundary under test ---
  const decoded = await window.decodeWavToPCM(url);
  URL.revokeObjectURL(url);

  // Round-trip the decoded PCM bytes back to an audible buffer.
  window.registerSamplePCM("test.sampler.wav", decoded.bytes, decoded.sr);
  await window.playSound("test.sampler.wav", 1.0);

  // Peak of the decoded PCM itself (independent of playback).
  const f32 = new Float32Array(decoded.bytes.buffer, decoded.bytes.byteOffset, decoded.length);
  let decodedPeak = 0;
  for (const v of f32) decodedPeak = Math.max(decodedPeak, Math.abs(v));
  return { length: decoded.length, sr: decoded.sr, decodedPeak };
});

if (!(result.length > 0)) {
  throw new Error(`decodeWavToPCM returned empty PCM (length=${result.length})`);
}
if (!(result.sr > 0)) {
  throw new Error(`decodeWavToPCM returned non-positive sample rate (sr=${result.sr})`);
}
if (!(result.decodedPeak > 0.05)) {
  throw new Error(`decoded PCM peak ${result.decodedPeak.toFixed(4)} too low — WAV decode produced no signal`);
}

await page.waitForFunction(() => window.__done === true, {}, { timeout: 5000 });
const samples = await page.evaluate(() => window.__samples);
await browser.close();
server.close();

const nonZero = samples.filter((v) => v !== 0).length;
if (nonZero < 100) {
  throw new Error(`decoded WAV produced no audible output through registerSamplePCM (nonZero=${nonZero})`);
}
let peak = 0;
for (const v of samples) peak = Math.max(peak, Math.abs(v));
if (peak < 0.05) {
  throw new Error(`playback peak ${peak.toFixed(4)} too low — decoded PCM did not round-trip`);
}

console.log(
  `[sampler-load-wav] OK: decoded length=${result.length} sr=${result.sr} ` +
  `decodedPeak=${result.decodedPeak.toFixed(3)} playbackPeak=${peak.toFixed(3)} nonZero=${nonZero}`,
);
