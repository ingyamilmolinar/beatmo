/**
 * Visual Regression Test
 *
 * Stores golden baseline PNGs for specific viewports. On each run, takes
 * a screenshot and compares with pixelmatch. If the diff exceeds a
 * threshold (0.5% of pixels), fails and saves a diff image for debugging.
 *
 * Also runs layout sanity checks via fullLayoutSnapshot() and saves the
 * layout JSON alongside golden PNGs for debugging.
 *
 * First run bootstraps golden files automatically.
 *
 * Golden update workflow:
 *   make test-visual-update   # Delete old goldens and re-bootstrap
 *   make test-visual          # Run regression check
 */

import { chromium } from "playwright";
import fs from "fs";
import {
  buildWasm,
  startServer,
  initPage,
  captureScreenshot,
  loadMultiRowFixture,
  captureLayoutSnapshot,
  compareWithGolden,
  saveDiffImage,
  saveGolden,
  goldenDir,
} from "./visual_test_helpers.js";
import { runAllLayoutChecks } from "./layout_parity_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

// ─── Configuration ───────────────────────────────────────────────────

const VIEWPORTS = [
  { name: "desktop_1280x720", width: 1280, height: 720 },
  { name: "mobile_390x844", width: 390, height: 844 },
  { name: "mobile_landscape_844x390", width: 844, height: 390 },
];

// Maximum allowed pixel diff as percentage (0.5%)
const MAX_DIFF_PCT = 0.5;

// ─── Build WASM ──────────────────────────────────────────────────────

console.log("Building WASM...");
buildWasm();

const { server, port } = await startServer();
const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});

let failures = 0;
let bootstrapped = 0;

// ─── Test Each Viewport ──────────────────────────────────────────────

for (const vp of VIEWPORTS) {
  console.log(`\n=== ${vp.name} (${vp.width}x${vp.height}) ===`);

  const page = await initPage(browser, { width: vp.width, height: vp.height }, port);

  try {
    await loadMultiRowFixture(page);

    const screenshot = await captureScreenshot(page);
    const result = compareWithGolden(vp.name, screenshot);

    if (result.bootstrapped) {
      console.log(`  Bootstrapped golden baseline: ${goldenDir}/${vp.name}.png`);
      bootstrapped++;
      console.log("  PASS (new baseline saved)");
    } else if (result.error) {
      console.error(`  FAIL: ${result.error}`);
      // Save current screenshot for comparison
      saveGolden(`${vp.name}.current`, screenshot);
      failures++;
    } else {
      console.log(`  Pixel diff: ${result.diffCount} (${result.diffPct.toFixed(3)}%)`);

      if (result.match) {
        console.log("  Golden comparison: PASS");
      } else {
        console.error(`  FAIL: Diff exceeds threshold — ${result.diffPct.toFixed(3)}% > ${MAX_DIFF_PCT}%`);
        // Save current and diff for debugging
        saveGolden(`${vp.name}.current`, screenshot);
        if (result.diffPng) {
          saveDiffImage(vp.name, result.diffPng);
          console.log(`  Diff image saved: ${goldenDir}/${vp.name}.diff.png`);
        }
        failures++;
      }
    }

    // ── Layout sanity gate ──
    const snap = await captureLayoutSnapshot(page);
    if (snap) {
      // Save layout JSON alongside golden PNGs for debugging
      fs.mkdirSync(goldenDir, { recursive: true });
      fs.writeFileSync(
        `${goldenDir}/${vp.name}.layout.json`,
        JSON.stringify(snap, null, 2)
      );

      const layoutResult = runAllLayoutChecks(snap);
      if (!layoutResult.ok) {
        console.error("  FAIL: Layout sanity checks failed:");
        for (const err of layoutResult.errors) console.error(`    - ${err}`);
        failures++;
      } else {
        console.log("  Layout sanity: PASS");
      }
    } else {
      console.log("  Layout sanity: SKIPPED (fullLayoutSnapshot not available)");
    }
  } catch (e) {
    console.error(`  FAIL: ${e.message}`);
    failures++;
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "visual_regression");
    await page.close();
  }
}

// ─── Cleanup ─────────────────────────────────────────────────────────

await browser.close();
server.close();

if (bootstrapped > 0) {
  console.log(`\nBootstrapped ${bootstrapped} golden baseline(s) in: ${goldenDir}/`);
}

if (failures > 0) {
  console.error(`\n${failures} test(s) failed`);
  console.log(`Diagnostic images saved to: ${goldenDir}/`);
  process.exit(1);
}

console.log("\nAll visual regression tests passed.");
