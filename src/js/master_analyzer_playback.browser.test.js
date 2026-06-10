/**
 * Master-analyzer playback regression — Phase 0a of audio-panel redesign.
 *
 * Reproduces the bug seen in playback screenshots:
 *   - playback_eq_tab_spectrum.browser.png — Spectrum tab body is black
 *   - playback_eq_tab_levels.browser.png   — Levels reads `Pk -inf · RMS -inf`
 *   - playback_eq_tab_chain.browser.png    — Chain trace is flat
 * during clearly-audible playback (Beat 4–6, row hits visible).
 *
 * Verifies both halves of the wiring:
 *   1. getOutputCapture() proves master output is actually pumping audio.
 *   2. channelAnalyzerSnapshot("main") proves the analyzer sees it too.
 *
 * If (1) passes but (2) fails → analyzer is wired wrong (or not at all)
 * and the screenshots match a real bug. If both pass, the empty screenshots
 * were a screenshot-timing artifact (frame captured between hits) and the
 * fix is persistent peak-hold on the JS side, not a wiring re-route.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(__dirname, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  page.on("pageerror", (err) => console.log("[PAGE-ERROR]", String(err)));

  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof startPlay === "function" &&
    typeof stopPlay === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof getOutputCapture === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof channelAnalyzerSnapshot === "function" &&
    typeof enableChannelAnalyzer === "function"
  );

  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 },
  );
  await page.evaluate(() => window.audioReady);

  // Start playback. The default circuit boots with active nodes so hits
  // begin immediately. Settle 1s so several hits land in the analyser
  // window across snapshot reads.
  await page.evaluate(() => startPlay());
  await page.waitForTimeout(1000);

  // Capture master output to verify audio is actually pumping.
  await page.evaluate(() => {
    clearOutputCapture();
    startOutputCapture();
  });
  await page.waitForTimeout(1000);
  const output = await page.evaluate(() => Array.from(getOutputCapture()));
  await page.evaluate(() => stopOutputCapture());

  const peakOf = (arr) => {
    let p = 0;
    for (const v of arr) {
      const a = Math.abs(v);
      if (a > p) p = a;
    }
    return p;
  };
  const outputPeak = peakOf(output);
  console.log(`[TEST] master output: ${output.length} samples, peak=${outputPeak.toFixed(4)}`);
  if (outputPeak < 0.001) {
    throw new Error(`master output is silent (peak=${outputPeak}). Sequencer or audio routing is broken — the analyzer test cannot run.`);
  }

  // Sample the master analyzer snapshot multiple times across ~1.5s while
  // playback continues. We expect at least one snapshot to land during or
  // shortly after a hit, so the per-snapshot peak should exceed a sane
  // threshold (-60 dB → ~0.001 linear).
  const samples = await page.evaluate(async () => {
    const out = [];
    for (let i = 0; i < 90; i++) {
      const snap = window.channelAnalyzerSnapshot("main");
      out.push({ peak: snap?.peak ?? 0, rms: snap?.rms ?? 0, spectrumLen: Array.isArray(snap?.spectrum) ? snap.spectrum.length : (snap?.spectrumLen ?? 0) });
      await new Promise((r) => setTimeout(r, 16));
    }
    return out;
  });

  let maxAnalyzerPeak = 0;
  let maxAnalyzerRMS = 0;
  for (const s of samples) {
    if (s.peak > maxAnalyzerPeak) maxAnalyzerPeak = s.peak;
    if (s.rms > maxAnalyzerRMS) maxAnalyzerRMS = s.rms;
  }
  console.log(`[TEST] analyzer over ~1.5s (90 polls): max peak=${maxAnalyzerPeak.toFixed(4)} max rms=${maxAnalyzerRMS.toFixed(4)} (output peak=${outputPeak.toFixed(4)})`);

  // The bug: master output has signal (>0.01) but analyzer reports zeros.
  if (maxAnalyzerPeak < 0.001) {
    throw new Error(
      `MASTER ANALYZER WIRING BUG: master output has peak=${outputPeak.toFixed(4)} but channelAnalyzerSnapshot("main") never returned peak > 0.001 across 90 polls. Phase 0a fix needed.`
    );
  }

  // Sanity: analyzer peak should be on the same order of magnitude as output peak.
  // Allow a wide margin because the analyser captures only the most recent 512 samples.
  if (maxAnalyzerPeak < outputPeak * 0.05) {
    throw new Error(
      `MASTER ANALYZER UNDERREPORTS: max snapshot peak=${maxAnalyzerPeak.toFixed(4)} is <5% of output peak=${outputPeak.toFixed(4)}. Likely wired to wrong tap point.`
    );
  }

  await page.evaluate(() => stopPlay());
  if (isCoverageEnabled()) await flushCoverage(page);
  console.log("[TEST] PASS — master analyzer is wired correctly during playback.");
} catch (err) {
  console.error("[TEST] FAIL", err);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
  process.exit(exitCode);
}
