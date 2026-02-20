import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() =>
  typeof setEQView === 'function' &&
  typeof forceDraw === 'function' &&
  typeof eqControlsSnapshot === 'function'
);
await assertSimpleDrawMode(page, false, "eq controls wave");
await page.evaluate(() => window.audioReady);

// Unlock audio context and start playback so the analyser gets continuous signal.
// One-shot playSound is unreliable for AnalyserNode reads because the snare
// decays within ~200ms and the 512-sample analyser window (~10ms at 48kHz)
// easily misses the peak.
await page.evaluate(() => {
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();
});

await page.evaluate(() => {
  setEQView?.("wave");
  forceDraw?.();
  startPlay?.();
});

// Wait until the analyser captures audible signal from the playing demo.
// Capture the snapshot atomically inside waitForFunction to avoid TOCTOU race
// where the AnalyserNode buffer refreshes between the wait and a separate read.
const _before = await page.evaluate(async () => {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const snap = channelAnalyzerSnapshot('main');
    if (snap.wave && snap.wave.length > 0 && snap.peak > 0.01) return snap;
    await new Promise(r => setTimeout(r, 50));
  }
  return channelAnalyzerSnapshot('main');
}, { timeout: 15000 });

// Switch to EQ view and apply a gain.
await page.evaluate(() => {
  setEQView?.("eq");
  forceDraw?.();
  if (typeof setChannelEQ === 'function') {
    setChannelEQ('main', [{ freq: 80, q: 1, gainDB: 6 }]);
  }
  setEQView?.("wave");
  forceDraw?.();
});

// Wait until the analyser captures signal again after EQ changes. This confirms
// the analyser chain survives rewireChannel and the audio pipeline is intact.
// Capture atomically to avoid TOCTOU race with AnalyserNode buffer refresh.
const after = await page.evaluate(async () => {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const snap = channelAnalyzerSnapshot('main');
    if (snap.wave && snap.wave.length > 0 && snap.peak > 0.01) return snap;
    await new Promise(r => setTimeout(r, 50));
  }
  return channelAnalyzerSnapshot('main');
}, { timeout: 15000 });

await page.evaluate(() => stopPlay?.());
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "eq_controls_wave");
await browser.close();
server.close();

if (!after || !after.wave || after.wave.length === 0) {
  throw new Error("waveform missing after EQ apply");
}
if (!(after.peak > 0.01)) {
  throw new Error(`waveform peak too low after EQ apply: ${after.peak}`);
}
