// audio_channel_pan.browser.test.js
//
// Tests channel panning via window.setChannelPan / window.channelPan.
// Verifies:
//   1. Set pan, verify round-trip
//   2. Pan clamping behavior (-1 to +1)
//   3. Default pan value is 0 (center)
//   4. Pan on multiple channels independently
//   5. Pan with audio playback (produces output)
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_channel_pan.browser.test.js

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
    typeof setChannelPan === "function" &&
    typeof channelPan === "function" &&
    typeof playSound === "function" &&
    typeof startOutputCapture === "function" &&
    typeof stopOutputCapture === "function"
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
  // Scenario 1: Set pan, verify round-trip
  // ========================================================================
  console.log("--- Scenario 1: Pan round-trip ---");

  const s1 = await page.evaluate(() => {
    // Set pan left
    setChannelPan("kick", -0.75);
    const panLeft = channelPan("kick");

    // Set pan right
    setChannelPan("kick", 0.5);
    const panRight = channelPan("kick");

    // Set pan center
    setChannelPan("kick", 0);
    const panCenter = channelPan("kick");

    return { panLeft, panRight, panCenter };
  });

  if (Math.abs(s1.panLeft - (-0.75)) > 0.01) throw new Error(`Scenario 1 FAIL: expected pan=-0.75, got ${s1.panLeft}`);
  if (Math.abs(s1.panRight - 0.5) > 0.01) throw new Error(`Scenario 1 FAIL: expected pan=0.5, got ${s1.panRight}`);
  if (Math.abs(s1.panCenter) > 0.01) throw new Error(`Scenario 1 FAIL: expected pan=0, got ${s1.panCenter}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 2: Pan clamping behavior
  // ========================================================================
  console.log("--- Scenario 2: Pan clamping ---");

  const s2 = await page.evaluate(() => {
    // Set pan beyond +1
    setChannelPan("kick", 2.5);
    const clampedHi = channelPan("kick");

    // Set pan beyond -1
    setChannelPan("kick", -3.0);
    const clampedLo = channelPan("kick");

    // Reset
    setChannelPan("kick", 0);

    return { clampedHi, clampedLo };
  });

  if (s2.clampedHi > 1.01) throw new Error(`Scenario 2 FAIL: pan should be clamped to 1, got ${s2.clampedHi}`);
  if (s2.clampedLo < -1.01) throw new Error(`Scenario 2 FAIL: pan should be clamped to -1, got ${s2.clampedLo}`);
  console.log(`  Clamped high: ${s2.clampedHi}, Clamped low: ${s2.clampedLo}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 3: Default pan value is 0 (center)
  // ========================================================================
  console.log("--- Scenario 3: Default pan is center ---");

  const s3 = await page.evaluate(() => {
    // Query pan for an instrument that hasn't been panned yet
    const defaultPan = channelPan("snare");
    return { defaultPan };
  });

  if (Math.abs(s3.defaultPan) > 0.01) throw new Error(`Scenario 3 FAIL: default pan should be 0, got ${s3.defaultPan}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 4: Pan on multiple channels independently
  // ========================================================================
  console.log("--- Scenario 4: Independent channel panning ---");

  const s4 = await page.evaluate(() => {
    setChannelPan("kick", -1.0);
    setChannelPan("snare", 0.5);
    setChannelPan("hihat", 1.0);

    const kickPan = channelPan("kick");
    const snarePan = channelPan("snare");
    const hihatPan = channelPan("hihat");

    // Changing one shouldn't affect others
    setChannelPan("kick", 0.3);
    const kickAfter = channelPan("kick");
    const snareAfter = channelPan("snare");
    const hihatAfter = channelPan("hihat");

    // Clean up
    setChannelPan("kick", 0);
    setChannelPan("snare", 0);
    setChannelPan("hihat", 0);

    return { kickPan, snarePan, hihatPan, kickAfter, snareAfter, hihatAfter };
  });

  if (Math.abs(s4.kickPan - (-1.0)) > 0.01) throw new Error(`Scenario 4 FAIL: kick pan should be -1.0, got ${s4.kickPan}`);
  if (Math.abs(s4.snarePan - 0.5) > 0.01) throw new Error(`Scenario 4 FAIL: snare pan should be 0.5, got ${s4.snarePan}`);
  if (Math.abs(s4.hihatPan - 1.0) > 0.01) throw new Error(`Scenario 4 FAIL: hihat pan should be 1.0, got ${s4.hihatPan}`);
  // After changing kick, snare and hihat should be unchanged
  if (Math.abs(s4.kickAfter - 0.3) > 0.01) throw new Error(`Scenario 4 FAIL: kick pan should be 0.3, got ${s4.kickAfter}`);
  if (Math.abs(s4.snareAfter - 0.5) > 0.01) throw new Error(`Scenario 4 FAIL: snare pan should be unchanged at 0.5, got ${s4.snareAfter}`);
  if (Math.abs(s4.hihatAfter - 1.0) > 0.01) throw new Error(`Scenario 4 FAIL: hihat pan should be unchanged at 1.0, got ${s4.hihatAfter}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 5: Pan with audio playback produces output
  // ========================================================================
  let s5;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 5: Pan with playback (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s5 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      await ensureSynthSample("kick");

      // Pan kick hard left
      setChannelPan("kick", -1.0);
      await new Promise((r) => setTimeout(r, 100));

      const NOISE_FLOOR = 0.0001;

      // Warmup
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
      if (!warmupSignal) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Play with pan active
      for (let i = 0; i < 3; i++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 400));
      }

      const captured = Array.from(stopOutputCapture());

      let peak = 0, sum = 0, active = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        if (a > NOISE_FLOOR) { sum += captured[i] * captured[i]; active++; }
      }
      const activeRms = active > 0 ? Math.sqrt(sum / active) : 0;

      // Clean up
      setChannelPan("kick", 0);

      return { samples: captured.length, activeSamples: active, peak, activeRms };
    }, attempt);

    console.log(`  Samples: ${s5.activeSamples}/${s5.samples} active, Peak: ${s5.peak.toFixed(6)}, Active RMS: ${s5.activeRms.toFixed(6)}`);
    if (s5.activeRms >= 0.0001) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: Active RMS below threshold, retrying...`);
      await page.waitForTimeout(1000);
    }
  }
  if (s5.activeRms < 0.0001) throw new Error(`Scenario 5 FAIL: no audio energy with pan active (activeRms=${s5.activeRms})`);
  console.log("  PASS");

  console.log("\nAll channel pan tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_channel_pan");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
