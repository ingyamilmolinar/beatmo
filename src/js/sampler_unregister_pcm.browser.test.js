// unregisterSamplePCM must clear the chop from renderCache AND restore the synth mapping, or a built-in saved over
// keeps playing the chop (or goes silent). COVERED-BY-GO: sample_factory_test.go, sample_factory_desktop_test.go,
// sample_factory_unregister_test.go

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
await page.waitForFunction(() =>
  typeof window.registerSamplePCM === "function" &&
  typeof window.unregisterSamplePCM === "function" &&
  typeof window.playSound === "function");
await page.evaluate(() => document.dispatchEvent(new Event("mousedown")));

// A SILENT chop over the real kick: only both fixes together make kick audible again.
await page.evaluate(async () => {
  const sr = 44100;
  const silent = new Float32Array(8000); // all zeros
  const u8 = new Uint8Array(silent.buffer, silent.byteOffset, silent.byteLength);
  window.registerSamplePCM("kick", u8, sr);
  window.unregisterSamplePCM("kick");
  await window.playSound("kick", 1.0);
});

await page.waitForFunction(() => window.__done === true, {}, { timeout: 5000 });
const samples = await page.evaluate(() => window.__samples);
await browser.close();
server.close();

const nonZero = samples.filter((v) => v !== 0).length;
let peak = 0;
for (const v of samples) peak = Math.max(peak, Math.abs(v));
if (nonZero < 100 || peak < 0.05) {
  throw new Error(
    `unregisterSamplePCM did not restore the kick synth (nonZero=${nonZero} peak=${peak.toFixed(4)}); ` +
    `RENDER['kick'] not restored or renderCache not cleared — the reversed chop would persist after Reset`,
  );
}

console.log(`[sampler-unregister-pcm] OK: synth restored, nonZero=${nonZero} peak=${peak.toFixed(3)}`);
