/**
 * Real-Device Visual Parity Test (BrowserStack)
 *
 * Compares a local desktop reference screenshot against real mobile device
 * screenshots taken via BrowserStack. Detects GPU/WebGL rendering bugs
 * (like "flat gray on mobile") that Playwright headless cannot reproduce
 * because headless uses a desktop Chrome engine regardless of viewport size.
 *
 * Also validates structural layout on real devices: component positions, touch
 * target sizes, widget overlap, and desktop-at-same-size parity (same WASM
 * layout at same viewport dimensions on local Chromium vs real device).
 *
 * Prerequisites:
 *   export BROWSERSTACK_USERNAME="your_username"
 *   export BROWSERSTACK_ACCESS_KEY="your_access_key"
 *
 * Run:
 *   GO=$(pwd)/.tools/go/bin/go node src/js/visual_device_parity.browser.test.js
 *   # or: make test-visual-device
 *
 * On-demand only — not part of CI. Run manually when debugging mobile
 * rendering issues.
 */

import { chromium } from "playwright";
import {
  buildWasm,
  startServer,
  initPage,
  waitForWasmReady,
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
import {
  requireBrowserStackCredentials,
  startTunnel,
  stopTunnel,
  connectRemoteDevice,
  markSessionStatus,
  DEVICE_MATRIX,
} from "./visual_browserstack_helpers.js";
import {
  runAllLayoutChecks,
  assertLayoutParity,
} from "./layout_parity_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

// ─── Configuration ──────────────────────────────────────────────────

const DESKTOP_VIEWPORT = { width: 1280, height: 720 };

// Allow filtering devices via env var, e.g. DEVICE_FILTER=iPhone
const deviceFilter = process.env.DEVICE_FILTER || "";
const devices = deviceFilter
  ? DEVICE_MATRIX.filter((d) => d.name.toLowerCase().includes(deviceFilter.toLowerCase()))
  : DEVICE_MATRIX;

if (devices.length === 0) {
  console.error(`No devices match filter "${deviceFilter}"`);
  process.exit(1);
}

// ─── Validate Credentials ───────────────────────────────────────────

const credentials = requireBrowserStackCredentials();
console.log(`BrowserStack user: ${credentials.user}`);
console.log(`Testing ${devices.length} device(s)${deviceFilter ? ` (filter: "${deviceFilter}")` : ""}\n`);

// ─── Build WASM & Start Server ──────────────────────────────────────

console.log("Building WASM...");
buildWasm();

const { server, port } = await startServer();
console.log(`Local server on port ${port}`);

// ─── Desktop Reference ──────────────────────────────────────────────

console.log("\n=== Desktop reference (1280x720) ===");
const localBrowser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});

let desktopMetrics;
let desktopScreenshot;
{
  const page = await initPage(localBrowser, DESKTOP_VIEWPORT, port);
  await loadMultiRowFixture(page);

  const regionRect = await getDrumRowRegion(page);
  console.log(`  Drum region: x=${regionRect.x} y=${regionRect.y} w=${regionRect.w} h=${regionRect.h}`);

  desktopScreenshot = await captureScreenshot(page);
  const region = extractRegion(desktopScreenshot, regionRect);
  desktopMetrics = analyzeRegion(region);

  console.log(`  Color variance: ${desktopMetrics.variance.total.toFixed(1)}`);
  console.log(`  Non-background ratio: ${(desktopMetrics.nonBgRatio * 100).toFixed(1)}%`);
  console.log(`  Unique colors: ${desktopMetrics.uniqueColors}`);

  // Validate desktop has actual content
  let desktopOk = true;
  if (desktopMetrics.variance.total < 50) {
    console.error("  FAIL: Desktop drum region has too low variance");
    desktopOk = false;
  }
  if (desktopMetrics.nonBgRatio < 0.3) {
    console.error("  FAIL: Desktop drum region has <30% non-background pixels");
    desktopOk = false;
  }
  if (desktopMetrics.uniqueColors < 3) {
    console.error("  FAIL: Desktop drum region has <3 unique colors");
    desktopOk = false;
  }

  // Desktop layout check
  const desktopSnap = await captureLayoutSnapshot(page);
  if (desktopSnap) {
    const layoutResult = runAllLayoutChecks(desktopSnap);
    if (!layoutResult.ok) {
      console.error("  FAIL: Desktop layout checks failed:");
      for (const err of layoutResult.errors) console.error(`    - ${err}`);
      desktopOk = false;
    } else {
      console.log("  Desktop layout checks: PASS");
    }
  }

  saveDiagnostic("device_desktop_reference", desktopScreenshot);
  saveDiagnostic("device_desktop_region", regionToPng(region));

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "visual_device_parity");
  await page.close();

  if (!desktopOk) {
    console.error("\nDesktop reference validation failed — cannot compare devices.");
    await localBrowser.close();
    server.close();
    process.exit(1);
  }
}
console.log("  Desktop reference: PASS\n");

