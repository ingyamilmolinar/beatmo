// audio_output_capture.browser.test.js
//
// Tests the output capture API for recording audio pipeline output.
// Verifies:
//   1. Start capture -> play -> stop capture -> verify non-empty
//   2. Get capture mid-stream (without stopping)
//   3. Get capture stats
//   4. Clear capture buffer
//   5. Reset capture node
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_output_capture.browser.test.js

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

const MAX_RETRIES = 3;
let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof playSound === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function" &&
    typeof getOutputCapture === "function" &&
    typeof getOutputCaptureStats === "function" &&
    typeof clearOutputCapture === "function" &&
    typeof resetOutputCaptureNode === "function"
  );

  // Unlock audio context.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 10000 }
  );
  await page.evaluate(() => window.audioReady);

  // ========================================================================
  // Scenario 1: Start capture -> play -> stop capture -> verify non-empty
  // ========================================================================
  let s1;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 1: Capture round-trip (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s1 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      await ensureSynthSample("kick");

      // Warmup: poll for real audio signal to confirm pipeline is active.
      startOutputCapture();
      playSound("kick", 1.0);
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { warmupSignal = true; break; }
          }
        }
        if (warmupSignal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (warmupSignal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Actual measurement
      for (let i = 0; i < 3; i++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 400));
      }

      const captured = stopOutputCapture();
      const len = captured.length;
      let hasNonZero = false;
      let peak = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > 0.0001) hasNonZero = true;
        if (a > peak) peak = a;
      }

      return { len, hasNonZero, peak };
    }, attempt);

    console.log(`  Captured ${s1.len} samples, hasNonZero: ${s1.hasNonZero}, peak: ${s1.peak.toFixed(6)}`);
    if (s1.hasNonZero) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: no non-zero samples, retrying...`);
      await page.waitForTimeout(1000);
    }
  }
  if (s1.len === 0) throw new Error(`Scenario 1 FAIL: captured 0 samples`);
  if (!s1.hasNonZero) throw new Error(`Scenario 1 FAIL: all samples are zero`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 2: Get capture mid-stream (without stopping)
  // ========================================================================
  let s2;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 2: Mid-stream capture (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s2 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      await ensureSynthSample("kick");

      startOutputCapture();

      // Warmup
      playSound("kick", 1.0);
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = getOutputCapture();
        if (snap && snap.length > 0) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { warmupSignal = true; break; }
          }
        }
        if (warmupSignal) break;
        await new Promise((r) => setTimeout(r, 100));
      }
      if (!warmupSignal) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Play some audio and check mid-stream
      playSound("kick", 1.0);
      await new Promise((r) => setTimeout(r, 500));

      const midStream = getOutputCapture();
      const midLen = midStream.length;

      // Play more
      playSound("kick", 1.0);
      await new Promise((r) => setTimeout(r, 500));

      const afterMore = getOutputCapture();
      const afterMoreLen = afterMore.length;

      // Stop and get final
      const finalSamples = stopOutputCapture();
      const finalLen = finalSamples.length;

      return { midLen, afterMoreLen, finalLen };
    }, attempt);

    console.log(`  Mid-stream: ${s2.midLen}, After more: ${s2.afterMoreLen}, Final: ${s2.finalLen}`);
    if (s2.midLen > 0 && s2.afterMoreLen > s2.midLen) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: mid-stream or growth check failed, retrying...`);
      await page.waitForTimeout(1000);
    }
  }
  if (s2.midLen === 0) throw new Error(`Scenario 2 FAIL: mid-stream capture returned 0 samples`);
  if (s2.afterMoreLen <= s2.midLen) throw new Error(`Scenario 2 FAIL: buffer should grow after more audio (${s2.afterMoreLen} <= ${s2.midLen})`);
  if (s2.finalLen < s2.afterMoreLen) throw new Error(`Scenario 2 FAIL: final should be >= afterMore (${s2.finalLen} < ${s2.afterMoreLen})`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 3: Get capture stats
  // ========================================================================
  console.log("--- Scenario 3: Capture stats ---");

  const s3 = await page.evaluate(async () => {
    // Stats before capture
    const statsBefore = getOutputCaptureStats();

    startOutputCapture();

    const statsDuring = getOutputCaptureStats();

    playSound("kick", 1.0);
    await new Promise((r) => setTimeout(r, 500));

    const statsWithData = getOutputCaptureStats();

    stopOutputCapture();

    const statsAfterStop = getOutputCaptureStats();

    return { statsBefore, statsDuring, statsWithData, statsAfterStop };
  });

  // During capture, enabled should be true
  if (s3.statsDuring.enabled !== true) throw new Error(`Scenario 3 FAIL: enabled should be true during capture`);
  // After stop, enabled should be false
  if (s3.statsAfterStop.enabled !== false) throw new Error(`Scenario 3 FAIL: enabled should be false after stop`);
  // Stats should have sampleRate field
  if (!Number.isFinite(s3.statsWithData.sampleRate) || s3.statsWithData.sampleRate <= 0) {
    throw new Error(`Scenario 3 FAIL: sampleRate should be positive, got ${s3.statsWithData.sampleRate}`);
  }
  // hasNode should be true once capture was started
  if (s3.statsDuring.hasNode !== true) throw new Error(`Scenario 3 FAIL: hasNode should be true during capture`);
  // durationSec should be computable
  if (s3.statsWithData.samples > 0 && s3.statsWithData.durationSec <= 0) {
    throw new Error(`Scenario 3 FAIL: durationSec should be positive when samples > 0`);
  }
  console.log(`  Stats OK: sampleRate=${s3.statsWithData.sampleRate}, samples=${s3.statsWithData.samples}, duration=${s3.statsWithData.durationSec.toFixed(3)}s`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 4: Clear capture buffer
  // ========================================================================
  console.log("--- Scenario 4: Clear capture buffer ---");

  const s4 = await page.evaluate(async () => {
    startOutputCapture();
    playSound("kick", 1.0);
    await new Promise((r) => setTimeout(r, 500));

    const beforeClear = getOutputCapture().length;

    clearOutputCapture();

    const afterClear = getOutputCapture().length;

    // Stats should still show enabled
    const stats = getOutputCaptureStats();

    stopOutputCapture();

    return { beforeClear, afterClear, enabledAfterClear: stats.enabled };
  });

  if (s4.afterClear !== 0) throw new Error(`Scenario 4 FAIL: buffer should be empty after clear, got ${s4.afterClear} samples`);
  if (s4.enabledAfterClear !== true) throw new Error(`Scenario 4 FAIL: should still be enabled after clear`);
  console.log(`  Before clear: ${s4.beforeClear} samples, after clear: ${s4.afterClear} samples`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 5: Reset capture node
  // ========================================================================
  console.log("--- Scenario 5: Reset capture node ---");

  const s5 = await page.evaluate(() => {
    const statsBefore = getOutputCaptureStats();

    resetOutputCaptureNode();

    const statsAfter = getOutputCaptureStats();

    return {
      hasNodeBefore: statsBefore.hasNode,
      hasNodeAfter: statsAfter.hasNode,
      enabledAfter: statsAfter.enabled,
      samplesAfter: statsAfter.samples,
    };
  });

  if (s5.hasNodeAfter !== false) throw new Error(`Scenario 5 FAIL: hasNode should be false after reset`);
  if (s5.enabledAfter !== false) throw new Error(`Scenario 5 FAIL: enabled should be false after reset`);
  if (s5.samplesAfter !== 0) throw new Error(`Scenario 5 FAIL: samples should be 0 after reset`);
  console.log("  PASS");

  console.log("\nAll output capture tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_output_capture");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
