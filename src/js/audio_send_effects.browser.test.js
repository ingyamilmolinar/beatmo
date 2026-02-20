// audio_send_effects.browser.test.js
//
// Tests send effects (delay/reverb send) on instrument channels.
// Verifies:
//   1. Set delay send on an instrument, verify round-trip
//   2. Set reverb send on an instrument, verify round-trip
//   3. Play with send effects active (produces audio output)
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_send_effects.browser.test.js

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
    typeof setDelaySend === "function" &&
    typeof setReverbSend === "function" &&
    typeof delaySend === "function" &&
    typeof reverbSend === "function" &&
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
  // Scenario 1: Set delay send and verify round-trip
  // ========================================================================
  console.log("--- Scenario 1: Delay send round-trip ---");

  const s1 = await page.evaluate(() => {
    // Default should be 0
    const before = delaySend("kick");

    // Set delay send
    setDelaySend("kick", 0.6);
    const after = delaySend("kick");

    // Set to 0 should clear
    setDelaySend("kick", 0);
    const cleared = delaySend("kick");

    // Clamping: values > 1 should clamp to 1, < 0 should clamp to 0
    setDelaySend("kick", 1.5);
    const clamped_hi = delaySend("kick");
    setDelaySend("kick", -0.5);
    const clamped_lo = delaySend("kick");

    // Clean up
    setDelaySend("kick", 0);

    return { before, after, cleared, clamped_hi, clamped_lo };
  });

  if (s1.before !== 0) throw new Error(`Scenario 1 FAIL: default delay send should be 0, got ${s1.before}`);
  if (Math.abs(s1.after - 0.6) > 0.01) throw new Error(`Scenario 1 FAIL: delay send should be 0.6, got ${s1.after}`);
  if (s1.cleared !== 0) throw new Error(`Scenario 1 FAIL: cleared delay send should be 0, got ${s1.cleared}`);
  if (s1.clamped_hi > 1.01) throw new Error(`Scenario 1 FAIL: delay send should be clamped to 1, got ${s1.clamped_hi}`);
  if (s1.clamped_lo < -0.01) throw new Error(`Scenario 1 FAIL: delay send should be clamped to 0, got ${s1.clamped_lo}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 2: Set reverb send and verify round-trip
  // ========================================================================
  console.log("--- Scenario 2: Reverb send round-trip ---");

  const s2 = await page.evaluate(() => {
    const before = reverbSend("kick");

    setReverbSend("kick", 0.4);
    const after = reverbSend("kick");

    setReverbSend("kick", 0);
    const cleared = reverbSend("kick");

    // Clamping
    setReverbSend("kick", 2.0);
    const clamped_hi = reverbSend("kick");
    setReverbSend("kick", -1.0);
    const clamped_lo = reverbSend("kick");

    // Clean up
    setReverbSend("kick", 0);

    return { before, after, cleared, clamped_hi, clamped_lo };
  });

  if (s2.before !== 0) throw new Error(`Scenario 2 FAIL: default reverb send should be 0, got ${s2.before}`);
  if (Math.abs(s2.after - 0.4) > 0.01) throw new Error(`Scenario 2 FAIL: reverb send should be 0.4, got ${s2.after}`);
  if (s2.cleared !== 0) throw new Error(`Scenario 2 FAIL: cleared reverb send should be 0, got ${s2.cleared}`);
  if (s2.clamped_hi > 1.01) throw new Error(`Scenario 2 FAIL: reverb send should be clamped to 1, got ${s2.clamped_hi}`);
  if (s2.clamped_lo < -0.01) throw new Error(`Scenario 2 FAIL: reverb send should be clamped to 0, got ${s2.clamped_lo}`);
  console.log("  PASS");

  // ========================================================================
  // Scenario 3: Play with send effects active produces audio
  // ========================================================================
  let s3;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    console.log(`--- Scenario 3: Play with send effects active (attempt ${attempt}/${MAX_RETRIES}) ---`);

    s3 = await page.evaluate(async (attempt) => {
      if (attempt > 1) resetOutputCaptureNode();

      await ensureSynthSample("kick");

      // Set send levels
      setDelaySend("kick", 0.5);
      setReverbSend("kick", 0.3);
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
      if (warmupSignal) {
        await new Promise((r) => setTimeout(r, 300));
      } else {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 1500));
      }
      clearOutputCapture();

      // Actual measurement: play kicks with sends active
      for (let i = 0; i < 3; i++) {
        playSound("kick", 1.0);
        await new Promise((r) => setTimeout(r, 400));
      }
      // Wait extra for delay/reverb tails
      await new Promise((r) => setTimeout(r, 500));

      const captured = Array.from(stopOutputCapture());

      let peak = 0, sum = 0, active = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        if (a > NOISE_FLOOR) { sum += captured[i] * captured[i]; active++; }
      }
      const activeRms = active > 0 ? Math.sqrt(sum / active) : 0;

      // Clean up sends
      setDelaySend("kick", 0);
      setReverbSend("kick", 0);

      return { samples: captured.length, activeSamples: active, peak, activeRms };
    }, attempt);

    console.log(`  Samples: ${s3.activeSamples}/${s3.samples} active, Peak: ${s3.peak.toFixed(6)}, Active RMS: ${s3.activeRms.toFixed(6)}`);
    if (s3.activeRms >= 0.0001) break;
    if (attempt < MAX_RETRIES) {
      console.log(`  WARNING: Active RMS=${s3.activeRms.toFixed(6)} below threshold, retrying...`);
      await page.waitForTimeout(1000);
    }
  }
  if (s3.activeRms < 0.0001) throw new Error(`Scenario 3 FAIL: no audio energy with sends active (activeRms=${s3.activeRms})`);
  console.log("  PASS");

  console.log("\nAll send effects tests passed.");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_send_effects");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
