import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

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

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
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
  typeof setChannelEQ === 'function' &&
  typeof playSound === 'function' &&
  typeof startOutputCapture === 'function' &&
  typeof stopOutputCapture === 'function'
);

// Trigger user gesture to unlock audio context.
await page.evaluate(() => {
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();
});
await page.waitForTimeout(100);

// Wait for DSP module load and synth sample rendering to complete.
await page.evaluate(() => window.audioReady);

// Test: muting ONE band should NOT significantly change the volume of OTHER frequencies.
// Uses OfflineAudioContext to render audio deterministically — immune to CPU
// pressure from parallel test execution. OfflineAudioContext.startRendering()
// processes audio as fast as the CPU allows (not real-time), producing
// identical output every run regardless of system load.
const result = await page.evaluate(async () => {
  await ensureSynthSample('kick');
  const rawSamples = getCachedRenderData('kick');
  if (!rawSamples || rawSamples.length === 0) {
    throw new Error('No cached kick sample data available');
  }
  const sr = window.__renderMeta?.kick?.sr || 48000;
  const numSamples = rawSamples.length;

  // 10-band ISO standard definitions — must match BAND_DEFS in audio.js:885-896
  const BAND_DEFS = [
    { loHz: 22, hiHz: 44 },      // 31 Hz
    { loHz: 44, hiHz: 88 },      // 62 Hz
    { loHz: 88, hiHz: 177 },     // 125 Hz
    { loHz: 177, hiHz: 354 },    // 250 Hz
    { loHz: 354, hiHz: 707 },    // 500 Hz
    { loHz: 707, hiHz: 1414 },   // 1 kHz
    { loHz: 1414, hiHz: 2828 },  // 2 kHz
    { loHz: 2828, hiHz: 5657 },  // 4 kHz
    { loHz: 5657, hiHz: 11314 }, // 8 kHz
    { loHz: 11314, hiHz: 20000 }, // 16 kHz
  ];

  // Render the kick sample through a given EQ config using OfflineAudioContext.
  // Replicates createMultibandProcessor topology from audio.js:902-963:
  // serial peaking filters, muted bands get -60dB cascaded twice.
  async function renderWithEQ(bands) {
    const offline = new OfflineAudioContext(1, numSamples, sr);
    const buf = offline.createBuffer(1, numSamples, sr);
    buf.getChannelData(0).set(rawSamples);
    const src = offline.createBufferSource();
    src.buffer = buf;

    // Build serial peaking EQ chain (mirrors createMultibandProcessor)
    let lastNode = src;
    for (let i = 0; i < bands.length; i++) {
      const band = bands[i];
      const def = BAND_DEFS[i] || { loHz: 20, hiHz: 20000 };
      const isMuted = band?.muted === true;
      const gainDB = band?.gainDB || 0;

      // Skip unity-gain unmuted bands (matches audio.js:930)
      if (!isMuted && Math.abs(gainDB) < 0.01) continue;

      const effectiveGainDB = isMuted ? -60 : gainDB;
      const centerFreq = Math.sqrt(def.loHz * def.hiHz);
      const bwOctaves = Math.log2(def.hiHz / def.loHz);
      const Q = 1 / (2 * Math.sinh(Math.LN2 / 2 * bwOctaves));

      const peaking = offline.createBiquadFilter();
      peaking.type = 'peaking';
      peaking.frequency.value = centerFreq;
      peaking.Q.value = Q;
      peaking.gain.value = effectiveGainDB;
      lastNode.connect(peaking);
      lastNode = peaking;

      // Muted bands cascade a second identical filter (matches audio.js:947-955)
      if (isMuted) {
        const peaking2 = offline.createBiquadFilter();
        peaking2.type = 'peaking';
        peaking2.frequency.value = centerFreq;
        peaking2.Q.value = Q;
        peaking2.gain.value = effectiveGainDB;
        lastNode.connect(peaking2);
        lastNode = peaking2;
      }
    }

    lastNode.connect(offline.destination);
    src.start(0);
    const rendered = await offline.startRendering();
    const data = rendered.getChannelData(0);
    let sum = 0;
    for (let i = 0; i < data.length; i++) sum += data[i] * data[i];
    return { rms: Math.sqrt(sum / data.length), samples: data.length };
  }

  // All-unmuted band config
  const allUnmuted = Array.from({ length: 10 }, () => ({
    gainDB: 0, muted: false,
  }));

  // 1) Baseline: all bands unmuted (passthrough — all bands skipped at unity)
  const noEQ = await renderWithEQ(allUnmuted);

  // 2) High band muted (band 8: 8 kHz — kick has little energy here)
  const highMutedBands = allUnmuted.map((b, i) => ({
    ...b, muted: i === 8,
  }));
  const oneBandMuted = await renderWithEQ(highMutedBands);

  // 3) Mid band muted (band 4: 500 Hz — kick has significant energy here)
  const midMutedBands = allUnmuted.map((b, i) => ({
    ...b, muted: i === 4,
  }));
  const midBandMuted = await renderWithEQ(midMutedBands);

  return {
    noEQ,
    oneBandMuted,
    midBandMuted,
    oneBandMutedRatio: noEQ.rms > 0 ? oneBandMuted.rms / noEQ.rms : 0,
    midBandMutedRatio: noEQ.rms > 0 ? midBandMuted.rms / noEQ.rms : 0,
  };
});

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "eq_band_mute_volume");
await browser.close();
server.close();