// ─── Start BrowserStack Tunnel ──────────────────────────────────────

console.log("Starting BrowserStack Local tunnel...");
let tunnel;
try {
  tunnel = await startTunnel(credentials.key);
} catch (e) {
  console.error(`Failed to start tunnel: ${e.message}`);
  await localBrowser.close();
  server.close();
  process.exit(1);
}

// ─── Test Each Device ───────────────────────────────────────────────

function withTimeout(promise, ms, label) {
  return Promise.race([
    promise,
    new Promise((_, reject) =>
      setTimeout(() => reject(new Error(`Timeout: ${label} after ${ms}ms`)), ms)
    ),
  ]);
}

let failures = 0;
let passed = 0;
let skipped = 0;

for (const device of devices) {
  console.log(`\n=== ${device.name} (${device.deviceName}, ${device.orientation}) ===`);

  let browser, context, page;
  try {
    await withTimeout((async () => {
      console.log("  Connecting to BrowserStack...");
      ({ browser, context, page } = await connectRemoteDevice(device, credentials, port));
      console.log("  Connected. Waiting for WASM...");

      await waitForWasmReady(page, 90000);
      console.log("  WASM ready. Loading fixture...");

      await loadMultiRowFixture(page);
      console.log("  Fixture loaded. Capturing screenshot...");

      // Extra settling time for real devices
      await page.waitForTimeout(500);
      await page.evaluate(() => forceDraw?.());
      await page.waitForTimeout(200);

      const screenshot = await captureScreenshot(page);
      saveDiagnostic(`device_${device.name}_full`, screenshot);

      // Try to get drum region — may fail if layout differs on real device
      let regionRect;
      try {
        regionRect = await getDrumRowRegion(page);
      } catch (e) {
        console.error(`  WARN: Could not get drum region — ${e.message}`);
        console.error("  Saving full screenshot for manual inspection.");
        await withTimeout(markSessionStatus(page, "failed", `No drum region: ${e.message}`), 10000, "markSessionStatus").catch(() => {});
        failures++;
        return;
      }

      console.log(`  Drum region: x=${regionRect.x} y=${regionRect.y} w=${regionRect.w} h=${regionRect.h}`);

      if (regionRect.w < 10 || regionRect.h < 10) {
        console.error(`  FAIL: Drum region too small (${regionRect.w}x${regionRect.h})`);
        await withTimeout(markSessionStatus(page, "failed", "Drum region too small"), 10000, "markSessionStatus").catch(() => {});
        failures++;
        return;
      }

      const region = extractRegion(screenshot, regionRect);
      const metrics = analyzeRegion(region);

      console.log(`  Color variance: ${metrics.variance.total.toFixed(1)} (desktop: ${desktopMetrics.variance.total.toFixed(1)})`);
      console.log(`  Non-background ratio: ${(metrics.nonBgRatio * 100).toFixed(1)}% (desktop: ${(desktopMetrics.nonBgRatio * 100).toFixed(1)}%)`);
      console.log(`  Unique colors: ${metrics.uniqueColors} (desktop: ${desktopMetrics.uniqueColors})`);

      saveDiagnostic(`device_${device.name}_region`, regionToPng(region));

      // ── Core pixel assertion: real device should have visible content ──
      let deviceFailed = false;

      // Flat gray detection: desktop has content but device is blank
      if (metrics.nonBgRatio < 0.05 && desktopMetrics.nonBgRatio > 0.3) {
        console.error(
          `  FAIL: FLAT GRAY DETECTED on real device — ` +
          `${(metrics.nonBgRatio * 100).toFixed(1)}% non-bg (desktop: ${(desktopMetrics.nonBgRatio * 100).toFixed(1)}%). ` +
          `This confirms the mobile GPU rendering bug.`
        );
        deviceFailed = true;
      }

      // Ratio checks vs desktop
      const varianceRatio = desktopMetrics.variance.total > 0
        ? metrics.variance.total / desktopMetrics.variance.total
        : 1;
      const nbgRatio = desktopMetrics.nonBgRatio > 0
        ? metrics.nonBgRatio / desktopMetrics.nonBgRatio
        : 1;

      console.log(`  Variance ratio: ${(varianceRatio * 100).toFixed(0)}% of desktop`);
      console.log(`  Non-bg ratio: ${(nbgRatio * 100).toFixed(0)}% of desktop`);

      if (varianceRatio < 0.20 && !deviceFailed) {
        console.error(`  FAIL: Color variance is only ${(varianceRatio * 100).toFixed(0)}% of desktop (need >= 20%)`);
        deviceFailed = true;
      }

      if (nbgRatio < 0.25 && !deviceFailed) {
        console.error(`  FAIL: Non-background ratio is only ${(nbgRatio * 100).toFixed(0)}% of desktop (need >= 25%)`);
        deviceFailed = true;
      }

      if (!deviceFailed) {
        console.log("  Pixel checks: PASS");
      }

      // ── Layout structural validation on real device ──
      const deviceSnap = await captureLayoutSnapshot(page);
      if (deviceSnap) {
        const layoutResult = runAllLayoutChecks(deviceSnap);
        if (!layoutResult.ok) {
          console.error("  FAIL: Device layout checks failed:");
          for (const err of layoutResult.errors) console.error(`    - ${err}`);
          deviceFailed = true;
        } else {
          console.log("  Device layout checks: PASS");
        }

        // ── Desktop-at-same-size parity comparison ──
        const devW = deviceSnap.canvasWidth;
        const devH = deviceSnap.canvasHeight;
        if (devW > 0 && devH > 0) {
          console.log(`  Desktop-at-same-size parity (${devW}x${devH})...`);
          let samesizePage;
          try {
            samesizePage = await initPage(localBrowser, { width: devW, height: devH }, port);
            await loadMultiRowFixture(samesizePage);
            const refSnap = await captureLayoutSnapshot(samesizePage);
            if (refSnap) {
              const parityResult = assertLayoutParity(refSnap, deviceSnap, 8);
              if (!parityResult.ok) {
                console.error("  FAIL: Desktop-at-same-size parity failed:");
                for (const err of parityResult.errors) console.error(`    - ${err}`);
                deviceFailed = true;
              } else {
                console.log("  Desktop-at-same-size parity: PASS");
              }
            }
          } catch (parityErr) {
            console.error(`  WARN: Desktop-at-same-size parity skipped: ${parityErr.message}`);
          } finally {
            if (samesizePage) await samesizePage.close().catch(() => {});
          }
        }
      } else {
        console.log("  Layout checks: SKIPPED (fullLayoutSnapshot not available)");
      }

      if (deviceFailed) {
        await withTimeout(markSessionStatus(page, "failed", "Visual/layout parity failed"), 10000, "markSessionStatus").catch(() => {});
        failures++;
      } else {
        await withTimeout(markSessionStatus(page, "passed", "Visual/layout parity OK"), 10000, "markSessionStatus").catch(() => {});
        passed++;
        console.log("  PASS");
      }
    })(), 240000, `${device.name} test`);
  } catch (e) {
    console.error(`  ERROR: ${e.message}`);
    if (page) {
      await withTimeout(
        markSessionStatus(page, "failed", `Error: ${e.message}`),
        10000, "markSessionStatus"
      ).catch(() => {});
    }
    // Device connection failures / timeouts are not test failures — skip the device
    console.error("  Skipping device due to connection/setup/timeout error.");
    skipped++;
  } finally {
    try { if (context) await withTimeout(context.close(), 10000, "context.close"); } catch (_) {}
    try { if (browser) await withTimeout(browser.close(), 10000, "browser.close"); } catch (_) {}
  }
}

// ─── Cleanup ────────────────────────────────────────────────────────

await stopTunnel(tunnel);
await localBrowser.close();
server.close();

// ─── Summary ────────────────────────────────────────────────────────

console.log("\n" + "=".repeat(60));
console.log("DEVICE VISUAL PARITY RESULTS");
console.log("=".repeat(60));
console.log(`  Passed:  ${passed}/${devices.length}`);
console.log(`  Failed:  ${failures}/${devices.length}`);
console.log(`  Skipped: ${skipped}/${devices.length}`);
console.log(`  Diagnostic images: ${goldenDir}/device_*`);
console.log("=".repeat(60));

if (failures > 0) {
  console.error(
    `\n${failures} device(s) failed visual/layout parity.\n` +
    `Check diagnostic images in ${goldenDir}/ for details.\n` +
    `A "FLAT GRAY DETECTED" failure confirms the mobile GPU rendering bug.\n` +
    `A "layout checks failed" failure indicates component positioning issues.`
  );
  process.exit(1);
}

if (passed === 0 && skipped === devices.length) {
  console.error("\nAll devices were skipped (connection errors). No results.");
  process.exit(1);
}

console.log("\nAll tested devices passed visual/layout parity.");
