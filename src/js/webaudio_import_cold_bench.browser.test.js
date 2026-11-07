/** Load a heavy circuit, play at once, and record how long the engine keeps rendering while the user listens; the
 * warm-up gate's target is zero renders, neighbour plays or defers during playback. A same-session re-import must warm
 * nothing (a cache hit, measured). → bench-results/import_cold.json */
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

    // Stopped first so the second import does not race live triggers.
    await page.evaluate(() => {
      // Never window.stop(): that DOM method aborts page loads.
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
