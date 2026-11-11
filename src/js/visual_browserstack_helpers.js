/**
 * BrowserStack Helper Module
 *
 * Manages BrowserStack Local tunnel lifecycle, remote device connections,
 * and session status reporting for real-device visual testing.
 *
 * Uses the "legacy direct connect" pattern: browserstack-local npm package
 * for tunneling + Playwright's webkit.connect() (iOS) and _android.connect()
 * (Android) to BrowserStack's WebSocket endpoint. This works with Beatmo's
 * standalone Node.js test harness (not @playwright/test runner).
 */

import BrowserStackLocal from "browserstack-local";
import { webkit, _android } from "playwright";

// ─── Credentials ────────────────────────────────────────────────────

/**
 * Validate and return BrowserStack credentials from env vars.
 * Exits with error if not set.
 * @returns {{user: string, key: string}}
 */
export function requireBrowserStackCredentials() {
  const user = process.env.BROWSERSTACK_USERNAME;
  const key = process.env.BROWSERSTACK_ACCESS_KEY;
  if (!user || !key) {
    console.error(
      "ERROR: Set BROWSERSTACK_USERNAME and BROWSERSTACK_ACCESS_KEY environment variables.\n" +
      "  Get credentials at: https://www.browserstack.com/accounts/settings"
    );
    process.exit(1);
  }
  return { user, key };
}

// ─── Tunnel Lifecycle ───────────────────────────────────────────────

/**
 * Start a BrowserStack Local tunnel so real devices can reach localhost.
 * @param {string} accessKey BrowserStack access key
 * @returns {Promise<BrowserStackLocal.Local>} Tunnel instance (caller calls stopTunnel)
 */
export async function startTunnel(accessKey) {
  const bsLocal = new BrowserStackLocal.Local();
  await new Promise((resolve, reject) => {
    bsLocal.start({ key: accessKey, force: true }, (err) => {
      if (err) reject(new Error(`BrowserStack Local tunnel failed: ${err.message}`));
      else resolve();
    });
  });
  console.log("  BrowserStack Local tunnel started");
  return bsLocal;
}

/**
 * Stop the BrowserStack Local tunnel.
 * @param {BrowserStackLocal.Local} bsLocal
 */
export async function stopTunnel(bsLocal) {
  if (!bsLocal) return;
  await new Promise((resolve) => bsLocal.stop(resolve));
  console.log("  BrowserStack Local tunnel stopped");
}

// ─── Device Matrix ──────────────────────────────────────────────────

/**
 * Curated device matrix covering the primary suspects for mobile
 * WebGL/GPU rendering bugs.
 */
export const DEVICE_MATRIX = [
  // iOS Safari — primary suspect for the flat gray bug
  {
    name: "iPhone_14_Safari",
    engine: "webkit",
    browser: "safari",
    deviceName: "iPhone 14",
    osVersion: "16",
    orientation: "portrait",
  },
  {
    name: "iPhone_14_landscape",
    engine: "webkit",
    browser: "safari",
    deviceName: "iPhone 14",
    osVersion: "16",
    orientation: "landscape",
  },
  {
    name: "iPhone_15_Pro_Safari",
    engine: "webkit",
    browser: "safari",
    deviceName: "iPhone 15 Pro",
    osVersion: "17",
    orientation: "portrait",
  },
  {
    name: "iPhone_SE_Safari",
    engine: "webkit",
    browser: "safari",
    deviceName: "iPhone SE 2022",
    osVersion: "16",
    orientation: "portrait",
  },
  // Android Chrome — secondary target
  {
    name: "Galaxy_S23_Chrome",
    engine: "chromium",
    browser: "chrome",
    deviceName: "Samsung Galaxy S23 Ultra",
    osVersion: "13.0",
    orientation: "portrait",
  },
  {
    name: "Pixel_8_Chrome",
    engine: "chromium",
    browser: "chrome",
    deviceName: "Google Pixel 8",
    osVersion: "14.0",
    orientation: "portrait",
  },
  {
    name: "Pixel_8_landscape",
    engine: "chromium",
    browser: "chrome",
    deviceName: "Google Pixel 8",
    osVersion: "14.0",
    orientation: "landscape",
  },
];

// ─── Remote Browser Connect ─────────────────────────────────────────

/**
 * Connect to a real BrowserStack device via Playwright WebSocket.
 *
 * iOS uses webkit.connect(); Android uses playwright._android.connect()
 * which returns an AndroidDevice (launchBrowser → BrowserContext).
 *
 * @param {object} device Device definition from DEVICE_MATRIX
 * @param {{user: string, key: string}} credentials
 * @param {number} localPort Local server port (tunneled)
 * @returns {Promise<{browser: object, context: object, page: object}>}
 */
export async function connectRemoteDevice(device, credentials, localPort) {
  const caps = {
    browser: device.browser,
    deviceName: device.deviceName,
    osVersion: device.osVersion,
    realMobile: "true",
    deviceOrientation: device.orientation,
    name: `Beatmo Visual - ${device.name}`,
    build: `beatmo-visual-${Date.now()}`,
    project: "Beatmo",
    "browserstack.username": credentials.user,
    "browserstack.accessKey": credentials.key,
    "browserstack.local": "true",
    "browserstack.debug": "true",
    "browserstack.console": "info",
    "browserstack.networkLogs": "true",
  };

  const wsUrl = `wss://cdp.browserstack.com/playwright?caps=${encodeURIComponent(JSON.stringify(caps))}`;

  let browser, context, page;

  if (device.engine === "chromium") {
    // Android real devices use Playwright's _android API, not chromium.connect().
    // BrowserStack's /playwright endpoint speaks the Android wire protocol for
    // these devices, not the Chromium browser protocol.
    const androidDevice = await _android.connect(wsUrl);
    await androidDevice.shell("am force-stop com.android.chrome");
    context = await androidDevice.launchBrowser();
    page = await context.newPage();
    // Return androidDevice as "browser" so the caller's cleanup (.close()) works.
    browser = androidDevice;
  } else {
    browser = await webkit.connect({ wsEndpoint: wsUrl });
    context = await browser.newContext();
    page = await context.newPage();
  }

  await page.goto(`http://localhost:${localPort}/`, {
    waitUntil: "load",
    timeout: 60000,
  });

  return { browser, context, page };
}

// ─── Session Status ─────────────────────────────────────────────────

/**
 * Report test pass/fail to BrowserStack dashboard.
 * @param {object} page Playwright page
 * @param {"passed"|"failed"} status
 * @param {string} reason Human-readable reason
 */
export async function markSessionStatus(page, status, reason) {
  try {
    await page.evaluate(
      `browserstack_executor: ${JSON.stringify({
        action: "setSessionStatus",
        arguments: { status, reason },
      })}`
    );
  } catch (_) {
    // Best-effort; don't fail the test if dashboard update fails
  }
}
