/**
 * Visual Mobile Parity Test
 *
 * Compares desktop vs mobile screenshots of the same WASM build.
 * The drum row region should have similar visual structure (color variance,
 * non-background pixel ratio) on both. If mobile shows flat gray while
 * desktop shows cells, the test fails.
 *
 * Also validates structural layout: component positions, touch target sizes,
 * responsive breakpoints, widget overlap, and desktop-at-same-size parity.
 *
 * This detects the exact "flat gray on mobile" rendering bug AND layout
 * regressions that pixel-only checks would miss.
 */

import { chromium } from "playwright";
import {
  buildWasm,
  startServer,
  initPage,
  captureScreenshot,
  extractRegion,
  getDrumRowRegion,
  analyzeRegion,
  loadMultiRowFixture,
  captureLayoutSnapshot,
  saveDiagnostic,
  regionToPng,
  goldenDir,
} from "./visual_test_helpers.js";
import { runAllLayoutChecks } from "./layout_parity_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

// ─── Configuration ───────────────────────────────────────────────────

const DESKTOP_VIEWPORT = { width: 1280, height: 720 };

const MOBILE_VIEWPORTS = [
  { name: "iPhone14_portrait", width: 390, height: 844 },
  { name: "iPhone14_landscape", width: 844, height: 390 },
  { name: "iPhoneSE_portrait", width: 320, height: 568 },
  { name: "small_landscape", width: 480, height: 320 },
];

// ─── Build WASM ──────────────────────────────────────────────────────

console.log("Building WASM...");
buildWasm();

const { server, port } = await startServer();
const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});

let failures = 0;

// ─── Desktop Reference ───────────────────────────────────────────────

console.log("\n=== Desktop reference (1280x720) ===");
let desktopMetrics;
{
  const page = await initPage(browser, DESKTOP_VIEWPORT, port);
  await loadMultiRowFixture(page);

  const regionRect = await getDrumRowRegion(page);
  console.log(`  Drum region: x=${regionRect.x} y=${regionRect.y} w=${regionRect.w} h=${regionRect.h}`);

  const screenshot = await captureScreenshot(page);
  const region = extractRegion(screenshot, regionRect);
  desktopMetrics = analyzeRegion(region);

  console.log(`  Color variance: ${desktopMetrics.variance.total.toFixed(1)}`);
  console.log(`  Non-background ratio: ${(desktopMetrics.nonBgRatio * 100).toFixed(1)}%`);
  console.log(`  Unique colors: ${desktopMetrics.uniqueColors}`);

  // Validate desktop has actual content
  if (desktopMetrics.variance.total < 50) {
    console.error("  FAIL: Desktop drum region has too low variance — possible test setup issue");
    failures++;
  }
  if (desktopMetrics.nonBgRatio < 0.3) {
    console.error("  FAIL: Desktop drum region has <30% non-background pixels");
    failures++;
  }
  if (desktopMetrics.uniqueColors < 3) {
    console.error("  FAIL: Desktop drum region has <3 unique colors");
    failures++;
  }

  // Desktop layout check
  const desktopSnap = await captureLayoutSnapshot(page);
  if (desktopSnap) {
    const layoutResult = runAllLayoutChecks(desktopSnap);
    if (!layoutResult.ok) {
      console.error("  FAIL: Desktop layout checks failed:");
      for (const err of layoutResult.errors) console.error(`    - ${err}`);
      failures++;
    } else {
      console.log("  Desktop layout checks: PASS");
    }
  }

  // Save desktop diagnostic
  saveDiagnostic("parity_desktop", screenshot);
  saveDiagnostic("parity_desktop_region", regionToPng(region));
  console.log(`  Desktop reference saved to ${goldenDir}/`);

  await page.close();
}

if (failures > 0) {
  console.error("\nDesktop reference validation failed — cannot compare mobile.");
  await browser.close();
  server.close();
  process.exit(1);
}

console.log("  Desktop reference: PASS\n");

// ─── Mobile Viewports ────────────────────────────────────────────────

