// sampler_register_pcm.browser.test.js
//
// Validates the one new Go↔WebAudio seam introduced by the Sampler tab:
// registerSamplePCM(id, Uint8Array-of-float32-LE, sr) must turn a raw PCM
// buffer (exactly the little-endian float32 bytes Go marshals via
// f32ToJSBytes) into a playable AudioBuffer that produces audible output
// through the normal playSound path. The editing-math correctness (trim /
// pitch / gain / reverse / normalize / fade) is covered in Go
// (internal/audio/sample_edit_test.go); this test owns only the boundary.
//
// COVERED-BY-GO:
//   internal/audio/sample_edit_test.go      (BakeSample pipeline)
//   internal/audio/sample_capture_test.go   (RegisterSamplePCM availability)
//   internal/ui/sampler_panel_zone_test.go  (preview/save wiring)
//
// Reuses the playtest.wasm harness so audio.js loads in its normal Go-WASM
// context; the PCM register + play are driven from page.evaluate.

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
await page.waitForFunction(() => typeof window.registerSamplePCM === "function" && typeof window.playSound === "function");
await page.evaluate(() => document.dispatchEvent(new Event("mousedown")));

// Build a 440 Hz sine as little-endian float32 bytes (exactly what Go's
// f32ToJSBytes produces), register it, and play it.
const result = await page.evaluate(async () => {
  const sr = 44100;
  const n = 8000;
  const f32 = new Float32Array(n);
  for (let i = 0; i < n; i++) f32[i] = 0.5 * Math.sin((2 * Math.PI * 440 * i) / sr);
  const u8 = new Uint8Array(f32.buffer, f32.byteOffset, f32.byteLength);
  window.registerSamplePCM("test.sampler.pcm", u8, sr);
  await window.playSound("test.sampler.pcm", 1.0);
  return { n };
});

await page.waitForFunction(() => window.__done === true, {}, { timeout: 5000 });
const samples = await page.evaluate(() => window.__samples);
await browser.close();
server.close();

const nonZero = samples.filter((v) => v !== 0).length;
if (nonZero < 100) {
  throw new Error(`registerSamplePCM produced no audible output (nonZero=${nonZero})`);
}
let peak = 0;
for (const v of samples) peak = Math.max(peak, Math.abs(v));
if (peak < 0.05) {
  throw new Error(`output peak ${peak.toFixed(4)} too low — PCM byte reinterpretation likely wrong`);
}

console.log(`[sampler-register-pcm] OK: nonZero=${nonZero} peak=${peak.toFixed(3)} of ${samples.length} samples`);
