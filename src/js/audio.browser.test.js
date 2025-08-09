import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(__dirname, "../go");

// Build the tiny harness that invokes audio.Play("snare").
const build = spawnSync(
  "go",
  [
    "build",
    "-o",
    path.join(jsDir, "playtest.wasm"),
    "./internal/audio/playtest",
  ],
  {
    cwd: goDir,
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  },
);
if (build.status !== 0) {
  throw new Error("go build failed");
}

// Ensure playwright and its dependencies are installed.
spawnSync("npx", ["playwright", "install", "chromium"], {
  cwd: jsDir,
  stdio: "inherit",
});
spawnSync("npx", ["playwright", "install-deps", "chromium"], {
  cwd: jsDir,
  stdio: "inherit",
});

const port = 8123 + Math.floor(Math.random() * 1000);
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
  const filePath = path.join(jsDir, req.url);
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    const ct = filePath.endsWith(".wasm")
      ? "application/wasm"
      : "application/javascript";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();

// Hook into Web Audio to capture raw samples from the ScriptProcessorNode.
await page.addInitScript(() => {
  const RealAC = window.AudioContext || window.webkitAudioContext;
  // SAMPLE_TARGET determines how many audio samples to collect before
  // ending the test. 40000 samples at a 44.1kHz sample rate is ~0.9s of
  // audio, sufficient to capture playback and analyze output.
  const SAMPLE_TARGET = 40000;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      window.__samples = [];
      window.__firstSampleTime = undefined;
      sp.addEventListener("audioprocess", (e) => {
        const data = e.inputBuffer.getChannelData(0);
        if (window.__firstSampleTime === undefined) {
          for (let i = 0; i < data.length; i++) {
            if (data[i] !== 0) {
              window.__firstSampleTime = performance.now();
              break;
            }
          }
        }
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) {
          window.__done = true;
        }
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

await page.goto(`http://localhost:${port}/play.html`);
await page.waitForFunction(() => window.__wasmReady === true);
// Trigger the resume handler registered by oto's driver.
await page.evaluate(() => document.dispatchEvent(new Event("mousedown")));
// Wait for audio to be processed.
await page.waitForFunction(() => window.__done === true, {}, { timeout: 5000 });
const samples = await page.evaluate(() => window.__samples);
const playTime = await page.evaluate(() => window.__playTime);
const firstSampleTime = await page.evaluate(() => window.__firstSampleTime);
await browser.close();
server.close();

const sr = 44100;
const first = samples.findIndex((v) => v !== 0);
const second = samples.findIndex((v, i) => i >= sr / 4 && v !== 0);
if (first < 0 || second < 0) {
  throw new Error("missing audio data for multiple beats");
}

const delay = firstSampleTime - playTime;
if (delay > 100) {
  throw new Error(`audio start delay ${delay}ms exceeds 100ms`);
}

console.log(
  "captured audio samples:",
  samples.slice(0, 8).map((v) => v.toFixed(5)),
);
