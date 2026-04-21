/**
 * EQ Analyzer Tabs Browser Test
 *
 * Verifies that on WASM the EQ panel's Wave / Spectrum / Meters / Scope tabs
 * render live content (not blank) by synthesizing analyzer.State / scope.State
 * on the Go side from JS-side analyzer snapshots.
 *
 * Also verifies the WASM scope-export flight recorder: enableScopeExport()
 * starts the polling goroutine, forceScopeExportSnapshot() produces a buffered
 * JSONL line, and downloadScopeExport() returns bytes.
 */

import { setupFullWasm } from "./real_input_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

let cleanup;
let page;

try {
  console.log("eq_analyzer_tabs: Setting up WASM...");
  ({ page, cleanup } = await setupFullWasm());

  await page.waitForFunction(
    () =>
      typeof setEQTab === "function" &&
      typeof probeAnalyzerState === "function" &&
      typeof probeScopeState === "function" &&
      typeof setScopeTaps === "function" &&
      typeof enableScopeExport === "function" &&
      typeof downloadScopeExport === "function" &&
      typeof forceScopeExportSnapshot === "function",
    { timeout: 20000 }
  );

  // Install deterministic analyzer snapshots so the bridge sees non-zero data.
  await page.evaluate(() => {
    const sine = (n, amp, freq) => {
      const out = new Array(n);
      for (let i = 0; i < n; i++) out[i] = amp * Math.sin((2 * Math.PI * freq * i) / n);
      return out;
    };
    const spec = (scale) => {
      const out = new Array(64);
      for (let i = 0; i < 64; i++) {
        const x = (i - 28) / 10;
        out[i] = scale * Math.exp(-x * x);
      }
      return out;
    };
    window.channelAnalyzerSnapshot = (id) => ({
      rms: 0.3, peak: 0.7, spectrum: spec(0.9), wave: sine(512, 0.6, 4),
    });
    window.preEQAnalyzerSnapshot = (id) => ({
      rms: 0.2, peak: 0.5, spectrum: spec(0.5), wave: sine(512, 0.4, 3),
    });
  });

  // ─────────────────────────────────────────────────────────────────────
  // Step 1: Verify AnalyzerState callback produces non-nil, populated state.
  // This is the core win — on WASM before the fix, availability was false.
  // ─────────────────────────────────────────────────────────────────────
  const anal = await page.evaluate(() => probeAnalyzerState());
  console.log("eq_analyzer_tabs: probeAnalyzerState =", JSON.stringify(anal));
  if (!anal.available) {
    throw new Error("AnalyzerState callback returned nil — WASM bridge not wired");
  }
  if (anal.masterFFTBins <= 0) {
    throw new Error(`Master FFTBins empty (got ${anal.masterFFTBins}) — spectrum tab would be blank`);
  }
  if (anal.masterWaveformLen <= 0) {
    throw new Error(`Master Waveform empty (got ${anal.masterWaveformLen}) — wave tab would be blank`);
  }
  if (!anal.masterActive) {
    throw new Error("Master channel marked inactive despite non-zero peak/rms");
  }
  if (anal.masterPeakDB >= 0 || anal.masterPeakDB < -80) {
    throw new Error(`Master PeakDB out of range: ${anal.masterPeakDB} (expected -80..0)`);
  }
  // With peak=0.7 linear, PeakDB should be ~-3.1 dB.
  if (anal.masterPeakDB < -6 || anal.masterPeakDB > -1) {
    throw new Error(`Master PeakDB=${anal.masterPeakDB}, expected near -3.1 dB for linear 0.7`);
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 2: Iterate every analyzer-driven tab — verify no panics / empty.
  // ─────────────────────────────────────────────────────────────────────
  const tabs = ["wave", "spectrum", "meters", "scope"];
  for (const tab of tabs) {
    await page.evaluate((t) => {
      setEQTab(t);
      forceDraw?.();
      forceDraw?.();
    }, tab);
    await page.waitForTimeout(80);
    const still = await page.evaluate(() => probeAnalyzerState());
    if (!still.available) {
      throw new Error(`AnalyzerState went nil after setEQTab("${tab}")`);
    }
  }
  console.log("eq_analyzer_tabs: all 4 tabs surveyed — AnalyzerState stays live");

  // ─────────────────────────────────────────────────────────────────────
  // Step 3: Scope state — set synth/eq taps, verify both taps are active.
  // Before the fix, callbacks.ScopeState returned nil because ScopeService
  // was nil on WASM. Now it synthesizes from pre/post-EQ snapshots.
  // ─────────────────────────────────────────────────────────────────────
  await page.evaluate(() => {
    setEQTab("scope");
    setScopeTaps("synth", "eq");
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(100);
  const scope1 = await page.evaluate(() => probeScopeState());
  console.log("eq_analyzer_tabs: probeScopeState (Synth/EQ) =", JSON.stringify(scope1));
  if (!scope1.available) {
    throw new Error("ScopeState callback returned nil — bridge not wired");
  }
  if (!scope1.tapAActive) {
    throw new Error("TapA (Synth=pre-EQ) should be active with non-zero pre-EQ snapshot");
  }
  if (!scope1.tapBActive) {
    throw new Error("TapB (EQ=post-EQ) should be active with non-zero post-EQ snapshot");
  }
  if (scope1.tapASamples !== 512) {
    throw new Error(`TapA samples: got ${scope1.tapASamples}, expected 512`);
  }
  if (scope1.tapBSamples !== 512) {
    throw new Error(`TapB samples: got ${scope1.tapBSamples}, expected 512`);
  }

  // Unbridged stages (InsertFX / Master) must report inactive so the
  // renderer shows a blank trace rather than mis-attributing data.
  await page.evaluate(() => setScopeTaps("insertfx", "master"));
  const scope2 = await page.evaluate(() => probeScopeState());
  console.log("eq_analyzer_tabs: probeScopeState (InsertFX/Master) =", JSON.stringify(scope2));
  if (scope2.tapAActive) {
    throw new Error("InsertFX tap should be inactive on WASM (no bridge) — got active");
  }
  if (scope2.tapBActive) {
    throw new Error("Master tap should be inactive on WASM (no bridge) — got active");
  }

  // ─────────────────────────────────────────────────────────────────────
  // Step 4: Scope flight recorder — start, force a snapshot, flush, verify.
  // ─────────────────────────────────────────────────────────────────────
  await page.evaluate(() => enableScopeExport());
  await page.waitForTimeout(250);
  const forced = await page.evaluate(() => forceScopeExportSnapshot());
  if (!forced) {
    throw new Error("forceScopeExportSnapshot returned false — export service not running");
  }
  const len = await page.evaluate(() => scopeExportBufferLen());
  if (len <= 0) {
    throw new Error(`scope export buffer empty after forceScopeExportSnapshot (len=${len})`);
  }
  const downloaded = await page.evaluate(() => {
    window.__scopeExportDownload = null;
    window.downloadJSON = (name, text) => { window.__scopeExportDownload = { name, text }; };
    return downloadScopeExport();
  });
  if (!downloaded || downloaded <= 0) {
    throw new Error(`downloadScopeExport returned ${downloaded}`);
  }
  const captured = await page.evaluate(() => window.__scopeExportDownload);
  if (!captured || captured.name !== "scope_export.jsonl") {
    throw new Error(`downloadScopeExport didn't call downloadJSON: ${JSON.stringify(captured)}`);
  }
  const lines = captured.text.trim().split("\n");
  for (const line of lines) JSON.parse(line);
  const firstSnap = JSON.parse(lines[0]);
  if (!firstSnap.channels || firstSnap.channels.length === 0) {
    throw new Error("scope export JSONL has no channels");
  }
  console.log(`eq_analyzer_tabs: scope export OK — ${lines.length} line(s), ${downloaded} bytes`);

  // ─────────────────────────────────────────────────────────────────────
  // Step 5: Empty-mock regression — when JS returns empty snapshots the
  // bridge must NOT mark channels active (prevents ghost meter readings).
  // ─────────────────────────────────────────────────────────────────────
  await page.evaluate(() => {
    window.channelAnalyzerSnapshot = () => ({ rms: 0, peak: 0, spectrum: [], wave: [] });
    window.preEQAnalyzerSnapshot = () => ({ rms: 0, peak: 0, spectrum: [], wave: [] });
  });
  const silent = await page.evaluate(() => probeAnalyzerState());
  console.log("eq_analyzer_tabs: silent probeAnalyzerState =", JSON.stringify(silent));
  if (silent.masterActive) {
    throw new Error("Master should be inactive with all-zero analyzer snapshot");
  }

  console.log("eq_analyzer_tabs: all checks passed ✓");

  if (isCoverageEnabled()) {
    await flushCoverage(page, "eq_analyzer_tabs");
  }
} finally {
  if (cleanup) {
    try { await cleanup(); } catch (_) {}
  }
}
