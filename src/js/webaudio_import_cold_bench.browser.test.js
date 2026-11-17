/**
 * Import-cold bench: load a heavy circuit, immediately play, and record how
 * long the engine keeps rendering while the user is listening.
 *
 * BASELINE 2026-09-13 (beach-gaelic, 91 distinct (instrument,pitch) pairs):
 *   neighborPlays=66 (notes played at the WRONG PITCH), deferredPlays=4 at
 *   288ms off-grid, renderMsAvg=270ms, workerQueuePeak=12, still rendering at
 *   t=27s. See docs/superpowers/specs/2026-09-13-import-warmup-playback-gate-design.md
 *
 * TARGET after the warm-up gate: neighborPlays=0, deferredPlays=0, 0 renders
 * during playback.
 *
 * Also (Task 15 Step 3) re-imports the SAME document in the SAME page
 * session after the first playback window and asserts the second
 * warmupProgress().total is 0 — proof that "same instruments, same params,
 * same notes" is a cache hit, not just an assumption.
 *
 * OUTPUT: bench-results/import_cold.json with schema
 *   { template, gateMs, samples: [{tSec, renders, neighborPlays,
 *     deferredPlays, deferMsAvg, renderMsAvg, workerQueuePeak, updateAvgMS,
 *     drawAvgMS}], reimport: {reimportMs, done, total, ready} }
 */
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { setupFullWasm, clickAt } from "./real_input_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const tplDir = path.resolve(__dirname, "../go/internal/assets/templates");
const outDir = path.resolve(__dirname, "../../bench-results");
const TPL = process.env.TPL || "beach-gaelic.json";
const SECS = parseInt(process.env.SECS || "30", 10);

(async () => {
  const { page, cleanup } = await setupFullWasm({ logLevel: "ERROR" });
  const samples = [];
  try {
    const raw = fs.readFileSync(path.join(tplDir, TPL), "utf8");
    await clickAt(page, 640, 400);          // user gesture unlocks AudioContext
    await page.waitForTimeout(300);
    await page.evaluate((j) => window.importJSON(j), raw);

    // Wait for the readiness gate (absent pre-Task-10: falls through at once).
    const gateT0 = Date.now();
    await page.waitForFunction(
      () => !window.warmupProgress || window.warmupProgress().done >= window.warmupProgress().total, null,
      { timeout: 60000 },
    );
    const gateMs = Date.now() - gateT0;

    await page.evaluate(() => {
      if (window.resetPerfStats) window.resetPerfStats();
      if (window.resetRenderLatencyMetrics) window.resetRenderLatencyMetrics();
      window.__audioMetrics = { renders: {}, cacheHits: {} };
      window.startPlay();
    });
    const t0 = Date.now();
    for (let i = 0; i < SECS; i++) {
      await page.waitForTimeout(1000);
      const s = await page.evaluate(() => {
        const rl = window.getRenderLatencyMetrics ? window.getRenderLatencyMetrics() : {};
        const ps = window.perfStats ? window.perfStats() : {};
        const am = window.__audioMetrics || {};
        return {
          renders: Object.values(am.renders || {}).reduce((a, b) => a + b, 0),
          neighborPlays: rl.neighborPlays | 0,
          deferredPlays: rl.deferredPlays | 0,
          deferMsAvg: rl.deferMsAvg || 0,
          renderMsAvg: rl.renderMsAvg || 0,
          workerQueuePeak: rl.workerQueuePeak | 0,
          updateAvgMS: ps.updateAvgMS ?? null,
          drawAvgMS: ps.drawAvgMS ?? null,
        };
      });
      samples.push({ tSec: (Date.now() - t0) / 1000, ...s });
    }
    const last = samples[samples.length - 1];
    console.log(`[import-cold] ${TPL} gateMs=${gateMs} renders=${last.renders} ` +
      `neighborPlays=${last.neighborPlays} deferredPlays=${last.deferredPlays} ` +
      `renderMsAvg=${last.renderMsAvg.toFixed(1)} queuePeak=${last.workerQueuePeak}`);

    // Task 15 Step 3: re-import the SAME document in the SAME session and
    // require the warm set to be empty — "same instruments, same params,
    // same notes ⇒ cache hit", measured rather than assumed. Stop playback
    // first so the second importJSON isn't racing live triggers.
    await page.evaluate(() => {
      // NOT `window.stop()` as a fallback: that is the standard DOM method
      // that ABORTS PAGE LOADS, not the playback export (which is stopPlay).
      // The guard made it look like a graceful degradation; it would have
      // torn the page down instead of stopping the sequencer.
      if (typeof window.stopPlay === "function") window.stopPlay();
    });
    const reimportT0 = Date.now();
    await page.evaluate((j) => window.importJSON(j), raw);
    await page.waitForFunction(
      () => !window.warmupProgress || window.warmupProgress().done >= window.warmupProgress().total, null,
      { timeout: 10000 },
    );
    const reimportMs = Date.now() - reimportT0;
    const reimportProgress = await page.evaluate(() =>
      window.warmupProgress ? window.warmupProgress() : { done: 0, total: 0, ready: true });
    console.log(`[import-cold-reimport] ${TPL} reimportMs=${reimportMs} ` +
      `total=${reimportProgress.total} done=${reimportProgress.done} ready=${reimportProgress.ready}`);
    if (reimportProgress.total !== 0) {
      throw new Error(
        `re-import cache-hit check FAILED: warmupProgress().total=${reimportProgress.total} ` +
        `(expected 0 — every voice from the first import should already be cached)`);
    }

    fs.mkdirSync(outDir, { recursive: true });
    fs.writeFileSync(path.join(outDir, "import_cold.json"),
      JSON.stringify({ template: TPL, gateMs, samples, reimport: { reimportMs, ...reimportProgress } }, null, 2));
  } finally {
    await cleanup();
  }
})();