for (const vp of MOBILE_VIEWPORTS) {
  console.log(`=== ${vp.name} (${vp.width}x${vp.height}) ===`);

  const page = await initPage(browser, { width: vp.width, height: vp.height }, port);

  try {
    await loadMultiRowFixture(page);

    let regionRect;
    try {
      regionRect = await getDrumRowRegion(page);
    } catch (e) {
      console.error(`  FAIL: Could not get drum region — ${e.message}`);
      failures++;
      continue;
    }

    console.log(`  Drum region: x=${regionRect.x} y=${regionRect.y} w=${regionRect.w} h=${regionRect.h}`);

    if (regionRect.w < 10 || regionRect.h < 10) {
      console.error(`  FAIL: Drum region too small (${regionRect.w}x${regionRect.h})`);
      failures++;
      continue;
    }

    const screenshot = await captureScreenshot(page);
    const region = extractRegion(screenshot, regionRect);
    const metrics = analyzeRegion(region);

    console.log(`  Color variance: ${metrics.variance.total.toFixed(1)} (desktop: ${desktopMetrics.variance.total.toFixed(1)})`);
    console.log(`  Non-background ratio: ${(metrics.nonBgRatio * 100).toFixed(1)}% (desktop: ${(desktopMetrics.nonBgRatio * 100).toFixed(1)}%)`);
    console.log(`  Unique colors: ${metrics.uniqueColors} (desktop: ${desktopMetrics.uniqueColors})`);

    // Save mobile diagnostic images
    saveDiagnostic(`parity_mobile_${vp.name}`, screenshot);
    saveDiagnostic(`parity_mobile_${vp.name}_region`, regionToPng(region));

    // ── Core assertion: mobile should have visible content ──
    // If desktop has content (>30% non-bg) but mobile is flat (<5%), that's the bug.
    let pixelFailed = false;
    if (metrics.nonBgRatio < 0.05 && desktopMetrics.nonBgRatio > 0.3) {
      console.error(
        `  FAIL: FLAT GRAY DETECTED — Mobile has ${(metrics.nonBgRatio * 100).toFixed(1)}% non-bg pixels ` +
        `while desktop has ${(desktopMetrics.nonBgRatio * 100).toFixed(1)}%. ` +
        `This is the mobile rendering bug.`
      );
      failures++;
      pixelFailed = true;
    }

    if (!pixelFailed) {
      // Mobile metrics should be at least 25% of desktop metrics
      const varianceRatio = desktopMetrics.variance.total > 0
        ? metrics.variance.total / desktopMetrics.variance.total
        : 1;
      const nbgRatio = desktopMetrics.nonBgRatio > 0
        ? metrics.nonBgRatio / desktopMetrics.nonBgRatio
        : 1;
      const colorRatio = desktopMetrics.uniqueColors > 0
        ? metrics.uniqueColors / desktopMetrics.uniqueColors
        : 1;

      console.log(`  Variance ratio: ${(varianceRatio * 100).toFixed(0)}% of desktop`);
      console.log(`  Non-bg ratio: ${(nbgRatio * 100).toFixed(0)}% of desktop`);
      console.log(`  Color ratio: ${(colorRatio * 100).toFixed(0)}% of desktop`);

      if (varianceRatio < 0.20) {
        console.error(`  FAIL: Color variance is only ${(varianceRatio * 100).toFixed(0)}% of desktop (need >= 20%)`);
        failures++;
        pixelFailed = true;
      }

      if (!pixelFailed && nbgRatio < 0.25) {
        console.error(`  FAIL: Non-background ratio is only ${(nbgRatio * 100).toFixed(0)}% of desktop (need >= 25%)`);
        failures++;
        pixelFailed = true;
      }
    }

    // ── Layout structural validation ──
    const snap = await captureLayoutSnapshot(page);
    if (snap) {
      const layoutResult = runAllLayoutChecks(snap);
      if (!layoutResult.ok) {
        console.error("  FAIL: Layout checks failed:");
        for (const err of layoutResult.errors) console.error(`    - ${err}`);
        failures++;
      } else {
        console.log("  Layout checks: PASS");
      }
    } else {
      console.log("  Layout checks: SKIPPED (fullLayoutSnapshot not available)");
    }

    if (!pixelFailed) {
      console.log(`  Pixel checks: PASS`);
    }
  } catch (e) {
    console.error(`  FAIL: ${e.message}`);
    failures++;
  } finally {
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "visual_mobile_parity");
    await page.close();
  }
}

// ─── Cleanup ─────────────────────────────────────────────────────────

await browser.close();
server.close();

if (failures > 0) {
  console.error(`\n${failures} test(s) failed`);
  console.log(`Diagnostic images saved to: ${goldenDir}/`);
  process.exit(1);
}

console.log("\nAll visual mobile parity tests passed.");