console.log("Test results:");
console.log(`  No EQ:           RMS=${result.noEQ.rms.toFixed(6)}, samples=${result.noEQ.samples}`);
console.log(`  High band muted: RMS=${result.oneBandMuted.rms.toFixed(6)}, ratio=${result.oneBandMutedRatio.toFixed(4)}`);
console.log(`  Mid band muted:  RMS=${result.midBandMuted.rms.toFixed(6)}, ratio=${result.midBandMutedRatio.toFixed(4)}`);

// Verify baseline has audio
if (result.noEQ.rms < 0.001) {
  throw new Error(`Baseline RMS too low (${result.noEQ.rms}), synth sample may not be rendered`);
}

// With OfflineAudioContext, results are deterministic. Serial peaking EQ can
// only cut, never boost, so muted-band ratios must be <= 1.0. Allow a tiny
// margin (1.05) for floating-point rounding in the biquad filters.
const maxAllowedRatio = 1.05;

if (result.oneBandMutedRatio > maxAllowedRatio) {
  throw new Error(
    `FAIL: Muting high band caused volume to increase by ${((result.oneBandMutedRatio - 1) * 100).toFixed(1)}%. ` +
    `Ratio=${result.oneBandMutedRatio.toFixed(4)}, max allowed=${maxAllowedRatio}. ` +
    `This indicates a gain staging bug in the multiband processor.`
  );
}

if (result.midBandMutedRatio > maxAllowedRatio) {
  throw new Error(
    `FAIL: Muting mid band caused volume to increase by ${((result.midBandMutedRatio - 1) * 100).toFixed(1)}%. ` +
    `Ratio=${result.midBandMutedRatio.toFixed(4)}, max allowed=${maxAllowedRatio}. ` +
    `This indicates a gain staging bug in the multiband processor.`
  );
}

// Muting one band shouldn't silence everything — verify lower bound
const minAllowedRatio = 0.01;
if (result.oneBandMutedRatio < minAllowedRatio) {
  throw new Error(
    `FAIL: Muting high band silenced the signal (ratio=${result.oneBandMutedRatio.toFixed(4)}). ` +
    `Expected partial reduction, not near-silence.`
  );
}
if (result.midBandMutedRatio < minAllowedRatio) {
  throw new Error(
    `FAIL: Muting mid band silenced the signal (ratio=${result.midBandMutedRatio.toFixed(4)}). ` +
    `Expected partial reduction, not near-silence.`
  );
}

console.log("PASS: EQ band mute volume test passed");
