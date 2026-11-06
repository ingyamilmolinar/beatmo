// audio_ready_gate.browser.test.js
//
// Verifies that the audio readiness gate works correctly:
//   1. every instrument in the LOADED circuit is pre-rendered by Go's circuit
//      warm-up (the circuit — not a hardcoded id list — decides what is warm)
//   2. Game is not interactive until audioReady + WASM both resolve
//   3. No stale-burst on first play (mobile emulation)
//   4. Deferred render plays immediately (no stale when timestamp)
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/audio_ready_gate.browser.test.js

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

  // ========================================================================
  // Scenario 1: every voice the LOADED circuit plays is warm. Go's circuit
  // warm-up gate renders them after seeding each instrument's recipe defaults;
  // audio.js used to pre-render a hardcoded "core" list instead, which rendered
  // the wrong (unseeded) voice for every config-first instrument and was then
  // evicted — so it asserted a list that no longer described the app.
  // ========================================================================
  console.log("--- Scenario 1: the loaded circuit's voices are warm ---");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() => typeof addNode === "function" && typeof rowInstrument === "function" &&
      typeof voiceCachedFor === "function" && typeof drumRowCount === "function" && drumRowCount() > 0, null, { timeout: 30000 });

    // Non-melodic voices share one bare-id cache key; melodic ones are cached
    // per node pitch, so for those any cached pitch proves the warm-up ran.
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

  // ========================================================================
  // Scenario 2: Game not interactive until audioReady resolves
  // Verify that the gating in index.html actually works — WASM exports should
  // not exist before Promise.all([wasmReady, audioReady]) resolves.
  // ========================================================================
  console.log("\n--- Scenario 2: Game gated on audioReady ---");
  {
    const page = await browser.newPage();

    // We check that after navigation, audioReady is a Promise (exists on window).
    // Then once WASM exports appear, audioReady has already resolved.
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

    // At this point audioReady must have resolved (the game gate waited on it),
    // which means the C DSP module is loaded and can render synchronously.
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

  // ========================================================================
  // Scenario 3: No stale-burst on first play (mobile emulation)
  // Build a circuit, start playback, verify audio starts promptly and doesn't
  // pile up queued events.
  // ========================================================================
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

    // Play is DELIBERATELY ignored while the circuit readiness gate is warming
    // (game_update.go: "never queue the press"), so wait for the app's own
    // gate — the same signal the Play button renders from — before pressing.
    await page.waitForFunction(() => typeof warmupProgress === "function" && warmupProgress().ready === true, null, { timeout: 20000 });

    // Enable output capture and start playback.
    // Use a polling warmup loop to wait for the WebAudio pipeline to produce
    // real signal before measuring — pipeline warmup time is variable under
    // CPU contention.
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

    // After warmup confirms the pipeline is active, audio should be
    // continuously present in the measurement window (no stale-burst).
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

  // ========================================================================
  // Scenario 4: Deferred render repopulates cache and audio plays
  // Force a render cache miss, trigger a sound, verify the deferred render
  // path re-renders the sample and that audio from the repopulated cache
  // is audible.
  // ========================================================================
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

    // The probe voice is a NON-melodic instrument of the loaded circuit (one
    // bare-id cache key), warmed by the circuit warm-up — not a hardcoded id.
    await page.waitForFunction(() => typeof rowInstrument === "function" && drumRowCount() > 0, null, { timeout: 30000 });
    await page.evaluate(() => {
      for (let i = 0; i < drumRowCount(); i++) {
        const id = rowInstrument(i);
        if (id && !isMelodicInstrumentJS(id)) { window.__probeId = id; return; }
      }
    });
    // Pre-warm the probe channel by playing once. This ensures the WebAudio
    // channel pipeline (bus -> channel -> main -> limiter) is fully established
    // before we test the deferred render path.
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

    // Enable synchronous sample capture (records at WebAudio submission point,
    // not dependent on real-time audio thread processing).
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

      // Cache is now repopulated. Play multiple kicks from the repopulated
      // cache to verify it produces audible audio.
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
