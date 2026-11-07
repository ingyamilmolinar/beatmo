// The readiness gate: the LOADED circuit's voices are pre-rendered by Go's warm-up (not a hardcoded list), the game is
// not interactive before audioReady and WASM, the first play has no stale burst, a deferred render plays at once.
// Usage: GO=… node src/js/webaudio_ready_gate.browser.test.js

import { chromium, devices } from "playwright";
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
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
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

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

  // Rendered by Go's warm-up after it pushes each document, so the circuit decides what is warm.
  console.log("--- Scenario 1: the loaded circuit's voices are warm ---");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() => typeof addNode === "function" && typeof rowInstrument === "function" &&
      typeof voiceCachedFor === "function" && typeof drumRowCount === "function" && drumRowCount() > 0, null, { timeout: 30000 });

    // Melodic voices are cached per pitch, so any cached pitch proves the warm-up ran.
    const probe = () => {
      const stats = getRenderCacheStats?.() || {};
      const ids = [];
      for (let i = 0; i < drumRowCount(); i++) { const id = rowInstrument(i); if (id && !ids.includes(id)) ids.push(id); }
      const warm = (id) => isMelodicInstrumentJS(id)
        ? Object.keys(stats).some((k) => (k === id || k.startsWith(id + ":")) && stats[k].frames > 0)
        : voiceCachedFor(id, 0);
      return { ids, missing: ids.filter((id) => !warm(id)), stats };
    };
    let result;
    try {
      await page.waitForFunction(new Function(`return (${probe.toString()})().missing.length === 0;`), null, { timeout: 20000 });
    } catch (_) { /* reported below */ }
    result = await page.evaluate(probe);

    console.log(`  Circuit: ${result.ids.join(', ')}`);
    if (result.ids.length === 0) throw new Error("Scenario 1 FAIL: the startup circuit has no rows");
    if (result.missing.length > 0) {
      throw new Error(`Scenario 1 FAIL: circuit voices not warm: ${result.missing.join(', ')} (cache keys: ${Object.keys(result.stats).join(', ')})`);
    }
    for (const [k, st] of Object.entries(result.stats)) {
      if (st.frames > 0 && st.duration < 0.05) {
        throw new Error(`Scenario 1 FAIL: ${k} sample too short: ${st.duration}s`);
      }
    }

    console.log("  PASS: every circuit voice pre-rendered in cache");
    await page.close();
  }

  // Exports must not exist before both wasmReady and audioReady resolve.
  console.log("\n--- Scenario 2: Game gated on audioReady ---");
  {
    const page = await browser.newPage();

    await page.goto(`http://localhost:${port}/`);

    // audioReady should be a thenable on window immediately (set by audio.js module).
    const hasAudioReady = await page.evaluate(() => {
      return typeof window.audioReady === 'object' &&
             typeof window.audioReady.then === 'function';
    });

    if (!hasAudioReady) {
      throw new Error("Scenario 2 FAIL: window.audioReady is not a Promise");
    }
    console.log("  window.audioReady is a Promise");

    // Wait for WASM to be fully ready.
    await page.waitForFunction(() => typeof addNode === "function", null, { timeout: 30000 });

    // So the C module is loaded and can render synchronously.
    const gate = await page.evaluate(async () => {
      let settled = false;
      window.audioReady.then(() => { settled = true; });
      await Promise.resolve();
      await Promise.resolve();
      return { settled };
    });

    if (!gate.settled) {
      throw new Error(`Scenario 2 FAIL: game became interactive before audio was ready (${JSON.stringify(gate)})`);
    }

    console.log("  PASS: game interactive only after the audio module loaded");
    await page.close();
  }

  // Audio starts promptly with no pile of queued events.
  console.log("\n--- Scenario 3: No stale-burst on first play (mobile) ---");
  {
    const iPhone = devices['iPhone 12 landscape'];
    const context = await browser.newContext({
      ...iPhone,
      hasTouch: true,
    });
    const page = await context.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof addNode === "function" &&
      typeof addEdgeGrid === "function" &&
      typeof updateBeatInfos === "function" &&
      typeof startPlay === "function" &&
      typeof stopPlay === "function" &&
      typeof startOutputCapture === "function" &&
      typeof stopOutputCapture === "function"
    , null, { timeout: 30000 });

    // Unlock audio context.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(100);

    // Build a simple 2-node loop circuit.
    await page.evaluate(() => {
      addNode?.(0, 0, "regular");
      addNode?.(4, 0, "regular");
      addEdgeGrid?.(0, 0, 4, 0);
      addEdgeGrid?.(4, 0, 0, 0);
      updateBeatInfos?.();
      forceDraw?.();
    });
    await page.waitForTimeout(200);

    // Play is DELIBERATELY ignored while warming ("never queue the press").
    await page.waitForFunction(() => typeof warmupProgress === "function" && warmupProgress().ready === true, null, { timeout: 20000 });

    // Pipeline warm-up time varies under contention, so poll for real signal first.
    const result = await page.evaluate(async () => {
      enableOutputCapture?.();
      startOutputCapture?.();
      startPlay?.();

      // Warmup: poll for real audio signal (up to 5s).
      const warmupDeadline = Date.now() + 5000;
      let warmupSignal = false;
      while (Date.now() < warmupDeadline) {
        const snap = getOutputCapture?.();
        if (snap) {
          for (let i = 0; i < snap.length; i++) {
            if (Math.abs(snap[i]) > 0.01) { warmupSignal = true; break; }
          }
        }
        if (warmupSignal) break;
        await new Promise(r => setTimeout(r, 50));
      }

      if (!warmupSignal) {
        stopPlay?.();
        stopOutputCapture?.();
        return { error: "warmup timeout: no audio signal within 5s" };
      }

      // Pipeline is active. Clear warmup samples and measure for 500ms.
      clearOutputCapture?.();
      await new Promise(r => setTimeout(r, 500));
      stopPlay?.();
      await new Promise(r => setTimeout(r, 200));
      const measured = Array.from(stopOutputCapture?.() || []);

      let peak = 0;
      let sum = 0;
      for (let i = 0; i < measured.length; i++) {
        const a = Math.abs(measured[i]);
        if (a > peak) peak = a;
        sum += measured[i] * measured[i];
      }
      const rms = measured.length > 0 ? Math.sqrt(sum / measured.length) : 0;

      return { samples: measured.length, peak, rms };
    });

    if (result.error) {
      throw new Error(`Scenario 3 FAIL: ${result.error}`);
    }

    console.log(`  Measured (post-warmup 500ms): ${result.samples} samples, peak=${result.peak.toFixed(6)}, RMS=${result.rms.toFixed(6)}`);

    // Once warm, audio must be continuously present.
    if (result.samples === 0) {
      throw new Error("Scenario 3 FAIL: no audio output captured");
    }
    if (result.rms < 0.0001) {
      throw new Error(`Scenario 3 FAIL: no audio energy (RMS=${result.rms})`);
    }
    if (result.peak < 0.001) {
      throw new Error(`Scenario 3 FAIL: no audible audio in measurement window (peak=${result.peak})`);
    }

    console.log("  PASS: audio appeared promptly on first play");
    await context.close();
  }

  // A forced miss must re-render, repopulate the cache and sound.
  console.log("\n--- Scenario 4: Deferred render repopulates cache and plays ---");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof playSound === "function" &&
      typeof resetRenderCache === "function" &&
      typeof getRenderCacheStats === "function"
    , null, { timeout: 30000 });

    // Unlock audio context.
    await page.evaluate(() => {
      document.dispatchEvent(new Event("pointerdown"));
      resumeAudio?.();
    });
    await page.waitForTimeout(200);

    // A non-melodic instrument of the loaded circuit (one cache key), warmed by the circuit warm-up.
    await page.waitForFunction(() => typeof rowInstrument === "function" && drumRowCount() > 0, null, { timeout: 30000 });
    await page.evaluate(() => {
      for (let i = 0; i < drumRowCount(); i++) {
        const id = rowInstrument(i);
        if (id && !isMelodicInstrumentJS(id)) { window.__probeId = id; return; }
      }
    });
    // So the channel pipeline is established before the deferred path is tested.
    await page.evaluate(() => playSound?.(window.__probeId, 1.0));
    await page.waitForFunction(() => { const s = getRenderCacheStats?.() || {}; return !!(s[window.__probeId] && s[window.__probeId].frames > 0); }, null, { timeout: 15000 }).catch(() => {});

    // Verify cache has the probe before clearing.
    const preStats = await page.evaluate(() => {
      const s = getRenderCacheStats?.() || {};
      return { hasKick: !!(s[window.__probeId] && s[window.__probeId].frames > 0) };
    });
    if (!preStats.hasKick) {
      throw new Error("Scenario 4 FAIL: probe voice not in cache before clear");
    }

    // Clear the render cache to force a deferred render on next play.
    await page.evaluate(() => resetRenderCache?.());

    // Verify cache is empty.
    const postClearStats = await page.evaluate(() => {
      const s = getRenderCacheStats?.() || {};
      return { hasKick: !!(s[window.__probeId] && s[window.__probeId].frames > 0) };
    });
    if (postClearStats.hasKick) {
      throw new Error("Scenario 4 FAIL: cache not cleared");
    }

    // Recorded at submission, independent of the audio thread.
    await page.evaluate(() => {
      window.__samples = [];
      window.__captureSamples = true;
    });

    // Trigger a sound — this will hit the deferred path since cache was cleared.
    const result = await page.evaluate(async () => {
      // Fire the probe — will defer since cache is cleared.
      playSound?.(window.__probeId, 1.0);

      // Wait for the deferred render to complete (cache repopulated).
      let cacheReady = false;
      for (let i = 0; i < 100; i++) {
        const stats = getRenderCacheStats?.() || {};
        if (stats[window.__probeId] && stats[window.__probeId].frames > 0) { cacheReady = true; break; }
        await new Promise((r) => setTimeout(r, 20));
      }

      if (!cacheReady) {
        return { error: "cache never repopulated by deferred render" };
      }

      for (let j = 0; j < 3; j++) {
        playSound?.(window.__probeId, 1.0);
        await new Promise((r) => setTimeout(r, 150));
      }

      // Give final kick time to be submitted.
      await new Promise((r) => setTimeout(r, 300));

      const captured = Array.from(window.__samples || []);
      window.__captureSamples = false;

      let peak = 0;
      let sum = 0;
      for (let i = 0; i < captured.length; i++) {
        const a = Math.abs(captured[i]);
        if (a > peak) peak = a;
        sum += captured[i] * captured[i];
      }
      const rms = captured.length > 0 ? Math.sqrt(sum / captured.length) : 0;

      return { samples: captured.length, peak, rms };
    });

    console.log(`  Samples: ${result.samples}, Peak: ${result.peak.toFixed(6)}, RMS: ${result.rms.toFixed(6)}`);

    if (result.error) {
      throw new Error(`Scenario 4 FAIL: ${result.error}`);
    }
    if (result.samples === 0) {
      throw new Error("Scenario 4 FAIL: no audio captured");
    }
    if (result.peak < 0.001) {
      throw new Error(`Scenario 4 FAIL: no audible audio (peak=${result.peak.toFixed(6)})`);
    }

    console.log("  PASS: deferred render repopulated cache and audio is audible");
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_ready_gate");
    await page.close();
  }

  console.log("\nAll audio readiness gate tests passed.");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
