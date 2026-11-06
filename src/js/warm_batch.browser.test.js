/**
 * Pins the browser warm bridge: warmInstrumentBatch must render every
 * requested (instrument, pitch) and report completion, and voiceCachedFor must
 * report TRUE for an item that is already in renderCache.
 *
 * Before this change voice_prewarm_js.go stubbed VoiceCached to a hardcoded
 * false, so the browser re-rendered voices it already held.
 */
import assert from "assert";
import { setupFullWasm, clickAt } from "./real_input_test_helpers.js";

(async () => {
  const { page, cleanup } = await setupFullWasm({ logLevel: "ERROR" });
  try {
    await clickAt(page, 640, 400);
    await page.waitForTimeout(300);

    const res = await page.evaluate(async () => {
      const items = [["cello", 0], ["cello", 7], ["cello", 12]];
      const before = items.map(([id, p]) => window.voiceCachedFor(id, p));
      let completed = 0;
      await new Promise((resolve) => {
        window.__warmBatchDone = () => { completed++; if (completed === items.length) resolve(); };
        window.warmInstrumentBatch(JSON.stringify(items), "__warmBatchDone");
        setTimeout(resolve, 60000);
      });
      const after = items.map(([id, p]) => window.voiceCachedFor(id, p));
      return { before, after, completed, n: items.length };
    });

    assert.strictEqual(res.completed, res.n, `only ${res.completed}/${res.n} warms completed`);
    assert.ok(res.after.every(Boolean), `voiceCachedFor false after warm: ${JSON.stringify(res.after)}`);
    console.log("[warm-batch] PASS", JSON.stringify(res));
  } finally {
    await cleanup();
  }
})();
